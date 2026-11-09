package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestLoadWorld(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO stars (designation, x, y, z) VALUES ('SOL', 0, 0, 0), ('ZETA', 5, 0, 0)`,
		`INSERT INTO blueprints (device_type, short_description, description, print_time, attach_capacity, cargo_capacity, stow_capacity, queue_size, relay_range, strength)
			VALUES ('surge_platform', '', '', 0, 4, 0, 0, 0, 0, 0)`,
		`INSERT INTO devices (device_code, device_type, owner, status, location, attached_to, stowed_in) VALUES
			('V1', 'heaven_vessel', 'REP', 'idle', 'SOL-3', NULL, NULL),
			('M1', 'replicant_matrix', 'REP', 'stowed', NULL, NULL, 'V1'),
			('V2', 'heaven_vessel', 'REP', 'idle', 'ZETA-1', NULL, NULL),
			('P1', 'surge_platform', 'REP', 'idle', 'ZETA-1', NULL, NULL),
			('B1', 'ftl_beacon', 'REP', 'idle', 'ZETA-1', 'P1', NULL),
			('P2', 'surge_platform', 'REP', 'travelling', NULL, NULL, NULL),
			('F1', 'cargo_freighter', 'REP', 'idle', 'ZETA-2', NULL, NULL),
			('F2', 'cargo_freighter', 'REP', 'travelling', NULL, NULL, NULL),
			('R1', 'ftl_relay', 'REP', 'relaying', 'ZETA-3-L4', NULL, NULL),
			('R2', 'ftl_relay', 'REP', 'inactive', 'SOL-BELT-1', NULL, NULL)`,
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
	w, err := loadWorld(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	if p, err := w.position("ZETA"); err != nil || p != (model.Position{X: 5}) {
		t.Errorf("position(ZETA) = %v, %v", p, err)
	}
	if _, err := w.position("NOPE"); err == nil {
		t.Error("position(NOPE): no error")
	}
	if !slices.Equal(w.relays, []string{"ZETA"}) {
		t.Errorf("relays = %v", w.relays)
	}
	if vs := idleVessels(w.devices); len(vs) != 1 || vs[0] != (vessel{Code: "V1", At: place{Loc: "SOL-3"}}) {
		t.Errorf("idleVessels = %+v", vs)
	}
	cs, err := idleCarriers(w.devices)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0] != (carrier{Code: "P1", Type: "surge_platform", At: place{Loc: "ZETA-1"}, Free: 3}) {
		t.Errorf("idleCarriers = %+v", cs)
	}
	if fs := idleFreighters(w.devices); len(fs) != 1 || fs[0] != (freighter{Code: "F1", Star: "ZETA"}) {
		t.Errorf("idleFreighters = %+v", fs)
	}
	if bs := ofTypes(w.devices, beaconType); len(bs) != 1 || bs[0].AttachedToDeviceCode != "P1" {
		t.Errorf("beacons = %+v", bs)
	}
}

func TestIdleCarriersNoBlueprint(t *testing.T) {
	_, err := idleCarriers([]model.Device{{DeviceCode: "C1", DeviceType: "surge_carrier", Status: "idle", AttachCapacity: -1}})
	if err == nil || !strings.Contains(err.Error(), "no blueprint") {
		t.Errorf("err = %v", err)
	}
}

func TestMismatch(t *testing.T) {
	st := model.Device{DeviceCode: "V1", DeviceType: "heaven_vessel", Status: "idle", Location: "SOL-3",
		AttachCapacity: -1, StowedDevices: []model.StowedDevice{{DeviceCode: "M1"}}}
	same := st
	same.AttachCapacity = 0
	if errs := mismatch(st, same); len(errs) != 0 {
		t.Errorf("same: %v", errs)
	}
	moved := same
	moved.Status, moved.Location, moved.StowedDevices = "travelling", "", nil
	if errs := mismatch(st, moved); len(errs) != 3 {
		t.Errorf("moved: %v", errs)
	}
}

func TestUsedDevices(t *testing.T) {
	spares := []spare{{Code: "B1"}, {Code: "B2"}}
	carriers := []carrier{{Code: "P1"}, {Code: "P2"}}
	dp := devicePlan{Routes: [][]stop{{{Spare: 1, Pickup: true}, {Spare: 1}}, nil}}
	ds := map[int]dispatch{0: {Freighter: freighter{Code: "F1"}}}
	vr := []vesselRoute{{Vessel: vessel{Code: "V1"}, Stops: []completion{{}}}, {Vessel: vessel{Code: "V2"}}}
	if got := usedDevices(ds, dp, spares, carriers, vr); !slices.Equal(got, []string{"B2", "F1", "P1", "V1"}) {
		t.Errorf("usedDevices = %v", got)
	}
}
