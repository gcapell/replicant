// plant_relays plants ftl_relays at every star we can connect to the relay
// mesh, using every idle heaven_vessel that carries a replicant.
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
//	plant_relays [-n] [-stars 50]
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

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

func main() {
	nStars := flag.Int("stars", 50, "how many nearby stars to consider")
	dryRun := flag.Bool("n", false, "print the plan only")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-n] [-stars N]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), *nStars, *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, nStars int, dryRun bool) error {
	devices, err := readyVessels(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no idle heaven_vessels carrying a replicant")
	}
	relays, err := auto.Devices(ctx, "ftl_relay")
	if err != nil {
		return err
	}
	stars, err := api.Stars(ctx, devices[0].ReplicantCode, nStars)
	if err != nil {
		return err
	}

	starPos := map[string]model.Position{}
	for _, s := range stars.Stars {
		starPos[s.Designation] = s.Position
	}
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

	var vs []vessel
	for _, d := range devices {
		v := vessel{
			Code: d.DeviceCode, Replicant: d.ReplicantCode, Loc: d.Location,
			Relays: stowedRelayCodes(&d), Free: d.StowCapacity - d.StowUsed,
		}
		if v.Pos, err = position(auto.StarOf(d.Location)); err != nil {
			return err
		}
		vs = append(vs, v)
	}

	in := split(stars.Stars, relayingStars(relays))
	for _, name := range in.offMap {
		p, err := position(name)
		if err != nil {
			return err
		}
		log.Printf("relay star %s isn't among the %d nearest stars; adding it to the mesh", name, nStars)
		in.mesh = append(in.mesh, star{Name: name, Pos: p, Explored: true})
	}
	belts := availableBelts(relays)
	for i := range belts {
		if belts[i].Pos, err = position(auto.StarOf(belts[i].Loc)); err != nil {
			return err
		}
	}

	targets := connectable(in.mesh, in.candidates)
	log.Printf("%d stars: %d relay stars, %d without a relay, %d of them connectable",
		len(stars.Stars), len(in.mesh), len(in.candidates), len(targets))

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
	return runTeams(ctx, teams)
}

// readyVessels returns the idle heaven_vessels carrying a replicant, read
// one by one for their full details.
func readyVessels(ctx context.Context) ([]model.Device, error) {
	list, err := auto.Devices(ctx, "heaven_vessel")
	if err != nil {
		return nil, err
	}
	var out []model.Device
	for _, l := range list {
		d, err := api.Device(ctx, l.DeviceCode)
		if err != nil {
			return nil, err
		}
		if !auto.ReadyReplicant(*d) {
			log.Printf("skipping %s at %s (%s, no ready replicant)", d.DeviceCode, d.Location, d.Status)
			continue
		}
		out = append(out, *d)
	}
	log.Printf("%d of %d heaven_vessels ready", len(out), len(list))
	return out, nil
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
func split(stars []model.Star, hasRelay map[string]bool) inputs {
	var in inputs
	seen := map[string]bool{}
	for _, s := range stars {
		st := toStar(s)
		if hasRelay[s.Designation] {
			in.mesh = append(in.mesh, st)
			seen[s.Designation] = true
		} else {
			in.candidates = append(in.candidates, st)
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

func toStar(s model.Star) star {
	return star{Name: s.Designation, Pos: s.Position, Explored: s.Explored, EntryPoint: s.EntryPoint}
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
			if !s.Explored {
				mark = " (unexplored)"
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
