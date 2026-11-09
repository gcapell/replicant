package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

// freshObservatories fetches each of the store's observatories from the API,
// for its status. It's an error if the API disagrees with the store about
// where one is (location, attachment, stowing); all such problems are
// reported together.
func freshObservatories(ctx context.Context, stored []model.Device) ([]model.Device, error) {
	var out []model.Device
	var errs []error
	for _, s := range stored {
		d, err := api.Device(ctx, s.DeviceCode)
		if err != nil {
			return nil, err
		}
		errs = append(errs, mismatch(s, *d)...)
		out = append(out, *d)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("the store disagrees with the API (is monitor running?):\n%w", err)
	}
	return out, nil
}

// mismatch lists the ways the API's view of a device (api) differs from the
// store's (st), in where it is.
func mismatch(st, api model.Device) []error {
	var errs []error
	check := func(what, s, a string) {
		if s != a {
			errs = append(errs, fmt.Errorf("%s %s: store has %q, API %q", st.DeviceCode, what, s, a))
		}
	}
	check("type", st.DeviceType, api.DeviceType)
	check("location", st.Location, api.Location)
	check("attached_to", st.AttachedToDeviceCode, api.AttachedToDeviceCode)
	check("stowed_in", st.StowedInDeviceCode, api.StowedInDeviceCode)
	return errs
}

// verifyTransports fetches every transport a plan uses and checks it against
// the store's view, which the plan was made from: where it is, and its status
// (and, if travelling, its destination). One that is to pick up an observatory
// must still be idle with nothing attached. All problems are reported together.
func verifyTransports(ctx context.Context, plans []obsPlan, transports []model.Device) error {
	stored := map[string]model.Device{}
	for _, t := range transports {
		stored[t.DeviceCode] = t
	}
	var errs []error
	for _, p := range plans {
		if p.Transport == "" || len(p.Steps) == 0 {
			continue
		}
		st, ok := stored[p.Transport]
		if !ok {
			return fmt.Errorf("plan for %s uses transport %s, which isn't in the store", p.Obs.DeviceCode, p.Transport)
		}
		d, err := api.Device(ctx, p.Transport)
		if err != nil {
			return err
		}
		errs = append(errs, transportMismatch(p, st, *d)...)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("transports have changed since the store last heard (is monitor running?):\n%w", err)
	}
	return nil
}

func transportMismatch(p obsPlan, st, api model.Device) []error {
	errs := mismatch(st, api)
	if st.Status != api.Status {
		errs = append(errs, fmt.Errorf("%s status: store has %q, API %q", st.DeviceCode, st.Status, api.Status))
	}
	if st.Status == "travelling" && st.Travel.FinalDestination != api.Travel.FinalDestination {
		errs = append(errs, fmt.Errorf("%s destination: store has %q, API %q",
			st.DeviceCode, st.Travel.FinalDestination, api.Travel.FinalDestination))
	}
	if picksUp(p) && len(api.AttachedDevices) > 0 {
		errs = append(errs, fmt.Errorf("%s is to carry %s but is already carrying %v",
			st.DeviceCode, p.Obs.DeviceCode, api.AttachedDevices))
	}
	return errs
}

// picksUp reports whether p attaches its observatory to its transport.
func picksUp(p obsPlan) bool {
	for _, s := range p.Steps {
		if s.Kind == attach {
			return true
		}
	}
	return false
}
