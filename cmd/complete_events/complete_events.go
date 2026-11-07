// complete_events works out how to meet the location events that ask for
// resources. For each location with an in-progress event it takes the
// event's first option, works out what's still missing (required - current),
// and finds the nearest star with a relaying ftl_relay whose planets, moons
// and belts hold all of it, taking each claim off the stock so two events
// don't count on the same resources. It also flags events whose location
// has no ftl_beacon: we never complete an event without one.
//
// Only the dry run (-n) exists so far.
//
//	complete_events -n
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

func main() {
	dryRun := flag.Bool("n", false, "print the plan only")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s -n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if !*dryRun {
		log.Fatal("only the dry run (-n) is implemented so far")
	}
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	needs, err := eventNeeds(ctx)
	if err != nil {
		return err
	}
	relays, err := auto.RelayStars(ctx)
	if err != nil {
		return err
	}
	beacons, err := beaconLocations(ctx)
	if err != nil {
		return err
	}
	stock, err := sourceStock(ctx, needs, relays)
	if err != nil {
		return err
	}

	starPos := map[string]model.Position{}
	position := func(star string) (model.Position, error) {
		if p, ok := starPos[star]; ok {
			return p, nil
		}
		loc, err := api.Location(ctx, star)
		if err != nil {
			return model.Position{}, fmt.Errorf("star %s: %w", star, err)
		}
		starPos[star] = loc.Star.Position
		return loc.Star.Position, nil
	}
	for i := range needs {
		n := &needs[i]
		n.Beacon = beacons[n.Loc]
		n.Relay = slices.Contains(relays, n.Star)
		if n.Pos, err = position(n.Star); err != nil {
			return err
		}
	}
	for star := range starTotals(stock) {
		if _, err := position(star); err != nil {
			return err
		}
	}

	for _, line := range describe(plan(needs, stock, starPos), needs) {
		fmt.Println(line)
	}
	return nil
}

// eventNeeds fetches every location with an in-progress event.
func eventNeeds(ctx context.Context) ([]need, error) {
	list, err := api.Locations(ctx)
	if err != nil {
		return nil, err
	}
	var needs []need
	for code, s := range list.Locations {
		if s.LocationEvents == 0 {
			continue
		}
		loc, err := api.Location(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("location %s: %w", code, err)
		}
		if loc.LocationEvent == nil {
			log.Printf("%s: summary says it has an event, but none is shown", code)
			continue
		}
		needs = append(needs, toNeed(code, loc.LocationEvent))
	}
	log.Printf("%d locations with events", len(needs))
	return needs, nil
}

// beaconLocations returns the locations where an ftl_beacon sits (not stowed).
func beaconLocations(ctx context.Context) (map[string]bool, error) {
	beacons, err := auto.Devices(ctx, "ftl_beacon")
	if err != nil {
		return nil, err
	}
	locs := map[string]bool{}
	for _, b := range beacons {
		if b.StowedInDeviceCode == "" {
			locs[b.Location] = true
		}
	}
	return locs, nil
}

// sourceStock returns the stock we may collect from: planets, moons and belts
// of relay stars, excluding any event location.
func sourceStock(ctx context.Context, needs []need, relays []string) (map[string]amounts, error) {
	inv, err := api.InventoryAt(ctx, "")
	if err != nil {
		return nil, err
	}
	stock := map[string]amounts{}
	for _, l := range inv {
		if !isSource(l.Location) || !slices.Contains(relays, auto.StarOf(l.Location)) ||
			slices.ContainsFunc(needs, func(n need) bool { return n.Loc == l.Location }) {
			continue
		}
		stock[l.Location] = l.Amounts()
	}
	return stock, nil
}
