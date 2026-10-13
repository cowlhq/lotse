package store

import "testing"

func TestRebind(t *testing.T) {
	pg := &Store{dialect: Postgres}
	lite := &Store{dialect: SQLite}
	for _, c := range []struct{ in, want string }{
		{`SELECT 1`, `SELECT 1`},
		{`SELECT * FROM t WHERE a = ? AND b = ?`, `SELECT * FROM t WHERE a = $1 AND b = $2`},
		{`SELECT '?' FROM t WHERE a = ?`, `SELECT '?' FROM t WHERE a = $1`},
		{`SELECT "a?" FROM t WHERE a = ?`, `SELECT "a?" FROM t WHERE a = $1`},
		{`SELECT 'it''s ?' WHERE a = ?`, `SELECT 'it''s ?' WHERE a = $1`},
	} {
		if got := pg.rebind(c.in); got != c.want {
			t.Errorf("rebind(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := lite.rebind(c.in); got != c.in {
			t.Errorf("sqlite rebind(%q) = %q, want it unchanged", c.in, got)
		}
	}
}
