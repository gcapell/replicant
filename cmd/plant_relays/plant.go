package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// runTeams runs every team with a route at once. A team stops at its first
// failure; the others carry on.
func runTeams(ctx context.Context, teams []team) error {
	hub := api.NewHub(ctx)
	errs := make([]error, len(teams))
	var wg sync.WaitGroup
	for i, t := range teams {
		if len(t.Route) == 0 {
			continue
		}
		wg.Go(func() {
			if err := runTeam(ctx, hub, t); err != nil {
				errs[i] = fmt.Errorf("%s: %w", t.Vessel.Code, err)
				log.Printf("%s stopped: %v", t.Vessel.Code, err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// runTeam fetches the team's relays (if it needs to), then plants one at
// each star on its route.
func runTeam(ctx context.Context, hub *api.Hub, t team) error {
	v := t.Vessel
	if f := t.Fetch; f.Loc != "" {
		log.Printf("%s fetching %d relay(s) from %s", v.Code, len(f.Relays), f.Loc)
		if err := auto.GoTo(ctx, hub, v.Code, f.Loc); err != nil {
			return err
		}
		for i, r := range f.Relays {
			if err := auto.StowInto(ctx, r.Code, r.Owner, v.Code, v.Replicant); err != nil {
				return fmt.Errorf("fetching from %s (%d/%d stowed): %w", f.Loc, i, len(f.Relays), err)
			}
		}
	}
	n := len(t.Route)
	for i, s := range t.Route {
		log.Printf("=== %s stop %d/%d: %s", v.Code, i+1, n, s.Name)
		if err := plantAt(ctx, hub, v.Code, s, t.Relays[i]); err != nil {
			return fmt.Errorf("stop %d/%d %s: %w", i+1, n, s.Name, err)
		}
	}
	log.Printf("%s planted %d relay(s)", v.Code, n)
	return nil
}

// plantAt takes vessel to s's entry point (scanning for it if we don't know
// it), then deploys and activates relay there.
func plantAt(ctx context.Context, hub *api.Hub, vessel string, s star, relay string) error {
	entry := s.EntryPoint
	if entry == "" {
		if err := auto.GoTo(ctx, hub, vessel, oort(s.Name)); err != nil {
			return err
		}
		scan, err := api.SystemScan(ctx, vessel)
		if err != nil {
			return err
		}
		entry = scan.EntryPoint
		log.Printf("%s entry point is %s", s.Name, entry)
	}
	if auto.StarOf(entry) != s.Name {
		return fmt.Errorf("entry point %s isn't in %s", entry, s.Name)
	}
	if err := auto.GoTo(ctx, hub, vessel, entry); err != nil {
		return err
	}

	// Deploy and activate are both immediate, so check the relay straight after each.
	if err := api.Deploy(ctx, relay); err != nil {
		return fmt.Errorf("deploy %s: %w", relay, err)
	}
	d, err := api.Device(ctx, relay)
	if err != nil {
		return err
	}
	if d.Location != entry {
		return fmt.Errorf("%s is at %q after deploy, not %s", relay, d.Location, entry)
	}
	log.Printf("deployed %s at %s", relay, entry)

	if err := api.Activate(ctx, relay); err != nil {
		return fmt.Errorf("activate %s: %w", relay, err)
	}
	d, err = api.Device(ctx, relay)
	if err != nil {
		return err
	}
	if d.Status != "relaying" {
		return fmt.Errorf("%s at %s is %q after activate, not relaying", relay, d.Location, d.Status)
	}
	log.Printf("%s relaying at %s", relay, d.Location)
	return nil
}

func oort(star string) string { return star + "-OORT" }

// stowedRelayCodes returns the codes of the ftl_relays stowed in v.
func stowedRelayCodes(v *model.Device) []string {
	var codes []string
	for _, d := range v.StowedDevices {
		if d.DeviceType == "ftl_relay" {
			codes = append(codes, d.DeviceCode)
		}
	}
	return codes
}
