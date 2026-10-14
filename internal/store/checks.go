package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Check is a check of something from outside (design 8.3).
type Check struct {
	ID            int64
	EnvironmentID int64
	ContainerID   *int64
	Slug          string
	// Path is the stable key: the environment's path and the slug.
	Path string
	Name string
	Type string // http, tls, tcp, heartbeat
	// Spec is what to check, as JSON; its shape depends on Type.
	Spec json.RawMessage
	// PingSealed is a heartbeat's ping token, sealed.
	PingSealed *string
	// State is up, down or unknown, since StateSince.
	State      string
	StateSince *int64
	LastPing   *int64
	CreatedAt  int64
}

const checkColumns = `id, environment_id, container_id, slug, path, name, type, spec_json,
	ping_token_sealed, state, state_since, last_ping, created_at`

func scanCheck(row interface{ Scan(...any) error }) (Check, error) {
	var c Check
	var spec string
	err := row.Scan(&c.ID, &c.EnvironmentID, &c.ContainerID, &c.Slug, &c.Path, &c.Name, &c.Type,
		&spec, &c.PingSealed, &c.State, &c.StateSince, &c.LastPing, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	c.Spec = json.RawMessage(spec)
	return c, err
}

// CreateCheck adds a check to an environment. A heartbeat gets its ping
// token's hash and sealed copy.
func (s *Store) CreateCheck(ctx context.Context, actor Actor, c Check, pingHash string) (Check, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		env, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), c.EnvironmentID))
		if err != nil {
			return err
		}
		if env.Kind != "environment" {
			return ErrNotFound
		}
		c.Path = env.Path + "/" + c.Slug
		var hash *string
		if pingHash != "" {
			hash = &pingHash
		}
		row := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO checks (environment_id, container_id, slug, path, name, type, spec_json,
				ping_token_hash, ping_token_sealed, state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'unknown', ?)
			ON CONFLICT (path) DO NOTHING
			RETURNING `+checkColumns),
			c.EnvironmentID, c.ContainerID, c.Slug, c.Path, c.Name, c.Type, string(c.Spec),
			hash, c.PingSealed, c.CreatedAt)
		c, err = scanCheck(row)
		if errors.Is(err, ErrNotFound) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "check", c.ID, c.Path, "created", nil,
			map[string]any{"name": c.Name, "type": c.Type, "spec": c.Spec})
	})
	return c, err
}

// UpdateCheck changes a check's name and what it checks.
func (s *Store) UpdateCheck(ctx context.Context, actor Actor, id int64, name string, spec json.RawMessage) (Check, error) {
	var c Check
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := scanCheck(tx.QueryRowContext(ctx, s.rebind(`SELECT `+checkColumns+` FROM checks WHERE id = ?`), id))
		if err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, s.rebind(`UPDATE checks SET name = ?, spec_json = ? WHERE id = ?
			RETURNING `+checkColumns), name, string(spec), id)
		if c, err = scanCheck(row); err != nil {
			return err
		}
		r := s.recorder(tx, actor)
		if old.Name != name {
			if err := r.record(ctx, "check", id, c.Path, "name", old.Name, name); err != nil {
				return err
			}
		}
		if string(old.Spec) != string(spec) {
			return r.record(ctx, "check", id, c.Path, "spec", old.Spec, spec)
		}
		return nil
	})
	return c, err
}

// DeleteCheck deletes a check and its results.
func (s *Store) DeleteCheck(ctx context.Context, actor Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		c, err := scanCheck(tx.QueryRowContext(ctx, s.rebind(`SELECT `+checkColumns+` FROM checks WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM checks WHERE id = ?`), id); err != nil {
			return err
		}
		if err := s.forget(ctx, tx, "check", id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "check", c.ID, c.Path, "deleted", map[string]any{"name": c.Name, "type": c.Type}, nil)
	})
}

func (s *Store) Check(ctx context.Context, id int64) (Check, error) {
	return scanCheck(s.queryRow(ctx, `SELECT `+checkColumns+` FROM checks WHERE id = ?`, id))
}

// Checks returns the checks of an environment, or all with envID 0, by
// path.
func (s *Store) Checks(ctx context.Context, envID int64) ([]Check, error) {
	q := `SELECT ` + checkColumns + ` FROM checks`
	var args []any
	if envID != 0 {
		q += ` WHERE environment_id = ?`
		args = append(args, envID)
	}
	rows, err := s.read.QueryContext(ctx, s.rebind(q+` ORDER BY path`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Check{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// SetCheckState records a check's state if it changed, and says whether
// it did.
func (s *Store) SetCheckState(ctx context.Context, id int64, state string, since int64) (bool, error) {
	res, err := s.exec(ctx, `UPDATE checks SET state = ?, state_since = ? WHERE id = ? AND state <> ?`, state, since, id, state)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Ping records a heartbeat's ping, given its token's hash, and returns the
// check.
func (s *Store) Ping(ctx context.Context, pingHash string, ts int64) (Check, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`UPDATE checks SET last_ping = ? WHERE ping_token_hash = ?
		RETURNING `+checkColumns), ts, pingHash)
	return scanCheck(row)
}

// CheckResult is one run of a check by one probe.
type CheckResult struct {
	CheckID      int64
	ProbeID      int64
	TS           int64
	OK           bool
	LatencyMS    *float64
	StatusCode   *int
	CertNotAfter *int64
	Error        string
}

// LatestResults returns each probe's newest result of a check.
func (s *Store) LatestResults(ctx context.Context, checkID int64) ([]CheckResult, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT r.check_id, r.probe_id, r.ts, r.ok, r.latency_ms, r.status_code, r.cert_not_after, r.error
		FROM check_results r
		WHERE r.check_id = ? AND r.ts = (SELECT max(ts) FROM check_results WHERE check_id = r.check_id AND probe_id = r.probe_id)
		ORDER BY r.probe_id`), checkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CheckResult
	for rows.Next() {
		var r CheckResult
		var ok int
		var errText *string
		if err := rows.Scan(&r.CheckID, &r.ProbeID, &r.TS, &ok, &r.LatencyMS, &r.StatusCode, &r.CertNotAfter, &errText); err != nil {
			return nil, err
		}
		r.OK = ok == 1
		if errText != nil {
			r.Error = *errText
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// Results returns a check's results from from to to, newest first, at most
// limit.
func (s *Store) Results(ctx context.Context, checkID, from, to int64, limit int) ([]CheckResult, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT check_id, probe_id, ts, ok, latency_ms, status_code, cert_not_after, error
		FROM check_results WHERE check_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts DESC, probe_id LIMIT ?`), checkID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []CheckResult{}
	for rows.Next() {
		var r CheckResult
		var ok int
		var errText *string
		if err := rows.Scan(&r.CheckID, &r.ProbeID, &r.TS, &ok, &r.LatencyMS, &r.StatusCode, &r.CertNotAfter, &errText); err != nil {
			return nil, err
		}
		r.OK = ok == 1
		if errText != nil {
			r.Error = *errText
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
