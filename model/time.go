package model

import (
	"fmt"
	"time"
)

// ParseEventTime parses a time from an SSE event payload, such as
// device.compacting's completes_at. Most carry an offset (RFC 3339), but some
// don't ("2026-11-09T00:21:43.987437"): those are UTC.
func ParseEventTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("event time %q: neither RFC 3339 nor a bare UTC time", s)
	}
	return t, nil
}
