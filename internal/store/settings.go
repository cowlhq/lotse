package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Subject is an object or scope that settings are set on.
type Subject struct {
	Kind string
	ID   int64
	// Path names it in the history.
	Path string
}

// Settings returns what is set on each subject, by key.
func (s *Store) Settings(ctx context.Context, subjects []Subject) ([]map[string]json.RawMessage, error) {
	out := make([]map[string]json.RawMessage, len(subjects))
	for i, sub := range subjects {
		rows, err := s.read.QueryContext(ctx, s.rebind(
			`SELECT key, value_json FROM settings WHERE subject_kind = ? AND subject_id = ?`), sub.Kind, sub.ID)
		if err != nil {
			return nil, err
		}
		out[i] = map[string]json.RawMessage{}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				rows.Close()
				return nil, err
			}
			out[i][k] = json.RawMessage(v)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetSetting sets key on a subject, or unsets it with a nil value.
func (s *Store) SetSetting(ctx context.Context, actor Actor, sub Subject, key string, value json.RawMessage, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var old *string
		err := tx.QueryRowContext(ctx, s.rebind(`SELECT value_json FROM settings
			WHERE subject_kind = ? AND subject_id = ? AND key = ?`), sub.Kind, sub.ID, key).Scan(&old)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var before, after any
		if old != nil {
			before = json.RawMessage(*old)
		}
		if value == nil {
			if old == nil {
				return nil
			}
			_, err = tx.ExecContext(ctx, s.rebind(`DELETE FROM settings
				WHERE subject_kind = ? AND subject_id = ? AND key = ?`), sub.Kind, sub.ID, key)
		} else {
			if old != nil && *old == string(value) {
				return nil
			}
			after = value
			_, err = tx.ExecContext(ctx, s.rebind(`
				INSERT INTO settings (subject_kind, subject_id, key, value_json, updated_at) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (subject_kind, subject_id, key) DO UPDATE SET
					value_json = excluded.value_json, updated_at = excluded.updated_at`),
				sub.Kind, sub.ID, key, string(value), now)
		}
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, sub.Kind, sub.ID, sub.Path, key, before, after)
	})
}
