package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// execute carries out p's steps in order, stopping at the first failure. The
// last step may start something long (compact, unfurl, prospect), which we
// don't wait for.
func execute(ctx context.Context, hub *api.Hub, led *ledgerFile, p obsPlan) error {
	obs := p.Obs.DeviceCode
	for i, s := range p.Steps {
		log.Printf("%s step %d/%d: %s", obs, i+1, len(p.Steps), s)
		if err := do(ctx, hub, led, obs, s); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}

func do(ctx context.Context, hub *api.Hub, led *ledgerFile, obs string, s step) error {
	switch s.Kind {
	case waitTrip:
		return auto.FinishTrip(ctx, hub, s.Transport)

	case attach:
		if err := api.AttachDevice(ctx, s.Transport, obs); err != nil {
			return err
		}
		d, err := api.Device(ctx, obs)
		if err != nil {
			return err
		}
		if d.AttachedToDeviceCode != s.Transport {
			return fmt.Errorf("%s is attached to %q after attach, not %s", obs, d.AttachedToDeviceCode, s.Transport)
		}
		log.Printf("%s attached to %s at %s", obs, s.Transport, d.Location)

	case fetch, travel:
		return auto.GoTo(ctx, hub, s.Transport, s.Dest)

	case detach:
		if err := api.DetachDevice(ctx, s.Transport, obs); err != nil {
			return err
		}
		d, err := api.Device(ctx, obs)
		if err != nil {
			return err
		}
		if d.AttachedToDeviceCode != "" {
			return fmt.Errorf("%s is still attached to %s after detach", obs, d.AttachedToDeviceCode)
		}
		// Unfurling takes hours, so make sure we're in the right system first.
		if auto.StarOf(d.Location) != auto.StarOf(s.Dest) {
			return fmt.Errorf("%s is at %s after detach, not in %s", obs, d.Location, auto.StarOf(s.Dest))
		}
		log.Printf("%s detached from %s at %s", obs, s.Transport, d.Location)

	case unfurl:
		return startLong(ctx, obs, "unfurl", api.Unfurl)

	case compact:
		return startLong(ctx, obs, "compact", api.Compact)

	case prospect:
		return doProspect(ctx, led, obs, s)

	default:
		return fmt.Errorf("unknown step kind %d", s.Kind)
	}
	return nil
}

// startLong sends a long-running command and logs the device's status after.
func startLong(ctx context.Context, obs, name string, cmd func(context.Context, string) error) error {
	if err := cmd(ctx, obs); err != nil {
		return err
	}
	d, err := api.Device(ctx, obs)
	if err != nil {
		return err
	}
	log.Printf("%s started %s: now %s%s", obs, name, d.Status, eta(d))
	return nil
}

func eta(d *model.Device) string {
	for _, p := range []model.PendingStatus{d.Compact, d.Prospect} {
		if p.CompletesAt != "" {
			return ", completes " + p.CompletesAt
		}
	}
	return ""
}

// doProspect prospects from s.Star and records it in the ledger. If the
// observatory refuses (area too dense, or already surveyed from), it records
// that instead, and compacts so the next run moves it on.
func doProspect(ctx context.Context, led *ledgerFile, obs string, s step) error {
	r, err := api.Prospect(ctx, obs, s.Dir)
	if err == nil {
		log.Printf("%s prospecting from %s: %s, completes %s", obs, s.Star, r.Status, r.CompletesAt)
		return led.add(s.Star, "prospected "+time.Now().Format(time.RFC3339))
	}
	note, ok := refusal(err)
	if !ok {
		return err
	}
	log.Printf("%s refused to prospect from %s: %s", obs, s.Star, note)
	if err := led.add(s.Star, note); err != nil {
		return err
	}
	return startLong(ctx, obs, "compact", api.Compact)
}

var ratioRE = regexp.MustCompile(`"((?:outward_)?ratio)"\s*:\s*([-0-9.eE+]+)`)

// refusal reports whether err is prospect refusing to run here for good: the
// area is too dense (a 400 reporting ratios), or this star has already been
// surveyed from. It returns the ledger note to record.
func refusal(err error) (string, bool) {
	var he *api.HTTPError
	if !errors.As(err, &he) || he.StatusCode != 400 {
		return "", false
	}
	body := string(he.Body)
	if m := ratioRE.FindAllStringSubmatch(body, -1); m != nil {
		note := "dense"
		for _, kv := range m {
			note += " " + kv[1] + "=" + kv[2]
		}
		return note, true
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "already") || strings.Contains(lower, "surveyed") {
		return "already surveyed (refused)", true
	}
	return "", false
}
