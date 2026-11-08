package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// fetchLocation is api.Location, replaceable in tests.
var fetchLocation = api.Location

// experienced handles experience.gained. The only one that changes the store
// is a scan (source "scan") at a star whose system we haven't read yet
// (planet_count is NULL, or the star isn't in the store): then the system is
// read from /locations/STAR and stored, as at startup. That's what a vessel's
// system_scan produces (500 XP); AMI planet and moon scans (10 and 100 XP)
// share the source but happen in systems already read, so they cost nothing.
// The envelope's location must be the bare star.
func experienced(ctx context.Context, db store.Reader, env api.Envelope, payload any) (change, error) {
	p, ok := payload.(*model.ExperienceGained)
	if !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	if p.Source != "scan" {
		return nil, nil
	}
	star := env.Star
	if star == "" || env.Location != star {
		return nil, fmt.Errorf("scan experience at location %q, star %q: want a bare star", env.Location, star)
	}
	var known bool
	err := db.QueryRowContext(ctx, `SELECT planet_count IS NOT NULL FROM stars WHERE designation = ?`, star).Scan(&known)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if known {
		return nil, nil
	}
	loc, err := fetchLocation(ctx, star)
	if err != nil {
		return nil, fmt.Errorf("reading scanned system %s: %w", star, err)
	}
	if loc.Location != star {
		return nil, fmt.Errorf("asked for /locations/%s, got %q", star, loc.Location)
	}
	return func(ctx context.Context, tx *sql.Tx) error { return store.PutSystem(ctx, tx, *loc) }, nil
}
