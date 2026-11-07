package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// transportEventTimeout bounds the wait for transport.collected/delivered
// after a (synchronous) collect or deposit.
const transportEventTimeout = 10 * time.Second

// deliverAll runs every dispatched freighter at once. A freighter that fails
// stops; the others carry on.
func deliverAll(ctx context.Context, hub *api.Hub, as []assignment, ds map[int]dispatch) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, i := range slices.Sorted(maps.Keys(ds)) {
		a, f := as[i], ds[i].Freighter.Code
		wg.Go(func() {
			if err := deliver(ctx, hub, f, a); err != nil {
				log.Printf("%s: FAILED delivering to %s: %v", f, a.Need.Loc, err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s -> %s: %w", f, a.Need.Loc, err))
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// deliver takes a's pickups to a's event location with freighter f, making
// as many trips as its hold needs. Each trip visits the collection locations
// in order, collecting what fits (first unloading cargo that isn't for this
// delivery, if that's what it takes), then goes to the event and deposits
// just what it collected.
func deliver(ctx context.Context, hub *api.Hub, f string, a assignment) error {
	dest := a.Need.Loc
	if strings.Contains(dest, "-OBJ-") {
		return fmt.Errorf("won't send a freighter to an -OBJ- location")
	}
	remaining := map[string]amounts{}
	for _, p := range a.Collect {
		if remaining[p.Loc] == nil {
			remaining[p.Loc] = amounts{}
		}
		remaining[p.Loc][p.Res] += p.Qty
	}
	left := func() int {
		n := 0
		for _, am := range remaining {
			n += total(am)
		}
		return n
	}
	delivered := amounts{}

	for trip := 1; left() > 0; trip++ {
		log.Printf("%s: trip %d to %s, %d still to collect", f, trip, dest, left())
		carrying := amounts{}
		for _, loc := range slices.Sorted(maps.Keys(remaining)) {
			if total(remaining[loc]) == 0 {
				continue
			}
			full, err := collectAt(ctx, hub, f, loc, remaining[loc], carrying)
			if err != nil {
				return err
			}
			if full {
				break
			}
		}
		if total(carrying) == 0 {
			return fmt.Errorf("collected nothing on trip %d, %d still wanted", trip, left())
		}
		if err := auto.GoTo(ctx, hub, f, dest); err != nil {
			return err
		}
		w := hub.Expect(api.ForDevice[model.TransportDelivered](f))
		if err := api.DepositResources(ctx, f, carrying); err != nil {
			w.Cancel()
			return fmt.Errorf("deposit at %s: %w", dest, err)
		}
		log.Printf("%s: deposited %s at %s", f, formatAmounts(carrying), dest)
		confirm(ctx, w, f, "delivered")
		for r, q := range carrying {
			delivered[r] += q
		}
	}
	log.Printf("%s: done, delivered %s to %s (wanted %s)", f, formatAmounts(delivered), dest, formatAmounts(a.Need.Want))
	return nil
}

// collectAt goes to loc and collects as much of want as fits, updating want
// (what's still to collect here) and carrying (what's aboard for this
// delivery). Anything wanted that the stockpile no longer has is dropped
// from want with a warning. It reports whether the hold is now full.
func collectAt(ctx context.Context, hub *api.Hub, f, loc string, want, carrying amounts) (bool, error) {
	d, err := api.Device(ctx, f)
	if err != nil {
		return false, err
	}
	if d.CargoUsed >= d.CargoCapacity && total(carrying) > 0 {
		return true, nil
	}
	if err := auto.GoTo(ctx, hub, f, loc); err != nil {
		return false, err
	}
	if d, err = api.Device(ctx, f); err != nil {
		return false, err
	}
	inv, err := api.InventoryAt(ctx, loc)
	if err != nil {
		return false, err
	}
	avail := amounts{}
	for _, l := range inv {
		if l.Location == loc {
			avail = l.Amounts()
		}
	}
	hold := amounts{}
	for _, c := range d.Cargo {
		hold[model.ResourceType(c.ResourceType)] += c.Quantity
	}

	deposit, take, short := loadStep(want, avail, hold, carrying, d.CargoCapacity)
	if len(short) > 0 {
		log.Printf("%s: warning: %s is short of %s; delivering less", f, loc, formatAmounts(short))
		for r, q := range short {
			want[r] -= q
		}
	}
	if len(deposit) > 0 {
		w := hub.Expect(api.ForDevice[model.TransportDelivered](f))
		if err := api.DepositResources(ctx, f, deposit); err != nil {
			w.Cancel()
			return false, fmt.Errorf("making room at %s: %w", loc, err)
		}
		log.Printf("%s: deposited %s at %s to make room", f, formatAmounts(deposit), loc)
		confirm(ctx, w, f, "delivered")
	}
	if len(take) == 0 {
		return total(want) > 0, nil
	}
	w := hub.Expect(api.ForDevice[model.TransportCollected](f))
	if err := api.CollectResources(ctx, f, take); err != nil {
		w.Cancel()
		return false, fmt.Errorf("collect at %s: %w", loc, err)
	}
	log.Printf("%s: collected %s at %s", f, formatAmounts(take), loc)
	confirm(ctx, w, f, "collected")
	for r, q := range take {
		want[r] -= q
		carrying[r] += q
	}
	// Anything wanted here but not taken didn't fit.
	return total(want) > 0, nil
}

// confirm waits briefly for the hub to report a collect/deposit. The command
// itself is synchronous, so a missing event is only worth a warning.
func confirm(ctx context.Context, w *api.Waiter, f, what string) {
	ctx, cancel := context.WithTimeout(ctx, transportEventTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		log.Printf("%s: warning: no transport.%s event: %v", f, what, err)
		return
	}
	var c model.TransportCommon
	switch p := ev.Payload.(type) {
	case *model.TransportCollected:
		c = p.TransportCommon
	case *model.TransportDelivered:
		c = p.TransportCommon
	}
	log.Printf("%s: confirmed %s %.0f; cargo now %d/%d", f, what, c.Total, c.CargoAfter, c.CargoCapacity)
}
