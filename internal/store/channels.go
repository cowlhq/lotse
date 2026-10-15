package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Channel is where notifications go (design 8.4), defined on a scope and
// usable on it and below it.
type Channel struct {
	ID        int64
	ScopeID   int64
	ScopePath string // "" for the global scope
	Slug      string
	Name      string
	Type      string // email, googlechat, webhook, slack, discord, telegram, ntfy, pushover
	// Spec is what is not secret, as JSON; its shape depends on Type.
	Spec json.RawMessage
	// Secrets holds the sealed secret fields, as the hub sealed them, and
	// SecretRefs the references to secrets it resolves when it sends.
	Secrets    *string
	SecretRefs json.RawMessage
	CreatedAt  int64
}

const channelColumns = `c.id, c.scope_id, x.path, c.slug, c.name, c.type, c.spec_json,
	c.secret_sealed, c.secret_refs_json, c.created_at`

func scanChannel(row interface{ Scan(...any) error }) (Channel, error) {
	var c Channel
	var spec, refs string
	err := row.Scan(&c.ID, &c.ScopeID, &c.ScopePath, &c.Slug, &c.Name, &c.Type, &spec, &c.Secrets, &refs, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	c.Spec, c.SecretRefs = json.RawMessage(spec), json.RawMessage(refs)
	return c, err
}

// Channels returns every channel, by scope and key.
func (s *Store) Channels(ctx context.Context) ([]Channel, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+channelColumns+`
		FROM channels c JOIN scopes x ON x.id = c.scope_id ORDER BY x.path, c.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) Channel(ctx context.Context, id int64) (Channel, error) {
	return scanChannel(s.queryRow(ctx, `SELECT `+channelColumns+`
		FROM channels c JOIN scopes x ON x.id = c.scope_id WHERE c.id = ?`, id))
}

// channelPath names a channel in the history.
func channelPath(scopePath, slug string) string {
	if scopePath == "" {
		return slug
	}
	return scopePath + "/" + slug
}

// CreateChannel adds a channel to a scope. The history keeps what it is,
// but of its secrets only which are set.
func (s *Store) CreateChannel(ctx context.Context, actor Actor, c Channel) (Channel, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		scope, err := scanScope(tx.QueryRowContext(ctx, s.rebind(`SELECT `+scopeColumns+` FROM scopes WHERE id = ?`), c.ScopeID))
		if err != nil {
			return err
		}
		var id int64
		err = tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO channels (scope_id, slug, name, type, spec_json, secret_sealed, secret_refs_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (scope_id, slug) DO NOTHING RETURNING id`),
			c.ScopeID, c.Slug, c.Name, c.Type, string(c.Spec), c.Secrets, string(defaultRefs(c.SecretRefs)), c.CreatedAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		c.ID, c.ScopePath, c.SecretRefs = id, scope.Path, defaultRefs(c.SecretRefs)
		return s.recorder(tx, actor).record(ctx, "channel", id, channelPath(scope.Path, c.Slug), "created", nil,
			map[string]any{"name": c.Name, "type": c.Type, "spec": c.Spec, "secrets": c.SecretRefs})
	})
	return c, err
}

// UpdateChannel changes a channel's name, spec and secrets.
func (s *Store) UpdateChannel(ctx context.Context, actor Actor, c Channel) (Channel, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		old, err := scanChannel(tx.QueryRowContext(ctx, s.rebind(`SELECT `+channelColumns+`
			FROM channels c JOIN scopes x ON x.id = c.scope_id WHERE c.id = ?`), c.ID))
		if err != nil {
			return err
		}
		c.SecretRefs = defaultRefs(c.SecretRefs)
		if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE channels SET name = ?, spec_json = ?, secret_sealed = ?, secret_refs_json = ?
			WHERE id = ?`), c.Name, string(c.Spec), c.Secrets, string(c.SecretRefs), c.ID); err != nil {
			return err
		}
		c.ScopeID, c.ScopePath, c.Slug, c.Type, c.CreatedAt = old.ScopeID, old.ScopePath, old.Slug, old.Type, old.CreatedAt
		r, path := s.recorder(tx, actor), channelPath(old.ScopePath, old.Slug)
		if old.Name != c.Name {
			if err := r.record(ctx, "channel", c.ID, path, "name", old.Name, c.Name); err != nil {
				return err
			}
		}
		if string(old.Spec) != string(c.Spec) {
			if err := r.record(ctx, "channel", c.ID, path, "spec", old.Spec, c.Spec); err != nil {
				return err
			}
		}
		if string(old.SecretRefs) != string(c.SecretRefs) || !ptrEqual(old.Secrets, c.Secrets) {
			// Which secrets are set, never what they are.
			return r.record(ctx, "channel", c.ID, path, "secrets", old.SecretRefs, c.SecretRefs)
		}
		return nil
	})
	return c, err
}

func (s *Store) DeleteChannel(ctx context.Context, actor Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		c, err := scanChannel(tx.QueryRowContext(ctx, s.rebind(`SELECT `+channelColumns+`
			FROM channels c JOIN scopes x ON x.id = c.scope_id WHERE c.id = ?`), id))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM channels WHERE id = ?`), id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "channel", id, channelPath(c.ScopePath, c.Slug), "deleted",
			map[string]any{"name": c.Name, "type": c.Type}, nil)
	})
}

func defaultRefs(refs json.RawMessage) json.RawMessage {
	if len(refs) == 0 || string(refs) == "null" {
		return json.RawMessage(`{}`)
	}
	return refs
}

// MailServer is the SMTP server email channels send through, unless they
// name their own.
type MailServer struct {
	Spec       json.RawMessage
	Secrets    *string
	SecretRefs json.RawMessage
	UpdatedAt  int64
}

// MailServer returns the mail server, or nil if there is none.
func (s *Store) MailServer(ctx context.Context) (*MailServer, error) {
	var m MailServer
	var spec, refs string
	err := s.queryRow(ctx, `SELECT spec_json, secret_sealed, secret_refs_json, updated_at FROM mail_server WHERE id = 1`).
		Scan(&spec, &m.Secrets, &refs, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	m.Spec, m.SecretRefs = json.RawMessage(spec), json.RawMessage(refs)
	return &m, err
}

// SetMailServer saves the mail server, or removes it with nil. The history
// is the global scope's.
func (s *Store) SetMailServer(ctx context.Context, actor Actor, m *MailServer) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var oldSpec, oldRefs *string
		err := tx.QueryRowContext(ctx, `SELECT spec_json, secret_refs_json FROM mail_server WHERE id = 1`).Scan(&oldSpec, &oldRefs)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var before, after any
		if oldSpec != nil {
			before = map[string]json.RawMessage{"spec": json.RawMessage(*oldSpec), "secrets": json.RawMessage(*oldRefs)}
		}
		if m == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM mail_server WHERE id = 1`)
		} else {
			m.SecretRefs = defaultRefs(m.SecretRefs)
			after = map[string]json.RawMessage{"spec": m.Spec, "secrets": m.SecretRefs}
			_, err = tx.ExecContext(ctx, s.rebind(`
				INSERT INTO mail_server (id, spec_json, secret_sealed, secret_refs_json, updated_at) VALUES (1, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET spec_json = excluded.spec_json, secret_sealed = excluded.secret_sealed,
					secret_refs_json = excluded.secret_refs_json, updated_at = excluded.updated_at`),
				string(m.Spec), m.Secrets, string(m.SecretRefs), m.UpdatedAt)
		}
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "global", GlobalScope, "", "mail_server", before, after)
	})
}
