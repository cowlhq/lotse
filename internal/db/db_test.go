package db_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"lotse.sh/internal/db"
	"lotse.sh/internal/db/dbtest"
)

func TestOpen(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		if _, err := d.Exec(t.Context(), `INSERT INTO settings (key, value) VALUES (?, ?)`, "a", `"b"`); err != nil {
			t.Fatal(err)
		}
		var v string
		if err := d.QueryRow(t.Context(), `SELECT value FROM settings WHERE key = ?`, "a").Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v != `"b"` {
			t.Errorf("read %q, want %q", v, `"b"`)
		}
	})
}

func TestTx(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := t.Context()
		boom := errors.New("boom")
		err := d.Tx(ctx, func(d *db.DB) error {
			if _, err := d.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('gone', '1')`); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Tx returned %v, want %v", err, boom)
		}
		err = d.Tx(ctx, func(d *db.DB) error {
			_, err := d.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('kept', '1')`)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		rows, err := d.Query(ctx, `SELECT key FROM settings ORDER BY key`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				t.Fatal(err)
			}
			keys = append(keys, k)
		}
		if !slices.Equal(keys, []string{"kept"}) {
			t.Errorf("keys %v, want only the committed one", keys)
		}
	})
}

// Every migration exists for both databases, with the same number and name.
func TestMigrationsMatch(t *testing.T) {
	sqlite, postgres := migrationNames(t, "sqlite"), migrationNames(t, "postgres")
	if !slices.Equal(sqlite, postgres) {
		t.Errorf("migrations differ:\nsqlite:   %v\npostgres: %v", sqlite, postgres)
	}
}

func migrationNames(t *testing.T, dialect string) []string {
	t.Helper()
	entries, err := fs.ReadDir(db.Migrations, "migrations/"+dialect)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	return names
}
