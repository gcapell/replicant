package store

import (
	"context"
	"database/sql"
	"errors"
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

// SurveyCompleted records that a survey_system directive finished at star:
// every planet and moon is scanned. If the last digest's total
// (survey_total) is more than the planets and moons we know of, the extras
// are taken to be moons (so far they always have been) and moon_count rises
// to match. It's an error if the star isn't in the store or its system hasn't
// been read.
func SurveyCompleted(ctx context.Context, ex ExecQueryer, star string) error {
	var planets, moons, total sql.NullInt64
	err := ex.QueryRowContext(ctx, `SELECT planet_count, moon_count, survey_total FROM stars WHERE designation = ?`, star).
		Scan(&planets, &moons, &total)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("survey of %s completed: star isn't in the store", star)
	}
	if err != nil {
		return err
	}
	if !planets.Valid || !moons.Valid {
		return fmt.Errorf("survey of %s completed: we haven't read its system (no planet or moon count)", star)
	}
	m := moons.Int64
	if total.Valid && total.Int64-planets.Int64 > m {
		m = total.Int64 - planets.Int64
	}
	_, err = ex.ExecContext(ctx, `UPDATE stars SET moon_count = ?, planets_scanned = planet_count, moons_scanned = ?
		WHERE designation = ?`, m, m, star)
	return err
}

// SurveyProgress applies an ami.survey.digest's progress at star. Progress
// counts planets and moons together, and its total grows as scanning planets
// turns up their moons, so it can't be split into planets_scanned and
// moons_scanned. All it can tell us is when the survey has finished: nothing
// remaining, with as many scanned as the system's planets and moons put
// together. Then both scanned counts are set to the totals. Anything else
// changes nothing. The progress itself is kept (survey_scanned,
// survey_total), for SurveyCompleted.
//
// The total can be more than the planets and moons we know of: /locations
// doesn't count moons nobody has found yet (ISTERRELAS: 3 planets and 3 moons
// known, total 7). The counts are then marked fully scanned as they stand, and
// the controller's directive.completed (SurveyCompleted) raises moon_count.
//
// A total of 0 means no system survey (a belt search's digest) and is
// ignored. It's an error if the star isn't in the store or its system hasn't
// been read, or if scanned and remaining don't add up to the total.
func SurveyProgress(ctx context.Context, ex ExecQueryer, star string, p model.SurveyProgress) error {
	if p.Total == 0 {
		return nil
	}
	if p.Scanned+p.Remaining != p.Total || p.Scanned < 0 || p.Remaining < 0 {
		return fmt.Errorf("survey of %s: scanned %d + remaining %d != total %d", star, p.Scanned, p.Remaining, p.Total)
	}
	var planets, moons sql.NullInt64
	err := ex.QueryRowContext(ctx, `SELECT planet_count, moon_count FROM stars WHERE designation = ?`, star).Scan(&planets, &moons)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("survey of %s: star isn't in the store", star)
	}
	if err != nil {
		return err
	}
	if !planets.Valid || !moons.Valid {
		return fmt.Errorf("survey of %s: we haven't read its system (no planet or moon count)", star)
	}
	if _, err := ex.ExecContext(ctx, `UPDATE stars SET survey_scanned = ?, survey_total = ? WHERE designation = ?`,
		p.Scanned, p.Total, star); err != nil {
		return err
	}
	known := int(planets.Int64 + moons.Int64)
	if p.Remaining > 0 || p.Scanned < known {
		return nil
	}
	_, err = ex.ExecContext(ctx, `UPDATE stars SET planets_scanned = planet_count, moons_scanned = moon_count WHERE designation = ?`, star)
	return err
}

// ScanReport applies one scan from an ami.survey.digest at star to the
// locations table.
//
// A planet scan updates (or adds) the planet: type, life stage, habitable
// zone, moon_count (the moons listed) and scanned. Each listed moon is added
// (or updated), with its planet's habitable zone; a moon already marked
// scanned stays so. Then the star's planet_count and moon_count rise to what
// the locations table now holds, if that's more.
//
// A moon scan marks the moon scanned (adding it if need be), with its type
// and life stage. Its planet must be in the store.
//
// It's an error if the star isn't in the store or its system hasn't been
// read, if the target isn't in the star's system or doesn't match the report,
// or if the scan type is anything but planet or moon.
func ScanReport(ctx context.Context, ex ExecQueryer, star string, s model.SurveyScan) error {
	var planets, moons sql.NullInt64
	err := ex.QueryRowContext(ctx, `SELECT planet_count, moon_count FROM stars WHERE designation = ?`, star).Scan(&planets, &moons)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("scan of %s: star %s isn't in the store", s.ScanTarget, star)
	}
	if err != nil {
		return err
	}
	if !planets.Valid || !moons.Valid {
		return fmt.Errorf("scan of %s: we haven't read %s's system", s.ScanTarget, star)
	}
	parts := strings.Split(s.ScanTarget, "-")
	if parts[0] != star {
		return fmt.Errorf("scan of %s: not in system %s", s.ScanTarget, star)
	}
	switch s.ScanType {
	case "planet":
		p := s.Report.Planet
		if len(parts) != 2 || p.Designation != s.ScanTarget {
			return fmt.Errorf("planet scan of %s: report is for planet %q", s.ScanTarget, p.Designation)
		}
		if _, err := ex.ExecContext(ctx, `INSERT INTO locations
			(designation, type, in_habitable_zone, life_stage, moon_count, scanned)
			VALUES (?, ?, ?, ?, ?, 1)
			ON CONFLICT (designation) DO UPDATE SET
				type = excluded.type,
				in_habitable_zone = excluded.in_habitable_zone,
				life_stage = excluded.life_stage,
				moon_count = excluded.moon_count,
				scanned = 1`,
			p.Designation, null(p.Type), p.InHabitableZone, null(p.LifeStage), len(s.Report.Moons)); err != nil {
			return fmt.Errorf("planet %s: %w", p.Designation, err)
		}
		for _, m := range s.Report.Moons {
			if !strings.HasPrefix(m.Designation, p.Designation+"-") || strings.Count(m.Designation, "-") != 2 {
				return fmt.Errorf("planet scan of %s lists moon %q", p.Designation, m.Designation)
			}
			if _, err := ex.ExecContext(ctx, `INSERT INTO locations (designation, type, in_habitable_zone, scanned)
				VALUES (?, ?, ?, ?)
				ON CONFLICT (designation) DO UPDATE SET
					type = coalesce(excluded.type, type),
					scanned = max(scanned, excluded.scanned)`,
				m.Designation, null(m.Type), p.InHabitableZone, m.Scanned); err != nil {
				return fmt.Errorf("moon %s: %w", m.Designation, err)
			}
		}
		// Planets are STAR-N, moons STAR-N-M.
		_, err := ex.ExecContext(ctx, `UPDATE stars SET
			planet_count = max(planet_count, (SELECT count(*) FROM locations
				WHERE star = ?1 AND designation NOT LIKE ?1 || '-%-%')),
			moon_count = max(moon_count, (SELECT coalesce(sum(moon_count), 0) FROM locations
				WHERE star = ?1 AND designation NOT LIKE ?1 || '-%-%'))
			WHERE designation = ?1`, star)
		return err
	case "moon":
		m := s.Report.Moon
		if len(parts) != 3 || m.Designation != s.ScanTarget {
			return fmt.Errorf("moon scan of %s: report is for moon %q", s.ScanTarget, m.Designation)
		}
		planet := strings.Join(parts[:2], "-")
		var hab bool
		err := ex.QueryRowContext(ctx, `SELECT in_habitable_zone FROM locations WHERE designation = ?`, planet).Scan(&hab)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("moon scan of %s: planet %s isn't in the store", m.Designation, planet)
		}
		if err != nil {
			return err
		}
		_, err = ex.ExecContext(ctx, `INSERT INTO locations (designation, type, in_habitable_zone, life_stage, scanned)
			VALUES (?, ?, ?, ?, 1)
			ON CONFLICT (designation) DO UPDATE SET
				type = coalesce(excluded.type, type),
				life_stage = coalesce(excluded.life_stage, life_stage),
				scanned = 1`,
			m.Designation, null(m.Type), hab, null(m.LifeStage))
		return err
	}
	return fmt.Errorf("scan of %s: unknown scan type %q", s.ScanTarget, s.ScanType)
}
