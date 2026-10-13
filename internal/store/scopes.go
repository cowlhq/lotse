package store

import (
	"context"
	"database/sql"
	"errors"
)

// Scope is a level of the tree: the one global scope, a customer, a
// project or an environment.
type Scope struct {
	ID       int64
	ParentID *int64
	Kind     string
	Slug     string
	Name     string
	// Path is the stable key: acme, acme/shop, acme/shop/prod.
	Path      string
	CreatedAt int64
}

// GlobalScope is the ID of the scope above all customers.
const GlobalScope = 1

// Kinds below each kind of scope.
var childKind = map[string]string{"global": "customer", "customer": "project", "project": "environment"}

const scopeColumns = `id, parent_id, kind, slug, name, path, created_at`

func scanScope(row interface{ Scan(...any) error }) (Scope, error) {
	var x Scope
	err := row.Scan(&x.ID, &x.ParentID, &x.Kind, &x.Slug, &x.Name, &x.Path, &x.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

// Scopes returns every scope but the global one, by path.
func (s *Store) Scopes(ctx context.Context) ([]Scope, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+scopeColumns+` FROM scopes WHERE kind <> 'global' ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Scope{}
	for rows.Next() {
		x, err := scanScope(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	return list, rows.Err()
}

func (s *Store) Scope(ctx context.Context, id int64) (Scope, error) {
	return scanScope(s.queryRow(ctx, `SELECT `+scopeColumns+` FROM scopes WHERE id = ?`, id))
}

// ScopeName is a scope's key in its parent and its name.
type ScopeName struct{ Slug, Name string }

// EnsureEnvironment returns the environment customer/project/environment,
// creating whatever of the three does not exist yet. Existing ones keep
// their names.
func (s *Store) EnsureEnvironment(ctx context.Context, actor Actor, levels [3]ScopeName) (Scope, error) {
	var env Scope
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r := s.recorder(tx, actor)
		parent := Scope{ID: GlobalScope, Kind: "global"}
		for _, l := range levels {
			path := l.Slug
			if parent.Path != "" {
				path = parent.Path + "/" + l.Slug
			}
			x, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE path = ?`), path))
			if errors.Is(err, ErrNotFound) {
				parentID := parent.ID
				x = Scope{ParentID: &parentID, Kind: childKind[parent.Kind], Slug: l.Slug, Name: l.Name, Path: path, CreatedAt: r.at}
				err = tx.QueryRowContext(ctx, s.rebind(`
					INSERT INTO scopes (parent_id, kind, slug, name, path, created_at)
					VALUES (?, ?, ?, ?, ?, ?) RETURNING id`),
					x.ParentID, x.Kind, x.Slug, x.Name, x.Path, x.CreatedAt).Scan(&x.ID)
				if err == nil {
					err = r.record(ctx, x.Kind, x.ID, x.Path, "created", nil, map[string]string{"name": x.Name})
				}
			}
			if err != nil {
				return err
			}
			parent = x
		}
		env = parent
		return nil
	})
	return env, err
}

// RenameScope gives a scope another name; its key stays.
func (s *Store) RenameScope(ctx context.Context, actor Actor, id int64, name string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		x, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if x.Name == name {
			return nil
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE scopes SET name = ? WHERE id = ?`), name, id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, x.Kind, x.ID, x.Path, "name", x.Name, name)
	})
}

// ErrNotEmpty is returned for a scope that still has scopes or hosts in it.
var ErrNotEmpty = errors.New("not empty")

// DeleteScope deletes an empty scope.
func (s *Store) DeleteScope(ctx context.Context, actor Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		x, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if x.Kind == "global" {
			return ErrNotEmpty
		}
		var n int
		if err := tx.QueryRowContext(ctx, s.rebind(`
			SELECT (SELECT count(*) FROM scopes WHERE parent_id = ?) + (SELECT count(*) FROM hosts WHERE environment_id = ?)`),
			id, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrNotEmpty
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM scopes WHERE id = ?`), id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, x.Kind, x.ID, x.Path, "deleted", map[string]string{"name": x.Name}, nil)
	})
}

// SetHostEnvironment moves a host into an environment, or out of all with
// nil.
func (s *Store) SetHostEnvironment(ctx context.Context, actor Actor, hostID int64, envID *int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		h, err := scanHost(tx.QueryRowContext(ctx, s.rebind(`SELECT `+hostColumns+` FROM hosts WHERE id = ?`), hostID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var from, to *string
		for _, x := range []struct {
			id  *int64
			out **string
		}{{h.EnvironmentID, &from}, {envID, &to}} {
			if x.id == nil {
				continue
			}
			sc, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), *x.id))
			if err != nil {
				return err
			}
			if sc.Kind != "environment" {
				return errors.New("hosts live in environments")
			}
			*x.out = &sc.Path
		}
		if ptrEqual(from, to) {
			return nil
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE hosts SET environment_id = ? WHERE id = ?`), envID, hostID); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "host", h.ID, h.Name, "environment", value(from), value(to))
	})
}

// value is what p points to, or an untyped nil: a nil *T in an interface
// is not nil.
func value[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func ptrEqual[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
