package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// AlertKey names where one rule stands on one object: instance tells apart
// several alerts of a rule on it, such as two full filesystems.
type AlertKey struct {
	Rule        string `json:"rule"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   int64  `json:"subject_id"`
	Instance    string `json:"instance"`
}

// AlertState is where a rule stands on an object (design 8.1).
type AlertState struct {
	AlertKey
	State           string // ok, warn, crit, unknown
	Since           int64
	Pending         *string // the state it is held at before it fires
	PendingSince    *int64
	RecoveringSince *int64
	// Previous is the state before UNKNOWN.
	Previous *string
	Value    *float64
	// AlertID is the transition into the state.
	AlertID *int64
	// NotifiedState is the state last sent, at NotifiedAt.
	NotifiedState string
	NotifiedAt    *int64
	AckedBy       *string
	AckedAt       *int64
}

const alertStateColumns = `rule, subject_kind, subject_id, instance, state, since, pending, pending_since,
	recovering_since, previous, value, alert_id, notified_state, notified_at, acked_by, acked_at`

func scanAlertState(row interface{ Scan(...any) error }) (AlertState, error) {
	var a AlertState
	err := row.Scan(&a.Rule, &a.SubjectKind, &a.SubjectID, &a.Instance, &a.State, &a.Since, &a.Pending,
		&a.PendingSince, &a.RecoveringSince, &a.Previous, &a.Value, &a.AlertID, &a.NotifiedState, &a.NotifiedAt,
		&a.AckedBy, &a.AckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// AlertStates returns where every rule stands that is not plainly OK.
func (s *Store) AlertStates(ctx context.Context) ([]AlertState, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+alertStateColumns+` FROM alert_states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []AlertState{}
	for rows.Next() {
		a, err := scanAlertState(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// Alert is a transition of a rule on an object, with what it was measured
// against: the value, the threshold it crossed, and the effective rule.
type Alert struct {
	ID int64
	AlertKey
	SubjectPath string
	From, To    string
	At          int64
	Value       *float64
	Threshold   *float64
	Reason      string
	// Effective is the rule as it applied, with where each of its settings
	// comes from, as JSON.
	Effective     json.RawMessage
	InMaintenance bool
}

const alertColumns = `id, rule, subject_kind, subject_id, instance, subject_path, from_state, to_state,
	at, value, threshold, reason, rule_json, in_maintenance`

func scanAlert(row interface{ Scan(...any) error }) (Alert, error) {
	var a Alert
	var effective string
	var maintenance int
	err := row.Scan(&a.ID, &a.Rule, &a.SubjectKind, &a.SubjectID, &a.Instance, &a.SubjectPath, &a.From, &a.To,
		&a.At, &a.Value, &a.Threshold, &a.Reason, &effective, &maintenance)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.Effective, a.InMaintenance = json.RawMessage(effective), maintenance != 0
	return a, err
}

// SaveAlertState saves where a rule stands, and the transition that got it
// there, if any; it returns the state with the transition's ID. A state
// that is plainly OK, and was sent as such or never sent, is forgotten.
func (s *Store) SaveAlertState(ctx context.Context, st AlertState, transition *Alert) (AlertState, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if transition != nil {
			maintenance := 0
			if transition.InMaintenance {
				maintenance = 1
			}
			if err := tx.QueryRowContext(ctx, s.rebind(`
				INSERT INTO alerts (rule, subject_kind, subject_id, instance, subject_path, from_state, to_state,
					at, value, threshold, reason, rule_json, in_maintenance)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`),
				st.Rule, st.SubjectKind, st.SubjectID, st.Instance, transition.SubjectPath, transition.From,
				transition.To, transition.At, transition.Value, transition.Threshold, transition.Reason,
				string(transition.Effective), maintenance).Scan(&transition.ID); err != nil {
				return err
			}
			st.AlertID = &transition.ID
		}
		return s.saveAlertState(ctx, tx, st)
	})
	return st, err
}

func (s *Store) saveAlertState(ctx context.Context, tx *sql.Tx, st AlertState) error {
	if st.State == "ok" && st.Pending == nil && st.RecoveringSince == nil && st.NotifiedState == "ok" {
		_, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM alert_states
			WHERE rule = ? AND subject_kind = ? AND subject_id = ? AND instance = ?`),
			st.Rule, st.SubjectKind, st.SubjectID, st.Instance)
		return err
	}
	_, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO alert_states (`+alertStateColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (rule, subject_kind, subject_id, instance) DO UPDATE SET
			state = excluded.state, since = excluded.since, pending = excluded.pending,
			pending_since = excluded.pending_since, recovering_since = excluded.recovering_since,
			previous = excluded.previous, value = excluded.value, alert_id = excluded.alert_id, notified_state = excluded.notified_state,
			notified_at = excluded.notified_at, acked_by = excluded.acked_by, acked_at = excluded.acked_at`),
		st.Rule, st.SubjectKind, st.SubjectID, st.Instance, st.State, st.Since, st.Pending, st.PendingSince,
		st.RecoveringSince, st.Previous, st.Value, st.AlertID, st.NotifiedState, st.NotifiedAt, st.AckedBy, st.AckedAt)
	return err
}

// Notification is a message about an alert to one channel, or one held
// back during maintenance.
type Notification struct {
	ID        int64
	AlertID   int64
	ChannelID *int64
	// Channel is the channel's path when it was made, which outlives it.
	Channel   string
	Kind      string // change, repeat
	Status    string // pending, sent, failed, suppressed
	Attempts  int
	Error     string
	CreatedAt int64
	NextAt    *int64
	SentAt    *int64
}

const notificationColumns = `id, alert_id, channel_id, channel, kind, status, attempts, error, created_at, next_at, sent_at`

func scanNotification(row interface{ Scan(...any) error }) (Notification, error) {
	var n Notification
	var errText *string
	err := row.Scan(&n.ID, &n.AlertID, &n.ChannelID, &n.Channel, &n.Kind, &n.Status, &n.Attempts, &errText,
		&n.CreatedAt, &n.NextAt, &n.SentAt)
	if errText != nil {
		n.Error = *errText
	}
	return n, err
}

// Notify records that a state was told, or held back: it saves the state,
// with its notified state, and the notifications for it.
func (s *Store) Notify(ctx context.Context, st AlertState, list []Notification) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, n := range list {
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO notifications (alert_id, channel_id, channel, kind, status, created_at, next_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`),
				n.AlertID, n.ChannelID, n.Channel, n.Kind, n.Status, n.CreatedAt, n.NextAt); err != nil {
				return err
			}
		}
		return s.saveAlertState(ctx, tx, st)
	})
}

// PendingNotifications returns the notifications to send by now, with
// their alerts, oldest first.
func (s *Store) PendingNotifications(ctx context.Context, now int64) ([]Notification, map[int64]Alert, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`SELECT `+notificationColumns+` FROM notifications
		WHERE status = 'pending' AND next_at <= ? ORDER BY created_at, id`), now)
	if err != nil {
		return nil, nil, err
	}
	var list []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		list = append(list, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	alerts := map[int64]Alert{}
	for _, n := range list {
		if _, ok := alerts[n.AlertID]; ok {
			continue
		}
		a, err := s.Alert(ctx, n.AlertID)
		if err != nil {
			return nil, nil, err
		}
		alerts[a.ID] = a
	}
	return list, alerts, nil
}

// FinishNotifications records how sending some notifications went.
func (s *Store) FinishNotifications(ctx context.Context, list []Notification) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, n := range list {
			var errText *string
			if n.Error != "" {
				errText = &n.Error
			}
			if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE notifications SET status = ?, attempts = ?, error = ?,
				next_at = ?, sent_at = ? WHERE id = ?`), n.Status, n.Attempts, errText, n.NextAt, n.SentAt, n.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Acknowledge notes that someone looks after a firing alert, which stops
// it being sent again. It is in the history of the alert's object.
func (s *Store) Acknowledge(ctx context.Context, actor Actor, key AlertKey, path string, now int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		st, err := scanAlertState(tx.QueryRowContext(ctx, s.rebind(`SELECT `+alertStateColumns+` FROM alert_states
			WHERE rule = ? AND subject_kind = ? AND subject_id = ? AND instance = ?`),
			key.Rule, key.SubjectKind, key.SubjectID, key.Instance))
		if err != nil {
			return err
		}
		if st.State != "warn" && st.State != "crit" || st.AckedAt != nil {
			return ErrNotFound
		}
		st.AckedBy, st.AckedAt = &actor.Name, &now
		if err := s.saveAlertState(ctx, tx, st); err != nil {
			return err
		}
		field := "alert " + key.Rule
		if key.Instance != "" {
			field += " " + key.Instance
		}
		return s.recorder(tx, actor).record(ctx, key.SubjectKind, key.SubjectID, path, field, nil, "acknowledged")
	})
}

func (s *Store) Alert(ctx context.Context, id int64) (Alert, error) {
	return scanAlert(s.queryRow(ctx, `SELECT `+alertColumns+` FROM alerts WHERE id = ?`, id))
}

// AlertFilter selects transitions: of one object, or of all; between From
// and To.
type AlertFilter struct {
	SubjectKind string
	SubjectID   int64
	From, To    int64
	Limit       int
}

// Alerts returns transitions, the newest first.
func (s *Store) Alerts(ctx context.Context, f AlertFilter) ([]Alert, error) {
	where, args := []string{"at >= ?", "at <= ?"}, []any{f.From, f.To}
	if f.SubjectKind != "" {
		where, args = append(where, "subject_kind = ?", "subject_id = ?"), append(args, f.SubjectKind, f.SubjectID)
	}
	rows, err := s.read.QueryContext(ctx, s.rebind(`SELECT `+alertColumns+` FROM alerts WHERE `+
		strings.Join(where, " AND ")+` ORDER BY at DESC, id DESC LIMIT ?`), append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// Notifications returns what was sent about an alert.
func (s *Store) Notifications(ctx context.Context, alertID int64) ([]Notification, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`SELECT `+notificationColumns+` FROM notifications
		WHERE alert_id = ? ORDER BY created_at, id`), alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}
