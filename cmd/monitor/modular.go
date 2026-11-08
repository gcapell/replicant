package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// Modular devices (galactic_observatories so far) compact and unfurl. The
// envelope's device is the one changing; its location is ignored.

// compacting and unfurling: the device has that status until completes_at
// (a missing or unparseable time is an error).
func compacting(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.DeviceCompacting)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return transforming(ctx, tx, env, "compacting", p.CompletesAt)
}

func unfurling(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.DeviceUnfurling)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return transforming(ctx, tx, env, "unfurling", p.CompletesAt)
}

func transforming(ctx context.Context, tx *sql.Tx, env api.Envelope, status, completesAt string) error {
	if completesAt == "" {
		return fmt.Errorf("%s %s: no completes_at", env.Event, env.DeviceCode)
	}
	at, err := model.ParseEventTime(completesAt)
	if err != nil {
		return fmt.Errorf("%s %s: completes_at: %w", env.Event, env.DeviceCode, err)
	}
	return store.Transforming(ctx, tx, env.DeviceCode, env.DeviceType, status, at)
}

// compacted: the device is compacted (a status the API is known to report).
func compacted(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	if _, ok := payload.(*model.DeviceCompacted); !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return store.Transforming(ctx, tx, env.DeviceCode, env.DeviceType, "compacted", time.Time{})
}

// unfurled: we haven't seen what status an unfurled device has, so, as for
// directives, it's fetched from /devices/{code} (when replaying, that's the
// status now, not then).
func unfurled(ctx context.Context, _ store.Reader, env api.Envelope, payload any) (change, error) {
	if _, ok := payload.(*model.DeviceUnfurled); !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	d, err := eventDevice(ctx, env)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx *sql.Tx) error {
		return store.Transforming(ctx, tx, d.DeviceCode, d.DeviceType, d.Status, time.Time{})
	}, nil
}
