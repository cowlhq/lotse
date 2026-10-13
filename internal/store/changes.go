package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Actor is who changes something, and how: through the UI, apply, a
// container's labels, the API, setup, or lotse itself.
type Actor struct {
	Kind string // user, token, label, system
	ID   *int64
	Name string
	Via  string // ui, apply, label, api, setup, system
}

// Change is one changed field of one object.
type Change struct {
	ID          int64           `json:"id"`
	At          int64           `json:"at"`
	BatchID     string          `json:"batch_id"`
	ActorKind   string          `json:"actor_kind"`
	ActorID     *int64          `json:"actor_id"`
	ActorName   string          `json:"actor_name"`
	Via         string          `json:"via"`
	SubjectKind string          `json:"subject_kind"`
	SubjectID   int64           `json:"subject_id"`
	SubjectPath string          `json:"subject_path"`
	Field       string          `json:"field"`
	Old         json.RawMessage `json:"old"`
	New         json.RawMessage `json:"new"`
}

// recorder writes the history of one batch of changes, in the transaction
// that makes them.
type recorder struct {
	s     *Store
	tx    *sql.Tx
	actor Actor
	batch string
	at    int64
}

func (s *Store) recorder(tx *sql.Tx, actor Actor) *recorder {
	b := make([]byte, 8)
	rand.Read(b)
	return &recorder{s: s, tx: tx, actor: actor, batch: hex.EncodeToString(b), at: time.Now().Unix()}
}

// record notes that field of the subject went from old to new. nil stands
// for "not there": created objects have no old value, deleted ones no new.
func (r *recorder) record(ctx context.Context, kind string, id int64, path, field string, old, new any) error {
	var o, n *string
	for _, x := range []struct {
		v   any
		out **string
	}{{old, &o}, {new, &n}} {
		if x.v == nil {
			continue
		}
		b, err := json.Marshal(x.v)
		if err != nil {
			return err
		}
		s := string(b)
		*x.out = &s
	}
	_, err := r.tx.ExecContext(ctx, r.s.rebind(`
		INSERT INTO changes (at, batch_id, actor_kind, actor_id, actor_name, via,
			subject_kind, subject_id, subject_path, field, old_json, new_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		r.at, r.batch, r.actor.Kind, r.actor.ID, r.actor.Name, r.actor.Via,
		kind, id, path, field, o, n)
	return err
}

// Changes returns the history of an object, newest first.
func (s *Store) Changes(ctx context.Context, kind string, id int64, limit int) ([]Change, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT id, at, batch_id, actor_kind, actor_id, actor_name, via, subject_kind, subject_id,
			subject_path, field, old_json, new_json
		FROM changes WHERE subject_kind = ? AND subject_id = ?
		ORDER BY at DESC, id DESC LIMIT ?`), kind, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Change{}
	for rows.Next() {
		var c Change
		var o, n *string
		if err := rows.Scan(&c.ID, &c.At, &c.BatchID, &c.ActorKind, &c.ActorID, &c.ActorName, &c.Via,
			&c.SubjectKind, &c.SubjectID, &c.SubjectPath, &c.Field, &o, &n); err != nil {
			return nil, err
		}
		if o != nil {
			c.Old = json.RawMessage(*o)
		}
		if n != nil {
			c.New = json.RawMessage(*n)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
