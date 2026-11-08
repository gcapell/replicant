package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Event is one event from the stream, as recorded in the events table.
type Event struct {
	ID            string
	Version       int
	Category      string
	Name          string // e.g. travel.arrived
	CreatedAt     time.Time
	DeviceCode    string
	DeviceType    string
	ReplicantCode string
	Star          string
	Location      string
	Payload       []byte // JSON
}

// RecordEvent adds e to the events table and returns its seq. If an event
// with the same ID is already there it records nothing and returns
// recorded=false. An event without an ID or a time is an error.
func RecordEvent(ctx context.Context, q Queryer, e Event) (seq int64, recorded bool, err error) {
	if e.ID == "" || e.Name == "" || e.CreatedAt.IsZero() {
		return 0, false, fmt.Errorf("event %q (%s) has no ID, name or time", e.ID, e.Name)
	}
	err = q.QueryRowContext(ctx, `INSERT INTO events
		(event_id, version, category, event, created_at, device_code, device_type,
		 replicant_code, star, location, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING seq`,
		e.ID, e.Version, e.Category, e.Name, e.CreatedAt.UnixMilli(), null(e.DeviceCode), null(e.DeviceType),
		null(e.ReplicantCode), null(e.Star), null(e.Location), string(e.Payload)).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("recording event %s: %w", e.ID, err)
	}
	return seq, true, nil
}

// LastEventID is the ID of the last event recorded, where the stream should
// resume from, or "" if there's none.
func LastEventID(ctx context.Context, q Queryer) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT event_id FROM events ORDER BY seq DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
