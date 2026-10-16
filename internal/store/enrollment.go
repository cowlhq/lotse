package store

import (
	"context"
	"database/sql"
	"errors"
)

// EnrollmentToken is an environment's current enrollment token.
type EnrollmentToken struct {
	ID            int64
	EnvironmentID int64
	Sealed        string
	CreatedBy     string
	CreatedAt     int64
}

// NewEnrollmentToken makes the environment's enrollment token, revoking the
// one it had.
func (s *Store) NewEnrollmentToken(ctx context.Context, actor Actor, envID int64, hash, sealed string, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		env, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), envID))
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, s.rebind(`UPDATE enrollment_tokens SET revoked_at = ?
			WHERE environment_id = ? AND revoked_at IS NULL`), now, envID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO enrollment_tokens (environment_id, token_hash, token_sealed, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)`), envID, hash, sealed, actor.Name, now); err != nil {
			return err
		}
		field := "enrollment token created"
		if n, _ := res.RowsAffected(); n > 0 {
			field = "enrollment token replaced"
		}
		return s.recorder(tx, actor).record(ctx, env.Kind, env.ID, env.Path, field, nil, nil)
	})
}

// EnrollmentToken returns an environment's current token.
func (s *Store) EnrollmentToken(ctx context.Context, envID int64) (EnrollmentToken, error) {
	var t EnrollmentToken
	err := s.queryRow(ctx, `SELECT id, environment_id, token_sealed, created_by, created_at
		FROM enrollment_tokens WHERE environment_id = ? AND revoked_at IS NULL`, envID).Scan(
		&t.ID, &t.EnvironmentID, &t.Sealed, &t.CreatedBy, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// Enrollment is what enrolling gave an agent.
type Enrollment struct {
	Host Host
	// TookOver says the machine was known already: the agent took over its
	// host, history and all, and the host's earlier credentials are revoked.
	TookOver bool
}

// ErrElsewhere is returned for enrolling a machine that is a host in
// another environment, or in none.
var ErrElsewhere = errors.New("the machine is a host in another environment")

// Enroll enrolls a machine with an enrollment token, given as its hash: it
// becomes a host of the token's environment, or takes over the host with
// its machine ID (design 7), and gets the credential credHash.
func (s *Store) Enroll(ctx context.Context, tokenHash, machineID, hostname, credHash string, now int64) (Enrollment, error) {
	var out Enrollment
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var envID int64
		err := tx.QueryRowContext(ctx, s.rebind(`SELECT environment_id FROM enrollment_tokens
			WHERE token_hash = ? AND revoked_at IS NULL`), tokenHash).Scan(&envID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		env, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), envID))
		if err != nil {
			return err
		}
		r := s.recorder(tx, Actor{Kind: "system", Name: "enrollment", Via: "system"})

		h, err := scanHost(tx.QueryRowContext(ctx, s.rebind(`SELECT `+hostColumns+` FROM hosts WHERE machine_id = ?`), machineID))
		if errors.Is(err, sql.ErrNoRows) {
			// A host named in lotse.yaml before its agent came: it is this
			// machine, if the names match, with its settings in place.
			h, err = scanHost(tx.QueryRowContext(ctx, s.rebind(`SELECT `+hostColumns+` FROM hosts
				WHERE environment_id = ? AND name = ? AND machine_id LIKE 'waiting:%'`), envID, hostname))
			if err == nil {
				if err := tx.QueryRowContext(ctx, s.rebind(`UPDATE hosts SET machine_id = ?, hostname = ?, first_seen = ?, last_seen = ?
					WHERE id = ? RETURNING `+hostColumns), machineID, hostname, now, now, h.ID).Scan(hostDest(&h)...); err != nil {
					return err
				}
				if err := r.record(ctx, "host", h.ID, h.Name, "enrolled", nil, env.Path); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, s.rebind(`INSERT INTO agents (host_id, token_hash, created_at)
					VALUES (?, ?, ?)`), h.ID, credHash, now); err != nil {
					return err
				}
				out.Host = h
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		switch {
		case errors.Is(err, sql.ErrNoRows):
			err = tx.QueryRowContext(ctx, s.rebind(`
				INSERT INTO hosts (machine_id, name, hostname, first_seen, last_seen, environment_id)
				VALUES (?, ?, ?, ?, ?, ?) RETURNING `+hostColumns),
				machineID, hostname, hostname, now, now, envID).Scan(hostDest(&h)...)
			if err != nil {
				return err
			}
			if err := r.record(ctx, "host", h.ID, h.Name, "enrolled", nil, env.Path); err != nil {
				return err
			}
		case err != nil:
			return err
		case h.EnvironmentID == nil || *h.EnvironmentID != envID:
			// Taking over moves no host: whoever may enroll in one
			// environment may not take a machine, and its history, from
			// another (decision 0023).
			return ErrElsewhere
		default:
			out.TookOver = true
			if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE agents SET revoked_at = ?
				WHERE host_id = ? AND revoked_at IS NULL`), now, h.ID); err != nil {
				return err
			}
			if err := r.record(ctx, "host", h.ID, h.Name, "enrolled again", nil, env.Path); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`INSERT INTO agents (host_id, token_hash, created_at)
			VALUES (?, ?, ?)`), h.ID, credHash, now); err != nil {
			return err
		}
		out.Host = h
		return nil
	})
	return out, err
}

// AgentHost returns the host whose agent holds the credential credHash, if
// it is not revoked, and notes that it was seen.
func (s *Store) AgentHost(ctx context.Context, credHash string, now int64) (Host, error) {
	var hostID int64
	err := s.queryRow(ctx, `SELECT host_id FROM agents WHERE token_hash = ? AND revoked_at IS NULL`, credHash).Scan(&hostID)
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, ErrNotFound
	}
	if err != nil {
		return Host{}, err
	}
	if _, err := s.exec(ctx, `UPDATE agents SET last_seen_at = ? WHERE token_hash = ?`, now, credHash); err != nil {
		return Host{}, err
	}
	return s.Host(ctx, hostID)
}

// RevokeAgents revokes every credential of a host's agents. The agent can
// only come back by enrolling again.
func (s *Store) RevokeAgents(ctx context.Context, actor Actor, hostID int64, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		h, err := scanHost(tx.QueryRowContext(ctx, s.rebind(`SELECT `+hostColumns+` FROM hosts WHERE id = ?`), hostID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE agents SET revoked_at = ? WHERE host_id = ? AND revoked_at IS NULL`), now, hostID); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "host", h.ID, h.Name, "agent revoked", nil, nil)
	})
}

// AgentRevoked says whether every credential a host's agents ever had is
// revoked: it had an agent, and that agent cannot come back without
// enrolling again.
func (s *Store) AgentRevoked(ctx context.Context, hostID int64) (bool, error) {
	var active, total int
	err := s.queryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN revoked_at IS NULL THEN 1 ELSE 0 END), 0), COUNT(*)
		FROM agents WHERE host_id = ?`, hostID).Scan(&active, &total)
	return total > 0 && active == 0, err
}

// RevokedHosts returns the hosts whose agents are all revoked.
func (s *Store) RevokedHosts(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT host_id FROM agents
		GROUP BY host_id HAVING SUM(CASE WHEN revoked_at IS NULL THEN 1 ELSE 0 END) = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revoked := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		revoked[id] = true
	}
	return revoked, rows.Err()
}
