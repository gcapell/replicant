package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ledger is the set of stars we've already observed from (each star can be
// prospected from only once), or that were refused as too dense. It's kept in a
// text file, one star per line. Anything after the star name is a note (e.g.
// "dense ratio=3.2"), and blank lines and lines starting with '#' are ignored.
type ledger map[string]string // star -> note

func (l ledger) done(star string) bool {
	_, ok := l[star]
	return ok
}

func ledgerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "observed_from.txt"), nil
}

// ledgerFile appends to the ledger file. It's safe for concurrent use.
type ledgerFile struct {
	mu   sync.Mutex
	path string
}

// add appends "star note" to the file, creating it if need be.
func (l *ledgerFile) add(star, note string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, strings.TrimSpace(star+" "+note))
	return errors.Join(err, f.Close())
}

// loadLedger reads the ledger at path. A missing file is an empty ledger.
func loadLedger(path string) (ledger, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ledger{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	l := ledger{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		star, note, _ := strings.Cut(line, " ")
		l[star] = strings.TrimSpace(note)
	}
	return l, sc.Err()
}
