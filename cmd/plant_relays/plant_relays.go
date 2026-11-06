// plant_relays plans a route for a heaven_vessel to plant its stowed
// ftl_relays at nearby stars that don't have one yet. Each star planted at must
// be within 7.5 ly of a star with a relay (existing, or planted earlier on the
// route). It prints the plan, then follows it for as many stops as it has
// relays: at each star it goes to the system's entry point (travelling to the
// Oort cloud and scanning first if we don't know it), deploys a relay and
// activates it. It exits at the first failure. -n prints the plan only.
//
//	plant_relays [-n] [-stars 50] VESSEL_ID
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
	nStars := flag.Int("stars", 50, "how many nearby stars to consider")
	dryRun := flag.Bool("n", false, "print the plan only")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-n] [-stars N] VESSEL_ID\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), flag.Arg(0), *nStars, *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, vesselID string, nStars int, dryRun bool) error {
	v, err := api.Device(ctx, vesselID)
	if err != nil {
		return err
	}
	if v.DeviceType != "heaven_vessel" {
		return fmt.Errorf("%s is a %s, not a heaven_vessel", vesselID, v.DeviceType)
	}
	if v.Status != "idle" {
		return fmt.Errorf("%s is %s, not idle", vesselID, v.Status)
	}
	stowed := stowedRelayCodes(v)
	carrying := len(stowed)
	if carrying == 0 {
		return fmt.Errorf("%s has no ftl_relays stowed", vesselID)
	}
	log.Printf("%s (replicant %s) at %s carrying %d ftl_relay(s)", vesselID, v.ReplicantCode, v.Location, carrying)

	relays, err := auto.Devices(ctx, "ftl_relay")
	if err != nil {
		return err
	}
	stars, err := api.Stars(ctx, v.ReplicantCode, nStars)
	if err != nil {
		return err
	}

	in := split(stars.Stars, relayStars(relays))
	log.Printf("%d stars: %d with a relay, %d without", len(stars.Stars), len(in.relays), len(in.candidates))
	for _, name := range in.offMap {
		log.Printf("relay star %s isn't among the %d nearest stars; ignoring it", name, nStars)
	}
	here, ok := find(stars.Stars, auto.StarOf(v.Location))
	if !ok {
		return fmt.Errorf("vessel's star %s isn't among the %d nearest stars", auto.StarOf(v.Location), nStars)
	}

	route := planInTwo(here.Pos, in.relays, in.candidates, carrying)
	printPlan(here, route, in, carrying)
	if dryRun || len(route) == 0 {
		return nil
	}
	return plantAll(ctx, vesselID, route, stowed)
}

// relayStars returns the set of stars where a relay sits (not stowed),
// whether active or inactive.
func relayStars(relays []model.Device) map[string]bool {
	stars := map[string]bool{}
	for _, r := range relays {
		if r.StowedInDeviceCode == "" && r.Location != "" {
			stars[auto.StarOf(r.Location)] = true
		}
	}
	return stars
}

type inputs struct {
	relays     []star   // stars with a relay
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
			in.relays = append(in.relays, st)
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

func find(stars []model.Star, name string) (star, bool) {
	for _, s := range stars {
		if s.Designation == name {
			return toStar(s), true
		}
	}
	return star{}, false
}

func printPlan(here star, route []star, in inputs, carrying int) {
	names := make([]string, len(in.relays))
	for i, r := range in.relays {
		names[i] = r.Name
	}
	slices.Sort(names)
	fmt.Printf("\nRelay stars (%d): %v\n", len(names), names)

	fmt.Printf("\nRoute from %s (%d stops, carrying %d relays):\n", here.Name, len(route), carrying)
	fmt.Printf("%4s  %-20s %8s %8s  %-20s %6s\n", "#", "star", "leg ly", "total", "via", "ly")
	for i, s := range describe(here.Pos, route, in.relays) {
		if i == carrying {
			fmt.Printf("      ---- out of relays: stops below need %d more ----\n", len(route)-carrying)
		}
		mark := ""
		if !s.Explored {
			mark = " (unexplored)"
		}
		fmt.Printf("%4d  %-20s %8.2f %8.2f  %-20s %6.2f%s\n", i+1, s.Name, s.Leg, s.Total, s.Via, s.ViaDist, mark)
	}

	unreached := unreachable(route, in.candidates)
	if len(unreached) == 0 {
		return
	}
	covered := append(slices.Clone(in.relays), route...)
	fmt.Printf("\nOut of reach even with the route above (%d):\n", len(unreached))
	for _, s := range unreached {
		n, d, _ := nearest(s.Pos, covered)
		fmt.Printf("      %-20s nearest relay star %s at %.2f ly\n", s.Name, n.Name, d)
	}
}

// unreachable returns the candidates not on route.
func unreachable(route, candidates []star) []star {
	var out []star
	for _, c := range candidates {
		if !slices.ContainsFunc(route, func(s star) bool { return s.Name == c.Name }) {
			out = append(out, c)
		}
	}
	return out
}
