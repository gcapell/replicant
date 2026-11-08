package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Deployed records that device (of deviceType), stowed in host, has been
// deployed at location with status. Being already deployed there (not stowed,
// at location) is fine, as the snapshot may be newer than the event. Being
// stowed somewhere else, or deployed somewhere else, or of another type, is an
// error.
func Deployed(ctx context.Context, ex ExecQueryer, device, deviceType, host, location, status string) error {
	if device == "" || host == "" || location == "" {
		return fmt.Errorf("deployed: device %q, host %q, location %q: none may be empty", device, host, location)
	}
	var typ string
	var stowedIn, loc sql.NullString
	err := ex.QueryRowContext(ctx, `SELECT device_type, stowed_in, location FROM devices WHERE device_code = ?`, device).
		Scan(&typ, &stowedIn, &loc)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("deployed %s: not in the store", device)
	}
	if err != nil {
		return err
	}
	if typ != deviceType {
		return fmt.Errorf("deployed %s: event says it's a %s, store a %s", device, deviceType, typ)
	}
	switch {
	case stowedIn.String == host:
	case !stowedIn.Valid && loc.String == location:
	case stowedIn.Valid:
		return fmt.Errorf("deployed %s from %s, but the store has it stowed in %s", device, host, stowedIn.String)
	default:
		return fmt.Errorf("deployed %s from %s at %s, but the store has it unstowed at %q", device, host, location, loc.String)
	}
	if err := updateOne(ctx, ex, device,
		`UPDATE devices SET stowed_in = NULL, location = ? WHERE device_code = ?`, location, device); err != nil {
		return err
	}
	return SetStatus(ctx, ex, device, status)
}

// RelayActivated records that the ftl_relay device, at location, is now
// relaying. A device not in the store, not an ftl_relay, stowed, or somewhere
// else, is an error.
func RelayActivated(ctx context.Context, ex ExecQueryer, device, location string) error {
	if device == "" || location == "" {
		return fmt.Errorf("relay activated: device %q, location %q: neither may be empty", device, location)
	}
	var typ string
	var stowedIn, loc sql.NullString
	err := ex.QueryRowContext(ctx, `SELECT device_type, stowed_in, location FROM devices WHERE device_code = ?`, device).
		Scan(&typ, &stowedIn, &loc)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("relay activated %s: not in the store", device)
	}
	if err != nil {
		return err
	}
	switch {
	case typ != "ftl_relay":
		return fmt.Errorf("relay activated %s: store has a %s", device, typ)
	case stowedIn.Valid:
		return fmt.Errorf("relay activated %s at %s, but the store has it stowed in %s", device, location, stowedIn.String)
	case loc.String != location:
		return fmt.Errorf("relay activated %s at %s, but the store has it at %q", device, location, loc.String)
	}
	return SetStatus(ctx, ex, device, "relaying")
}
