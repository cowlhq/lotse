package store

import (
	"context"
	"database/sql"
	"errors"
)

// Maintenance is a window in which alerts on an object, or on everything
// in a scope, are recorded but not sent: once, from StartsAt to EndsAt, or
// by Schedule, such as "sun 02:00-04:00".
type Maintenance struct {
	ID          int64   `json:"id"`
	SubjectKind string  `json:"subject_kind"`
	SubjectID   int64   `json:"subject_id"`
	StartsAt    *int64  `json:"starts_at"`
	EndsAt      *int64  `json:"ends_at"`
	Schedule    *string `json:"schedule"`
	Reason      string  `json:"reason"`
	CreatedBy   string  `json:"created_by"`
	CreatedAt   int64   `json:"created_at"`
}

const maintenanceColumns = `id, subject_kind, subject_id, starts_at, ends_at, schedule, reason, created_by, created_at`

func scanMaintenance(row interface{ Scan(...any) error }) (Maintenance, error) {
	var m Maintenance
	err := row.Scan(&m.ID, &m.SubjectKind, &m.SubjectID, &m.StartsAt, &m.EndsAt, &m.Schedule, &m.Reason, &m.CreatedBy, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// Maintenances returns every maintenance window, the newest first.
func (s *Store) Maintenances(ctx context.Context) ([]Maintenance, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+maintenanceColumns+` FROM maintenance ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Maintenance{}
	for rows.Next() {
		m, err := scanMaintenance(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// AddMaintenance adds a window to a subject, whose path names it in the
// history.
func (s *Store) AddMaintenance(ctx context.Context, actor Actor, m Maintenance, path string) (Maintenance, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO maintenance (subject_kind, subject_id, starts_at, ends_at, schedule, reason, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`),
			m.SubjectKind, m.SubjectID, m.StartsAt, m.EndsAt, m.Schedule, m.Reason, m.CreatedBy, m.CreatedAt).Scan(&m.ID)
		if err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, m.SubjectKind, m.SubjectID, path, "maintenance", nil, maintenanceRecord(m))
	})
	return m, err
}

func (s *Store) Maintenance(ctx context.Context, id int64) (Maintenance, error) {
	return scanMaintenance(s.queryRow(ctx, `SELECT `+maintenanceColumns+` FROM maintenance WHERE id = ?`, id))
}

// DeleteMaintenance ends a window early, or removes one that is to come.
// path names its subject in the history.
func (s *Store) DeleteMaintenance(ctx context.Context, actor Actor, id int64, path string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		m, err := scanMaintenance(tx.QueryRowContext(ctx, s.rebind(`SELECT `+maintenanceColumns+` FROM maintenance WHERE id = ?`), id))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM maintenance WHERE id = ?`), id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, m.SubjectKind, m.SubjectID, path, "maintenance", maintenanceRecord(m), nil)
	})
}

func maintenanceRecord(m Maintenance) map[string]any {
	out := map[string]any{"reason": m.Reason}
	if m.Schedule != nil {
		out["schedule"] = *m.Schedule
	} else {
		out["starts_at"], out["ends_at"] = *m.StartsAt, *m.EndsAt
	}
	return out
}
