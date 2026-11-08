package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gcapell/replicant/model"
)

// world is what plant_relays reads from the store. Devices are model.Devices
// filled in from the store's columns, so the same checks work on them as on
// the API's answers.
type world struct {
	stars   []star         // every star in the store
	vessels []model.Device // every heaven_vessel, with StowedDevices, StowCapacity and StowUsed
	relays  []model.Device // every ftl_relay
}

func load(ctx context.Context, db *sql.DB) (*world, error) {
	w := &world{}
	var err error
	if w.stars, err = loadStars(ctx, db); err != nil {
		return nil, err
	}
	if w.relays, err = loadDevices(ctx, db, "ftl_relay"); err != nil {
		return nil, err
	}
	if w.vessels, err = loadDevices(ctx, db, "heaven_vessel"); err != nil {
		return nil, err
	}

	var capacity int
	err = db.QueryRowContext(ctx, `SELECT stow_capacity FROM blueprints WHERE device_type = 'heaven_vessel'`).Scan(&capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no heaven_vessel blueprint in the store")
	} else if err != nil {
		return nil, err
	}
	stowed, err := loadStowed(ctx, db)
	if err != nil {
		return nil, err
	}
	for i := range w.vessels {
		v := &w.vessels[i]
		v.StowedDevices = stowed[v.DeviceCode]
		v.StowCapacity = capacity
		v.StowUsed = len(v.StowedDevices)
	}
	return w, nil
}

// loadStars reads every star. One with an entry point counts as scanned.
func loadStars(ctx context.Context, db *sql.DB) ([]star, error) {
	rows, err := db.QueryContext(ctx, `SELECT designation, x, y, z, coalesce(entry_point, '')
		FROM stars ORDER BY designation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []star
	for rows.Next() {
		var s star
		if err := rows.Scan(&s.Name, &s.Pos.X, &s.Pos.Y, &s.Pos.Z, &s.EntryPoint); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// loadDevices reads every device of type typ, as far as the store knows it.
func loadDevices(ctx context.Context, db *sql.DB, typ string) ([]model.Device, error) {
	rows, err := db.QueryContext(ctx, `SELECT device_code, device_type, coalesce(owner, ''), status,
			coalesce(location, ''), coalesce(attached_to, ''), coalesce(stowed_in, '')
		FROM devices WHERE device_type = ? ORDER BY device_code`, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Device
	for rows.Next() {
		var d model.Device
		if err := rows.Scan(&d.DeviceCode, &d.DeviceType, &d.ReplicantCode, &d.Status,
			&d.Location, &d.AttachedToDeviceCode, &d.StowedInDeviceCode); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// loadStowed returns what's stowed in each device, by host.
func loadStowed(ctx context.Context, db *sql.DB) (map[string][]model.StowedDevice, error) {
	rows, err := db.QueryContext(ctx, `SELECT stowed_in, device_code, device_type, status
		FROM devices WHERE stowed_in IS NOT NULL ORDER BY device_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]model.StowedDevice{}
	for rows.Next() {
		var host string
		var s model.StowedDevice
		if err := rows.Scan(&host, &s.DeviceCode, &s.DeviceType, &s.Status); err != nil {
			return nil, err
		}
		out[host] = append(out[host], s)
	}
	return out, rows.Err()
}
