package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustMigrate(t *testing.T, db *sql.DB, schema string, opts MigrateOptions) []string {
	t.Helper()
	changes, err := Migrate(t.Context(), db, schema, opts)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return changes
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func checkChanges(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("changes = %q, want %q", got, want)
	}
}

func cols(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	c, err := columns(t.Context(), db, table)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreateFromScratchThenNoChange(t *testing.T) {
	db := testDB(t)
	schema := `CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT);
		CREATE INDEX a_name ON a (name);
		CREATE VIEW a_names AS SELECT name FROM a;`
	checkChanges(t, mustMigrate(t, db, schema, MigrateOptions{}),
		"create table a", "create index a_name", "create view a_names")
	checkChanges(t, mustMigrate(t, db, schema, MigrateOptions{}))

	// Reformatting and comments aren't changes.
	reformatted := `-- the a table
		CREATE TABLE a (
			id INTEGER PRIMARY KEY,  -- key
			name TEXT
		);
		CREATE INDEX a_name ON a(name);
		CREATE VIEW a_names AS SELECT name FROM a;`
	checkChanges(t, mustMigrate(t, db, reformatted, MigrateOptions{}))
}

func TestAddColumnKeepsData(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT);
		CREATE INDEX a_name ON a (name);`, MigrateOptions{})
	mustExec(t, db, `INSERT INTO a VALUES (1, 'one'), (2, 'two')`)

	checkChanges(t, mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT, n INTEGER NOT NULL DEFAULT 7);
		CREATE INDEX a_name ON a (name);`, MigrateOptions{}),
		"rebuild table a") // the index is recreated silently

	var name string
	var n int
	if err := db.QueryRow(`SELECT name, n FROM a WHERE id = 2`).Scan(&name, &n); err != nil {
		t.Fatal(err)
	}
	if name != "two" || n != 7 {
		t.Errorf("row 2 = %q, %d; want two, 7", name, n)
	}
	var indexes int
	db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'a_name'`).Scan(&indexes)
	if indexes != 1 {
		t.Errorf("index a_name missing after rebuild")
	}
	if _, err := db.Exec(`ALTER TABLE a RENAME TO b`); err != nil {
		t.Errorf("table left in a bad state: %v", err)
	}
}

func TestDropColumnNeedsAllowDrops(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT, junk TEXT)`, MigrateOptions{})
	mustExec(t, db, `INSERT INTO a VALUES (1, 'one', 'x')`)
	smaller := `CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT)`

	_, err := Migrate(t.Context(), db, smaller, MigrateOptions{})
	if err == nil || !strings.Contains(err.Error(), "a.junk") {
		t.Fatalf("Migrate without AllowDrops: err = %v, want one naming a.junk", err)
	}
	if got := cols(t, db, "a"); !slices.Equal(got, []string{"id", "name", "junk"}) {
		t.Errorf("refused migration changed columns to %q", got)
	}

	checkChanges(t, mustMigrate(t, db, smaller, MigrateOptions{AllowDrops: true}), "rebuild table a")
	if got := cols(t, db, "a"); !slices.Equal(got, []string{"id", "name"}) {
		t.Errorf("columns = %q", got)
	}
}

func TestDropTableNeedsAllowDrops(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY); CREATE TABLE b (id INTEGER PRIMARY KEY)`, MigrateOptions{})
	if _, err := Migrate(t.Context(), db, `CREATE TABLE a (id INTEGER PRIMARY KEY)`, MigrateOptions{}); err == nil {
		t.Fatal("dropped table b without AllowDrops")
	}
	checkChanges(t, mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY)`, MigrateOptions{AllowDrops: true}),
		"drop table b")
}

func TestChangeTypeConvertsData(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY, v TEXT)`, MigrateOptions{})
	mustExec(t, db, `INSERT INTO a VALUES (1, '42')`)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY, v INTEGER) STRICT`, MigrateOptions{})
	var typ string
	db.QueryRow(`SELECT typeof(v) FROM a`).Scan(&typ)
	if typ != "integer" {
		t.Errorf("typeof(v) = %s, want integer", typ)
	}
}

func TestNewNotNullColumnWithoutDefaultFailsCleanly(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (id INTEGER PRIMARY KEY)`, MigrateOptions{})
	mustExec(t, db, `INSERT INTO a VALUES (1)`)
	if _, err := Migrate(t.Context(), db, `CREATE TABLE a (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)`, MigrateOptions{}); err == nil {
		t.Fatal("expected a NOT NULL failure")
	}
	if got := cols(t, db, "a"); !slices.Equal(got, []string{"id"}) {
		t.Errorf("failed migration left columns %q", got)
	}
}

func TestIndexChanges(t *testing.T) {
	db := testDB(t)
	mustMigrate(t, db, `CREATE TABLE a (x, y); CREATE INDEX a_i ON a (x)`, MigrateOptions{})
	checkChanges(t, mustMigrate(t, db, `CREATE TABLE a (x, y); CREATE INDEX a_i ON a (x, y)`, MigrateOptions{}),
		"drop index a_i", "create index a_i")
	checkChanges(t, mustMigrate(t, db, `CREATE TABLE a (x, y)`, MigrateOptions{}), "drop index a_i")
}

func TestRebuildWithViewTriggerAndForeignKey(t *testing.T) {
	db := testDB(t)
	schema := `
		CREATE TABLE parent (id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent (id));
		CREATE VIEW names AS SELECT name FROM parent;
		CREATE TABLE log (msg TEXT);
		CREATE TRIGGER parent_log AFTER INSERT ON parent BEGIN INSERT INTO log VALUES (new.name); END;`
	mustMigrate(t, db, schema, MigrateOptions{})
	mustExec(t, db, `INSERT INTO parent VALUES (1, 'p')`)
	mustExec(t, db, `INSERT INTO child VALUES (1, 1)`)

	changed := strings.Replace(schema, "name TEXT)", "name TEXT, extra TEXT)", 1)
	checkChanges(t, mustMigrate(t, db, changed, MigrateOptions{}), "rebuild table parent")

	// The child's foreign key still points at parent, and still holds.
	if _, err := db.Exec(`INSERT INTO child VALUES (2, 99)`); err == nil {
		t.Error("foreign key not enforced after rebuild")
	}
	var fk string
	db.QueryRow(`SELECT "table" FROM pragma_foreign_key_list('child')`).Scan(&fk)
	if fk != "parent" {
		t.Errorf("child references %q, want parent", fk)
	}
	// The trigger and view came back.
	mustExec(t, db, `INSERT INTO parent (id, name) VALUES (2, 'q')`)
	var logged, named int
	db.QueryRow(`SELECT count(*) FROM log`).Scan(&logged)
	db.QueryRow(`SELECT count(*) FROM names`).Scan(&named)
	if logged != 2 || named != 2 {
		t.Errorf("log rows = %d, view rows = %d; want 2, 2", logged, named)
	}
}

func TestGeneratedColumnsSurviveRebuild(t *testing.T) {
	for _, kind := range []string{"VIRTUAL", "STORED"} {
		t.Run(kind, func(t *testing.T) {
			db := testDB(t)
			mustMigrate(t, db, `CREATE TABLE a (loc TEXT, star TEXT AS (substr(loc, 1, 3)) `+kind+`)`, MigrateOptions{})
			mustExec(t, db, `INSERT INTO a (loc) VALUES ('ABC-1')`)
			mustMigrate(t, db, `CREATE TABLE a (loc TEXT, star TEXT AS (substr(loc, 1, 2)) `+kind+`, n INTEGER)`, MigrateOptions{})
			var star string
			db.QueryRow(`SELECT star FROM a`).Scan(&star)
			if star != "AB" { // recomputed with the new expression
				t.Errorf("star = %q, want AB", star)
			}
		})
	}
}

func TestConcurrentMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []string
	for range 4 {
		wg.Go(func() {
			db, err := sql.Open("sqlite", dsn(path))
			if err != nil {
				t.Error(err)
				return
			}
			defer db.Close()
			changes, err := Migrate(context.Background(), db, Schema, MigrateOptions{})
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			all = append(all, changes...)
			mu.Unlock()
		})
	}
	wg.Wait()
	if n := slices.Index(all, "create table stars"); n < 0 || slices.Index(all[n+1:], "create table stars") >= 0 {
		t.Errorf("want stars created exactly once, got changes %q", all)
	}
}

func TestOpenRealSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "replicant.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO devices (device_code, device_type, status, location, raw)
		VALUES ('D1', 'ftl_relay', 'relaying', 'MIRETH-4-L4', '{}'),
		       ('D2', 'heaven_vessel', 'idle', NULL, '{}')`)
	var star string
	db.QueryRow(`SELECT star FROM relay_stars`).Scan(&star)
	if star != "MIRETH" {
		t.Errorf("relay star = %q, want MIRETH", star)
	}
	var noStar sql.NullString
	db.QueryRow(`SELECT star FROM devices WHERE device_code = 'D2'`).Scan(&noStar)
	if noStar.Valid {
		t.Errorf("star with no location = %q, want NULL", noStar.String)
	}

	mustExec(t, db, `INSERT INTO stars (designation, x, y, z) VALUES ('A', 0, 0, 0)`)
	mustExec(t, db, `INSERT INTO stars VALUES ('B', 0, 0, 0, NULL, 3, 3, 2, 1), ('C', 0, 0, 0, NULL, 3, 3, 2, 2)`)
	rows, err := db.Query(`SELECT designation, is_fully_scanned FROM stars ORDER BY designation`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var d string
		var full sql.NullBool
		rows.Scan(&d, &full)
		got = append(got, fmt.Sprintf("%s:%v/%v", d, full.Bool, full.Valid))
	}
	rows.Close()
	if want := []string{"A:false/false", "B:false/true", "C:true/true"}; !slices.Equal(got, want) {
		t.Errorf("is_fully_scanned = %q, want %q", got, want)
	}

	addEvent := `INSERT INTO events (event_id, version, category, event, created_at, payload)
		VALUES (?, 1, 'travel', 'travel.arrived', 0, '{}')`
	mustExec(t, db, addEvent, "1794115235797-10")
	mustExec(t, db, addEvent, "1794115235797-9")
	if _, err := db.Exec(addEvent, "1794115235797-9"); err == nil {
		t.Error("recorded the same event twice")
	}
	var last string
	db.QueryRow(`SELECT event_id FROM events ORDER BY seq DESC LIMIT 1`).Scan(&last)
	if last != "1794115235797-9" {
		t.Errorf("last event = %q, want the last one received", last)
	}
	db.Close()

	db, err = Open(t.Context(), path) // reopening changes nothing
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

func TestRenameCreate(t *testing.T) {
	for _, in := range []string{
		`CREATE TABLE a (x)`,
		`create table if not exists a(x)`,
		`CREATE TABLE "a" (x)`,
		"CREATE TABLE `a` (x)",
		`CREATE TABLE [a] (x)`,
	} {
		got, err := renameCreate(in, "b")
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if normalize(got) != `CREATE TABLE "b"(x)` {
			t.Errorf("renameCreate(%s) = %s", in, got)
		}
	}
}
