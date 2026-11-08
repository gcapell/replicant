package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func TestListEventsReconnectsAfterLastEvent(t *testing.T) {
	firstBackoff = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	at := time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)
	var resumedFrom []string
	stream := func(ctx context.Context, id string, fn func(api.Envelope, any) error) error {
		resumedFrom = append(resumedFrom, id)
		switch len(resumedFrom) {
		case 1:
			fn(api.Envelope{ID: "100-0", Event: "ami.survey.digest", CreatedAt: at}, &model.SurveyDigest{})
			fn(api.Envelope{ID: "100-1", Event: "ami.survey.digest", CreatedAt: at}, &model.SurveyDigest{})
			fn(api.Envelope{ID: "101-0", Event: "travel.arrived", DeviceCode: "7DA60868", DeviceType: "cargo_freighter",
				Location: "ULRDUNAS-2", CreatedAt: at}, &model.TravelArrived{Destination: "ULRDUNAS-2"})
			fn(api.Envelope{ID: "102-0", Event: "ami.survey.digest", CreatedAt: at}, &model.SurveyDigest{})
			return errors.New("connection reset")
		case 2:
			fn(api.Envelope{ID: "103-0", Event: "brand.new", CreatedAt: at, Payload: []byte(`{"x":1}`)}, nil)
			return errors.New("connection reset")
		default:
			cancel()
			return ctx.Err()
		}
	}

	var out strings.Builder
	if err := listEvents(ctx, stream, &out, "99-0"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if want := []string{"99-0", "102-0", "103-0"}; !slices.Equal(resumedFrom, want) {
		t.Errorf("resumed from %q, want %q", resumedFrom, want)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("output:\n%s", out.String())
	}
	if lines[0] != "ami.survey.digest, ami.survey.digest, " {
		t.Errorf("boring events line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "101-0 travel.arrived 7DA60868 (cargo_freighter) @ULRDUNAS-2") {
		t.Errorf("event line = %q", lines[1])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, `brand.new`) || !strings.Contains(last, `UNDECODED {"x":1}`) {
		t.Errorf("undecoded line = %q", last)
	}
	// The boring run before the reconnection got its line ended.
	if !slices.Contains(lines, "ami.survey.digest, ") {
		t.Errorf("boring run before the drop wasn't ended:\n%s", out.String())
	}
}
