// collect_ftl_relays stows the inactive FTL relays at a vessel's location into
// that vessel, first transferring any owned by another replicant (on the same
// account) to the vessel's replicant. It stops when the vessel's hold is full
// or there are no inactive relays left, and exits at the first failure.
//
//	collect_ftl_relays VESSEL_ID
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s VESSEL_ID\n", os.Args[0])
		os.Exit(2)
	}
	if err := run(context.Background(), os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, vesselID string) error {
	v, err := api.Device(ctx, vesselID)
	if err != nil {
		return err
	}
	if v.DeviceType != "heaven_vessel" {
		return fmt.Errorf("%s is a %s, not a heaven_vessel", vesselID, v.DeviceType)
	}
	if v.Status == "travelling" {
		return fmt.Errorf("%s is travelling; try again once it arrives", vesselID)
	}
	log.Printf("%s (replicant %s) at %s, hold %d/%d", vesselID, v.ReplicantCode, v.Location, v.StowUsed, v.StowCapacity)

	all, err := auto.DevicesAt(ctx, v.Location, "ftl_relay")
	if err != nil {
		return err
	}
	relays := inactiveRelaysAt(all, v.Location)
	log.Printf("%d inactive ftl_relay(s) at %s", len(relays), v.Location)

	free := v.StowCapacity - v.StowUsed
	var stowed []string
	for _, r := range relays {
		if free <= 0 {
			log.Printf("%s hold is full", vesselID)
			break
		}
		if err := collect(ctx, r, v); err != nil {
			return fmt.Errorf("%s: %w (stowed %d before this: %s)", r.DeviceCode, err, len(stowed), stowed)
		}
		// Stow is immediate: no error means it's in the hold.
		stowed = append(stowed, r.DeviceCode)
		free--
	}

	log.Printf("stowed %d relay(s) %s; %d inactive relay(s) left at %s",
		len(stowed), stowed, len(relays)-len(stowed), v.Location)
	return nil
}

// collect transfers relay r to the vessel's replicant if needed, then stows it.
func collect(ctx context.Context, r model.Device, v *model.Device) error {
	if r.ReplicantCode != v.ReplicantCode {
		if err := api.ChangeOwner(ctx, r.DeviceCode, v.ReplicantCode); err != nil {
			return fmt.Errorf("change owner %s -> %s: %w", r.ReplicantCode, v.ReplicantCode, err)
		}
		log.Printf("%s now owned by %s (was %s)", r.DeviceCode, v.ReplicantCode, r.ReplicantCode)
	}
	if err := api.Stow(ctx, r.DeviceCode, v.DeviceCode); err != nil {
		return fmt.Errorf("stow in %s: %w", v.DeviceCode, err)
	}
	log.Printf("stowed %s in %s", r.DeviceCode, v.DeviceCode)
	return nil
}

// inactiveRelaysAt returns the inactive ftl_relays exactly at loc, in the order given.
func inactiveRelaysAt(devices []model.Device, loc string) []model.Device {
	var relays []model.Device
	for _, d := range devices {
		if d.DeviceType == "ftl_relay" && d.Status == "inactive" && d.Location == loc && d.StowedInDeviceCode == "" {
			relays = append(relays, d)
		}
	}
	return relays
}
