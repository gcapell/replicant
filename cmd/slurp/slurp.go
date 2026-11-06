// slurp uses every idle cargo_freighter (and every travelling one, once it
// arrives) to greedily move resources from planets
// and moons to belts. Each freighter repeatedly claims the nearest unclaimed
// stockpile, fills up (topping up within the same star), and delivers to the
// nearest belt already holding more than 1000 resources. A freighter stops
// once there's nothing left to claim anywhere; slurp exits when all have stopped.
//
//	slurp [-replicant ID]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// transportEventTimeout bounds the wait for the transport.collected/delivered
// confirmation of a (synchronous) collect or deposit.
const transportEventTimeout = 10 * time.Second

func main() {
	replicant := flag.String("replicant", "4698E8E6", "replicant whose nearest stars give distances")
	flag.Parse()
	ctx := context.Background()

	freighters, err := auto.Devices(ctx, "cargo_freighter")
	if err != nil {
		log.Fatal(err)
	}
	// Take the idle freighters, and the travelling ones once they arrive.
	var idle, travelling []string
	for _, f := range freighters {
		switch f.Status {
		case "idle":
			idle = append(idle, f.DeviceCode)
		case "travelling":
			travelling = append(travelling, f.DeviceCode)
		}
	}
	log.Printf("%d cargo_freighter(s): %d idle %s, %d travelling %s",
		len(freighters), len(idle), idle, len(travelling), travelling)
	if len(idle)+len(travelling) == 0 {
		return
	}
	// Only the nearest stars (not the whole catalogue, which is huge); stockpiles
	// at stars outside this set are ignored.
	stars, err := api.Stars(ctx, *replicant)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d nearest stars to %s", len(stars.Stars), *replicant)

	s := &slurper{
		hub:    api.NewHub(ctx),
		ledger: newLedger(func(ctx context.Context) ([]model.LocationInventory, error) { return api.InventoryAt(ctx, "") }, stars.Stars),
	}
	var wg sync.WaitGroup
	for _, f := range slices.Concat(idle, travelling) {
		wg.Go(func() {
			err := s.takeOver(ctx, f)
			s.ledger.Release(f)
			if err != nil {
				log.Printf("%s stopped: %v", f, err)
			}
		})
	}
	wg.Wait()
}

type slurper struct {
	hub    *api.Hub
	ledger *ledger
}

// takeOver waits for f to finish any trip it's on, then runs it if it's idle.
func (s *slurper) takeOver(ctx context.Context, f string) error {
	if err := auto.FinishTrip(ctx, s.hub, f); err != nil {
		return err
	}
	d, err := api.Device(ctx, f)
	if err != nil {
		return err
	}
	if d.Status != "idle" {
		return fmt.Errorf("status %q after arriving; leaving it alone", d.Status)
	}
	return s.run(ctx, f)
}

// run drives one freighter until there's nothing left to claim, or it hits an error.
func (s *slurper) run(ctx context.Context, f string) error {
	d, err := api.Device(ctx, f)
	if err != nil {
		return err
	}
	if d.CargoUsed > 0 {
		log.Printf("%s already carrying %d; topping up in %s, then delivering", f, d.CargoUsed, auto.StarOf(d.Location))
		if _, err := s.load(ctx, f, true); err != nil {
			return err
		}
		if err := s.deliver(ctx, f); err != nil {
			return err
		}
	}
	for {
		loaded, err := s.load(ctx, f, false)
		if err != nil {
			return err
		}
		if loaded == 0 {
			// load only gives up empty-handed when no claim is possible anywhere.
			log.Printf("%s: nothing left to claim anywhere; done", f)
			return nil
		}
		if err := s.deliver(ctx, f); err != nil {
			return err
		}
	}
}

// load claims and collects from the nearest stockpile, then tops up from others
// in the same star until full. With topUp, it stays in the freighter's current
// star from the start. It returns the amount collected.
func (s *slurper) load(ctx context.Context, f string, topUp bool) (int, error) {
	loaded := 0
	for {
		d, err := api.Device(ctx, f)
		if err != nil {
			return loaded, err
		}
		free := d.CargoCapacity - d.CargoUsed
		if free <= 0 {
			return loaded, nil
		}
		c, err := s.ledger.Claim(ctx, f, d.Location, free, topUp || loaded > 0)
		if err != nil || c.loc == "" {
			return loaded, err
		}
		log.Printf("%s claimed %v at %s", f, c.amounts, c.loc)
		n, err := s.collect(ctx, f, c)
		s.ledger.Release(f)
		if err != nil {
			return loaded, err
		}
		loaded += n
	}
}

// collect goes to the claim's location and collects what's claimed, or as
// much of it as is actually there.
func (s *slurper) collect(ctx context.Context, f string, c claim) (int, error) {
	if err := auto.GoTo(ctx, s.hub, f, c.loc); err != nil {
		return 0, err
	}
	inv, err := api.InventoryAt(ctx, c.loc)
	if err != nil {
		return 0, err
	}
	have := map[model.ResourceType]int{}
	for _, li := range inv {
		if li.Location == c.loc {
			have = li.Amounts()
		}
	}
	take := map[model.ResourceType]int{}
	n := 0
	for _, t := range slices.Sorted(maps.Keys(c.amounts)) {
		if q := min(c.amounts[t], have[t]); q > 0 {
			take[t] = q
			n += q
		}
	}
	if n == 0 {
		log.Printf("%s: nothing left at %s on arrival", f, c.loc)
		return 0, nil
	}
	w := s.hub.Expect(api.ForDevice[model.TransportCollected](f))
	if err := api.CollectResources(ctx, f, take); err != nil {
		w.Cancel()
		return 0, fmt.Errorf("collect at %s: %w", c.loc, err)
	}
	log.Printf("%s collected %d from %s: %v", f, n, c.loc, take)
	confirm(ctx, w, f, "collected")
	return n, nil
}

// deliver takes the cargo to the nearest qualifying belt and deposits it all.
func (s *slurper) deliver(ctx context.Context, f string) error {
	d, err := api.Device(ctx, f)
	if err != nil {
		return err
	}
	belt, err := s.ledger.NearestBelt(ctx, d.Location)
	if err != nil {
		return err
	}
	if belt == "" {
		return fmt.Errorf("no belt holds more than %d resources", minBeltStock)
	}
	if err := auto.GoTo(ctx, s.hub, f, belt); err != nil {
		return err
	}
	w := s.hub.Expect(api.ForDevice[model.TransportDelivered](f))
	if err := api.DepositAllResources(ctx, f); err != nil {
		w.Cancel()
		return fmt.Errorf("deposit at %s: %w", belt, err)
	}
	log.Printf("%s delivered cargo to %s", f, belt)
	confirm(ctx, w, f, "delivered")
	return nil
}

// confirm waits briefly for the hub to report a collect/deposit. The command
// itself is synchronous, so a missing event is only worth a warning.
func confirm(ctx context.Context, w *api.Waiter, f, what string) {
	ctx, cancel := context.WithTimeout(ctx, transportEventTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		log.Printf("warning: no transport.%s event for %s: %v", what, f, err)
		return
	}
	var c model.TransportCommon
	switch p := ev.Payload.(type) {
	case *model.TransportCollected:
		c = p.TransportCommon
	case *model.TransportDelivered:
		c = p.TransportCommon
	}
	log.Printf("%s confirmed %s %.0f; cargo now %d/%d", f, what, c.Total, c.CargoAfter, c.CargoCapacity)
}
