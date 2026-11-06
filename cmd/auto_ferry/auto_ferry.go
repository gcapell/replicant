// auto_ferry repeatedly loads a cargo_freighter with the resource stockpiles at a
// star and delivers them to a destination, until the star's stockpiles are empty.
//
//	auto_ferry STAR FREIGHTER_ID DESTINATION
package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// transportEventTimeout bounds the wait for the transport.collected/delivered
// confirmation of a (synchronous) collect or deposit.
const transportEventTimeout = 10 * time.Second

type ferry struct {
	hub       *api.Hub
	freighter string
	star      string
	dest      string
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr, "usage: %s STAR FREIGHTER_ID DESTINATION\n", os.Args[0])
		os.Exit(2)
	}
	ctx := context.Background()
	f := &ferry{
		hub:       api.NewHub(ctx),
		star:      os.Args[1],
		freighter: os.Args[2],
		dest:      os.Args[3],
	}
	if err := f.run(ctx); err != nil {
		log.Fatal(err)
	}
}

func (f *ferry) run(ctx context.Context) error {
	d, err := api.Device(ctx, f.freighter)
	if err != nil {
		return err
	}
	if d.DeviceType != "cargo_freighter" {
		return fmt.Errorf("%s is a %s, not a cargo_freighter", f.freighter, d.DeviceType)
	}
	if !slices.Contains(d.Features, "transport") {
		log.Printf("warning: %s has no transport feature (features %s)", f.freighter, d.Features)
	}

	// If it's already en route somewhere, let it get there first.
	if err := auto.FinishTrip(ctx, f.hub, f.freighter); err != nil {
		return err
	}
	if d, err = api.Device(ctx, f.freighter); err != nil {
		return err
	}

	// Leftover cargo from an earlier run goes to the destination first.
	if cargoTotal(d) > 0 {
		log.Printf("%s already carrying %d; delivering first", f.freighter, cargoTotal(d))
		if err := f.deliver(ctx); err != nil {
			return err
		}
	}

	for trip := 1; ; trip++ {
		sites, err := f.stockpiles(ctx)
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			log.Printf("no resources left at %s; done after %d trip(s)", f.star, trip-1)
			return nil
		}
		log.Printf("trip %d: %d resources at %d location(s) in %s", trip, sumTotals(sites), len(sites), f.star)

		loaded, err := f.load(ctx, sites)
		if err != nil {
			return err
		}
		if loaded == 0 {
			// Stockpiles exist but nothing could be collected: bail rather than spin.
			return fmt.Errorf("trip %d: collected nothing despite stockpiles at %s", trip, f.star)
		}
		if err := f.deliver(ctx); err != nil {
			return err
		}
	}
}

// stockpiles returns the non-empty stockpiles on the star's planets and moons
// (not belts or Lagrange points, and not the destination), largest first.
func (f *ferry) stockpiles(ctx context.Context) ([]model.LocationInventory, error) {
	all, err := api.InventoryAt(ctx, f.star)
	if err != nil {
		return nil, err
	}
	var sites []model.LocationInventory
	for _, l := range all {
		if auto.IsPlanetOrMoon(f.star, l.Location) && l.Location != f.dest && l.Total() > 0 {
			sites = append(sites, l)
		}
	}
	slices.SortFunc(sites, func(a, b model.LocationInventory) int {
		return cmp.Or(cmp.Compare(b.Total(), a.Total()), cmp.Compare(a.Location, b.Location))
	})
	return sites, nil
}

// load visits sites in order, collecting as much as fits, until the freighter
// is full or the sites are exhausted. It returns the amount collected.
func (f *ferry) load(ctx context.Context, sites []model.LocationInventory) (int, error) {
	loaded := 0
	for _, site := range sites {
		d, err := api.Device(ctx, f.freighter)
		if err != nil {
			return loaded, err
		}
		free := d.CargoCapacity - d.CargoUsed
		if free <= 0 {
			break
		}
		if err := auto.GoTo(ctx, f.hub, f.freighter, site.Location); err != nil {
			return loaded, err
		}
		// Re-read this site's stockpile now we're here; it may have changed en route.
		inv, err := api.InventoryAt(ctx, site.Location)
		if err != nil {
			return loaded, err
		}
		take := pick(inv, site.Location, free)
		if len(take) == 0 {
			log.Printf("%s: nothing left on arrival", site.Location)
			continue
		}
		w := f.hub.Expect(api.ForDevice[model.TransportCollected](f.freighter))
		if err := api.CollectResources(ctx, f.freighter, take); err != nil {
			w.Cancel()
			return loaded, fmt.Errorf("collect at %s: %w", site.Location, err)
		}
		n := 0
		for _, q := range take {
			n += q
		}
		loaded += n
		log.Printf("collected %d from %s: %v", n, site.Location, take)
		f.confirm(ctx, w, "collected")
	}
	return loaded, nil
}

// pick chooses up to free units from the stockpile at loc, largest resources first.
func pick(inv []model.LocationInventory, loc string, free int) map[model.ResourceType]int {
	var items map[model.ResourceType]int
	for _, l := range inv {
		if l.Location == loc {
			items = l.Amounts()
		}
	}
	types := make([]model.ResourceType, 0, len(items))
	for t := range items {
		types = append(types, t)
	}
	slices.SortFunc(types, func(a, b model.ResourceType) int {
		return cmp.Or(cmp.Compare(items[b], items[a]), strings.Compare(string(a), string(b)))
	})
	take := map[model.ResourceType]int{}
	for _, t := range types {
		if free <= 0 {
			break
		}
		if q := min(items[t], free); q > 0 {
			take[t] = q
			free -= q
		}
	}
	return take
}

func (f *ferry) deliver(ctx context.Context) error {
	if err := auto.GoTo(ctx, f.hub, f.freighter, f.dest); err != nil {
		return err
	}
	w := f.hub.Expect(api.ForDevice[model.TransportDelivered](f.freighter))
	if err := api.DepositAllResources(ctx, f.freighter); err != nil {
		w.Cancel()
		return fmt.Errorf("deposit at %s: %w", f.dest, err)
	}
	log.Printf("delivered cargo to %s", f.dest)
	f.confirm(ctx, w, "delivered")
	return nil
}

// confirm waits briefly for the hub to report a collect/deposit. The command
// itself is synchronous, so a missing event is only worth a warning.
func (f *ferry) confirm(ctx context.Context, w *api.Waiter, what string) {
	ctx, cancel := context.WithTimeout(ctx, transportEventTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		log.Printf("warning: no transport.%s event for %s: %v", what, f.freighter, err)
		return
	}
	var c model.TransportCommon
	switch p := ev.Payload.(type) {
	case *model.TransportCollected:
		c = p.TransportCommon
	case *model.TransportDelivered:
		c = p.TransportCommon
	}
	log.Printf("confirmed %s %.0f; cargo now %d/%d", what, c.Total, c.CargoAfter, c.CargoCapacity)
}

func cargoTotal(d *model.Device) int {
	n := 0
	for _, c := range d.Cargo {
		n += c.Quantity
	}
	return n
}

func sumTotals(sites []model.LocationInventory) int {
	n := 0
	for _, s := range sites {
		n += s.Total()
	}
	return n
}
