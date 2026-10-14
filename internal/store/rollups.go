package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Readings over time (milestone 9): raw readings are kept for a while,
// then only their averages and maxima for every 5 minutes, and later for
// every hour. Rollup makes these steps; the retention deletes what is past
// its time, step by step.

// Resolutions of readings: Raw as they came, every 5 minutes, every hour.
const (
	Raw         = ""
	FiveMinutes = "5m"
	Hour        = "1h"
)

// Resolutions are the kept steps, finest first.
var Resolutions = []string{Raw, FiveMinutes, Hour}

// ResolutionSeconds is how long a step of a resolution is.
var ResolutionSeconds = map[string]int64{Raw: 1, FiveMinutes: 300, Hour: 3600}

// rollupChunk bounds one transaction of a rollup, so that it never holds
// the database for long.
const rollupChunk = 6 * 3600

func table(k seriesKind, res string) string {
	if res == Raw {
		return k.table
	}
	return k.table + "_" + res
}

// Rollup makes the 5-minute steps of readings older than until, and the
// hourly steps of the 5-minute ones.
func (s *Store) Rollup(ctx context.Context, until int64) error {
	for _, kind := range seriesOrder {
		k := seriesKinds[kind]
		if err := s.rollup(ctx, k, FiveMinutes, until/300*300); err != nil {
			return fmt.Errorf("%s every 5 minutes: %w", kind, err)
		}
		made, _, err := s.watermark(ctx, table(k, FiveMinutes))
		if err != nil {
			return err
		}
		if err := s.rollup(ctx, k, Hour, made/3600*3600); err != nil {
			return fmt.Errorf("%s every hour: %w", kind, err)
		}
	}
	return nil
}

// Resolution is the finest resolution that holds an object's readings
// from from on, as far as there are any: raw readings are deleted sooner
// than averages, and those of 5 minutes sooner than the hourly ones. The
// oldest of each is compared by the hour, which the averages are rounded
// to.
func (s *Store) Resolution(ctx context.Context, kind string, id, from int64) (string, error) {
	k, ok := seriesKinds[kind]
	if !ok {
		return "", fmt.Errorf("no series of kind %q", kind)
	}
	// start is where the readings begin: from, or the oldest there is.
	oldest := make([]sql.NullInt64, len(Resolutions))
	start, found := from, false
	for i, res := range Resolutions {
		if err := s.queryRow(ctx, `SELECT min(ts) FROM `+table(k, res)+` WHERE `+k.key+` = ?`, id).Scan(&oldest[i]); err != nil {
			return "", err
		}
		if oldest[i].Valid && (!found || oldest[i].Int64 < start) {
			start, found = oldest[i].Int64, true
		}
	}
	start = max(start, from)
	for i, res := range Resolutions {
		if oldest[i].Valid && oldest[i].Int64/3600 <= start/3600 {
			return res, nil
		}
	}
	return Raw, nil
}

// watermark is how far a rollup table is made.
func (s *Store) watermark(ctx context.Context, name string) (int64, bool, error) {
	var upto int64
	err := s.queryRow(ctx, `SELECT upto FROM rollups WHERE name = ?`, name).Scan(&upto)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return upto, err == nil, err
}

func (s *Store) rollup(ctx context.Context, k seriesKind, res string, end int64) error {
	step := ResolutionSeconds[res]
	source := k.table
	if res == Hour {
		source = table(k, FiveMinutes)
	}
	target := table(k, res)
	start, ok, err := s.watermark(ctx, target)
	if err != nil {
		return err
	}
	if !ok {
		// The first time: from the oldest reading there is.
		var oldest sql.NullInt64
		if err := s.queryRow(ctx, `SELECT min(ts) FROM `+source).Scan(&oldest); err != nil {
			return err
		}
		if !oldest.Valid {
			return nil
		}
		start = oldest.Int64 / step * step
	}
	cols := []string{k.key, "ts"}
	sel := []string{k.key, fmt.Sprintf("(ts / %d) * %d", step, step)}
	for _, c := range k.columns {
		cols = append(cols, c, c+"_max")
		if res == Hour {
			sel = append(sel, "avg("+c+")", "max("+c+"_max)")
		} else {
			sel = append(sel, "avg("+c+")", "max("+c+")")
		}
	}
	cols = append(cols, "n")
	if res == Hour {
		sel = append(sel, "sum(n)")
	} else {
		sel = append(sel, "count(*)")
	}
	insert := `INSERT INTO ` + target + ` (` + strings.Join(cols, ", ") + `)
		SELECT ` + strings.Join(sel, ", ") + ` FROM ` + source + `
		WHERE ts >= ? AND ts < ?
		GROUP BY ` + k.key + `, (ts / ` + fmt.Sprint(step) + `) * ` + fmt.Sprint(step) + `
		ON CONFLICT (` + k.key + `, ts) DO NOTHING`
	for from := start; from < end; {
		to := min(from+rollupChunk, end)
		err := s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, s.rebind(insert), from, to); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, s.rebind(`INSERT INTO rollups (name, upto) VALUES (?, ?)
				ON CONFLICT (name) DO UPDATE SET upto = excluded.upto`), target, to)
			return err
		})
		if err != nil {
			return err
		}
		from = to
	}
	return nil
}

// DeleteBefore deletes readings of a resolution older than before, object
// by object, so that no statement holds the database for long. It returns
// how many it deleted.
func (s *Store) DeleteBefore(ctx context.Context, res string, before int64) (int64, error) {
	var total int64
	for _, kind := range seriesOrder {
		k := seriesKinds[kind]
		ids, err := s.ids(ctx, `SELECT id FROM `+k.parent)
		if err != nil {
			return total, err
		}
		for _, id := range ids {
			r, err := s.exec(ctx, `DELETE FROM `+table(k, res)+` WHERE `+k.key+` = ? AND ts < ?`, id, before)
			if err != nil {
				return total, err
			}
			n, _ := r.RowsAffected()
			total += n
		}
	}
	return total, nil
}

// DeleteHistoryBefore deletes events and alerts (with what was sent about
// them) older than before. The history of changes is kept.
func (s *Store) DeleteHistoryBefore(ctx context.Context, before int64) (int64, error) {
	var total int64
	for _, q := range []string{`DELETE FROM events WHERE ts < ?`, `DELETE FROM alerts WHERE at < ?
		AND id NOT IN (SELECT alert_id FROM alert_states WHERE alert_id IS NOT NULL)`} {
		r, err := s.exec(ctx, q, before)
		if err != nil {
			return total, err
		}
		n, _ := r.RowsAffected()
		total += n
	}
	return total, nil
}
