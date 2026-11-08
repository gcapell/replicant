package store

import (
	"context"
	"fmt"
	"time"
)

// Departed records that device has set off for destination, due at arrives,
// carrying attached. While travelling a device has no location (as the API
// shows it). A device that can't carry others keeps the status it had, to
// restore on arrival (an AMI drone's status comes from its directive). The
// attached devices' status is left alone.
func Departed(ctx context.Context, ex Execer, device, destination string, arrives time.Time, attached []string) error {
	if destination == "" || arrives.IsZero() {
		return fmt.Errorf("%s departed with no destination or arrival time", device)
	}
	if err := updateOne(ctx, ex, device, `UPDATE devices SET
		status_before_travel = CASE
			WHEN can_carry THEN NULL
			WHEN status = 'travelling' THEN status_before_travel -- departing again mid-trip
			ELSE status END,
		status = 'travelling', location = NULL, destination = ?, arrives_at = ?
		WHERE device_code = ?`, destination, arrives.UnixMilli(), device); err != nil {
		return err
	}
	for _, a := range attached {
		if err := updateOne(ctx, ex, a, `UPDATE devices SET location = NULL WHERE device_code = ?`, a); err != nil {
			return fmt.Errorf("attached to %s: %w", device, err)
		}
	}
	return nil
}

// Arrived records that device has reached location, with attached. A device
// that can carry others becomes idle; any other gets back the status it had
// when it departed, or idle if we didn't see it depart. The attached devices'
// status is left alone.
func Arrived(ctx context.Context, ex Execer, device, location string, attached []string) error {
	if location == "" {
		return fmt.Errorf("%s arrived with no location", device)
	}
	if err := updateOne(ctx, ex, device, `UPDATE devices SET
		status = CASE WHEN can_carry THEN 'idle' ELSE coalesce(status_before_travel, 'idle') END,
		status_before_travel = NULL,
		location = ?, destination = NULL, arrives_at = NULL
		WHERE device_code = ?`, location, device); err != nil {
		return err
	}
	for _, a := range attached {
		if err := updateOne(ctx, ex, a, `UPDATE devices SET location = ? WHERE device_code = ?`, location, a); err != nil {
			return fmt.Errorf("attached to %s: %w", device, err)
		}
	}
	return nil
}

// updateOne runs an UPDATE that must change exactly one device's row.
func updateOne(ctx context.Context, ex Execer, device, query string, args ...any) error {
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("device %s: %w", device, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("device %s: not in the store", device)
	}
	return nil
}
