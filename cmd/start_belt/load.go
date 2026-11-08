package main

import (
	"context"
	"database/sql"

	"github.com/gcapell/replicant/model"
)

// world is what start_belt reads from the store.
type world struct {
	belts      []Belt
	devices    []Device
	relayStars map[string]bool
	starPos    map[string]model.Position
}

func load(ctx context.Context, db *sql.DB) (*world, error) {
	w := &world{relayStars: map[string]bool{}, starPos: map[string]model.Position{}}

	rows, err := db.QueryContext(ctx, `SELECT designation, x, y, z FROM stars`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		var p model.Position
		if err := rows.Scan(&s, &p.X, &p.Y, &p.Z); err != nil {
			return nil, err
		}
		w.starPos[s] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT star FROM relay_stars`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		w.relayStars[s] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT b.designation, b.star, s.x, s.y, s.z, coalesce(b.density, ''),
			coalesce(b.volatiles, ''), coalesce(b.rares, ''), coalesce(b.carbon, ''),
			coalesce(b.conductive, ''), coalesce(b.silicates, ''), coalesce(b.structural, '')
		FROM belts b JOIN stars s ON s.designation = b.star`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var b Belt
		var v, r, c, co, si, st string
		if err := rows.Scan(&b.Designation, &b.Star, &b.Pos.X, &b.Pos.Y, &b.Pos.Z, &b.Density,
			&v, &r, &c, &co, &si, &st); err != nil {
			return nil, err
		}
		b.Levels = map[model.ResourceType]string{
			model.Volatiles: v, model.Rares: r, model.Carbon: c,
			model.Conductive: co, model.Silicates: si, model.Structural: st,
		}
		w.belts = append(w.belts, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT device_code, device_type, status, coalesce(location, ''), coalesce(star, ''),
			coalesce(attached_to, ''), coalesce(stowed_in, ''), coalesce(controller, ''),
			coalesce(destination, '')
		FROM devices`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.Code, &d.Type, &d.Status, &d.Location, &d.Star,
			&d.AttachedTo, &d.StowedIn, &d.Control, &d.Destination); err != nil {
			return nil, err
		}
		w.devices = append(w.devices, d)
	}
	return w, rows.Err()
}
