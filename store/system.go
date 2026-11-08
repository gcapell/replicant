package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gcapell/replicant/model"
)

// resources are the resource types, which are also the column names in
// belts and inventory.
var resources = []string{"carbon", "conductive", "rares", "silicates", "structural", "volatiles"}

// PutSystem stores what a star's /locations/STAR response says about its
// system: the star's details, its planets and its belts. A belt that's gone
// from the response is deleted. Anything unexpected, such as an unknown
// resource type, is an error. The planets' inventories are left to
// ReplaceInventory, so the inventory table has one source.
func PutSystem(ctx context.Context, ex Execer, loc model.Location) error {
	star := loc.Location
	if loc.LocationType != "star" || star == "" {
		return fmt.Errorf("PutSystem: %q is a %q, not a star", star, loc.LocationType)
	}
	if err := PutStar(ctx, ex, model.Star{Designation: star, EntryPoint: loc.EntryPoint, Position: loc.Star.Position}); err != nil {
		return err
	}
	_, err := ex.ExecContext(ctx, `UPDATE stars SET
		planet_count = ?, planets_scanned = ?, moon_count = ?, moons_scanned = ?,
		life_detected = ?, mining_bonus_pct = ?, system_tags = ?
		WHERE designation = ?`,
		loc.PlanetsTotal, loc.PlanetsScanned, loc.MoonsTotal, loc.MoonsScanned,
		loc.LifeDetected, loc.MiningBonusPct, strings.Join(loc.SystemTags, ","), star)
	if err != nil {
		return fmt.Errorf("star %s: %w", star, err)
	}

	for _, p := range loc.Planets {
		if err := putPlanet(ctx, ex, star, p); err != nil {
			return fmt.Errorf("planet %s: %w", p.Designation, err)
		}
	}

	if _, err := ex.ExecContext(ctx, `DELETE FROM belts WHERE star = ?`, star); err != nil {
		return err
	}
	for _, b := range loc.AsteroidBelt.Belts {
		if err := putBelt(ctx, ex, star, b); err != nil {
			return fmt.Errorf("belt %s: %w", b.Designation, err)
		}
	}
	return nil
}

func putPlanet(ctx context.Context, ex Execer, star string, p model.SystemPlanet) error {
	if !strings.HasPrefix(p.Designation, star+"-") {
		return fmt.Errorf("not in system %s", star)
	}
	_, err := ex.ExecContext(ctx, `INSERT INTO locations
		(designation, type, in_habitable_zone, life_stage, moon_count, scanned)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (designation) DO UPDATE SET
			type = excluded.type,
			in_habitable_zone = excluded.in_habitable_zone,
			life_stage = excluded.life_stage,
			moon_count = excluded.moon_count,
			scanned = excluded.scanned`,
		p.Designation, null(p.Type), p.InHabitableZone, null(p.LifeStage), p.MoonCount, p.Scanned)
	return err
}

// ReplaceInventory replaces the whole inventory table with an /inventory
// listing (planets, moons and belts). A location listed twice, or an unknown
// resource type, is an error.
func ReplaceInventory(ctx context.Context, ex Execer, all []model.LocationInventory) error {
	if _, err := ex.ExecContext(ctx, `DELETE FROM inventory`); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, l := range all {
		if l.Location == "" || seen[l.Location] {
			return fmt.Errorf("inventory: location %q listed twice or empty", l.Location)
		}
		seen[l.Location] = true
		if err := putInventory(ctx, ex, l.Location, l.Items); err != nil {
			return fmt.Errorf("inventory at %s: %w", l.Location, err)
		}
	}
	return nil
}

// putInventory replaces what's lying at designation.
func putInventory(ctx context.Context, ex Execer, designation string, items []model.InventoryItem) error {
	if len(items) == 0 {
		_, err := ex.ExecContext(ctx, `DELETE FROM inventory WHERE designation = ?`, designation)
		return err
	}
	qty := make(map[string]int, len(resources))
	for _, it := range items {
		if !slices.Contains(resources, it.ResourceType) {
			return fmt.Errorf("unknown resource type %q", it.ResourceType)
		}
		if _, dup := qty[it.ResourceType]; dup {
			return fmt.Errorf("%s listed twice", it.ResourceType)
		}
		qty[it.ResourceType] = it.Quantity
	}
	args := []any{designation}
	for _, r := range resources {
		args = append(args, qty[r])
	}
	var set []string
	for _, r := range resources {
		set = append(set, r+" = excluded."+r)
	}
	_, err := ex.ExecContext(ctx, fmt.Sprintf(`INSERT INTO inventory (designation, %s) VALUES (?%s)
		ON CONFLICT (designation) DO UPDATE SET %s`,
		strings.Join(resources, ", "), strings.Repeat(", ?", len(resources)), strings.Join(set, ", ")), args...)
	return err
}

func putBelt(ctx context.Context, ex Execer, star string, b model.SystemBelt) error {
	if !strings.HasPrefix(b.Designation, star+"-") {
		return fmt.Errorf("not in system %s", star)
	}
	for r := range b.Resources {
		if !slices.Contains(resources, string(r)) {
			return fmt.Errorf("unknown resource type %q", r)
		}
	}
	args := []any{b.Designation, null(b.Density)}
	for _, r := range resources {
		args = append(args, null(b.Resources[model.ResourceType(r)]))
	}
	_, err := ex.ExecContext(ctx, fmt.Sprintf(`INSERT INTO belts (designation, density, %s) VALUES (?, ?%s)`,
		strings.Join(resources, ", "), strings.Repeat(", ?", len(resources))), args...)
	return err
}
