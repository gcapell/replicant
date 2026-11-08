package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestScanReadsNewSystem(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	data, err := os.ReadFile("../../store/testdata/locations_OTHYONAL.json")
	if err != nil {
		t.Fatal(err)
	}
	var othyonal model.Location
	if err := json.Unmarshal(data, &othyonal); err != nil {
		t.Fatal(err)
	}
	// Known only by position, as after prospecting.
	if err := store.PutStar(ctx, db, model.Star{Designation: "OTHYONAL", Position: othyonal.Star.Position}); err != nil {
		t.Fatal(err)
	}
	var fetched []string
	fetchLocation = func(ctx context.Context, star string) (*model.Location, error) {
		fetched = append(fetched, star)
		switch star {
		case "OTHYONAL":
			return &othyonal, nil
		case "LIAR":
			return &othyonal, nil // answers for the wrong star
		}
		return nil, errors.New("404 not found")
	}
	defer func() { fetchLocation = api.Location }()

	n := 0
	send := func(star, loc, source string) error {
		n++
		e := ev(strings.Repeat("9", n)+"-0", "experience.gained")
		e.Star, e.Location = star, loc
		return recordAll(ctx, db, feed(received{e, &model.ExperienceGained{Amount: 500, Source: source}}),
			&printer{w: &strings.Builder{}}, false)
	}

	if err := send("OTHYONAL", "OTHYONAL", "print"); err != nil || len(fetched) != 0 {
		t.Errorf("print experience: err %v, fetched %v", err, fetched)
	}
	if err := send("OTHYONAL", "OTHYONAL", "scan"); err != nil {
		t.Fatal(err)
	}
	var entry string
	var planets int
	if err := db.QueryRow(`SELECT entry_point, planet_count FROM stars WHERE designation = 'OTHYONAL'`).Scan(&entry, &planets); err != nil {
		t.Fatal(err)
	}
	if entry != "OTHYONAL-6-L4" || planets != 8 {
		t.Errorf("after the scan: entry_point %q, planet_count %d", entry, planets)
	}
	// A planet scan in a system we've read costs nothing.
	if err := send("OTHYONAL", "OTHYONAL", "scan"); err != nil || len(fetched) != 1 {
		t.Errorf("second scan: err %v, fetched %v", err, fetched)
	}

	for _, c := range []struct{ what, star, loc string }{
		{"not a bare star", "OTHYONAL", "OTHYONAL-3"},
		{"no star", "", ""},
		{"API refuses", "NOWHERE", "NOWHERE"},
		{"API answers for another star", "LIAR", "LIAR"},
	} {
		if err := send(c.star, c.loc, "scan"); err == nil {
			t.Errorf("%s: accepted", c.what)
		}
	}
}

// catchUp with several event types applies them in the order they arrived,
// whatever order they're named in.
func TestCatchUpSeveral(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	const vessel, relay, at = "BA7BB196", "4A71C3A1", "KYRZHBIPOS-7-L4"
	for _, d := range []model.Device{
		{DeviceCode: vessel, DeviceType: "heaven_vessel", Status: "idle", Location: at},
		{DeviceCode: relay, DeviceType: "ftl_relay", Status: "stowed", StowedInDeviceCode: vessel},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		return &model.Device{DeviceCode: code, DeviceType: "ftl_relay", Status: "relaying"}, nil
	}
	defer func() { fetchDevice = api.Device }()
	for _, e := range []store.Event{
		{ID: "1-0", Name: "device.deployed", Payload: []byte(`{"deployed_from":"` + vessel + `"}`)},
		{ID: "2-0", Name: "travel.cancelled", Payload: []byte(`{}`)},
		{ID: "3-0", Name: "relay.activated", Payload: []byte(`{"location":"` + at + `","star":"KYRZHBIPOS"}`)},
	} {
		e.CreatedAt = time.UnixMilli(1794177885619)
		e.Category, e.DeviceCode, e.DeviceType, e.Star, e.Location = "device", relay, "ftl_relay", "KYRZHBIPOS", at
		if _, _, err := store.RecordEvent(ctx, db, e); err != nil {
			t.Fatal(err)
		}
	}

	if err := catchUp(ctx, db, []string{"relay.activated", "travel.cancelled"}, &printer{w: &strings.Builder{}}); err == nil {
		t.Error("catching up an event monitor doesn't handle: accepted")
	}
	var out strings.Builder
	if err := catchUp(ctx, db, []string{"relay.activated", "device.deployed"}, &printer{w: &out}); err != nil {
		t.Fatal(err)
	}
	var status, loc string
	if err := db.QueryRow(`SELECT status, location FROM devices WHERE device_code = ?`, relay).Scan(&status, &loc); err != nil {
		t.Fatal(err)
	}
	if status != "relaying" || loc != at {
		t.Errorf("relay: status %q, location %q", status, loc)
	}
	if s := out.String(); strings.Index(s, "1-0 device.deployed") > strings.Index(s, "3-0 relay.activated") ||
		strings.Contains(s, "travel.cancelled") {
		t.Errorf("output: %q", s)
	}
}
