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

const (
	// travelTimeout bounds a single trip; there's no ETA up front, so be generous.
	travelTimeout = 12 * time.Hour
	// transportEventTimeout bounds the wait for the transport.collected/delivered
	// confirmation of a (synchronous) collect or deposit.
	transportEventTimeout = 10 * time.Second
)

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
	if d.Status == "travelling" {
		log.Printf("%s is travelling to %s (ETA %s); waiting for arrival",
			f.freighter, d.Travel.FinalDestination, formatSeconds(d.Travel.RouteEtaSeconds))
		w := f.expectArrival(d.Travel.FinalDestination)
		// Re-check, in case it arrived between the fetch and registering the waiter.
		if d, err = api.Device(ctx, f.freighter); err != nil {
			w.Cancel()
			return err
		}
		if d.Status == "travelling" {
			if err := f.waitArrival(ctx, w, d.Travel.FinalDestination); err != nil {
				return err
			}
		} else {
			w.Cancel()
		}
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
		if err := f.goTo(ctx, site.Location); err != nil {
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
	if err := f.goTo(ctx, f.dest); err != nil {
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

// goTo sends the freighter to loc and waits for it to arrive. It's a no-op if
// the freighter is already there.
func (f *ferry) goTo(ctx context.Context, loc string) error {
	d, err := api.Device(ctx, f.freighter)
	if err != nil {
		return err
	}
	if d.Location == loc && d.Status != "travelling" {
		return nil
	}
	// Register both before travel, so quick events aren't missed.
	departed := f.hub.Expect(api.ForDevice[model.TravelDeparted](f.freighter))
	w := f.expectArrival(loc)
	if err := api.Travel(ctx, f.freighter, loc); err != nil {
		departed.Cancel()
		w.Cancel()
		return fmt.Errorf("travel to %s: %w", loc, err)
	}
	log.Printf("%s travelling %s -> %s (ETA %s)", f.freighter, d.Location, loc, f.departureETA(ctx, departed))
	return f.waitArrival(ctx, w, loc)
}

// departureETA waits briefly for the travel.departed event and returns the trip
// time it reports, or "unknown" if it doesn't arrive in time.
func (f *ferry) departureETA(ctx context.Context, w *api.Waiter) string {
	ctx, cancel := context.WithTimeout(ctx, transportEventTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		log.Printf("warning: no travel.departed event for %s: %v", f.freighter, err)
		return "unknown"
	}
	return formatSeconds(ev.Payload.(*model.TravelDeparted).TravelTimeSeconds)
}

// formatSeconds renders a duration in seconds like "3m22s".
func formatSeconds(s float64) string {
	return time.Duration(s * float64(time.Second)).Round(time.Second).String()
}

func (f *ferry) expectArrival(loc string) *api.Waiter {
	return f.hub.Expect(api.AnyOf(
		func(ev api.Event) bool {
			a, ok := ev.Payload.(*model.TravelArrived)
			// Multi-leg trips may report intermediate arrivals; only the final one counts.
			return ok && ev.Env.DeviceCode == f.freighter && a.Destination == loc
		},
		api.ForDevice[model.TravelCancelled](f.freighter),
	))
}

func (f *ferry) waitArrival(ctx context.Context, w *api.Waiter, loc string) error {
	ctx, cancel := context.WithTimeout(ctx, travelTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		return fmt.Errorf("waiting to arrive at %s: %w", loc, err)
	}
	if c, ok := ev.Payload.(*model.TravelCancelled); ok {
		return fmt.Errorf("travel %s -> %s cancelled", c.Origin, c.Destination)
	}
	log.Printf("%s arrived at %s", f.freighter, loc)
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
