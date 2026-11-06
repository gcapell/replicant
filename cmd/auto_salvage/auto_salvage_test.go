package main

// Run with a dummy key, e.g. API_KEY=x go test ./cmd/auto_salvage (api's init exits without one).

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

func TestPickBusiestStar(t *testing.T) {
	locs := map[string]model.LocationSummary{
		"ALPHA-1":      {ResourceSites: 2},
		"ALPHA-3-1":    {ResourceSites: 2},
		"BETA-2":       {ResourceSites: 3},
		"BETA-1":       {ResourceSites: 0},
		"GAMMA-BELT-1": {ResourceSites: 50}, // belts don't count
		"GAMMA-OBJ-7":  {ResourceSites: 50}, // nor objects
		"GAMMA-4":      {ResourceSites: 1},
	}
	star, site := pickBusiestStar(locs)
	if star != "ALPHA" || site != "ALPHA-1" {
		t.Errorf("got %s %s, want ALPHA ALPHA-1", star, site)
	}
	if star, site := pickBusiestStar(map[string]model.LocationSummary{"X-BELT-1": {ResourceSites: 9}}); star != "" || site != "" {
		t.Errorf("got %q %q, want nothing", star, site)
	}
}
