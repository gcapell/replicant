package api

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

// A trimmed GET /locations/URIRALAX response.
const starLocation = `{
  "asteroid_belt": {"present": false},
  "entry_point": "URIRALAX-6-L4",
  "location": "URIRALAX",
  "location_type": "star",
  "moons_scanned": 0, "moons_total": 28, "moons_total_estimated": true,
  "outer_system": {"kuiper": null, "oort": {"designation": "URIRALAX-OORT", "distance_au": 1669.52}},
  "planets": [{"designation": "URIRALAX-1", "moon_count": 1, "name": null, "orbital_distance_au": 0.135,
    "salvage": [{"designation": "URIRALAX-1-SAL-1", "resources_available": ["rares"], "salvage_type": "instrument_package"}],
    "scanned": false, "type": "dwarf"}],
  "planets_scanned": 2, "planets_total": 6,
  "star": {"designation": "URIRALAX", "position": {"x": -47.2113, "y": -18.9736, "z": -162.4601}, "spectral_type": "M6"},
  "system_scanned": true,
  "system_tags": []
}`

func TestDecodeStarLocation(t *testing.T) {
	var l model.Location
	if err := unmarshal([]byte(starLocation), &l); err != nil {
		t.Fatal(err)
	}
	if l.PlanetsTotal != 6 || l.PlanetsScanned != 2 || l.MoonsTotal != 28 || !l.SystemScanned || l.Unscanned() != 32 {
		t.Errorf("counts: %+v", l)
	}
	if l.Star.Position.Z != -162.4601 || l.EntryPoint != "URIRALAX-6-L4" || l.OuterSystem.Oort.Designation != "URIRALAX-OORT" {
		t.Errorf("star: %+v", l)
	}
	if p := l.Planets[0]; p.MoonCount != 1 || p.OrbitalDistanceAU != 0.135 || p.Salvage[0].SalvageType != "instrument_package" {
		t.Errorf("planet: %+v", p)
	}
}
