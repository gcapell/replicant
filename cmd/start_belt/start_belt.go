// Command start_belt sends a mining team to the best asteroid belt that
// nothing is mining yet, and sets it to work. See README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func main() {
	dryRun := flag.Bool("n", false, "print the plan only")
	dbPath := flag.String("db", store.DefaultPath(), "store path")
	flag.Parse()
	ctx := context.Background()

	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	w, err := load(ctx, db)
	db.Close()
	if err != nil {
		log.Fatal(err)
	}
	p, err := makePlan(w.belts, w.devices, w.relayStars, w.starPos)
	if err != nil {
		log.Fatal(err)
	}
	printPlan(p)
	if *dryRun {
		return
	}
	if err := run(ctx, api.NewHub(ctx), p); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s started", p.Belt.Designation)
}

func printPlan(p *Plan) {
	fmt.Println("Undeveloped belts at relay stars, best first:")
	for i, b := range p.Belts {
		mark := "  "
		if i == 0 {
			mark = "->"
		}
		var ls []string
		for _, r := range model.Resources {
			ls = append(ls, fmt.Sprintf("%s=%s", r.Abbrev(), b.Levels[r]))
		}
		fmt.Printf("%s %-20s %-8s %s\n", mark, b.Designation, b.Density, strings.Join(ls, " "))
	}
	t := p.Team
	fmt.Printf("\nTeam at %s (%.1f ly from %s):\n", t.Location, p.Dist, p.Belt.Star)
	fmt.Printf("  ami_mining_controller %s\n", t.MiningControl)
	fmt.Printf("  ami_survey_controller %s\n", t.SurveyControl)
	fmt.Printf("  belt_surveyor         %s\n", t.Surveyor)
	fmt.Printf("  maintenance_drone     %s\n", t.Maintenance)
	fmt.Printf("  mining_drones         %s\n", strings.Join(t.Miners, " "))
	fmt.Printf("\nsurge_carrier %s at %s (%.1f ly from the team)\n\n", p.Carrier.Code, p.Carrier.Location, p.Carrier.Dist)
}

// run carries the team to the belt and sets it to work, stopping at the
// first failure.
func run(ctx context.Context, hub *api.Hub, p *Plan) error {
	t, c, belt := p.Team, p.Carrier.Code, p.Belt.Designation
	team := t.Devices()

	if err := auto.GoTo(ctx, hub, c, t.Location); err != nil {
		return err
	}
	log.Printf("attaching %s to %s", team, c)
	if err := api.AttachDevice(ctx, c, team...); err != nil {
		return fmt.Errorf("attach to %s: %w", c, err)
	}
	if err := checkAttached(ctx, c, team, true); err != nil {
		return err
	}
	if err := auto.GoTo(ctx, hub, c, belt); err != nil {
		return err
	}
	log.Printf("detaching %s from %s", team, c)
	if err := api.DetachDevice(ctx, c, team...); err != nil {
		return fmt.Errorf("detach from %s: %w", c, err)
	}
	if err := checkAttached(ctx, c, team, false); err != nil {
		return err
	}

	if err := adopt(ctx, t.MiningControl, t.Miners); err != nil {
		return err
	}
	if err := api.GatherEvenly(ctx, t.MiningControl); err != nil {
		return fmt.Errorf("gather_evenly %s: %w", t.MiningControl, err)
	}
	log.Printf("%s: gather_evenly", t.MiningControl)

	if err := adopt(ctx, t.SurveyControl, []string{t.Surveyor}); err != nil {
		return err
	}
	if err := api.BeltSearch(ctx, t.SurveyControl); err != nil {
		return fmt.Errorf("belt_search %s: %w", t.SurveyControl, err)
	}
	log.Printf("%s: belt_search", t.SurveyControl)

	if err := api.Patrol(ctx, t.Maintenance); err != nil {
		return fmt.Errorf("patrol %s: %w", t.Maintenance, err)
	}
	log.Printf("%s: patrol", t.Maintenance)
	return nil
}

// checkAttached re-reads carrier and checks that every device is (or, with
// want false, none is) attached to it.
func checkAttached(ctx context.Context, carrier string, devices []string, want bool) error {
	d, err := api.Device(ctx, carrier)
	if err != nil {
		return err
	}
	for _, code := range devices {
		has := slices.ContainsFunc(d.AttachedDevices, func(a model.AttachedDevice) bool { return a.DeviceCode == code })
		if has != want {
			return fmt.Errorf("%s: %s attached = %v, want %v", carrier, code, has, want)
		}
	}
	log.Printf("%s: attached %d device(s)", carrier, len(d.AttachedDevices))
	return nil
}

// adopt has controller adopt devices, then re-reads each to check.
func adopt(ctx context.Context, controller string, devices []string) error {
	log.Printf("%s adopting %s", controller, devices)
	if err := api.AdoptDevice(ctx, controller, devices...); err != nil {
		return fmt.Errorf("%s adopt: %w", controller, err)
	}
	for _, code := range devices {
		d, err := api.Device(ctx, code)
		if err != nil {
			return err
		}
		if d.ControllerDeviceCode != controller {
			return fmt.Errorf("%s: controller is %q after adopt, want %s", code, d.ControllerDeviceCode, controller)
		}
	}
	return nil
}
