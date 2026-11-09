package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gcapell/replicant/model"
)

// world is what gaze reads from the store. Devices are model.Devices filled in
// from the store's columns, as far as it knows them; statuses are checked
// against the API before they're relied on.
type world struct {
	stars      []model.Star    // every star in the store, with its entry point if known
	relaying   map[string]bool // stars with a relaying ftl_relay (the relay_stars view)
	obs        []model.Device  // every galactic_observatory
	transports []model.Device  // every device with surge and attach (AttachCapacity from its blueprint, AttachUsed counted)
	noFeatures int             // devices whose features the store doesn't know, so can't be transports
}

func load(ctx context.Context, db *sql.DB) (*world, error) {
	w := &world{relaying: map[string]bool{}}
	var err error
	if w.stars, err = loadStars(ctx, db); err != nil {
		return nil, err
	}
	if err := eachRow(ctx, db, `SELECT star FROM relay_stars WHERE star IS NOT NULL`, func(r *sql.Rows) error {
		var s string
		err := r.Scan(&s)
		w.relaying[s] = true
		return err
	}); err != nil {
		return nil, err
	}
	if w.obs, err = loadDevices(ctx, db, `d.device_type = 'galactic_observatory'`); err != nil {
		return nil, err
	}
	if w.transports, err = loadDevices(ctx, db, `instr(',' || d.features || ',', ',surge,') > 0
		AND instr(',' || d.features || ',', ',attach,') > 0`); err != nil {
		return nil, err
	}
	for _, t := range w.transports {
		if t.AttachCapacity < 0 {
			return nil, fmt.Errorf("transport %s: no blueprint for %s in the store", t.DeviceCode, t.DeviceType)
		}
	}
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM devices WHERE features IS NULL`).Scan(&w.noFeatures)
	return w, err
}

// loadStars reads every star.
func loadStars(ctx context.Context, db *sql.DB) ([]model.Star, error) {
	var out []model.Star
	err := eachRow(ctx, db, `SELECT designation, x, y, z, coalesce(entry_point, '')
		FROM stars ORDER BY designation`, func(r *sql.Rows) error {
		var s model.Star
		err := r.Scan(&s.Designation, &s.Position.X, &s.Position.Y, &s.Position.Z, &s.EntryPoint)
		out = append(out, s)
		return err
	})
	return out, err
}

// loadDevices reads the devices (aliased d) matching where. AttachCapacity is
// -1 if the store has no blueprint for the device's type.
func loadDevices(ctx context.Context, db *sql.DB, where string) ([]model.Device, error) {
	var out []model.Device
	err := eachRow(ctx, db, `SELECT d.device_code, d.device_type, d.status, coalesce(d.features, ''),
			coalesce(d.location, ''), coalesce(d.attached_to, ''), coalesce(d.stowed_in, ''),
			coalesce(d.destination, ''), coalesce(b.attach_capacity, -1),
			(SELECT count(*) FROM devices a WHERE a.attached_to = d.device_code)
		FROM devices d LEFT JOIN blueprints b ON b.device_type = d.device_type
		WHERE `+where+` ORDER BY d.device_code`, func(r *sql.Rows) error {
		var d model.Device
		var features string
		err := r.Scan(&d.DeviceCode, &d.DeviceType, &d.Status, &features, &d.Location,
			&d.AttachedToDeviceCode, &d.StowedInDeviceCode, &d.Travel.FinalDestination,
			&d.AttachCapacity, &d.AttachUsed)
		if features != "" {
			d.Features = strings.Split(features, ",")
		}
		out = append(out, d)
		return err
	})
	return out, err
}

func eachRow(ctx context.Context, db *sql.DB, query string, f func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
