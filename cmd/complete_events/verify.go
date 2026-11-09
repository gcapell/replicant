package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

// usedDevices lists the codes of every device the plans will move or use:
// dispatched freighters, carriers with a route and the spares on it, and
// vessels with completions to make. Sorted, without repeats.
func usedDevices(ds map[int]dispatch, dp devicePlan, spares []spare, carriers []carrier, vr []vesselRoute) []string {
	var codes []string
	for _, d := range ds {
		codes = append(codes, d.Freighter.Code)
	}
	for c, r := range dp.Routes {
		if len(r) == 0 {
			continue
		}
		codes = append(codes, carriers[c].Code)
		for _, s := range r {
			codes = append(codes, spares[s.Spare].Code)
		}
	}
	for _, r := range vr {
		if len(r.Stops) > 0 {
			codes = append(codes, r.Vessel.Code)
		}
	}
	slices.Sort(codes)
	return slices.Compact(codes)
}

// verify fetches each device in codes and checks it against the store's view,
// which the plans were made from. All differences are reported together.
func verify(ctx context.Context, codes []string, stored []model.Device) error {
	byCode := map[string]model.Device{}
	for _, d := range stored {
		byCode[d.DeviceCode] = d
	}
	var errs []error
	for _, c := range codes {
		st, ok := byCode[c]
		if !ok {
			return fmt.Errorf("device %s isn't in the store", c)
		}
		d, err := api.Device(ctx, c)
		if err != nil {
			return err
		}
		errs = append(errs, mismatch(st, *d)...)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("the store disagrees with the API (is monitor running?):\n%w", err)
	}
	return nil
}

// mismatch lists the ways the API's view of a device (a) differs from the
// store's (st): its type, status, where it is, how many devices are attached
// to it, and (for a heaven_vessel) what's stowed in it.
func mismatch(st, a model.Device) []error {
	var errs []error
	check := func(what string, s, x any) {
		if s != x {
			errs = append(errs, fmt.Errorf("%s %s: store has %v, API %v", st.DeviceCode, what, s, x))
		}
	}
	check("type", st.DeviceType, a.DeviceType)
	check("status", st.Status, a.Status)
	check("location", st.Location, a.Location)
	check("attached_to", st.AttachedToDeviceCode, a.AttachedToDeviceCode)
	check("stowed_in", st.StowedInDeviceCode, a.StowedInDeviceCode)
	check("attach_used", st.AttachUsed, a.AttachUsed)
	if st.AttachCapacity >= 0 {
		check("attach_capacity", st.AttachCapacity, a.AttachCapacity)
	}
	if st.DeviceType == "heaven_vessel" {
		check("stowed devices", stowedSet(st), stowedSet(a))
	}
	return errs
}

// stowedSet is d's stowed device codes, as a comparable string.
func stowedSet(d model.Device) string {
	set := map[string]bool{}
	for _, s := range d.StowedDevices {
		set[s.DeviceCode] = true
	}
	return fmt.Sprint(slices.Sorted(maps.Keys(set)))
}
