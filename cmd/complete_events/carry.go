package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// visit is one stop on a carrier's route: detach some devices, then attach
// others. Detaching first only frees room, so the route stays within
// capacity.
type visit struct {
	Loc    string
	Detach []string
	Attach []string
}

// visits groups a route's consecutive stops at one location.
func visits(r []stop, spares []spare) []visit {
	var vs []visit
	for _, s := range r {
		if len(vs) == 0 || vs[len(vs)-1].Loc != s.At.Loc {
			vs = append(vs, visit{Loc: s.At.Loc})
		}
		v := &vs[len(vs)-1]
		code := spares[s.Spare].Code
		if s.Pickup {
			v.Attach = append(v.Attach, code)
		} else {
			v.Detach = append(v.Detach, code)
		}
	}
	return vs
}

// carryAll runs every carrier's route at once. A carrier that fails stops;
// the others carry on.
func carryAll(ctx context.Context, hub *api.Hub, dp devicePlan, spares []spare, carriers []carrier) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for c, r := range dp.Routes {
		if len(r) == 0 {
			continue
		}
		code, vs := carriers[c].Code, visits(r, spares)
		wg.Go(func() {
			if err := carry(ctx, hub, code, vs); err != nil {
				log.Printf("%s: FAILED: %v", code, err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", code, err))
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// carry follows one carrier's visits. A device that isn't where we expected
// when the carrier arrives (moved, attached or stowed meanwhile) is skipped,
// along with its later detach.
func carry(ctx context.Context, hub *api.Hub, c string, vs []visit) error {
	skipped := map[string]bool{}
	for _, v := range vs {
		if err := auto.GoTo(ctx, hub, c, v.Loc); err != nil {
			return err
		}
		if detach := slices.DeleteFunc(slices.Clone(v.Detach), func(d string) bool { return skipped[d] }); len(detach) > 0 {
			if err := api.DetachDevice(ctx, c, detach...); err != nil {
				return fmt.Errorf("detach %v at %s: %w", detach, v.Loc, err)
			}
			log.Printf("%s: detached %v at %s", c, detach, v.Loc)
			if err := checkAttached(ctx, c, detach, false); err != nil {
				return err
			}
		}
		var attach []string
		for _, d := range v.Attach {
			dev, err := api.Device(ctx, d)
			if err != nil {
				return err
			}
			if dev.Location != v.Loc || dev.AttachedToDeviceCode != "" || dev.StowedInDeviceCode != "" {
				log.Printf("%s: warning: skipping %s: expected it free at %s, it's at %s (attached to %q, stowed in %q)",
					c, d, v.Loc, dev.Location, dev.AttachedToDeviceCode, dev.StowedInDeviceCode)
				skipped[d] = true
				continue
			}
			attach = append(attach, d)
		}
		if len(attach) > 0 {
			if err := api.AttachDevice(ctx, c, attach...); err != nil {
				return fmt.Errorf("attach %v at %s: %w", attach, v.Loc, err)
			}
			log.Printf("%s: attached %v at %s", c, attach, v.Loc)
			if err := checkAttached(ctx, c, attach, true); err != nil {
				return err
			}
		}
	}
	log.Printf("%s: route done", c)
	return nil
}

// checkAttached re-reads carrier c and checks that each of devices is (or,
// with want false, isn't) attached to it.
func checkAttached(ctx context.Context, c string, devices []string, want bool) error {
	d, err := api.Device(ctx, c)
	if err != nil {
		return err
	}
	for _, code := range devices {
		has := slices.ContainsFunc(d.AttachedDevices, func(a model.AttachedDevice) bool { return a.DeviceCode == code })
		if has != want {
			return fmt.Errorf("after %s, %s attached = %v", map[bool]string{true: "attach", false: "detach"}[want], code, has)
		}
	}
	return nil
}
