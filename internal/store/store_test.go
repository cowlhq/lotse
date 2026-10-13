package store_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestOpen(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		n, err := s.CountUsers(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("a new store has %d users, want none", n)
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
	entries, err := fs.ReadDir(store.Migrations, "migrations/"+dialect)
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
