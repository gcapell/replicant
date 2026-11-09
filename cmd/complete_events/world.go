package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"slices"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// world is what complete_events reads from the store. Devices are
// model.Devices filled in from the store's columns, as far as it knows them:
// AttachCapacity comes from the blueprint (-1 if there's none), AttachUsed is
// a count of what's attached, and StowedDevices lists what's stowed. Before a
// real run, the devices a plan uses are checked against the API (verify).
type world struct {
	starPos map[string]model.Position // every star in the store
	relays  []string                  // stars with a relaying ftl_relay, sorted
	devices []model.Device            // every device
}

func loadWorld(ctx context.Context, db *sql.DB) (*world, error) {
	w := &world{starPos: map[string]model.Position{}}
	if err := eachRow(ctx, db, `SELECT designation, x, y, z FROM stars`, func(r *sql.Rows) error {
		var s string
		var p model.Position
		err := r.Scan(&s, &p.X, &p.Y, &p.Z)
		w.starPos[s] = p
		return err
	}); err != nil {
		return nil, err
	}
	if err := eachRow(ctx, db, `SELECT star FROM relay_stars WHERE star IS NOT NULL ORDER BY star`, func(r *sql.Rows) error {
		var s string
		err := r.Scan(&s)
		w.relays = append(w.relays, s)
		return err
	}); err != nil {
		return nil, err
	}
	byCode := map[string]int{}
	if err := eachRow(ctx, db, `SELECT d.device_code, d.device_type, d.status,
			coalesce(d.location, ''), coalesce(d.attached_to, ''), coalesce(d.stowed_in, ''),
			coalesce(b.attach_capacity, -1),
			(SELECT count(*) FROM devices a WHERE a.attached_to = d.device_code)
		FROM devices d LEFT JOIN blueprints b ON b.device_type = d.device_type
		ORDER BY d.device_code`, func(r *sql.Rows) error {
		var d model.Device
		err := r.Scan(&d.DeviceCode, &d.DeviceType, &d.Status, &d.Location,
			&d.AttachedToDeviceCode, &d.StowedInDeviceCode, &d.AttachCapacity, &d.AttachUsed)
		byCode[d.DeviceCode] = len(w.devices)
		w.devices = append(w.devices, d)
		return err
	}); err != nil {
		return nil, err
	}
	for i := range w.devices {
		d := w.devices[i]
		if h, ok := byCode[d.StowedInDeviceCode]; ok {
			w.devices[h].StowedDevices = append(w.devices[h].StowedDevices,
				model.StowedDevice{DeviceCode: d.DeviceCode, DeviceType: d.DeviceType, Status: d.Status})
		}
	}
	log.Printf("store: %d stars, %d relay stars, %d devices", len(w.starPos), len(w.relays), len(w.devices))
	return w, nil
}

// position is where star is, or an error if the store doesn't have it.
func (w *world) position(star string) (model.Position, error) {
	p, ok := w.starPos[star]
	if !ok {
		return p, fmt.Errorf("star %q isn't in the store", star)
	}
	return p, nil
}

// ofTypes returns the devices whose type is one of types.
func ofTypes(devices []model.Device, types ...string) []model.Device {
	var out []model.Device
	for _, d := range devices {
		if slices.Contains(types, d.DeviceType) {
			out = append(out, d)
		}
	}
	return out
}

// idleCarriers returns the idle surge carriers with room to attach more. A
// carrier type with no blueprint in the store is an error.
func idleCarriers(devices []model.Device) ([]carrier, error) {
	var cs []carrier
	for _, d := range ofTypes(devices, carrierTypes...) {
		if d.AttachCapacity < 0 {
			return nil, fmt.Errorf("carrier %s: no blueprint for %s in the store", d.DeviceCode, d.DeviceType)
		}
		if d.Status == "idle" && d.AttachedToDeviceCode == "" && d.StowedInDeviceCode == "" && d.AttachUsed < d.AttachCapacity {
			cs = append(cs, carrier{Code: d.DeviceCode, Type: d.DeviceType, At: place{Loc: d.Location}, Free: d.AttachCapacity - d.AttachUsed})
		}
	}
	log.Printf("%d idle carriers with room", len(cs))
	return cs, nil
}

// idleFreighters returns the cargo_freighters whose status is "idle".
func idleFreighters(devices []model.Device) []freighter {
	all := ofTypes(devices, "cargo_freighter")
	var fs []freighter
	for _, d := range all {
		if d.Status == "idle" {
			fs = append(fs, freighter{Code: d.DeviceCode, Star: auto.StarOf(d.Location)})
		}
	}
	log.Printf("%d idle cargo_freighters (of %d)", len(fs), len(all))
	return fs
}

// idleVessels returns the idle heaven_vessels carrying a replicant_matrix.
func idleVessels(devices []model.Device) []vessel {
	all := ofTypes(devices, "heaven_vessel")
	var vs []vessel
	for _, d := range all {
		if auto.ReadyReplicant(d) {
			vs = append(vs, vessel{Code: d.DeviceCode, At: place{Loc: d.Location}})
		}
	}
	log.Printf("%d idle heaven_vessels with a replicant (of %d)", len(vs), len(all))
	return vs
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
