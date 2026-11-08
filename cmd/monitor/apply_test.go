package main

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

type deviceRow struct {
	status                string
	location, destination sql.NullString
	arrivesAt             sql.NullInt64
}

func device(t *testing.T, db *sql.DB, code string) deviceRow {
	t.Helper()
	var d deviceRow
	if err := db.QueryRow(`SELECT status, location, destination, arrives_at FROM devices WHERE device_code = ?`, code).
		Scan(&d.status, &d.location, &d.destination, &d.arrivesAt); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTravel(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "CARRIER1", DeviceType: "surge_carrier", Features: []string{"surge", "attach"}, Status: "idle", Location: "SOL-4"},
		{DeviceCode: "BEACON01", DeviceType: "ftl_beacon", Status: "attached", Location: "SOL-4", AttachedToDeviceCode: "CARRIER1"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}

	// A multi-leg surge with no arrives_at: due at created_at + travel time.
	dep := ev("200-0", "travel.departed")
	dep.DeviceCode = "CARRIER1"
	err := recordAll(ctx, db, feed(received{dep, &model.TravelDeparted{
		Origin: "SOL-4", Destination: "CHAMAKUY-3", TravelTimeSeconds: 185, TravelType: "surge",
		Legs:            []model.TravelLeg{{To: "SOL-4-L4"}, {To: "CHAMAKUY"}, {To: "CHAMAKUY-3"}},
		AttachedDevices: []string{"BEACON01"},
	}}), &printer{w: &strings.Builder{}}, false)
	if err != nil {
		t.Fatal(err)
	}
	c := device(t, db, "CARRIER1")
	if c.status != "travelling" || c.location.Valid || c.destination.String != "CHAMAKUY-3" ||
		c.arrivesAt.Int64 != at.Add(185*time.Second).UnixMilli() {
		t.Errorf("carrier after departing: %+v", c)
	}
	if b := device(t, db, "BEACON01"); b.location.Valid || b.status != "attached" {
		t.Errorf("beacon after departing: %+v", b)
	}

	// Arriving after a surge: the payload says the bare star, the envelope
	// where it really is.
	arr := ev("201-0", "travel.arrived")
	arr.DeviceCode, arr.Location = "CARRIER1", "CHAMAKUY-3"
	err = recordAll(ctx, db, feed(received{arr, &model.TravelArrived{
		Origin: "SOL-4", Destination: "CHAMAKUY", TravelType: "surge", AttachedDevices: []string{"BEACON01"},
	}}), &printer{w: &strings.Builder{}}, false)
	if err != nil {
		t.Fatal(err)
	}
	c = device(t, db, "CARRIER1")
	if c.status != "idle" || c.location.String != "CHAMAKUY-3" || c.destination.Valid || c.arrivesAt.Valid {
		t.Errorf("carrier after arriving: %+v", c)
	}
	if b := device(t, db, "BEACON01"); b.location.String != "CHAMAKUY-3" {
		t.Errorf("beacon after arriving: %+v", b)
	}
}

func TestDepartureArrival(t *testing.T) {
	env := api.Envelope{CreatedAt: at}
	got, err := departureArrival(env, &model.TravelDeparted{ArrivesAt: "2026-07-20T14:02:00Z", TravelTimeSeconds: 120})
	if err != nil || !got.Equal(time.Date(2026, 7, 20, 14, 2, 0, 0, time.UTC)) {
		t.Errorf("with arrives_at: %v, %v", got, err)
	}
	if _, err := departureArrival(env, &model.TravelDeparted{}); err == nil {
		t.Error("no arrives_at and no travel time: accepted")
	}
}

// A failure to apply an event leaves it unrecorded, so the next run retries it.
func TestFailedApplyIsNotRecorded(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	cases := map[string]received{
		"unknown device": func() received {
			e := ev("300-0", "travel.arrived")
			e.DeviceCode, e.Location = "NOSUCHDV", "SOL-4"
			return received{e, &model.TravelArrived{}}
		}(),
		"no location": func() received {
			e := ev("300-0", "travel.arrived")
			e.DeviceCode = "NOSUCHDV"
			return received{e, &model.TravelArrived{}}
		}(),
		"undecoded payload": {ev("300-0", "travel.arrived"), nil},
	}
	for name, r := range cases {
		err := recordAll(ctx, db, feed(r), &printer{w: &strings.Builder{}}, false)
		if err == nil {
			t.Errorf("%s: applied", name)
		}
		if last, _ := store.LastEventID(ctx, db); last != "" {
			t.Errorf("%s: event recorded although applying it failed", name)
		}
	}
}

// travel sends a departure and then an arrival for device.
func travel(t *testing.T, db *sql.DB, device, to string, seq int, departs, arrives bool) {
	t.Helper()
	var rs []received
	if departs {
		d := ev(fmt.Sprintf("%d-0", seq), "travel.departed")
		d.DeviceCode = device
		rs = append(rs, received{d, &model.TravelDeparted{Destination: to, ArrivesAt: "2026-11-08T07:05:00Z"}})
	}
	if arrives {
		a := ev(fmt.Sprintf("%d-1", seq), "travel.arrived")
		a.DeviceCode, a.Location = device, to
		rs = append(rs, received{a, &model.TravelArrived{Destination: to}})
	}
	if err := recordAll(t.Context(), db, feed(rs...), &printer{w: &strings.Builder{}}, false); err != nil {
		t.Fatal(err)
	}
}

func TestStatusAfterArrival(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "VESSEL01", DeviceType: "heaven_vessel", Features: []string{"surge", "cruise"}, Status: "idle"},
		{DeviceCode: "CARRIER1", DeviceType: "surge_carrier", Features: []string{"surge", "attach"}, Status: "idle"},
		{DeviceCode: "DRONE001", DeviceType: "mining_drone", Features: []string{"cruise", "mine"}, Status: "mining (carbon)"},
		{DeviceCode: "DRONE002", DeviceType: "survey_drone", Status: "travelling"}, // mid-trip at the snapshot
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}

	travel(t, db, "VESSEL01", "SOL-4", 400, true, true)
	travel(t, db, "CARRIER1", "SOL-5", 401, true, true)
	travel(t, db, "DRONE001", "SOL-6", 402, true, false)
	if d := device(t, db, "DRONE001"); d.status != "travelling" {
		t.Errorf("drone while travelling: %q", d.status)
	}
	travel(t, db, "DRONE001", "SOL-7", 403, true, true) // departs again mid-trip
	travel(t, db, "DRONE002", "SOL-8", 404, false, true)

	for code, want := range map[string]string{
		"VESSEL01": "idle",            // a heaven_vessel
		"CARRIER1": "idle",            // can attach
		"DRONE001": "mining (carbon)", // restored, despite departing twice
		"DRONE002": "idle",            // never seen to depart
	} {
		if got := device(t, db, code).status; got != want {
			t.Errorf("%s after arriving: %q, want %q", code, got, want)
		}
	}
	var before sql.NullString
	db.QueryRow(`SELECT status_before_travel FROM devices WHERE device_code = 'DRONE001'`).Scan(&before)
	if before.Valid {
		t.Errorf("status_before_travel left as %q", before.String)
	}
}
