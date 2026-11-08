package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Attached records that carrier, at location, has attached target (of
// targetType). The target is at the carrier's location; its status is left
// alone (the API shows carried devices as idle or coordinating). Being
// already attached to carrier is fine (the snapshot may be newer than the
// event); being attached to another device, or of another type, is an error.
func Attached(ctx context.Context, ex ExecQueryer, carrier, location, target, targetType string) error {
	if err := checkAttachment(ctx, ex, carrier, location, target, targetType, true); err != nil {
		return err
	}
	return updateOne(ctx, ex, target,
		`UPDATE devices SET attached_to = ?, location = ? WHERE device_code = ?`, carrier, location, target)
}

// Detached records that carrier, at location, has detached target, which is
// left at location. Being already detached is fine; being attached to some
// other device, or of another type, is an error.
func Detached(ctx context.Context, ex ExecQueryer, carrier, location, target, targetType string) error {
	if err := checkAttachment(ctx, ex, carrier, location, target, targetType, false); err != nil {
		return err
	}
	return updateOne(ctx, ex, target,
		`UPDATE devices SET attached_to = NULL, location = ? WHERE device_code = ?`, location, target)
}

// ExecQueryer is a *sql.DB or *sql.Tx.
type ExecQueryer interface {
	Execer
	Queryer
}

func checkAttachment(ctx context.Context, q Queryer, carrier, location, target, targetType string, attaching bool) error {
	verb := "detached"
	if attaching {
		verb = "attached"
	}
	if carrier == "" || location == "" || target == "" {
		return fmt.Errorf("%s: carrier %q, location %q, target %q: none may be empty", verb, carrier, location, target)
	}
	if carrier == target {
		return fmt.Errorf("%s %s to itself", verb, carrier)
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM devices WHERE device_code = ?`, carrier).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%s %s: carrier %s not in the store", verb, target, carrier)
	}
	var typ string
	var on sql.NullString
	err := q.QueryRowContext(ctx, `SELECT device_type, attached_to FROM devices WHERE device_code = ?`, target).Scan(&typ, &on)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %s to %s: device %s not in the store", verb, target, carrier, target)
	}
	if err != nil {
		return err
	}
	if typ != targetType {
		return fmt.Errorf("%s %s to %s: event says it's a %s, store a %s", verb, target, carrier, targetType, typ)
	}
	if on.Valid && on.String != carrier {
		return fmt.Errorf("%s %s to %s, but the store has it attached to %s", verb, target, carrier, on.String)
	}
	return nil
}
