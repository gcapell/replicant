package model

import (
	"testing"
	"time"
)

func TestParseEventTime(t *testing.T) {
	want := time.Date(2026, 11, 9, 0, 21, 43, 987437000, time.UTC)
	for _, s := range []string{
		"2026-11-09T00:21:43.987437",
		"2026-11-09T00:21:43.987437Z",
		"2026-11-09T11:21:43.987437+11:00",
	} {
		if got, err := ParseEventTime(s); err != nil || !got.Equal(want) {
			t.Errorf("ParseEventTime(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	if got, err := ParseEventTime("2026-11-09T00:21:43"); err != nil || !got.Equal(want.Truncate(time.Second)) {
		t.Errorf("no fraction: %v, %v", got, err)
	}
	for _, s := range []string{"", "2026-11-09", "tomorrow"} {
		if _, err := ParseEventTime(s); err == nil {
			t.Errorf("ParseEventTime(%q): accepted", s)
		}
	}
}
