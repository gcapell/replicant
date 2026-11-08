// plant_relays plants ftl_relays at every star we can connect to the relay
// mesh, using every idle heaven_vessel that carries a replicant.
//
// It plans entirely from the store, so -n never calls the API. The stars
// considered are every star in the store; one with no entry_point counts as
// unscanned. Before a real run it checks the plan's vessels, relays and the
// mesh against the API, and stops if anything differs. See README.md.
//
// A star can join the mesh if it's within 7.5 ly of a star with a relaying
// ftl_relay, or of another star joining. Relays may be planted temporarily out
// of range, so long as the finished mesh is connected.
//
// Phase 1 decides which vessels fetch more relays, from which -BELT- location,
// so that between them they hold enough: the fewest short, then every vessel
// holding some, then the shortest longest trip, then the shortest total. A
// vessel fetching takes all the relays there it has room for.
//
// Phase 2 shares the stars among the vessels: one spread-out seed star per
// vessel, each seed given the nearest vessel, then the vessel with the
// cheapest trip so far (to its belt, then along its stars) takes the
// unclaimed star nearest its own, until each has as many stars as relays.
//
// It prints the plan, then all vessels run at once: fetch, then at each star
// go to the entry point (travelling to the Oort cloud and scanning first if we
// don't know it), deploy a relay and activate it. A vessel stops at its first
// failure; the others carry on. -n prints the plan only.
//
//	plant_relays [-n] [-db PATH]
package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func main() {
	dryRun := flag.Bool("n", false, "print the plan only")
	dbPath := flag.String("db", store.DefaultPath(), "store path")
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
	w, err := load(ctx, db)
	db.Close()
	if err != nil {
		return err
	}

	starPos := map[string]model.Position{}
	for _, s := range w.stars {
		starPos[s.Name] = s.Pos
	}
	position := func(star string) (model.Position, error) {
		if p, ok := starPos[star]; ok {
			return p, nil
		}
		return model.Position{}, fmt.Errorf("star %s isn't in the store", star)
	}

	devices := readyVessels(w.vessels)
	if len(devices) == 0 {
		return fmt.Errorf("no idle heaven_vessels carrying a replicant")
	}
	var vs []vessel
	for _, d := range devices {
		v := vessel{
			Code: d.DeviceCode, Replicant: d.ReplicantCode, Loc: d.Location,
			Relays: stowedRelayCodes(&d), Free: d.StowCapacity - d.StowUsed,
		}
		if v.Replicant == "" {
			return fmt.Errorf("vessel %s has no owner in the store", v.Code)
		}
		if v.Pos, err = position(auto.StarOf(d.Location)); err != nil {
			return fmt.Errorf("vessel %s: %w", v.Code, err)
		}
		vs = append(vs, v)
	}

	relayStars := relayingStars(w.relays)
	in := split(w.stars, relayStars)
	if len(in.offMap) > 0 {
		return fmt.Errorf("relay stars not in the store: %v", in.offMap)
	}
	belts := availableBelts(w.relays)
	for i := range belts {
		if belts[i].Pos, err = position(auto.StarOf(belts[i].Loc)); err != nil {
			return fmt.Errorf("relays at %s: %w", belts[i].Loc, err)
		}
	}

	targets := connectable(in.mesh, in.candidates)
	log.Printf("%d stars: %d relay stars, %d without a relay, %d of them connectable",
		len(w.stars), len(in.mesh), len(in.candidates), len(targets))

	fs := planFetch(vs, belts, len(targets))
	teams := make([]team, len(vs))
	holding := 0
	for i, v := range vs {
		teams[i] = newTeam(v, fs[i])
		holding += len(teams[i].Relays)
	}
	planned := targets[:min(len(targets), holding)]
	left := divide(teams, planned)

	printPlan(in, teams, belts, targets, planned, left)
	if dryRun {
		return nil
	}
	if err := verify(ctx, teams, relayStars); err != nil {
		return fmt.Errorf("the store doesn't match the API, so not starting:\n%w", err)
	}
	return runTeams(ctx, teams)
}

// readyVessels returns the idle heaven_vessels carrying a replicant.
func readyVessels(all []model.Device) []model.Device {
	var out []model.Device
	for _, d := range all {
		if !auto.ReadyReplicant(d) {
			log.Printf("skipping %s at %q (%s, no ready replicant)", d.DeviceCode, d.Location, d.Status)
			continue
		}
		out = append(out, d)
	}
	log.Printf("%d of %d heaven_vessels ready", len(out), len(all))
	return out
}

// relayingStars returns the set of stars with a relaying (not stowed) ftl_relay.
func relayingStars(relays []model.Device) map[string]bool {
	stars := map[string]bool{}
	for _, r := range relays {
		if r.Status == "relaying" && r.StowedInDeviceCode == "" && r.Location != "" {
			stars[auto.StarOf(r.Location)] = true
		}
	}
	return stars
}

// availableBelts groups the ftl_relays sitting (not stowed, not relaying) at
// -BELT- locations, sorted by location. Positions are left for the caller.
func availableBelts(relays []model.Device) []belt {
	byLoc := map[string][]relay{}
	for _, r := range relays {
		if r.StowedInDeviceCode == "" && r.Status != "relaying" && strings.Contains(r.Location, "-BELT-") {
			byLoc[r.Location] = append(byLoc[r.Location], relay{Code: r.DeviceCode, Owner: r.ReplicantCode})
		}
	}
	var belts []belt
	for loc, rs := range byLoc {
		slices.SortFunc(rs, func(a, b relay) int { return cmp.Compare(a.Code, b.Code) })
		belts = append(belts, belt{Loc: loc, Relays: rs})
	}
	slices.SortFunc(belts, func(a, b belt) int { return cmp.Compare(a.Loc, b.Loc) })
	return belts
}

type inputs struct {
	mesh       []star   // stars with a relaying relay
	candidates []star   // stars without one
	offMap     []string // relay stars not in the star list (sorted)
}

// split divides stars into those with a relay and those without.
func split(stars []star, hasRelay map[string]bool) inputs {
	var in inputs
	seen := map[string]bool{}
	for _, s := range stars {
		if hasRelay[s.Name] {
			in.mesh = append(in.mesh, s)
			seen[s.Name] = true
		} else {
			in.candidates = append(in.candidates, s)
		}
	}
	for name := range hasRelay {
		if !seen[name] {
			in.offMap = append(in.offMap, name)
		}
	}
	slices.Sort(in.offMap)
	return in
}

func printPlan(in inputs, teams []team, belts []belt, targets, planned, left []star) {
	names := make([]string, len(in.mesh))
	for i, r := range in.mesh {
		names[i] = r.Name
	}
	slices.Sort(names)
	fmt.Printf("\nRelay stars (%d): %v\n", len(names), names)

	fmt.Printf("\nRelays waiting at belts:\n")
	if len(belts) == 0 {
		fmt.Printf("      none\n")
	}
	for _, b := range belts {
		fmt.Printf("      %-20s %d\n", b.Loc, len(b.Relays))
	}

	holding := 0
	for _, t := range teams {
		holding += len(t.Relays)
	}
	fmt.Printf("\n%d connectable stars; the vessels will hold %d relays", len(targets), holding)
	if short := len(targets) - len(planned); short > 0 {
		fmt.Printf(", %d short", short)
	}
	fmt.Println()

	mesh := append(slices.Clone(in.mesh), planned...)
	longest := 0.0
	for _, t := range teams {
		v := t.Vessel
		fmt.Printf("\n%s (replicant %s) at %s, carrying %d relay(s), %d free slot(s)\n",
			v.Code, v.Replicant, v.Loc, len(v.Relays), v.Free)
		if f := t.Fetch; f.Loc != "" {
			fmt.Printf("      fetch %d from %s (%.2f ly)\n", len(f.Relays), f.Loc, f.Dist)
		}
		if len(t.Route) == 0 {
			fmt.Printf("      no stars; staying put\n")
			continue
		}
		fmt.Printf("%4s  %-20s %8s %8s  %-20s %6s\n", "#", "star", "leg ly", "total", "via", "ly")
		stops := describe(t.Start, t.Fetch.Dist, t.Route, mesh)
		for i, s := range stops {
			mark := ""
			if s.EntryPoint == "" {
				mark = " (unscanned)"
			}
			fmt.Printf("%4d  %-20s %8.2f %8.2f  %-20s %6.2f%s\n", i+1, s.Name, s.Leg, s.Total, s.Via, s.ViaDist, mark)
		}
		longest = max(longest, stops[len(stops)-1].Total)
		if spare := len(t.Relays) - len(t.Route); spare > 0 {
			fmt.Printf("      %d relay(s) to spare\n", spare)
		}
	}
	fmt.Printf("\nLongest trip: %.2f ly\n", longest)

	if rest := append(without(targets, planned), left...); len(rest) > 0 {
		fmt.Printf("\nConnectable but not planned, for want of relays (%d):\n", len(rest))
		for _, s := range rest {
			fmt.Printf("      %s\n", s.Name)
		}
	}
	unreached := without(in.candidates, targets)
	if len(unreached) == 0 {
		return
	}
	fmt.Printf("\nOut of reach of the finished mesh (%d):\n", len(unreached))
	for _, s := range unreached {
		n, d, _ := nearest(s.Pos, mesh, "")
		fmt.Printf("      %-20s nearest relay star %s at %.2f ly\n", s.Name, n.Name, d)
	}
}
