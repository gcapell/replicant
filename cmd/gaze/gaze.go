// gaze cycles our galactic_observatory devices through observation points:
// compact, attach to a surge transport, travel to the next star's entry point,
// detach, unfurl, and prospect outward from the centre of explored space. See
// README.md for the full spec.
//
// It plans from the store (stars, relays, where devices are), checks the
// statuses it relies on with the API, does every short step it can (fetch a
// transport, attach, travel, detach), starts the next long one (compact,
// unfurl, prospect), and exits. -n prints the plan only.
//
//	gaze [-n] [-db PATH]
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
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func main() {
	dbPath := flag.String("db", store.DefaultPath(), "store path")
	dryRun := flag.Bool("n", false, "show the plan only; send no commands")
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
	path, err := ledgerPath()
	if err != nil {
		return err
	}
	led, err := loadLedger(path)
	if err != nil {
		return fmt.Errorf("ledger %s: %w", path, err)
	}
	db, err := store.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	w, err := load(ctx, db)
	if err != nil {
		return fmt.Errorf("reading the store: %w", err)
	}
	if w.noFeatures > 0 {
		log.Printf("%d devices have no features in the store, so can't be considered as transports", w.noFeatures)
	}

	// Observatories' statuses come from the API; the store must agree on
	// where they are.
	obs, err := freshObservatories(ctx, w.obs)
	if err != nil {
		return err
	}

	cands, centre, offMap, noEntry := candidates(w.stars, w.relaying)
	for _, name := range offMap {
		log.Printf("relay star %s isn't in the store's stars; ignoring it", name)
	}
	for _, name := range noEntry {
		log.Printf("relay star %s has no known entry point; ignoring it", name)
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

	starPos := map[string]model.Position{}
	for _, s := range w.stars {
		starPos[s.Designation] = s.Position
	}
	fmt.Printf("\nObservatories (%d):\n", len(obs))
	plans := planAll(obs, w.transports, cands, starPos, led)
	for _, p := range plans {
		printPlan(p)
	}
	// Transports were chosen from the store's statuses: check them before
	// anything is sent.
	if err := verifyTransports(ctx, plans, w.transports); err != nil {
		return err
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
