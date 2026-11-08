package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
