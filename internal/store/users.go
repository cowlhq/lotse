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
	// LinkExpiresAt is set while a link to choose a password works: an
	// invitation, or a new password (decision 0023).
	LinkExpiresAt *int64
	LastLoginAt   *int64
}

// Invited says whether the user has not chosen a password yet.
func (u User) Invited() bool { return u.PasswordHash == "" }

const userColumns = `id, email, name, password_hash, role, created_at, disabled_at, link_expires_at, last_login_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.DisabledAt, &u.LinkExpiresAt, &u.LastLoginAt)
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
			u.id, u.email, u.name, u.password_hash, u.role, u.created_at, u.disabled_at, u.link_expires_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id_hash = ? AND s.expires_at > ? AND u.disabled_at IS NULL`, idHash, now).Scan(
		&x.IDHash, &x.UserID, &x.CreatedAt, &x.ExpiresAt, &x.LastSeenAt,
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.DisabledAt, &u.LinkExpiresAt, &u.LastLoginAt)
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

// Users returns everyone, in the order they were added.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// UserByID returns one user.
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.queryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// ErrLastAdmin is returned for a change that would leave the hub without an
// admin who can log in.
var ErrLastAdmin = errors.New("the hub needs at least one admin")

// The history of users is the hub's, with the user's address in the field.
func (r *recorder) user(ctx context.Context, email, what string, old, new any) error {
	return r.record(ctx, "global", 1, "", "user "+email+" "+what, old, new)
}

// InviteUser adds a user who has no password yet, with a link that works
// until expires. Its hash is all that is kept.
func (s *Store) InviteUser(ctx context.Context, actor Actor, u User, linkHash string, expires int64) (User, error) {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	var created User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		created, err = scanUser(tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO users (email, name, password_hash, role, created_at, link_hash, link_expires_at)
			VALUES (?, ?, '', ?, ?, ?, ?)
			ON CONFLICT (email) DO NOTHING
			RETURNING `+userColumns), u.Email, u.Name, u.Role, u.CreatedAt, linkHash, expires))
		if errors.Is(err, ErrNotFound) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).user(ctx, created.Email, "invited", nil, map[string]string{"role": created.Role})
	})
	return created, err
}

// SetUserLink gives a user a new link to choose a password with, which
// replaces any earlier one.
func (s *Store) SetUserLink(ctx context.Context, actor Actor, id int64, linkHash string, expires int64) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		u, err = scanUser(tx.QueryRowContext(ctx, s.rebind(`
			UPDATE users SET link_hash = ?, link_expires_at = ? WHERE id = ? RETURNING `+userColumns), linkHash, expires, id))
		if err != nil {
			return err
		}
		what := "got a link for a new password"
		if u.Invited() {
			what = "got a new invitation link"
		}
		return s.recorder(tx, actor).user(ctx, u.Email, what, nil, nil)
	})
	return u, err
}

// UserByLink returns the user whose link has this hash and still works.
func (s *Store) UserByLink(ctx context.Context, linkHash string, now int64) (User, error) {
	return scanUser(s.queryRow(ctx, `SELECT `+userColumns+` FROM users
		WHERE link_hash = ? AND link_expires_at > ? AND disabled_at IS NULL`, linkHash, now))
}

// UseLink sets the password of the user whose link has this hash, and the
// name if one is given; the link is spent, and the user's sessions end.
func (s *Store) UseLink(ctx context.Context, linkHash, name, passwordHash string, now int64) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		u, err = scanUser(tx.QueryRowContext(ctx, s.rebind(`
			UPDATE users SET password_hash = ?, name = CASE WHEN ? = '' THEN name ELSE ? END,
				link_hash = NULL, link_expires_at = NULL
			WHERE link_hash = ? AND link_expires_at > ? AND disabled_at IS NULL
			RETURNING `+userColumns), passwordHash, name, name, linkHash, now))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM sessions WHERE user_id = ?`), u.ID); err != nil {
			return err
		}
		id := u.ID
		return s.recorder(tx, Actor{Kind: "user", ID: &id, Name: u.Email, Via: "ui"}).user(ctx, u.Email, "chose a password", nil, nil)
	})
	return u, err
}

// UserChange is what an admin changes of a user; nil leaves it.
type UserChange struct {
	Name     *string
	Role     *string
	Disabled *bool
}

// UpdateUser changes a user. A user who is disabled loses every session.
// It refuses to leave the hub without an admin who can log in.
func (s *Store) UpdateUser(ctx context.Context, actor Actor, id int64, c UserChange, now int64) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := scanUser(tx.QueryRowContext(ctx, s.rebind(`SELECT `+userColumns+` FROM users WHERE id = ?`), id))
		if err != nil {
			return err
		}
		u = old
		rec := s.recorder(tx, actor)
		if c.Name != nil && *c.Name != old.Name {
			u.Name = *c.Name
			if err := rec.user(ctx, u.Email, "name", old.Name, u.Name); err != nil {
				return err
			}
		}
		if c.Role != nil && *c.Role != old.Role {
			u.Role = *c.Role
			if err := rec.user(ctx, u.Email, "role", old.Role, u.Role); err != nil {
				return err
			}
		}
		if c.Disabled != nil && *c.Disabled != (old.DisabledAt != nil) {
			u.DisabledAt = nil
			if *c.Disabled {
				u.DisabledAt = &now
			}
			if err := rec.user(ctx, u.Email, "disabled", old.DisabledAt != nil, *c.Disabled); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE users SET name = ?, role = ?, disabled_at = ? WHERE id = ?`),
			u.Name, u.Role, u.DisabledAt, id); err != nil {
			return err
		}
		if u.DisabledAt != nil {
			if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM sessions WHERE user_id = ?`), id); err != nil {
				return err
			}
		}
		return s.checkAdmins(ctx, tx)
	})
	return u, err
}

// checkAdmins fails with ErrLastAdmin when no admin can log in any more.
func (s *Store) checkAdmins(ctx context.Context, tx *sql.Tx) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users
		WHERE role = 'admin' AND disabled_at IS NULL AND password_hash <> ''`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}

// ErrNotInvited is returned for withdrawing the invitation of a user who
// has accepted it.
var ErrNotInvited = errors.New("the user has accepted the invitation")

// WithdrawInvitation deletes a user who has not chosen a password yet.
func (s *Store) WithdrawInvitation(ctx context.Context, actor Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		u, err := scanUser(tx.QueryRowContext(ctx, s.rebind(`SELECT `+userColumns+` FROM users WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if !u.Invited() {
			return ErrNotInvited
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM users WHERE id = ?`), id); err != nil {
			return err
		}
		return s.recorder(tx, actor).user(ctx, u.Email, "invitation withdrawn", map[string]string{"role": u.Role}, nil)
	})
}

// SetOwnPassword changes a user's password and ends their other sessions.
func (s *Store) SetOwnPassword(ctx context.Context, id int64, passwordHash, keepSession string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var email string
		if err := tx.QueryRowContext(ctx, s.rebind(`UPDATE users SET password_hash = ? WHERE id = ? RETURNING email`), passwordHash, id).Scan(&email); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM sessions WHERE user_id = ? AND id_hash <> ?`), id, keepSession); err != nil {
			return err
		}
		uid := id
		return s.recorder(tx, Actor{Kind: "user", ID: &uid, Name: email, Via: "ui"}).user(ctx, email, "changed the password", nil, nil)
	})
}

// NoteLogin keeps when a user last logged in.
func (s *Store) NoteLogin(ctx context.Context, id, now int64) error {
	_, err := s.exec(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}
