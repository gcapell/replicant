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

// handler applies one event to the store, inside the transaction that
// records it.
type handler func(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error

// handlers are the events we apply. Every other event is recorded but not
// applied (for now).
var handlers = map[string]handler{
	"travel.departed": departed,
	"travel.arrived":  arrived,
}

func departed(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.TravelDeparted)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	arrives, err := departureArrival(env, p)
	if err != nil {
		return err
	}
	return store.Departed(ctx, tx, env.DeviceCode, p.Destination, arrives, p.AttachedDevices)
}

// departureArrival is when a departing device is due: arrives_at if the event
// has it, otherwise its creation time plus travel_time_seconds.
func departureArrival(env api.Envelope, p *model.TravelDeparted) (time.Time, error) {
	if p.ArrivesAt != "" {
		return time.Parse(time.RFC3339, p.ArrivesAt)
	}
	if p.TravelTimeSeconds <= 0 || env.CreatedAt.IsZero() {
		return time.Time{}, fmt.Errorf("no arrives_at, and travel_time_seconds %v", p.TravelTimeSeconds)
	}
	return env.CreatedAt.Add(time.Duration(p.TravelTimeSeconds * float64(time.Second))), nil
}

func arrived(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.TravelArrived)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	// After a surge the payload's destination is the bare star; the
	// envelope's location is where the device actually is.
	return store.Arrived(ctx, tx, env.DeviceCode, env.Location, p.AttachedDevices)
}
