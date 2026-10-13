package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// User is someone who logs in to the hub.
type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	// Role is admin or viewer.
	Role       string
	CreatedAt  int64
	DisabledAt *int64
}

const userColumns = `id, email, name, password_hash, role, created_at, disabled_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// CountUsers says how many users there are. A hub without any is not set up
// yet.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// ErrExists is returned for an object whose key is taken.
var ErrExists = errors.New("already exists")

// CreateUser adds a user. Email addresses are compared without case.
func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	row := s.db.QueryRowContext(ctx, s.rebind(`
		INSERT INTO users (email, name, password_hash, role, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (email) DO NOTHING
		RETURNING `+userColumns), u.Email, u.Name, u.PasswordHash, u.Role, u.CreatedAt)
	created, err := scanUser(row)
	if errors.Is(err, ErrNotFound) {
		return created, ErrExists
	}
	return created, err
}

// UserByEmail returns the user with that address.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.queryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))))
}

// Session is a login.
type Session struct {
	IDHash     string
	UserID     int64
	CreatedAt  int64
	ExpiresAt  int64
	LastSeenAt int64
}

func (s *Store) CreateSession(ctx context.Context, x Session) error {
	_, err := s.exec(ctx, `INSERT INTO sessions (id_hash, user_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`, x.IDHash, x.UserID, x.CreatedAt, x.ExpiresAt, x.LastSeenAt)
	return err
}

// SessionUser returns the user of a session that has not expired at now.
// A disabled user has no sessions.
func (s *Store) SessionUser(ctx context.Context, idHash string, now int64) (Session, User, error) {
	var x Session
	var u User
	err := s.queryRow(ctx, `
		SELECT s.id_hash, s.user_id, s.created_at, s.expires_at, s.last_seen_at,
			u.id, u.email, u.name, u.password_hash, u.role, u.created_at, u.disabled_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id_hash = ? AND s.expires_at > ? AND u.disabled_at IS NULL`, idHash, now).Scan(
		&x.IDHash, &x.UserID, &x.CreatedAt, &x.ExpiresAt, &x.LastSeenAt,
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return x, u, ErrNotFound
	}
	return x, u, err
}

// ExtendSession moves a session's expiry.
func (s *Store) ExtendSession(ctx context.Context, idHash string, now, expires int64) error {
	_, err := s.exec(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id_hash = ?`, now, expires, idHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.exec(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteExpiredSessions removes sessions that ended before now.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now int64) error {
	_, err := s.exec(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	return err
}
