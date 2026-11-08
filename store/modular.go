package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Transforming records that a modular device (e.g. a galactic_observatory)
// of deviceType now has status, as device.compacting, .compacted, .unfurling
// and .unfurled report. completesAt is when the change finishes, or zero once
// it has (stored as NULL). A device not in the store, or of another type, is
// an error.
func Transforming(ctx context.Context, ex ExecQueryer, device, deviceType, status string, completesAt time.Time) error {
	var typ string
	err := ex.QueryRowContext(ctx, `SELECT device_type FROM devices WHERE device_code = ?`, device).Scan(&typ)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("device %s: not in the store", device)
	}
	if err != nil {
		return err
	}
	if typ != deviceType {
		return fmt.Errorf("device %s: event says it's a %s, store a %s", device, deviceType, typ)
	}
	if err := SetStatus(ctx, ex, device, status); err != nil {
		return err
	}
	var at any
	if !completesAt.IsZero() {
		at = completesAt.UnixMilli()
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET completes_at = ? WHERE device_code = ?`, at, device)
}
