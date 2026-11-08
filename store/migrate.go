package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// MigrateOptions controls what Migrate may do.
type MigrateOptions struct {
	// AllowDrops lets Migrate drop tables and columns that are no longer in
	// the schema, losing their data. Without it, Migrate refuses and changes
	// nothing.
	AllowDrops bool
}

// Migrate changes db's schema to match schema (a list of CREATE statements),
// and returns a description of each change it made.
//
// It runs schema in an empty in-memory database and compares the result with
// db's sqlite_master:
//   - a new table is created;
//   - a table whose CREATE statement differs is rebuilt: a new table is made
//     from the new definition, the columns the two share are copied across,
//     the old one is dropped and the new one renamed;
//   - a table that's gone is dropped (only with AllowDrops);
//   - indexes, views and triggers that differ or are gone are dropped, and
//     missing ones are created.
//
// Everything happens in one write transaction, so a failure changes nothing
// and two processes can't migrate at once. A renamed column or table looks
// like a drop plus an add, and loses its data.
func Migrate(ctx context.Context, db *sql.DB, schema string, opts MigrateOptions) ([]string, error) {
	want, err := pristine(ctx, schema)
	if err != nil {
		return nil, err
	}

	// PRAGMA foreign_keys can't change inside a transaction, so pin a
	// connection, turn it off, and turn it back on before releasing it.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return nil, err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys=ON")

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, err
	}
	changes, err := migrate(ctx, conn, want, opts)
	if err != nil {
		conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, err
	}
	return changes, nil
}

// object is a row of sqlite_master.
type object struct {
	Type, Name, Table, SQL string
}

// pristine returns the objects schema creates in an empty database.
func pristine(ctx context.Context, schema string) ([]object, error) {
	mem, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer mem.Close()
	mem.SetMaxOpenConns(1) // each :memory: connection is a separate database
	if _, err := mem.ExecContext(ctx, schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return objects(ctx, mem)
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type execQuerier interface {
	querier
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// objects lists the schema objects in creation order, leaving out SQLite's
// own (sqlite_sequence, automatic indexes).
func objects(ctx context.Context, q querier) ([]object, error) {
	rows, err := q.QueryContext(ctx, `SELECT type, name, tbl_name, sql FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objs []object
	for rows.Next() {
		var o object
		if err := rows.Scan(&o.Type, &o.Name, &o.Table, &o.SQL); err != nil {
			return nil, err
		}
		objs = append(objs, o)
	}
	return objs, rows.Err()
}

func byName(objs []object) map[string]object {
	m := make(map[string]object, len(objs))
	for _, o := range objs {
		m[o.Name] = o
	}
	return m
}

func migrate(ctx context.Context, conn execQuerier, want []object, opts MigrateOptions) ([]string, error) {
	have, err := objects(ctx, conn)
	if err != nil {
		return nil, err
	}
	wantBy, haveBy := byName(want), byName(have)

	// Work out what happens to each table before changing anything, so a
	// refused drop leaves the database alone.
	var add, rebuild []object
	var gone, lost []string
	for _, w := range want {
		if w.Type != "table" {
			continue
		}
		h, ok := haveBy[w.Name]
		switch {
		case !ok:
			add = append(add, w)
		case h.Type != "table":
			return nil, fmt.Errorf("%s is a %s in the database but a table in the schema", w.Name, h.Type)
		case normalize(h.SQL) != normalize(w.SQL):
			rebuild = append(rebuild, w)
			oldCols, err := columns(ctx, conn, w.Name)
			if err != nil {
				return nil, err
			}
			newCols, err := pristineColumns(ctx, w.SQL, w.Name)
			if err != nil {
				return nil, err
			}
			for _, c := range oldCols {
				if !slices.Contains(newCols, c) {
					lost = append(lost, w.Name+"."+c)
				}
			}
		}
	}
	for _, h := range have {
		if h.Type == "table" && wantBy[h.Name].Type != "table" {
			gone = append(gone, h.Name)
		}
	}
	if !opts.AllowDrops && len(gone)+len(lost) > 0 {
		return nil, fmt.Errorf("schema would drop %s; migrate with AllowDrops to allow it",
			strings.Join(append(gone, lost...), ", "))
	}

	var changes []string
	exec := func(change, query string) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("%s: %w", change, err)
		}
		if change != "" {
			changes = append(changes, change)
		}
		return nil
	}

	// Drop indexes, views and triggers that are gone or different. When a
	// table is rebuilt, drop every view and trigger: a view naming a table
	// that's briefly missing makes the rename fail.
	for _, h := range have {
		if h.Type == "table" {
			continue
		}
		w, ok := wantBy[h.Name]
		same := ok && w.Type == h.Type && normalize(w.SQL) == normalize(h.SQL)
		if same && !(len(rebuild) > 0 && h.Type != "index") {
			continue
		}
		change := ""
		if !same {
			change = fmt.Sprintf("drop %s %s", h.Type, h.Name)
		}
		if err := exec(change, fmt.Sprintf("DROP %s %s", strings.ToUpper(h.Type), quote(h.Name))); err != nil {
			return nil, err
		}
	}

	for _, name := range gone {
		if err := exec("drop table "+name, "DROP TABLE "+quote(name)); err != nil {
			return nil, err
		}
	}
	for _, w := range add {
		if err := exec("create table "+w.Name, w.SQL); err != nil {
			return nil, err
		}
	}
	for _, w := range rebuild {
		if err := rebuildTable(ctx, conn, w); err != nil {
			return nil, fmt.Errorf("rebuild table %s: %w", w.Name, err)
		}
		changes = append(changes, "rebuild table "+w.Name)
	}

	// Create whatever indexes, views and triggers are now missing. Those on a
	// rebuilt table went with the old table. Recreating one that hasn't
	// changed isn't reported.
	before := haveBy
	now, err := objects(ctx, conn)
	if err != nil {
		return nil, err
	}
	nowBy := byName(now)
	for _, w := range want {
		if w.Type == "table" {
			continue
		}
		if _, ok := nowBy[w.Name]; ok {
			continue
		}
		change := fmt.Sprintf("create %s %s", w.Type, w.Name)
		if h, ok := before[w.Name]; ok && h.Type == w.Type && normalize(h.SQL) == normalize(w.SQL) {
			change = ""
		}
		if err := exec(change, w.SQL); err != nil {
			return nil, err
		}
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return nil, err
	}
	return changes, nil
}

// rebuildTable replaces table w.Name with one made from w.SQL, keeping the
// columns the two share. This is the procedure from
// https://www.sqlite.org/lang_altertable.html#otheralter.
func rebuildTable(ctx context.Context, conn execQuerier, w object) error {
	tmp := w.Name + "_migration_new"
	create, err := renameCreate(w.SQL, tmp)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, create); err != nil {
		return err
	}
	oldCols, err := columns(ctx, conn, w.Name)
	if err != nil {
		return err
	}
	newCols, err := columns(ctx, conn, tmp)
	if err != nil {
		return err
	}
	var common []string
	for _, c := range newCols {
		if slices.Contains(oldCols, c) {
			common = append(common, quote(c))
		}
	}
	if len(common) > 0 {
		cols := strings.Join(common, ", ")
		copy := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", quote(tmp), cols, cols, quote(w.Name))
		if _, err := conn.ExecContext(ctx, copy); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "DROP TABLE "+quote(w.Name)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quote(tmp), quote(w.Name)))
	return err
}

// createTable matches the start of a CREATE TABLE statement, up to and
// including the table's name.
var createTable = regexp.MustCompile("(?is)^\\s*CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?(?:\"(?:[^\"]|\"\")+\"|`[^`]+`|\\[[^\\]]+\\]|[\\w$]+)")

// renameCreate returns a CREATE TABLE statement with the table renamed.
func renameCreate(create, name string) (string, error) {
	loc := createTable.FindStringIndex(create)
	if loc == nil {
		return "", fmt.Errorf("can't find the table name in %q", create)
	}
	return "CREATE TABLE " + quote(name) + create[loc[1]:], nil
}

// columns returns a table's stored columns, leaving out generated ones (which
// can't be inserted into).
func columns(ctx context.Context, q querier, table string) ([]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT name, hidden FROM pragma_table_xinfo(?)", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		var hidden int
		if err := rows.Scan(&name, &hidden); err != nil {
			return nil, err
		}
		if hidden == 0 {
			cols = append(cols, name)
		}
	}
	return cols, rows.Err()
}

// pristineColumns returns the stored columns a CREATE TABLE statement makes.
func pristineColumns(ctx context.Context, create, table string) ([]string, error) {
	mem, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer mem.Close()
	mem.SetMaxOpenConns(1)
	if _, err := mem.ExecContext(ctx, create); err != nil {
		return nil, err
	}
	return columns(ctx, mem, table)
}

func foreignKeyCheck(ctx context.Context, q querier) error {
	rows, err := q.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		bad = append(bad, fmt.Sprintf("%s row %d -> %s", table, rowid.Int64, parent))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(bad) > 0 {
		return fmt.Errorf("foreign key violations after migrating: %s", strings.Join(bad, "; "))
	}
	return nil
}

var (
	lineComment  = regexp.MustCompile(`--[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	space        = regexp.MustCompile(`\s+`)
	punctSpace   = regexp.MustCompile(`\s*([(),])\s*`)
)

// normalize strips comments and insignificant whitespace from a CREATE
// statement, so reformatting schema.sql doesn't cause a rebuild. (It doesn't
// understand string literals, so "--" inside one confuses it; the worst that
// can happen is a needless rebuild.)
func normalize(s string) string {
	s = lineComment.ReplaceAllString(s, "")
	s = blockComment.ReplaceAllString(s, "")
	s = space.ReplaceAllString(s, " ")
	s = punctSpace.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
