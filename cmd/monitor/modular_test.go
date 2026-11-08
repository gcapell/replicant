package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestModular(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	const obs, typ = "68D1D1CA", "galactic_observatory"
	for _, d := range []model.Device{
		{DeviceCode: obs, DeviceType: typ, Status: "prospecting", Location: "INTERRANAE-3-L4"},
		{DeviceCode: "1BC70CEE", DeviceType: "maintenance_drone", Status: "idle", Location: "INTERRANAE-3-L4"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	now := map[string]model.Device{obs: {DeviceCode: obs, DeviceType: typ, Status: "idle"}}
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		d, ok := now[code]
		if !ok {
			return nil, errors.New("404 not found")
		}
		return &d, nil
	}
	defer func() { fetchDevice = api.Device }()

	n := 0
	send := func(name, code, typ string, p any) error {
		n++
		e := ev(strings.Repeat("9", n)+"-0", name)
		e.DeviceCode, e.DeviceType, e.Location = code, typ, "SOMEWHERE-ELSE"
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}
	check := func(when, status string, completes time.Time) {
		t.Helper()
		var s, loc string
		var at sql.NullInt64
		if err := db.QueryRow(`SELECT status, location, completes_at FROM devices WHERE device_code = ?`, obs).
			Scan(&s, &loc, &at); err != nil {
			t.Fatal(err)
		}
		var want sql.NullInt64
		if !completes.IsZero() {
			want = sql.NullInt64{Int64: completes.UnixMilli(), Valid: true}
		}
		if s != status || at != want || loc != "INTERRANAE-3-L4" {
			t.Errorf("%s: status %q, completes_at %v, location %s; want %q, %v", when, s, at, loc, status, want)
		}
	}

	// The time has no offset: it's UTC.
	if err := send("device.compacting", obs, typ, &model.DeviceCompacting{CompletesAt: "2026-11-09T00:21:43.987437"}); err != nil {
		t.Fatal(err)
	}
	check("compacting", "compacting", time.Date(2026, 11, 9, 0, 21, 43, 987000000, time.UTC))
	if err := send("device.compacted", obs, typ, &model.DeviceCompacted{}); err != nil {
		t.Fatal(err)
	}
	check("compacted", "compacted", time.Time{})
	if err := send("device.unfurling", obs, typ, &model.DeviceUnfurling{CompletesAt: "2026-11-09T03:00:00+11:00"}); err != nil {
		t.Fatal(err)
	}
	check("unfurling", "unfurling", time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC))
	if err := send("device.unfurled", obs, typ, &model.DeviceUnfurled{}); err != nil {
		t.Fatal(err)
	}
	check("unfurled", "idle", time.Time{}) // as /devices says

	for _, c := range []struct {
		what, name, code, typ string
		p                     any
	}{
		{"no completes_at", "device.compacting", obs, typ, &model.DeviceCompacting{}},
		{"bad completes_at", "device.unfurling", obs, typ, &model.DeviceUnfurling{CompletesAt: "soon"}},
		{"not in the store", "device.compacted", "DEADBEEF", typ, &model.DeviceCompacted{}},
		{"wrong type", "device.compacting", "1BC70CEE", typ, &model.DeviceCompacting{CompletesAt: "2026-11-09T00:21:43"}},
		{"unknown to the API", "device.unfurled", "1BC70CEE", "maintenance_drone", &model.DeviceUnfurled{}},
	} {
		if err := send(c.name, c.code, c.typ, c.p); err == nil {
			t.Errorf("%s: accepted", c.what)
		}
	}
	check("after the failures", "idle", time.Time{})
}
