package store

import (
	"context"
	"database/sql"
	"errors"
)

// Token is an API token: for apply and export, or for reading, on its
// scope and everything below it.
type Token struct {
	ID         int64
	Name       string
	Kind       string // read, apply
	ScopeID    int64
	ScopePath  string // "" for the global scope
	CreatedBy  string
	CreatedAt  int64
	LastUsedAt *int64
	RevokedAt  *int64
}

const tokenColumns = `t.id, t.name, t.kind, t.scope_id, x.path, t.created_by, t.created_at, t.last_used_at, t.revoked_at`

func scanToken(row interface{ Scan(...any) error }) (Token, error) {
	var t Token
	err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.ScopeID, &t.ScopePath, &t.CreatedBy, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// CreateToken keeps a new token's hash. The history is its scope's.
func (s *Store) CreateToken(ctx context.Context, actor Actor, t Token, hash string) (Token, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		scope, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), t.ScopeID))
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO api_tokens (name, kind, scope_id, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)
			RETURNING id`), t.Name, t.Kind, t.ScopeID, hash, t.CreatedBy, t.CreatedAt).Scan(&t.ID); err != nil {
			return err
		}
		t.ScopePath = scope.Path
		return s.recorder(tx, actor).record(ctx, scope.Kind, scope.ID, scope.Path, "api token created", nil, map[string]string{"name": t.Name, "kind": t.Kind})
	})
	return t, err
}

// Tokens returns every token, the newest first.
func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+tokenColumns+` FROM api_tokens t JOIN scopes x ON x.id = t.scope_id
		ORDER BY t.created_at DESC, t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Token{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// TokenByHash returns the token that is not revoked with this hash, and
// notes that it was used.
func (s *Store) TokenByHash(ctx context.Context, hash string, now int64) (Token, error) {
	t, err := scanToken(s.queryRow(ctx, `SELECT `+tokenColumns+` FROM api_tokens t JOIN scopes x ON x.id = t.scope_id
		WHERE t.token_hash = ? AND t.revoked_at IS NULL`, hash))
	if err != nil {
		return t, err
	}
	if t.LastUsedAt == nil || now-*t.LastUsedAt > 60 {
		if _, err := s.exec(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, t.ID); err != nil {
			return t, err
		}
		t.LastUsedAt = &now
	}
	return t, nil
}

// RevokeToken stops a token from working.
func (s *Store) RevokeToken(ctx context.Context, actor Actor, id, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		t, err := scanToken(tx.QueryRowContext(ctx, s.rebind(`SELECT `+tokenColumns+` FROM api_tokens t JOIN scopes x ON x.id = t.scope_id
			WHERE t.id = ? AND t.revoked_at IS NULL`), id))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE api_tokens SET revoked_at = ? WHERE id = ?`), now, id); err != nil {
			return err
		}
		scope, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), t.ScopeID))
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, scope.Kind, scope.ID, scope.Path, "api token revoked", map[string]string{"name": t.Name, "kind": t.Kind}, nil)
	})
}
