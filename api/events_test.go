package api

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

// A trimmed /accounts/events response (real, 2026-11-08).
const accountEvents = `{"events": [{
  "category": "defensive",
  "criteria": [{"devices": [{"count": 1, "device_type": "defence_grid"}], "name": "defence_grid", "resources": {"conductive": 100}}],
  "designation": "CORBRENEUS-2-EVT-006",
  "event_type": "solar_storm_warning",
  "location": "CORBRENEUS-2",
  "progress": {"met": false, "met_option": null, "options": [
    {"devices": [{"current": 0, "device_type": "defence_grid", "met": false, "required": 1}], "met": false, "name": "defence_grid",
     "resources": [{"current": 100, "met": true, "required": 100, "resource_type": "conductive"}]},
    {"devices": [{"current": 1, "device_type": "shield_generator", "met": true, "required": 1}], "met": false, "name": "shield_generator", "resources": []}
  ], "replicant_present": false},
  "status": "active", "tier": 2, "title": "Solar Storm Warning"
}], "next_cursor": 7}`

func TestAccountEventsDecode(t *testing.T) {
	var page model.AccountEvents
	if err := unmarshal([]byte(accountEvents), &page); err != nil {
		t.Fatal(err)
	}
	if c, ok := page.Cursor(); !ok || c != 7 {
		t.Errorf("cursor = %d, %v", c, ok)
	}
	e := page.Events[0]
	if e.Location != "CORBRENEUS-2" || e.Designation != "CORBRENEUS-2-EVT-006" || e.Status != "active" || e.Tier != 2 {
		t.Errorf("event = %+v", e)
	}
	opts := e.Progress.Options
	if len(opts) != 2 || opts[0].Name != "defence_grid" {
		t.Fatalf("options = %+v", opts)
	}
	if r := opts[0].Resources[0]; r.ResourceType != model.Conductive || r.Current != 100 || r.Required != 100 {
		t.Errorf("resource = %+v", r)
	}
	if d := opts[1].Devices[0]; d.DeviceType != "shield_generator" || d.Current != 1 || d.Required != 1 {
		t.Errorf("device = %+v", d)
	}
	if opts[0].Satisfied() || !opts[1].Satisfied() {
		t.Errorf("satisfied = %v, %v", opts[0].Satisfied(), opts[1].Satisfied())
	}
}
