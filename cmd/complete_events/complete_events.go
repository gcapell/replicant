// complete_events works out how to meet the location events listed by
// /accounts/events, and does it. We never complete an event at a location
// without an ftl_beacon.
//
// For each active event it chooses an option: a satisfied one (everything
// it asks for is there), else the first whose devices we have spare, else
// the first. Then:
//
//   - Resources: what's still missing (required - current) comes from the
//     nearest relay star whose planets, moons and belts hold all of it, each
//     claim taken off the stock. Each sourced event gets at most one idle
//     cargo_freighter, which collects (unloading unrelated cargo if it needs
//     the room), deposits just that at the event, and repeats until done.
//   - Devices: a beacon for each event without one, plus the option's
//     missing devices. Idle surge carriers attach spare devices and detach
//     them where they're wanted.
//   - Completions: events that are ready now (option satisfied, beacon
//     present) are split among idle heaven_vessels carrying a
//     replicant_matrix, which travel to each one and complete it.
//
// Star positions, relay stars and devices come from the store (kept up to
// date by monitor); active events and the inventory come from the API.
// Before a real run, every device the plans use is fetched and checked
// against the store, and any difference stops it.
//
// -n prints the plans only. Otherwise freighters, carriers and vessels all
// follow their plans at once. Events made ready by this run are completed by
// the next one.
//
//	complete_events [-n] [-db PATH]
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"sync"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func main() {
	dryRun := flag.Bool("n", false, "print the plan only")
	dbPath := flag.String("db", store.DefaultPath(), "SQLite database (read-only)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-n] [-db PATH]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), *dbPath, *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, dbPath string, dryRun bool) error {
	db, err := store.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return err
	}
	w, err := loadWorld(ctx, db)
	db.Close()
	if err != nil {
		return err
	}
	events, err := fetchEvents(ctx)
	if err != nil {
		return err
	}
	eventLocs := map[string]bool{}
	for _, e := range events {
		eventLocs[e.Loc] = true
	}
	relays := w.relays
	devices := ofTypes(w.devices, wantedTypes(events)...)
	beacons := beaconLocations(devices)
	var spares []spare
	spareCount := map[string]int{}
	for _, d := range devices {
		if isSpare(d, eventLocs) {
			spares = append(spares, spare{Code: d.DeviceCode, Type: d.DeviceType, At: place{Loc: d.Location}})
			spareCount[d.DeviceType]++
		}
	}
	// Beacons come first: every event without one needs one.
	for _, e := range events {
		if !beacons[e.Loc] {
			spareCount[beaconType] = max(0, spareCount[beaconType]-1)
		}
	}
	var needs []need
	for _, e := range events {
		needs = append(needs, toNeed(e.Loc, e.Ev, chooseOption(e.Ev, spareCount)))
	}
	stock, err := sourceStock(ctx, needs, relays)
	if err != nil {
		return err
	}

	locate := func(p *place) (err error) {
		p.Pos, err = w.position(auto.StarOf(p.Loc))
		return err
	}
	for i := range needs {
		n := &needs[i]
		n.Beacon = beacons[n.Loc]
		n.Relay = slices.Contains(relays, n.Star)
		if n.Pos, err = w.position(n.Star); err != nil {
			return fmt.Errorf("event at %s: %w", n.Loc, err)
		}
	}
	for star := range starTotals(stock) {
		if _, err := w.position(star); err != nil {
			return err
		}
	}
	freighters := idleFreighters(w.devices)
	for i := range freighters {
		if freighters[i].Pos, err = w.position(freighters[i].Star); err != nil {
			return fmt.Errorf("freighter %s: %w", freighters[i].Code, err)
		}
	}
	carriers, err := idleCarriers(w.devices)
	if err != nil {
		return err
	}
	for i := range carriers {
		if err := locate(&carriers[i].At); err != nil {
			return fmt.Errorf("carrier %s: %w", carriers[i].Code, err)
		}
	}
	for i := range spares {
		if err := locate(&spares[i].At); err != nil {
			return fmt.Errorf("spare %s: %w", spares[i].Code, err)
		}
	}
	vessels := idleVessels(w.devices)
	for i := range vessels {
		if err := locate(&vessels[i].At); err != nil {
			return fmt.Errorf("vessel %s: %w", vessels[i].Code, err)
		}
	}

	as := plan(needs, stock, w.starPos)
	ds := assignFreighters(as, freighters, w.starPos)
	for _, line := range describe(as, ds, needs) {
		fmt.Println(line)
	}
	jobs := deviceJobs(needs)
	dp := planDevices(jobs, spares, carriers)
	for _, line := range describeDevices(dp, jobs, spares, carriers) {
		fmt.Println(line)
	}
	ready := readyEvents(needs)
	vr := planCompletions(vessels, ready)
	for _, line := range describeCompletions(vr, ready) {
		fmt.Println(line)
	}
	if dryRun {
		return nil
	}
	used := usedDevices(ds, dp, spares, carriers, vr)
	log.Printf("checking %d devices against the API", len(used))
	if err := verify(ctx, used, w.devices); err != nil {
		return err
	}
	// Freighters (resources), carriers (devices) and vessels (completions)
	// are different devices, so all three plans run at once.
	hub := api.NewHub(ctx)
	var wg sync.WaitGroup
	var resErr, devErr, compErr error
	wg.Go(func() { resErr = deliverAll(ctx, hub, as, ds) })
	wg.Go(func() { devErr = carryAll(ctx, hub, dp, spares, carriers) })
	wg.Go(func() { compErr = completeAll(ctx, hub, vr) })
	wg.Wait()
	return errors.Join(resErr, devErr, compErr)
}

type locatedEvent struct {
	Loc string
	Ev  *model.LocationEvent
}

// fetchEvents fetches every active event, sorted by location.
func fetchEvents(ctx context.Context) ([]locatedEvent, error) {
	var events []locatedEvent
	for page, err := range api.Paged[model.AccountEvents](ctx, "/accounts/events?status=active") {
		if err != nil {
			return nil, err
		}
		for _, e := range page.Events {
			events = append(events, locatedEvent{e.Location, &e})
		}
	}
	slices.SortFunc(events, func(a, b locatedEvent) int { return cmp.Compare(a.Loc, b.Loc) })
	log.Printf("%d active events", len(events))
	return events, nil
}

// wantedTypes is ftl_beacon plus every device type any option asks for.
func wantedTypes(events []locatedEvent) []string {
	types := []string{beaconType}
	for _, e := range events {
		for _, o := range e.Ev.Progress.Options {
			for _, d := range o.Devices {
				types = append(types, d.DeviceType)
			}
		}
	}
	slices.Sort(types)
	return slices.Compact(types)
}

// beaconLocations returns the locations where an ftl_beacon sits (not
// stowed or attached), from a device list.
func beaconLocations(devices []model.Device) map[string]bool {
	locs := map[string]bool{}
	for _, b := range devices {
		if b.DeviceType == beaconType && b.StowedInDeviceCode == "" && b.AttachedToDeviceCode == "" {
			locs[b.Location] = true
		}
	}
	return locs
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
