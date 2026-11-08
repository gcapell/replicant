package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/gcapell/replicant/model"
)

func openTest(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPutDevice(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	d := model.Device{
		DeviceCode: "7DA60868", DeviceType: "cargo_freighter", ReplicantCode: "4698E8E6",
		Status: "travelling",
		Travel: model.Travel{FinalDestination: "ULRDUNAS-2", FinalArrivesAt: "2026-11-08T17:28:15+11:00"},
	}
	if err := PutDevice(ctx, db, d); err != nil {
		t.Fatal(err)
	}
	var loc, dest sql.NullString
	var arrives sql.NullInt64
	db.QueryRow(`SELECT location, destination, arrives_at FROM devices`).Scan(&loc, &dest, &arrives)
	if loc.Valid || dest.String != "ULRDUNAS-2" || arrives.Int64 != 1794119295000 {
		t.Errorf("travelling: location %v, destination %v, arrives_at %v", loc, dest, arrives)
	}

	// Arriving replaces it.
	d.Status, d.Location, d.Travel = "idle", "ULRDUNAS-2", model.Travel{}
	if err := PutDevice(ctx, db, d); err != nil {
		t.Fatal(err)
	}
	var star string
	db.QueryRow(`SELECT location, star, destination, arrives_at FROM devices`).Scan(&loc, &star, &dest, &arrives)
	if loc.String != "ULRDUNAS-2" || star != "ULRDUNAS" || dest.Valid || arrives.Valid {
		t.Errorf("arrived: location %v, star %q, destination %v, arrives_at %v", loc, star, dest, arrives)
	}
	if n, _ := Count(ctx, db, "devices"); n != 1 {
		t.Errorf("%d devices, want 1", n)
	}

	d.Travel.FinalArrivesAt = "soon"
	if err := PutDevice(ctx, db, d); err == nil {
		t.Error("accepted an unparseable time")
	}
}

func TestPutStarKeepsEntryPoint(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	s := model.Star{Designation: "TIIKY", EntryPoint: "TIIKY-6-L4", Position: model.Position{X: 1, Y: 2, Z: 3}}
	if err := PutStar(ctx, db, s); err != nil {
		t.Fatal(err)
	}
	s.EntryPoint = ""
	if err := PutStar(ctx, db, s); err != nil {
		t.Fatal(err)
	}
	var ep string
	var y float64
	db.QueryRow(`SELECT entry_point, y FROM stars`).Scan(&ep, &y)
	if ep != "TIIKY-6-L4" || y != 2 {
		t.Errorf("entry_point %q, y %v", ep, y)
	}
}

func TestReplicantInVessel(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	if r, err := ReplicantInVessel(ctx, db); r != "" || err != nil {
		t.Errorf("empty store: %q, %v", r, err)
	}
	for _, d := range []model.Device{
		{DeviceCode: "F761AB14", DeviceType: "heaven_vessel", ReplicantCode: "AAAAAAAA", Status: "idle"},
		{DeviceCode: "DEF5F3CB", DeviceType: "empty_replicant_matrix", Status: "stowed", StowedInDeviceCode: "F761AB14"},
		{DeviceCode: "3B9BB396", DeviceType: "heaven_vessel", ReplicantCode: "4698E8E6", Status: "idle"},
		{DeviceCode: "3F5416A0", DeviceType: "replicant_matrix", Status: "stowed", StowedInDeviceCode: "3B9BB396"},
	} {
		if err := PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	if r, err := ReplicantInVessel(ctx, db); r != "4698E8E6" || err != nil {
		t.Errorf("got %q, %v; want 4698E8E6", r, err)
	}
}
