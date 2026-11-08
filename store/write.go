package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gcapell/replicant/model"
)

// Execer is a *sql.DB or *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Queryer is a *sql.DB or *sql.Tx.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// PutDevice inserts d, or replaces what we know about it.
func PutDevice(ctx context.Context, ex Execer, d model.Device) error {
	arrives, err := millis(d.Travel.FinalArrivesAt)
	if err != nil {
		return fmt.Errorf("device %s: travel.final_arrives_at: %w", d.DeviceCode, err)
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO devices
		(device_code, device_type, features, owner, status, location,
		 attached_to, stowed_in, controller, destination, arrives_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (device_code) DO UPDATE SET
			device_type = excluded.device_type,
			features = excluded.features,
			owner = excluded.owner,
			status = excluded.status,
			status_before_travel = NULL, -- not known from the API
			location = excluded.location,
			attached_to = excluded.attached_to,
			stowed_in = excluded.stowed_in,
			controller = excluded.controller,
			destination = excluded.destination,
			arrives_at = excluded.arrives_at`,
		d.DeviceCode, d.DeviceType, null(strings.Join(d.Features, ",")), null(d.ReplicantCode), d.Status, null(d.Location),
		null(d.AttachedToDeviceCode), null(d.StowedInDeviceCode), null(d.ControllerDeviceCode),
		null(d.Travel.FinalDestination), arrives)
	if err != nil {
		return fmt.Errorf("device %s: %w", d.DeviceCode, err)
	}
	return nil
}

// PutStar inserts s, or updates its position and entry point (keeping a known
// entry point if s has none). It leaves the scan counts alone.
func PutStar(ctx context.Context, ex Execer, s model.Star) error {
	_, err := ex.ExecContext(ctx, `INSERT INTO stars (designation, x, y, z, entry_point)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (designation) DO UPDATE SET
			x = excluded.x, y = excluded.y, z = excluded.z,
			entry_point = coalesce(excluded.entry_point, entry_point)`,
		s.Designation, s.Position.X, s.Position.Y, s.Position.Z, null(s.EntryPoint))
	if err != nil {
		return fmt.Errorf("star %s: %w", s.Designation, err)
	}
	return nil
}

// Count returns the number of rows in table (a name from schema.sql, not
// from data).
func Count(ctx context.Context, q Queryer, table string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM "+quote(table)).Scan(&n)
	return n, err
}

// ReplicantInVessel returns the owner of a heaven_vessel carrying a
// replicant_matrix (the first by device code), or "" if there's none.
func ReplicantInVessel(ctx context.Context, q Queryer) (string, error) {
	var owner sql.NullString
	err := q.QueryRowContext(ctx, `SELECT v.owner FROM devices m
		JOIN devices v ON v.device_code = m.stowed_in
		WHERE m.device_type = 'replicant_matrix' AND v.device_type = 'heaven_vessel'
		ORDER BY v.device_code LIMIT 1`).Scan(&owner)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return owner.String, err
}

// null turns "" into NULL.
func null(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// millis turns an RFC 3339 time into Unix milliseconds, and "" into NULL.
func millis(s string) (any, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return t.UnixMilli(), nil
}
