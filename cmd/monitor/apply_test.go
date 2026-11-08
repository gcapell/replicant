package main

import (
	"context"
	"database/sql"
	"errors"
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

func TestPrinted(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	known := map[string]model.Device{
		"8D96D74B": {DeviceCode: "8D96D74B", DeviceType: "survey_drone", Status: "idle", Location: "CORBRENEUS-BELT-1",
			Features: []string{"cruise"}, ReplicantCode: "4698E8E6"},
	}
	var fetched []string
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		fetched = append(fetched, code)
		d, ok := known[code]
		if !ok {
			return nil, errors.New("404 not found")
		}
		return &d, nil
	}
	defer func() { fetchDevice = api.Device }()

	print := func(id, code, typ string) error {
		e := ev(id, "print.completed")
		e.DeviceCode, e.DeviceType, e.Location = "C28572B9", "autofactory", "CORBRENEUS-BELT-1"
		return recordAll(ctx, db, feed(received{e, &model.PrintCompleted{NewDeviceCode: code, DeviceType: typ, PrintMode: "autofactory"}}),
			&printer{w: &strings.Builder{}}, false)
	}
	if err := print("500-0", "8D96D74B", "survey_drone"); err != nil {
		t.Fatal(err)
	}
	d := device(t, db, "8D96D74B")
	if d.status != "idle" || d.location.String != "CORBRENEUS-BELT-1" {
		t.Errorf("printed drone: %+v", d)
	}

	// Strictness: each of these stops monitor, and leaves the event unrecorded.
	for name, err := range map[string]error{
		"unknown to the API": print("501-0", "NOSUCHDV", "survey_drone"),
		"wrong type":         print("502-0", "8D96D74B", "mining_drone"),
		"no new device":      print("503-0", "", "survey_drone"),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if last, _ := store.LastEventID(ctx, db); last != "500-0" {
		t.Errorf("last recorded event %q, want 500-0", last)
	}
	if len(fetched) != 3 { // no fetch without a new_device_code
		t.Errorf("fetched %q, want 3 fetches", fetched)
	}
}

func TestAttachDetach(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "E62C8DC1", DeviceType: "surge_carrier", Features: []string{"surge", "attach"}, Status: "idle", Location: "WYNUS-1"},
		{DeviceCode: "OTHERCAR", DeviceType: "surge_carrier", Features: []string{"surge", "attach"}, Status: "idle", Location: "WYNUS-1"},
		{DeviceCode: "8007BBAB", DeviceType: "mining_drone", Status: "idle", Location: "WYNUS-1"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	attachedTo := func() (string, string) {
		var on, loc sql.NullString
		db.QueryRow(`SELECT attached_to, location FROM devices WHERE device_code = '8007BBAB'`).Scan(&on, &loc)
		return on.String, loc.String
	}
	apply := func(id, name, carrier, target, typ string) error {
		e := ev(id, name)
		e.DeviceCode, e.DeviceType, e.Location = carrier, "surge_carrier", "WYNUS-1"
		var p any = &model.DeviceAttached{TargetCode: target, TargetType: typ}
		if name == "device.detached" {
			p = &model.DeviceDetached{TargetCode: target, TargetType: typ}
		}
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}

	// As in the archive: one event per device, the carrier in the envelope.
	if err := apply("600-0", "device.attached", "E62C8DC1", "8007BBAB", "mining_drone"); err != nil {
		t.Fatal(err)
	}
	if on, loc := attachedTo(); on != "E62C8DC1" || loc != "WYNUS-1" {
		t.Errorf("after attach: attached_to %q, location %q", on, loc)
	}
	// Attaching again to the same carrier is fine (the snapshot may be newer).
	if err := apply("601-0", "device.attached", "E62C8DC1", "8007BBAB", "mining_drone"); err != nil {
		t.Fatal(err)
	}

	// Strictness: each of these stops monitor, and leaves the event unrecorded.
	for name, err := range map[string]error{
		"attached elsewhere":  apply("610-0", "device.attached", "OTHERCAR", "8007BBAB", "mining_drone"),
		"detached from other": apply("611-0", "device.detached", "OTHERCAR", "8007BBAB", "mining_drone"),
		"wrong type":          apply("612-0", "device.detached", "E62C8DC1", "8007BBAB", "survey_drone"),
		"unknown target":      apply("613-0", "device.attached", "E62C8DC1", "NOSUCHDV", "mining_drone"),
		"unknown carrier":     apply("614-0", "device.attached", "NOSUCHCR", "8007BBAB", "mining_drone"),
		"attached to itself":  apply("615-0", "device.attached", "E62C8DC1", "E62C8DC1", "surge_carrier"),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if last, _ := store.LastEventID(ctx, db); last != "601-0" {
		t.Errorf("last recorded event %q, want 601-0", last)
	}

	if err := apply("620-0", "device.detached", "E62C8DC1", "8007BBAB", "mining_drone"); err != nil {
		t.Fatal(err)
	}
	if on, loc := attachedTo(); on != "" || loc != "WYNUS-1" {
		t.Errorf("after detach: attached_to %q, location %q", on, loc)
	}
	// Detaching again is fine too.
	if err := apply("621-0", "device.detached", "E62C8DC1", "8007BBAB", "mining_drone"); err != nil {
		t.Fatal(err)
	}
}

func TestMiningDigest(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "97CCFCC1", DeviceType: "ami_mining_controller", Status: "coordinating", Location: "WYNUS-BELT-1"},
		{DeviceCode: "011BDABE", DeviceType: "mining_drone", Status: "idle", Location: "WYNUS-BELT-1"},
		{DeviceCode: "2323FB10", DeviceType: "mining_drone", Status: "idle", Location: "WYNUS-BELT-1"},
		{DeviceCode: "55095407", DeviceType: "mining_drone", Status: "travelling",
			Travel: model.Travel{FinalDestination: "WYNUS-BELT-1", FinalArrivesAt: "2026-11-08T17:28:15+11:00"}},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(id string, devs ...model.AMIDevice) error {
		e := ev(id, "ami.mining.digest")
		e.DeviceCode, e.DeviceType, e.Location = "97CCFCC1", "ami_mining_controller", "WYNUS-BELT-1"
		p := &model.MiningDigest{AMIDigest: model.AMIDigest{Directive: "gather_evenly", Devices: devs}}
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}
	row := func(code string) (status, before, controller string) {
		var b, c sql.NullString
		db.QueryRow(`SELECT status, status_before_travel, controller FROM devices WHERE device_code = ?`, code).Scan(&status, &b, &c)
		return status, b.String, c.String
	}

	if err := digest("700-0",
		model.AMIDevice{DeviceCode: "011BDABE", Status: "mining (carbon)"},
		model.AMIDevice{DeviceCode: "2323FB10", Status: "travelling"},
		model.AMIDevice{DeviceCode: "55095407", Status: "mining (structural)"},
	); err != nil {
		t.Fatal(err)
	}
	for code, want := range map[string][3]string{
		"011BDABE": {"mining (carbon)", "", "97CCFCC1"},
		"2323FB10": {"travelling", "", "97CCFCC1"},                    // no travel events: the digest's word
		"55095407": {"travelling", "mining (structural)", "97CCFCC1"}, // kept for arrival
	} {
		if s, b, c := row(code); [3]string{s, b, c} != want {
			t.Errorf("%s: status %q, before travel %q, controller %q; want %q", code, s, b, c, want)
		}
	}
	// A digest saying travelling doesn't overwrite what arrival restores.
	if err := digest("701-0", model.AMIDevice{DeviceCode: "55095407", Status: "travelling"}); err != nil {
		t.Fatal(err)
	}
	if _, b, _ := row("55095407"); b != "mining (structural)" {
		t.Errorf("status before travel %q after a travelling digest", b)
	}
	arr := ev("702-0", "travel.arrived")
	arr.DeviceCode, arr.Location = "55095407", "WYNUS-BELT-1"
	if err := recordAll(ctx, db, feed(received{arr, &model.TravelArrived{Destination: "WYNUS-BELT-1"}}), &printer{w: &strings.Builder{}}, false); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := row("55095407"); s != "mining (structural)" {
		t.Errorf("status %q after arriving", s)
	}

	for name, err := range map[string]error{
		"unknown device": digest("710-0", model.AMIDevice{DeviceCode: "NOSUCHDV", Status: "idle"}),
		"unknown status": digest("711-0", model.AMIDevice{DeviceCode: "011BDABE", Status: "dancing"}),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if last, _ := store.LastEventID(ctx, db); last != "702-0" {
		t.Errorf("last recorded event %q, want 702-0", last)
	}
}

func TestAdoptReleaseAndSurveyDigest(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "113D1033", DeviceType: "ami_survey_controller", Status: "idle", Location: "MUKONDO-BELT-1"},
		{DeviceCode: "OTHERSC1", DeviceType: "ami_survey_controller", Status: "idle", Location: "MUKONDO-BELT-1"},
		{DeviceCode: "14903EF0", DeviceType: "belt_surveyor", Status: "idle", Location: "MUKONDO-BELT-1"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	send := func(id, name, controller string, p any) error {
		e := ev(id, name)
		e.DeviceCode, e.DeviceType, e.Location = controller, "ami_survey_controller", "MUKONDO-BELT-1"
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}
	surveyor := []model.EventDevice{{DeviceCode: "14903EF0", DeviceType: "belt_surveyor"}}
	row := func() (status, controller string) {
		var c sql.NullString
		db.QueryRow(`SELECT status, controller FROM devices WHERE device_code = '14903EF0'`).Scan(&status, &c)
		return status, c.String
	}

	// As seen live after start_belt.
	if err := send("800-0", "ami.adopted", "113D1033", &model.AMIAdopted{Devices: surveyor}); err != nil {
		t.Fatal(err)
	}
	if s, c := row(); s != "idle" || c != "113D1033" {
		t.Errorf("after adopt: status %q, controller %q", s, c)
	}
	if err := send("801-0", "ami.survey.digest", "113D1033", &model.SurveyDigest{AMIDigest: model.AMIDigest{
		Directive: "belt_search", Devices: []model.AMIDevice{{DeviceCode: "14903EF0", Status: "tracking"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if s, c := row(); s != "tracking" || c != "113D1033" {
		t.Errorf("after digest: status %q, controller %q", s, c)
	}

	for name, err := range map[string]error{
		"released by another": send("810-0", "ami.released", "OTHERSC1", &model.AMIReleased{Devices: surveyor}),
		"wrong type": send("811-0", "ami.adopted", "113D1033", &model.AMIAdopted{
			Devices: []model.EventDevice{{DeviceCode: "14903EF0", DeviceType: "survey_drone"}}}),
		"unknown device": send("812-0", "ami.adopted", "113D1033", &model.AMIAdopted{
			Devices: []model.EventDevice{{DeviceCode: "NOSUCHDV", DeviceType: "survey_drone"}}}),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if err := send("820-0", "ami.released", "113D1033", &model.AMIReleased{Devices: surveyor}); err != nil {
		t.Fatal(err)
	}
	if _, c := row(); c != "" {
		t.Errorf("after release: controller %q", c)
	}
}

func TestDirective(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	for _, d := range []model.Device{
		{DeviceCode: "55EA11A7", DeviceType: "ami_mining_controller", Status: "idle", Location: "MUKONDO-BELT-1"},
		{DeviceCode: "1BC70CEE", DeviceType: "maintenance_drone", Status: "idle", Location: "MUKONDO-BELT-1"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	now := map[string]model.Device{
		"55EA11A7": {DeviceCode: "55EA11A7", DeviceType: "ami_mining_controller", Status: "coordinating", Location: "ELSEWHERE-1"},
		"1BC70CEE": {DeviceCode: "1BC70CEE", DeviceType: "maintenance_drone", Status: "idle"},
	}
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		d, ok := now[code]
		if !ok {
			return nil, errors.New("404 not found")
		}
		return &d, nil
	}
	defer func() { fetchDevice = api.Device }()
	send := func(id, name, code, typ string, p any) error {
		e := ev(id, name)
		e.DeviceCode, e.DeviceType, e.Location = code, typ, "MUKONDO-BELT-1"
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}

	if err := send("900-0", "directive.set", "55EA11A7", "ami_mining_controller", &model.DirectiveSet{Directive: "gather_evenly"}); err != nil {
		t.Fatal(err)
	}
	// Only the status comes from the API.
	if d := device(t, db, "55EA11A7"); d.status != "coordinating" || d.location.String != "MUKONDO-BELT-1" {
		t.Errorf("controller after directive.set: %+v", d)
	}
	if err := send("901-0", "directive.cleared", "1BC70CEE", "maintenance_drone", &model.DirectiveCleared{PreviousDirective: "patrol"}); err != nil {
		t.Fatal(err)
	}
	if d := device(t, db, "1BC70CEE"); d.status != "idle" {
		t.Errorf("maintenance drone after directive.cleared: %+v", d)
	}

	for name, err := range map[string]error{
		"unknown to the API": send("910-0", "directive.completed", "NOSUCHDV", "ami_mining_controller", &model.DirectiveCompleted{}),
		"wrong type":         send("911-0", "directive.completed", "55EA11A7", "ami_survey_controller", &model.DirectiveCompleted{}),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if last, _ := store.LastEventID(ctx, db); last != "901-0" {
		t.Errorf("last recorded event %q, want 901-0", last)
	}
}
