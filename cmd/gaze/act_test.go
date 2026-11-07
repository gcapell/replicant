package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gcapell/replicant/api"
)

func TestRefusal(t *testing.T) {
	he := func(code int, body string) error {
		return fmt.Errorf("wrapped: %w", &api.HTTPError{StatusCode: code, Body: []byte(body)})
	}
	for _, tt := range []struct {
		err  error
		note string
		ok   bool
	}{
		{he(400, `{"detail":{"neighbours":12,"expected":3.1,"ratio":3.87,"outward_ratio":2.5}}`), "dense ratio=3.87 outward_ratio=2.5", true},
		{he(400, `{"detail":"Already prospected from this star"}`), "already surveyed (refused)", true},
		{he(400, `{"detail":"observatory is compacted"}`), "", false},
		{he(500, `{"ratio": 3}`), "", false},
		{errors.New("network down"), "", false},
	} {
		note, ok := refusal(tt.err)
		if note != tt.note || ok != tt.ok {
			t.Errorf("refusal(%v) = %q, %v; want %q, %v", tt.err, note, ok, tt.note, tt.ok)
		}
	}
}
