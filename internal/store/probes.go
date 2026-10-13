package store

import (
	"context"
	"database/sql"
	"errors"
)

// Probe runs checks: the hub's own (ID 1), or one in another place.
type Probe struct {
	ID        int64
	Slug      string
	Name      string
	Location  string
	Builtin   bool
	Version   string
	CreatedAt int64
	LastSeen  *int64
	RevokedAt *int64
}

// BuiltinProbe is the hub's own probe.
const BuiltinProbe = 1

const probeColumns = `id, slug, name, location, builtin, version, created_at, last_seen, revoked_at`

func scanProbe(row interface{ Scan(...any) error }) (Probe, error) {
	var p Probe
	var builtin int
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Location, &builtin, &p.Version, &p.CreatedAt, &p.LastSeen, &p.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	p.Builtin = builtin == 1
	return p, err
}

// Probes returns every probe, the hub's first.
func (s *Store) Probes(ctx context.Context) ([]Probe, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+probeColumns+` FROM probes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Probe{}
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// EnrollProbe makes a probe with the probe token (given as its hash): a
// new one, or the one with the same slug, which gets the new credential.
func (s *Store) EnrollProbe(ctx context.Context, tokenHash, slug, name, location, credHash string, now int64) (Probe, error) {
	var p Probe
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, s.rebind(`SELECT count(*) FROM enrollment_tokens
			WHERE token_hash = ? AND environment_id = ? AND revoked_at IS NULL`), tokenHash, GlobalScope).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		row := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO probes (slug, name, location, token_hash, created_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (slug) DO UPDATE SET name = excluded.name, location = excluded.location,
				token_hash = excluded.token_hash, revoked_at = NULL
			RETURNING `+probeColumns), slug, name, location, credHash, now)
		var err error
		p, err = scanProbe(row)
		if err != nil {
			return err
		}
		if p.Builtin {
			return errors.New("the hub's own probe cannot be enrolled")
		}
		return s.recorder(tx, Actor{Kind: "system", Name: "enrollment", Via: "system"}).
			record(ctx, "probe", p.ID, p.Slug, "enrolled", nil, map[string]string{"name": name, "location": location})
	})
	return p, err
}

// ProbeByCredential returns the probe that holds credHash, if not revoked,
// and notes that it was seen.
func (s *Store) ProbeByCredential(ctx context.Context, credHash, version string, now int64) (Probe, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`UPDATE probes SET last_seen = ?, version = ?
		WHERE token_hash = ? AND revoked_at IS NULL RETURNING `+probeColumns), now, version, credHash)
	return scanProbe(row)
}

// RevokeProbe revokes a probe's credential.
func (s *Store) RevokeProbe(ctx context.Context, actor Actor, id int64, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		p, err := scanProbe(tx.QueryRowContext(ctx, s.rebind(`SELECT `+probeColumns+` FROM probes WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if p.Builtin {
			return errors.New("the hub's own probe cannot be revoked")
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE probes SET revoked_at = ? WHERE id = ?`), now, id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "probe", p.ID, p.Slug, "revoked", nil, nil)
	})
}
