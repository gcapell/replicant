// Package store is the local SQLite database where we remember what we've
// learnt from the API (star positions, where devices are, ...), so we don't
// have to ask again.
//
// The schema is schema.sql. To change it, edit that file: Open brings the
// database into line (see Migrate).
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var Schema string

// DefaultPath is where the database lives unless told otherwise.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".replicant", "replicant.db")
}

// Open opens (creating if need be) the database at path and migrates it to
// Schema, refusing to drop tables or columns. Run cmd/migrate_db -allow-drops
// to allow that.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	return open(ctx, path, MigrateOptions{})
}

func open(ctx context.Context, path string, opts MigrateOptions) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	changes, err := Migrate(ctx, db, Schema, opts)
	if err != nil {
		db.Close()
		return nil, err
	}
	for _, c := range changes {
		log.Printf("store: %s", c)
	}
	return db, nil
}

// OpenAllowingDrops is Open, but lets the migration drop tables and columns
// that have left the schema.
func OpenAllowingDrops(ctx context.Context, path string) (*sql.DB, error) {
	return open(ctx, path, MigrateOptions{AllowDrops: true})
}

// dsn sets each connection up for several processes sharing the file:
//   - busy_timeout makes a writer wait for the lock instead of failing (it
//     comes first so it covers the other pragmas);
//   - WAL lets readers carry on while one connection writes;
//   - _txlock=immediate makes transactions take the write lock when they
//     begin, so two read-then-write transactions can't deadlock.
func dsn(path string) string {
	return "file:" + path +
		"?_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
}
