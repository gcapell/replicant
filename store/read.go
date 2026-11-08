package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gcapell/replicant/model"
)

// OpenReadOnly opens the existing database at path for reading. It doesn't
// migrate, and every write through it fails (PRAGMA query_only). Every command
// except monitor should use this.
//
// It's a normal connection with query_only rather than mode=ro: in WAL mode a
// mode=ro connection can't create the -wal and -shm files, so it fails
// whenever monitor isn't running (see README, "The store").
func OpenReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	// A normal open would create an empty database; we want to know it's missing.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("store: %w (has monitor run?)", err)
	}
	db, err := sql.Open("sqlite", dsn(path)+"&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Reader is a *sql.DB or *sql.Tx, for queries returning several rows.
type Reader interface {
	Queryer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// StarPosition is where star is, or an error if it isn't in the stars table.
func StarPosition(ctx context.Context, q Queryer, star string) (model.Position, error) {
	var p model.Position
	err := q.QueryRowContext(ctx, `SELECT x, y, z FROM stars WHERE designation = ?`, star).Scan(&p.X, &p.Y, &p.Z)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("star %s isn't in the store", star)
	}
	return p, err
}

// PlacedReplicant is a replicant in a heaven_vessel that isn't travelling.
type PlacedReplicant struct {
	Replicant, Vessel, Star string
}

// PlacedReplicants lists the owners of heaven_vessels carrying a
// replicant_matrix (assumed to be the replicant in each), with the star each
// vessel is at. Travelling vessels (no location) are left out.
func PlacedReplicants(ctx context.Context, q Reader) ([]PlacedReplicant, error) {
	rows, err := q.QueryContext(ctx, `SELECT v.owner, v.device_code, v.star FROM devices m
		JOIN devices v ON v.device_code = m.stowed_in
		WHERE m.device_type = 'replicant_matrix' AND v.device_type = 'heaven_vessel'
			AND v.location IS NOT NULL AND v.owner IS NOT NULL
		ORDER BY v.device_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rs []PlacedReplicant
	for rows.Next() {
		var r PlacedReplicant
		if err := rows.Scan(&r.Replicant, &r.Vessel, &r.Star); err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	return rs, rows.Err()
}

// EventRecorded reports whether the event with this ID is in the events table.
func EventRecorded(ctx context.Context, q Queryer, id string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id = ?`, id).Scan(&n)
	return n > 0, err
}

// EventsNamed returns the recorded events called name (e.g.
// prospect.completed), in the order they were received.
func EventsNamed(ctx context.Context, q Reader, name string) ([]Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id, version, category, event, created_at,
		coalesce(device_code, ''), coalesce(device_type, ''), coalesce(replicant_code, ''),
		coalesce(star, ''), coalesce(location, ''), payload
		FROM events WHERE event = ? ORDER BY seq`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var es []Event
	for rows.Next() {
		var e Event
		var created int64
		var payload string
		if err := rows.Scan(&e.ID, &e.Version, &e.Category, &e.Name, &created, &e.DeviceCode, &e.DeviceType,
			&e.ReplicantCode, &e.Star, &e.Location, &payload); err != nil {
			return nil, err
		}
		e.CreatedAt = time.UnixMilli(created)
		e.Payload = []byte(payload)
		es = append(es, e)
	}
	return es, rows.Err()
}
