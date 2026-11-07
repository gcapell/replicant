// gaze cycles our galactic_observatory devices through observation points:
// compact, attach to a surge transport, travel to the next star's entry point,
// detach, unfurl, and prospect outward from the centre of explored space. See
// README.md for the full spec.
//
// It does every short step it can (attach, travel, detach), starts the next
// long one (compact, unfurl, prospect), and exits. -n prints the plan only.
//
//	gaze [-n] [-stars 50]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

const replicant = "4698E8E6"

func main() {
	nStars := flag.Int("stars", 50, "how many nearby stars to consider")
	dryRun := flag.Bool("n", false, "show the plan only; send nothing")
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
	path, err := ledgerPath()
	if err != nil {
		return err
	}
	led, err := loadLedger(path)
	if err != nil {
		return fmt.Errorf("ledger %s: %w", path, err)
	}
	obs, err := auto.Devices(ctx, "galactic_observatory")
	if err != nil {
		return err
	}
	relays, err := auto.Devices(ctx, "ftl_relay")
	if err != nil {
		return err
	}
	devices, err := nearby(ctx, obs)
	if err != nil {
		return err
	}
	stars, err := api.Stars(ctx, replicant, nStars)
	if err != nil {
		return err
	}

	cands, centre, offMap := candidates(stars.Stars, relays)
	for _, name := range offMap {
		log.Printf("relay star %s isn't among the %d nearest stars; ignoring it", name, nStars)
	}

	fmt.Printf("Observed from (%s): %d stars\n", path, len(led))
	fmt.Printf("Centre %s, from %d relay stars\n\nCandidates (nearest the centre first):\n", centre, len(cands))
	for _, c := range cands {
		mark := ""
		if note, ok := led[c.Name]; ok {
			mark = strings.TrimRight(" observed "+note, " ")
		}
		fmt.Printf("  %-20s %6.2f  %-20s%s\n", c.Name, c.Dist, c.Dest, mark)
	}

	fmt.Printf("\nObservatories (%d):\n", len(obs))
	plans := planAll(obs, devices, cands, led)
	for _, p := range plans {
		printPlan(p)
	}
	if dryRun {
		return nil
	}
	fmt.Println()
	return executeAll(ctx, &ledgerFile{path: path}, plans)
}

// executeAll carries out each observatory's plan concurrently. A failure stops
// only that observatory; the others carry on.
func executeAll(ctx context.Context, led *ledgerFile, plans []obsPlan) error {
	var hub *api.Hub
	if needsHub(plans) {
		hub = api.NewHub(ctx)
	}
	var wg sync.WaitGroup
	errs := make([]error, len(plans))
	for i, p := range plans {
		if len(p.Steps) == 0 {
			continue
		}
		wg.Go(func() {
			if err := execute(ctx, hub, led, p); err != nil {
				errs[i] = fmt.Errorf("%s: %w", p.Obs.DeviceCode, err)
				log.Printf("FAILED %v", errs[i])
				return
			}
			log.Printf("%s done for this run", p.Obs.DeviceCode)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// nearby returns the devices that could carry obs: everything at each
// observatory's location, plus the transport any of them is attached to (which
// may be travelling, so not at a location).
func nearby(ctx context.Context, obs []model.Device) ([]model.Device, error) {
	var devices []model.Device
	seen := map[string]bool{}
	add := func(ds ...model.Device) {
		for _, d := range ds {
			if !seen[d.DeviceCode] {
				seen[d.DeviceCode] = true
				devices = append(devices, d)
			}
		}
	}
	locs := map[string]bool{}
	for _, o := range obs {
		if o.Location == "" || locs[o.Location] {
			continue
		}
		locs[o.Location] = true
		ds, err := auto.Everything(ctx, o.Location)
		if err != nil {
			return nil, err
		}
		add(ds...)
	}
	for _, o := range obs {
		if c := o.AttachedToDeviceCode; c != "" && !seen[c] {
			d, err := api.Device(ctx, c)
			if err != nil {
				return nil, err
			}
			add(*d)
		}
	}
	return devices, nil
}

func printPlan(p obsPlan) {
	fmt.Printf("\n%s at %s: %s\n", p.Obs.DeviceCode, p.Obs.Location, p.State)
	if p.Target != nil {
		fmt.Printf("  star: %s (%.2f from centre, entry %s)\n", p.Target.Name, p.Target.Dist, p.Target.Dest)
	}
	if p.Problem != "" {
		fmt.Printf("  STUCK: %s\n", p.Problem)
	}
	if len(p.Steps) > 0 {
		var ss []string
		for _, s := range p.Steps {
			ss = append(ss, s.String())
		}
		fmt.Printf("  next: %s\n", strings.Join(ss, " -> "))
	}
}
