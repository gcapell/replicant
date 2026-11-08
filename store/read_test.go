package store

import (
	"path/filepath"
	"testing"
)

func TestOpenReadOnly(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")
	if _, err := OpenReadOnly(ctx, path); err == nil {
		t.Error("opened a database that doesn't exist")
	}
	rw, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, rw, `INSERT INTO devices (device_code, device_type, status) VALUES ('A', 'mining_drone', 'idle')`)
	rw.Close()

	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if n, err := Count(ctx, ro, "devices"); err != nil || n != 1 {
		t.Errorf("Count = %d, %v; want 1", n, err)
	}
	if _, err := ro.ExecContext(ctx, `DELETE FROM devices`); err == nil {
		t.Error("a write succeeded")
	}
}
