package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// verify checks the plan's inputs against the API before anything is done:
// the relay mesh, each vessel that has a route, and each relay it will fetch.
// Every difference is reported together.
func verify(ctx context.Context, teams []team, storeMesh map[string]bool) error {
	relays, err := auto.Devices(ctx, "ftl_relay")
	if err != nil {
		return err
	}
	var errs []error
	if apiMesh := relayingStars(relays); !maps.Equal(apiMesh, storeMesh) {
		errs = append(errs, fmt.Errorf("relay stars: store has %v, API has %v",
			slices.Sorted(maps.Keys(storeMesh)), slices.Sorted(maps.Keys(apiMesh))))
	}
	byCode := map[string]model.Device{}
	for _, r := range relays {
		byCode[r.DeviceCode] = r
	}
	for _, t := range teams {
		if len(t.Route) == 0 {
			continue
		}
		d, err := api.Device(ctx, t.Vessel.Code)
		if err != nil {
			return err
		}
		errs = append(errs, vesselMismatch(t.Vessel, *d)...)
		for _, r := range t.Fetch.Relays {
			d, ok := byCode[r.Code]
			if !ok {
				errs = append(errs, fmt.Errorf("relay %s: the API doesn't list it", r.Code))
				continue
			}
			errs = append(errs, relayMismatch(r, t.Fetch.Loc, d)...)
		}
	}
	return errors.Join(errs...)
}

// vesselMismatch lists how the API's view of a vessel (d) differs from the
// store's (v).
func vesselMismatch(v vessel, d model.Device) []error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("vessel %s: "+format, append([]any{v.Code}, args...)...))
	}
	if !auto.ReadyReplicant(d) {
		bad("not ready (status %q, attached to %q, stowed in %q, or no replicant_matrix)",
			d.Status, d.AttachedToDeviceCode, d.StowedInDeviceCode)
	}
	if d.Location != v.Loc {
		bad("store says it's at %s, API says %q", v.Loc, d.Location)
	}
	if d.ReplicantCode != v.Replicant {
		bad("store says replicant %s, API says %q", v.Replicant, d.ReplicantCode)
	}
	if got := stowedRelayCodes(&d); !slices.Equal(got, v.Relays) {
		bad("store says it carries relays %v, API says %v", v.Relays, got)
	}
	if free := d.StowCapacity - d.StowUsed; free != v.Free {
		bad("store says %d free stow slots, API says %d (%d of %d used)", v.Free, free, d.StowUsed, d.StowCapacity)
	}
	return errs
}

// relayMismatch lists how the API's view of a relay (d) to be fetched from
// loc differs from the store's (r).
func relayMismatch(r relay, loc string, d model.Device) []error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("relay %s: "+format, append([]any{r.Code}, args...)...))
	}
	if d.Location != loc {
		bad("store says it's at %s, API says %q", loc, d.Location)
	}
	if d.StowedInDeviceCode != "" {
		bad("stowed in %s", d.StowedInDeviceCode)
	}
	if d.AttachedToDeviceCode != "" {
		bad("attached to %s", d.AttachedToDeviceCode)
	}
	if d.Status == "relaying" {
		bad("relaying")
	}
	if d.ReplicantCode != r.Owner {
		bad("store says owner %q, API says %q", r.Owner, d.ReplicantCode)
	}
	return errs
}
