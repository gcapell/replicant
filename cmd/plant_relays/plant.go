package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// plantAll follows route, planting one of relays (codes of stowed ftl_relays)
// at each stop, until it runs out of relays or stops. It exits at the first
// failure.
func plantAll(ctx context.Context, vessel string, route []star, relays []string) error {
	hub := api.NewHub(ctx)
	n := min(len(route), len(relays))
	for i, s := range route[:n] {
		log.Printf("=== stop %d/%d: %s", i+1, n, s.Name)
		if err := plantAt(ctx, hub, vessel, s, relays[i]); err != nil {
			return fmt.Errorf("stop %d/%d %s: %w", i+1, n, s.Name, err)
		}
	}
	log.Printf("planted %d relay(s)", n)
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
