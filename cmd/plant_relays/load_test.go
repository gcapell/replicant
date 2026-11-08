package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestLoad(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO stars (designation, x, y, z, entry_point) VALUES ('SOL', 0, 0, 0, 'SOL-3-L4'), ('ZETA', 5, 0, 0, NULL)`,
		`INSERT INTO blueprints (device_type, short_description, description, print_time, attach_capacity, cargo_capacity, stow_capacity, queue_size, relay_range, strength)
			VALUES ('heaven_vessel', '', '', 0, 0, 0, 10, 0, 0, 0)`,
		`INSERT INTO devices (device_code, device_type, owner, status, location, stowed_in) VALUES
			('V1', 'heaven_vessel', 'REP', 'idle', 'SOL-3', NULL),
			('M1', 'replicant_matrix', 'REP', 'stowed', NULL, 'V1'),
			('R2', 'ftl_relay', 'REP', 'stowed', NULL, 'V1'),
			('R1', 'ftl_relay', 'REP', 'stowed', NULL, 'V1'),
			('R3', 'ftl_relay', 'OTHER', 'inactive', 'ZETA-BELT-1', NULL),
			('R4', 'ftl_relay', 'REP', 'relaying', 'SOL-3-L4', NULL)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	db, err = store.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w, err := load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	if len(w.stars) != 2 || w.stars[0] != (star{Name: "SOL", EntryPoint: "SOL-3-L4"}) ||
		w.stars[1] != (star{Name: "ZETA", Pos: model.Position{X: 5}}) {
		t.Errorf("stars = %+v", w.stars)
	}
	if len(w.relays) != 4 {
		t.Errorf("relays = %+v", w.relays)
	}
	if got := relayingStars(w.relays); len(got) != 1 || !got["SOL"] {
		t.Errorf("relayingStars = %v", got)
	}
	if b := availableBelts(w.relays); len(b) != 1 || b[0].Loc != "ZETA-BELT-1" || !slices.Equal(b[0].Relays, []relay{{"R3", "OTHER"}}) {
		t.Errorf("availableBelts = %+v", b)
	}

	if len(w.vessels) != 1 {
		t.Fatalf("vessels = %+v", w.vessels)
	}
	v := w.vessels[0]
	if v.ReplicantCode != "REP" || v.Location != "SOL-3" || v.StowCapacity != 10 || v.StowUsed != 3 {
		t.Errorf("vessel = %+v", v)
	}
	if got := stowedRelayCodes(&v); !slices.Equal(got, []string{"R1", "R2"}) {
		t.Errorf("stowed relays = %v", got)
	}
	if ready := readyVessels(w.vessels); len(ready) != 1 {
		t.Errorf("readyVessels = %+v", ready)
	}
}

func TestVesselMismatch(t *testing.T) {
	v := vessel{Code: "V1", Replicant: "REP", Loc: "SOL-3", Relays: []string{"R1", "R2"}, Free: 7}
	d := model.Device{
		DeviceCode: "V1", ReplicantCode: "REP", Status: "idle", Location: "SOL-3",
		StowCapacity: 10, StowUsed: 3,
		StowedDevices: []model.StowedDevice{
			{DeviceCode: "R2", DeviceType: "ftl_relay"},
			{DeviceCode: "M1", DeviceType: "replicant_matrix"},
			{DeviceCode: "R1", DeviceType: "ftl_relay"},
		},
	}
	if errs := vesselMismatch(v, d); len(errs) != 0 {
		t.Errorf("matching vessel: %v", errs)
	}
	d.Status, d.Location, d.StowUsed = "travelling", "", 4
	if errs := vesselMismatch(v, d); len(errs) != 3 {
		t.Errorf("got %d errors, want 3: %v", len(errs), errs)
	}
}

func TestRelayMismatch(t *testing.T) {
	r := relay{Code: "R3", Owner: "OTHER"}
	d := model.Device{DeviceCode: "R3", ReplicantCode: "OTHER", Status: "inactive", Location: "ZETA-BELT-1"}
	if errs := relayMismatch(r, "ZETA-BELT-1", d); len(errs) != 0 {
		t.Errorf("matching relay: %v", errs)
	}
	d.Status, d.ReplicantCode = "relaying", "REP"
	if errs := relayMismatch(r, "ZETA-BELT-1", d); len(errs) != 2 {
		t.Errorf("got %d errors, want 2: %v", len(errs), errs)
	}
}
