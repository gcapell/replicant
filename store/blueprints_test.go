package store

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gcapell/replicant/model"
)

// blueprints is a real /blueprints response.
func blueprints(t *testing.T) []model.Blueprint {
	t.Helper()
	data, err := os.ReadFile("testdata/blueprints.json")
	if err != nil {
		t.Fatal(err)
	}
	var bs model.Blueprints
	if err := json.Unmarshal(data, &bs); err != nil {
		t.Fatal(err)
	}
	return bs.Blueprints
}

func TestReplaceBlueprints(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	all := blueprints(t)
	if err := ReplaceBlueprints(ctx, db, all); err != nil {
		t.Fatal(err)
	}
	if n, _ := Count(ctx, db, "blueprints"); n != len(all) || n != 42 {
		t.Errorf("%d rows, want %d (and 42)", n, len(all))
	}

	var features, directives string
	var attach, stow, printTime, carbon, volatiles int
	db.QueryRow(`SELECT features, directives, print_time, carbon, volatiles FROM blueprints
		WHERE device_type = 'ami_survey_controller'`).Scan(&features, &directives, &printTime, &carbon, &volatiles)
	if features != "ami,cruise,stow" || directives != "survey_system,belt_search" || printTime != 1500 || carbon != 20 || volatiles != 5 {
		t.Errorf("ami_survey_controller: %q %q %d %d %d", features, directives, printTime, carbon, volatiles)
	}
	db.QueryRow(`SELECT attach_capacity, stow_capacity FROM blueprints WHERE device_type = 'cargo_vessel'`).Scan(&attach, &stow)
	if attach != 3 || stow != 50 {
		t.Errorf("cargo_vessel: attach %d stow %d", attach, stow)
	}

	var relayRange float64
	db.QueryRow(`SELECT relay_range FROM blueprints WHERE device_type = 'ftl_relay'`).Scan(&relayRange)
	if relayRange != 7.5 {
		t.Errorf("ftl_relay relay_range = %v", relayRange)
	}

	// Missing resources are 0; no directives or components is NULL.
	var defCarbon, nulls int
	db.QueryRow(`SELECT carbon, (directives IS NULL) + (components IS NULL) FROM blueprints
		WHERE device_type = 'defence_grid'`).Scan(&defCarbon, &nulls)
	if defCarbon != 0 || nulls != 2 {
		t.Errorf("defence_grid: carbon %d, nulls %d", defCarbon, nulls)
	}
	var cores int
	db.QueryRow(`SELECT c.value FROM blueprints, json_each(components) c
		WHERE device_type = 'processing_array' AND c.key = 'compute_core'`).Scan(&cores)
	if cores != 5 {
		t.Errorf("processing_array compute_cores = %d", cores)
	}

	// A second listing replaces the first entirely.
	if err := ReplaceBlueprints(ctx, db, all[:2]); err != nil {
		t.Fatal(err)
	}
	if n, _ := Count(ctx, db, "blueprints"); n != 2 {
		t.Errorf("%d rows after replacing, want 2", n)
	}
}

func TestReplaceBlueprintsRejectsOddities(t *testing.T) {
	for name, spoil := range map[string]func([]model.Blueprint) []model.Blueprint{
		"empty":            func(bs []model.Blueprint) []model.Blueprint { return nil },
		"listed twice":     func(bs []model.Blueprint) []model.Blueprint { return append(bs, bs[0]) },
		"no device type":   func(bs []model.Blueprint) []model.Blueprint { bs[3].DeviceType = ""; return bs },
		"unknown resource": func(bs []model.Blueprint) []model.Blueprint { bs[1].Resources["unobtainium"] = 1; return bs },
	} {
		t.Run(name, func(t *testing.T) {
			db := openTest(t)
			if err := ReplaceBlueprints(t.Context(), db, spoil(blueprints(t))); err == nil {
				t.Error("no error")
			}
		})
	}
}
