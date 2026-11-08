package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

var at = time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)

func ev(id, name string) api.Envelope {
	return api.Envelope{ID: id, Event: name, Category: strings.Split(name, ".")[0], CreatedAt: at, Payload: []byte(`{}`)}
}

func TestReceiveReconnectsAfterLastEvent(t *testing.T) {
	firstBackoff = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var resumedFrom []string
	stream := func(ctx context.Context, id string, fn func(api.Envelope, any) error) error {
		resumedFrom = append(resumedFrom, id)
		switch len(resumedFrom) {
		case 1:
			fn(ev("100-0", "ami.survey.digest"), &model.SurveyDigest{})
			fn(ev("101-0", "travel.arrived"), &model.TravelArrived{})
			return errors.New("connection reset")
		case 2:
			fn(ev("102-0", "brand.new"), nil)
			return errors.New("connection reset")
		default:
			cancel()
			return ctx.Err()
		}
	}

	out := make(chan received, 10)
	receive(ctx, stream, "99-0", out)
	var got []string
	for r := range out { // receive closed it
		got = append(got, r.env.ID)
	}
	if want := []string{"100-0", "101-0", "102-0"}; !slices.Equal(got, want) {
		t.Errorf("received %q, want %q", got, want)
	}
	if want := []string{"99-0", "101-0", "102-0"}; !slices.Equal(resumedFrom, want) {
		t.Errorf("resumed from %q, want %q", resumedFrom, want)
	}
}

func openStore(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func feed(rs ...received) <-chan received {
	ch := make(chan received, len(rs))
	for _, r := range rs {
		ch <- r
	}
	close(ch)
	return ch
}

func TestRecordAll(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	arrived := ev("101-0", "travel.arrived")
	arrived.DeviceCode, arrived.DeviceType, arrived.Location = "7DA60868", "cargo_freighter", "ULRDUNAS-2"
	arrived.Payload = []byte(`{"destination":"ULRDUNAS-2"}`)

	if err := store.PutDevice(ctx, db, model.Device{DeviceCode: "7DA60868", DeviceType: "cargo_freighter", Status: "travelling"}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err := recordAll(ctx, db, feed(
		received{ev("100-0", "ami.survey.digest"), &model.SurveyDigest{}},
		received{ev("100-1", "ami.survey.digest"), &model.SurveyDigest{}},
		received{arrived, &model.TravelArrived{Destination: "ULRDUNAS-2"}},
		received{ev("102-0", "brand.new"), nil},
	), &printer{w: &out}, false)
	if err != nil {
		t.Fatal(err)
	}

	if last, _ := store.LastEventID(ctx, db); last != "102-0" {
		t.Errorf("last event = %q, want 102-0", last)
	}
	var loc, payload string
	var created int64
	db.QueryRowContext(ctx, `SELECT location, payload, created_at FROM events WHERE event_id = '101-0'`).Scan(&loc, &payload, &created)
	if loc != "ULRDUNAS-2" || payload != `{"destination":"ULRDUNAS-2"}` || created != at.UnixMilli() {
		t.Errorf("recorded %q %q %d", loc, payload, created)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 3 || lines[0] != "ami.survey.digest, ami.survey.digest, " ||
		!strings.Contains(lines[1], "101-0 travel.arrived 7DA60868 (cargo_freighter) @ULRDUNAS-2") ||
		!strings.Contains(lines[len(lines)-1], "brand.new UNDECODED {}") {
		t.Errorf("output:\n%s", out.String())
	}

	// The stream resending a recorded event is an error...
	err = recordAll(ctx, db, feed(received{arrived, nil}), &printer{w: &out}, false)
	if err == nil || !strings.Contains(err.Error(), "already recorded") {
		t.Errorf("resent event: err = %v", err)
	}
	// ...unless we asked to replay.
	out.Reset()
	if err := recordAll(ctx, db, feed(received{arrived, nil}), &printer{w: &out}, true); err != nil {
		t.Errorf("replay: %v", err)
	}
	if !strings.HasPrefix(out.String(), "(already recorded) ") {
		t.Errorf("replay output: %q", out.String())
	}

	// An event with no time is refused.
	bad := ev("103-0", "travel.arrived")
	bad.CreatedAt = time.Time{}
	if err := recordAll(ctx, db, feed(received{bad, nil}), &printer{w: &out}, false); err == nil {
		t.Error("recorded an event with no time")
	}
}
