package store

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gcapell/replicant/model"
)

// othyonal is a real /locations/OTHYONAL response.
func othyonal(t *testing.T) model.Location {
	t.Helper()
	data, err := os.ReadFile("testdata/locations_OTHYONAL.json")
	if err != nil {
		t.Fatal(err)
	}
	var loc model.Location
	if err := json.Unmarshal(data, &loc); err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestPutSystem(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	loc := othyonal(t)
	if err := PutSystem(ctx, db, loc); err != nil {
		t.Fatal(err)
	}

	var ep, tags string
	var planets, moons, full, life int
	var x, bonus float64
	if err := db.QueryRow(`SELECT entry_point, x, planet_count, moon_count, is_fully_scanned,
		life_detected, mining_bonus_pct, system_tags FROM stars WHERE designation = 'OTHYONAL'`).
		Scan(&ep, &x, &planets, &moons, &full, &life, &bonus, &tags); err != nil {
		t.Fatal(err)
	}
	if ep != "OTHYONAL-6-L4" || x != -56.4187 || planets != 8 || moons != 78 || full != 1 || life != 1 || bonus != 0 || tags != "resource_rich" {
		t.Errorf("star: %v %v %v %v %v %v %v %q", ep, x, planets, moons, full, life, bonus, tags)
	}

	var n, hz, moonCount int
	var stage, typ string
	db.QueryRow(`SELECT count(*) FROM locations WHERE star = 'OTHYONAL'`).Scan(&n)
	db.QueryRow(`SELECT type, in_habitable_zone, life_stage, moon_count FROM locations WHERE designation = 'OTHYONAL-3'`).
		Scan(&typ, &hz, &stage, &moonCount)
	if n != 8 || typ != "terrestrial" || hz != 1 || stage != "intelligent" || moonCount != 0 {
		t.Errorf("locations: %d rows; OTHYONAL-3 %q %d %q %d", n, typ, hz, stage, moonCount)
	}

	if n, _ := Count(ctx, db, "inventory"); n != 0 {
		t.Errorf("PutSystem wrote %d inventory rows; that's ReplaceInventory's job", n)
	}

	var belt, density, beltSilicates, beltStar string
	db.QueryRow(`SELECT designation, star, density, silicates FROM belts`).Scan(&belt, &beltStar, &density, &beltSilicates)
	if belt != "OTHYONAL-BELT-1" || beltStar != "OTHYONAL" || density != "dense" || beltSilicates != "high" {
		t.Errorf("belt: %s %s %s %s", belt, beltStar, density, beltSilicates)
	}

	// Reading it again after the belt vanished.
	loc.AsteroidBelt.Belts = nil
	if err := PutSystem(ctx, db, loc); err != nil {
		t.Fatal(err)
	}
	if n, _ := Count(ctx, db, "belts"); n != 0 {
		t.Errorf("belts: %d rows left, want 0", n)
	}
	if n, _ := Count(ctx, db, "locations"); n != 8 {
		t.Errorf("locations: %d rows, want 8", n)
	}
}

func TestPutSystemRejectsOddities(t *testing.T) {
	for name, spoil := range map[string]func(*model.Location){
		"not a star":             func(l *model.Location) { l.LocationType = "planet" },
		"unknown belt resource":  func(l *model.Location) { l.AsteroidBelt.Belts[0].Resources["unobtainium"] = "high" },
		"planet in other system": func(l *model.Location) { l.Planets[0].Designation = "SOL-3" },
	} {
		t.Run(name, func(t *testing.T) {
			db := openTest(t)
			loc := othyonal(t)
			spoil(&loc)
			if err := PutSystem(t.Context(), db, loc); err == nil {
				t.Error("accepted it")
			}
		})
	}
}

func TestReplaceInventory(t *testing.T) {
	db := openTest(t)
	ctx := t.Context()
	item := func(r string, q int) model.InventoryItem { return model.InventoryItem{ResourceType: r, Quantity: q} }
	first := []model.LocationInventory{
		{Location: "OTHYONAL-3", Items: []model.InventoryItem{item("silicates", 100), item("rares", 200)}},
		{Location: "OTHYONAL-5-2", Items: []model.InventoryItem{item("carbon", 7)}},
		{Location: "OTHYONAL-BELT-1", Items: []model.InventoryItem{item("volatiles", 1500)}},
	}
	if err := ReplaceInventory(ctx, db, first); err != nil {
		t.Fatal(err)
	}
	var star string
	var silicates, rares, carbon, volatiles int
	db.QueryRow(`SELECT silicates, rares, carbon FROM inventory WHERE designation = 'OTHYONAL-3'`).Scan(&silicates, &rares, &carbon)
	db.QueryRow(`SELECT star, volatiles FROM inventory WHERE designation = 'OTHYONAL-BELT-1'`).Scan(&star, &volatiles)
	if silicates != 100 || rares != 200 || carbon != 0 || star != "OTHYONAL" || volatiles != 1500 {
		t.Errorf("got %d %d %d %q %d", silicates, rares, carbon, star, volatiles)
	}

	// A second listing replaces the first entirely.
	if err := ReplaceInventory(ctx, db, first[2:]); err != nil {
		t.Fatal(err)
	}
	if n, _ := Count(ctx, db, "inventory"); n != 1 {
		t.Errorf("%d rows after replacing, want 1", n)
	}

	for name, bad := range map[string][]model.LocationInventory{
		"unknown resource": {{Location: "A-1", Items: []model.InventoryItem{item("unobtainium", 1)}}},
		"resource twice":   {{Location: "A-1", Items: []model.InventoryItem{item("rares", 1), item("rares", 2)}}},
		"location twice":   {{Location: "A-1"}, {Location: "A-1"}},
	} {
		tx, _ := db.Begin()
		if err := ReplaceInventory(ctx, tx, bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
		tx.Rollback()
	}
}
