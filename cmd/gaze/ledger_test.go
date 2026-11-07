package main

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLedger(t *testing.T) {
	dir := t.TempDir()
	l, err := loadLedger(filepath.Join(dir, "missing.txt"))
	if err != nil || len(l) != 0 {
		t.Fatalf("missing file: %v, %v", l, err)
	}

	path := filepath.Join(dir, "observed_from.txt")
	os.WriteFile(path, []byte("# seeded by hand\nMIRETH\n\n  SYLSHENOS  dense ratio=3.2\n"), 0o644)
	l, err = loadLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	want := ledger{"MIRETH": "", "SYLSHENOS": "dense ratio=3.2"}
	if !maps.Equal(l, want) {
		t.Errorf("got %v, want %v", l, want)
	}

	lf := &ledgerFile{path: filepath.Join(dir, "new.txt")}
	if err := lf.add("ALPHA", "prospected now"); err != nil {
		t.Fatal(err)
	}
	if err := lf.add("BETA", ""); err != nil {
		t.Fatal(err)
	}
	l, err = loadLedger(lf.path)
	if err != nil {
		t.Fatal(err)
	}
	if want := (ledger{"ALPHA": "prospected now", "BETA": ""}); !maps.Equal(l, want) {
		t.Errorf("after add: got %v, want %v", l, want)
	}
}
