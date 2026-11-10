package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gcapell/replicant/model"
)

// Observation statuses (the observations table).
const (
	ObsNotYet          = "not_yet"     // reserved; the observatory is on its way, or unfurling
	ObsProspecting     = "prospecting" // prospect accepted; waiting for prospect.completed
	ObsSuccess         = "success"
	ObsTooDense        = "too_dense"             // prospect refused: too many neighbours
	ObsAlreadySurveyed = "already_surveyed_from" // prospect refused: someone already prospected from here
)

// Observation is a row of the observations table.
type Observation struct {
	Star       string
	Dir        model.Position // zero for rows seeded by hand
	Device     string         // "" for rows seeded by hand
	Status     string
	StarsFound []string
}

// ActiveObservation is device's observation that isn't finished (not_yet or
// prospecting), if it has one.
func ActiveObservation(ctx context.Context, q Queryer, device string) (o Observation, ok bool, err error) {
	var x, y, z sql.NullFloat64
	var found sql.NullString
	err = q.QueryRowContext(ctx, `SELECT star, dir_x, dir_y, dir_z, status, stars_found FROM observations
		WHERE device_code = ? AND status IN ('not_yet', 'prospecting')`, device).
		Scan(&o.Star, &x, &y, &z, &o.Status, &found)
	if errors.Is(err, sql.ErrNoRows) {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	if !x.Valid || !y.Valid || !z.Valid {
		return Observation{}, false, fmt.Errorf("observation from %s by %s has no direction", o.Star, device)
	}
	o.Dir = model.Position{X: x.Float64, Y: y.Float64, Z: z.Float64}
	o.Device = device
	if found.String != "" {
		o.StarsFound = strings.Split(found.String, ",")
	}
	return o, true, nil
}

// ObservedStars is the set of stars with an observation of any status:
// those no observatory may choose again.
func ObservedStars(ctx context.Context, q Reader) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT star FROM observations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		seen[s] = true
	}
	return seen, rows.Err()
}

// ReserveObservation records that device will observe from star in
// direction dir (status not_yet). A star already in the table, or a device
// with an observation under way, is an error.
func ReserveObservation(ctx context.Context, ex Execer, star string, dir model.Position, device string, now time.Time) error {
	_, err := ex.ExecContext(ctx, `INSERT INTO observations (star, dir_x, dir_y, dir_z, device_code, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'not_yet', ?, ?)`, star, dir.X, dir.Y, dir.Z, device, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return fmt.Errorf("reserving %s for %s: %w", star, device, err)
	}
	return nil
}

// SetObservationStatus moves device's observation from star from status
// from to status to. Anything else (no such row, another device, another
// status) is an error.
func SetObservationStatus(ctx context.Context, ex Execer, star, device, from, to string, now time.Time) error {
	return updateObservation(ctx, ex, star, device, from, `UPDATE observations SET status = ?, updated_at = ?
		WHERE star = ? AND device_code = ? AND status = ?`, to, now.UnixMilli(), star, device, from)
}

// ObservationCompleted records what device's prospect from star found: its
// observation (which must be prospecting) becomes success, with stars_found.
// One that's already success with the same stars found is left alone, so
// replaying the event (-catchup) is fine.
func ObservationCompleted(ctx context.Context, ex ExecQueryer, star, device string, found []string, now time.Time) error {
	list := strings.Join(found, ",")
	var n int
	err := ex.QueryRowContext(ctx, `SELECT count(*) FROM observations WHERE star = ? AND device_code = ?
		AND status = 'success' AND coalesce(stars_found, '') = ?`, star, device, list).Scan(&n)
	if err != nil || n == 1 {
		return err
	}
	return updateObservation(ctx, ex, star, device, ObsProspecting, `UPDATE observations SET status = 'success', stars_found = ?, updated_at = ?
		WHERE star = ? AND device_code = ? AND status = 'prospecting'`, null(list), now.UnixMilli(), star, device)
}

func updateObservation(ctx context.Context, ex Execer, star, device, from, query string, args ...any) error {
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("observation from %s by %s: %w", star, device, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("no %s observation from %s by %s", from, star, device)
	}
	return nil
}

// Stars lists every star in the store, by name, with its position and entry
// point (EntryPoint "" if unknown).
func Stars(ctx context.Context, q Reader) ([]model.Star, error) {
	rows, err := q.QueryContext(ctx, `SELECT designation, x, y, z, coalesce(entry_point, '') FROM stars ORDER BY designation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Star
	for rows.Next() {
		var s model.Star
		if err := rows.Scan(&s.Designation, &s.Position.X, &s.Position.Y, &s.Position.Z, &s.EntryPoint); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetTaskLocation moves task id to location.
func SetTaskLocation(ctx context.Context, ex Execer, id int64, location string, now time.Time) error {
	res, err := ex.ExecContext(ctx, `UPDATE tasks SET location = ?, updated_at = ? WHERE id = ?`, null(location), now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("setting task %d's location: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("setting task %d's location: no such task", id)
	}
	return nil
}

// DeviceAt is a device's code and location ("" if it has none).
type DeviceAt struct{ Code, Location string }

// DevicesOfType lists the devices of type typ, by code.
func DevicesOfType(ctx context.Context, q Reader, typ string) ([]DeviceAt, error) {
	rows, err := q.QueryContext(ctx, `SELECT device_code, coalesce(location, '') FROM devices WHERE device_type = ? ORDER BY device_code`, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceAt
	for rows.Next() {
		var d DeviceAt
		if err := rows.Scan(&d.Code, &d.Location); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
