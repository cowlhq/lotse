// Package dbtest runs tests against every database the hub supports.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"lotse.sh/internal/db"
)

// PostgresEnv names the variable with the URL of a Postgres server for
// tests. Each test gets a database of its own on it, dropped afterwards.
const PostgresEnv = "LOTSE_TEST_POSTGRES_URL"

// Each runs test once per database, each time on a new, empty one. In CI
// (CI is set) a missing or unreachable Postgres fails the test; elsewhere
// the Postgres run is skipped.
func Each(t *testing.T, test func(t *testing.T, d *db.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "lotse.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		test(t, d)
	})
	t.Run("postgres", func(t *testing.T) {
		test(t, openPostgres(t))
	})
}

func openPostgres(t *testing.T) *db.DB {
	t.Helper()
	unavailable := t.Skipf
	if os.Getenv("CI") != "" {
		unavailable = t.Fatalf
	}
	server := os.Getenv(PostgresEnv)
	if server == "" {
		unavailable("%s is not set", PostgresEnv)
	}
	admin, err := sql.Open("pgx", server)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.PingContext(t.Context()); err != nil {
		unavailable("postgres: %v", err)
	}
	name := "lotse_test_" + random()
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	d, err := db.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		admin, err := sql.Open("pgx", server)
		if err != nil {
			t.Error(err)
			return
		}
		defer admin.Close()
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	return d
}

func random() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
