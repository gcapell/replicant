package main

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// fetchStar is api.StarNear, replaceable in tests.
var fetchStar = api.StarNear

// prospected handles prospect.completed: an observatory (the envelope's
// device) at origin (the envelope's star) has found new stars, given by name
// only. We look each one up with /replicants/{code}/stars/{star}, for the
// placed replicant nearest the origin, and store what it says. The
// observatory's observation from origin becomes success, with the stars
// found (store.ObservationCompleted), and its status is fetched from
// /devices/{code} (we set it to prospecting ourselves). Then plant_relay
// tasks are filed for any star that can now join the mesh. It's an error if
// the payload doesn't hang together, if the envelope's device isn't a
// galactic_observatory, if the origin or a replicant's star isn't in the
// store, if no replicant is placed, if a lookup fails, or if the observatory
// has no observation prospecting from origin.
func prospected(ctx context.Context, db store.Reader, env api.Envelope, payload any) (change, error) {
	p, ok := payload.(*model.ProspectCompleted)
	if !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	if p.Origin != env.Star {
		return nil, fmt.Errorf("origin %q, but the event's star is %q", p.Origin, env.Star)
	}
	if len(p.Stars) != p.StarsGenerated {
		return nil, fmt.Errorf("stars_generated %d, but %d stars listed", p.StarsGenerated, len(p.Stars))
	}
	if env.DeviceType != observatory {
		return nil, fmt.Errorf("prospect.completed from %s, a %s", env.DeviceCode, env.DeviceType)
	}
	d, err := eventDevice(ctx, env)
	if err != nil {
		return nil, err
	}
	done := func(ctx context.Context, tx *sql.Tx) error {
		if err := store.ObservationCompleted(ctx, tx, p.Origin, env.DeviceCode, p.Stars, time.Now()); err != nil {
			return err
		}
		if err := store.SetStatus(ctx, tx, d.DeviceCode, d.Status); err != nil {
			return err
		}
		return filePlantRelays(ctx, tx, log.Writer())
	}
	if len(p.Stars) == 0 {
		return done, nil
	}

	origin, err := store.StarPosition(ctx, db, p.Origin)
	if err != nil {
		return nil, err
	}
	placed, err := store.PlacedReplicants(ctx, db)
	if err != nil {
		return nil, err
	}
	pos := make(map[string]model.Position)
	for _, r := range placed {
		if pos[r.Star], err = store.StarPosition(ctx, db, r.Star); err != nil {
			return nil, fmt.Errorf("replicant %s (vessel %s): %w", r.Replicant, r.Vessel, err)
		}
	}
	r, err := nearestReplicant(origin, placed, pos)
	if err != nil {
		return nil, err
	}

	found, err := findStars(ctx, r.Replicant, p.Stars)
	if err != nil {
		return nil, err
	}
	log.Printf("prospect from %s: found %s near replicant %s (at %s)", p.Origin, strings.Join(p.Stars, ", "), r.Replicant, r.Star)
	return func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range found {
			if err := store.PutStar(ctx, tx, s); err != nil {
				return err
			}
		}
		return done(ctx, tx)
	}, nil
}

// nearestReplicant is the replicant whose star (in pos) is nearest origin.
func nearestReplicant(origin model.Position, placed []store.PlacedReplicant, pos map[string]model.Position) (store.PlacedReplicant, error) {
	if len(placed) == 0 {
		return store.PlacedReplicant{}, errors.New("no replicant in a heaven_vessel that isn't travelling")
	}
	return slices.MinFunc(placed, func(a, b store.PlacedReplicant) int {
		return cmp.Compare(origin.Distance(pos[a.Star]), origin.Distance(pos[b.Star]))
	}), nil
}

// findStars looks up each of wanted as seen from replicant.
func findStars(ctx context.Context, replicant string, wanted []string) ([]model.Star, error) {
	var stars []model.Star
	for _, w := range wanted {
		s, err := fetchStar(ctx, replicant, w)
		if err != nil {
			return nil, fmt.Errorf("star %s: %w", w, err)
		}
		if s.Designation != w {
			return nil, fmt.Errorf("asked replicant %s for star %s, got %q", replicant, w, s.Designation)
		}
		stars = append(stars, *s)
	}
	return stars, nil
}
