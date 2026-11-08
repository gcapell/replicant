package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// deployed: the envelope's device has been deployed from deployed_from (the
// vessel it was stowed in) at the envelope's location. The event doesn't say
// what its status becomes, so, as for directives, it's fetched from
// /devices/{code} (when replaying, that's the status now, not then).
func deployed(ctx context.Context, _ store.Reader, env api.Envelope, payload any) (change, error) {
	p, ok := payload.(*model.DeviceDeployed)
	if !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	if p.DeployedFrom == "" {
		return nil, fmt.Errorf("no deployed_from")
	}
	d, err := eventDevice(ctx, env)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx *sql.Tx) error {
		return store.Deployed(ctx, tx, env.DeviceCode, env.DeviceType, p.DeployedFrom, env.Location, d.Status)
	}, nil
}

// relayActivated: the envelope's ftl_relay is relaying where it is. The
// payload's location and star must agree with the envelope's.
func relayActivated(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.RelayActivated)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	if env.DeviceType != "ftl_relay" {
		return fmt.Errorf("relay.activated for a %s", env.DeviceType)
	}
	if p.Location != env.Location || p.Star != auto.StarOf(env.Location) {
		return fmt.Errorf("relay.activated at %s (star %s), but the event is at %s", p.Location, p.Star, env.Location)
	}
	return store.RelayActivated(ctx, tx, env.DeviceCode, env.Location)
}
