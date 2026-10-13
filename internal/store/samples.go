package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// HostSample is one reading of a host. Nil means not measured.
type HostSample struct {
	HostID       int64
	TS           int64
	CPU          *float64
	IOWait       *float64
	Steal        *float64
	Load1        *float64
	Load5        *float64
	Load15       *float64
	MemUsed      *int64
	MemAvailable *int64
	SwapUsed     *int64
	SwapTotal    *int64
	NetRx        *float64
	NetTx        *float64
	DiskRead     *float64
	DiskWrite    *float64
	Uptime       *int64
}

// FSSample is one reading of a filesystem, which is saved along with it.
type FSSample struct {
	HostID      int64
	Mount       string
	Device      string
	FSType      string
	TS          int64
	Used        int64
	Total       int64
	InodesUsed  *int64
	InodesTotal *int64
}

// ContainerSample is one reading of a container.
type ContainerSample struct {
	ContainerID int64
	TS          int64
	CPU         *float64
	Mem         *int64
	MemLimit    *int64
	NetRx       *float64
	NetTx       *float64
}

// Event is something that happened to an object.
type Event struct {
	ID          int64  `json:"id"`
	TS          int64  `json:"ts"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   int64  `json:"subject_id"`
	Kind        string `json:"kind"`
	Data        string `json:"data"`
}

// Batch is what the hub writes at once: everything that arrived in about a
// second, in one transaction.
type Batch struct {
	HostSamples      []HostSample
	FSSamples        []FSSample
	ContainerSamples []ContainerSample
	Events           []Event
	CheckResults     []CheckResult
	// Seen is when each host was last heard from, by host ID.
	Seen map[int64]int64
}

func (b *Batch) Empty() bool {
	return len(b.HostSamples)+len(b.FSSamples)+len(b.ContainerSamples)+len(b.Events)+len(b.Seen)+len(b.CheckResults) == 0
}

// Write saves a batch. A sample that is already there, sent again after a
// reconnect, is skipped.
func (s *Store) Write(ctx context.Context, b Batch) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, x := range b.HostSamples {
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO host_samples (host_id, ts, cpu, iowait, steal, load1, load5, load15,
					mem_used, mem_available, swap_used, swap_total, net_rx, net_tx,
					disk_read, disk_write, uptime)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT DO NOTHING`),
				x.HostID, x.TS, x.CPU, x.IOWait, x.Steal, x.Load1, x.Load5, x.Load15,
				x.MemUsed, x.MemAvailable, x.SwapUsed, x.SwapTotal, x.NetRx, x.NetTx,
				x.DiskRead, x.DiskWrite, x.Uptime); err != nil {
				return fmt.Errorf("host sample: %w", err)
			}
		}
		for _, x := range b.FSSamples {
			var fsID int64
			if err := tx.QueryRowContext(ctx, s.rebind(`
				INSERT INTO filesystems (host_id, mount, device, fstype, last_seen)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (host_id, mount) DO UPDATE SET
					device = excluded.device, fstype = excluded.fstype, last_seen = excluded.last_seen
				RETURNING id`),
				x.HostID, x.Mount, x.Device, x.FSType, x.TS).Scan(&fsID); err != nil {
				return fmt.Errorf("filesystem: %w", err)
			}
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO fs_samples (filesystem_id, ts, used, total, inodes_used, inodes_total)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT DO NOTHING`),
				fsID, x.TS, x.Used, x.Total, x.InodesUsed, x.InodesTotal); err != nil {
				return fmt.Errorf("filesystem sample: %w", err)
			}
		}
		for _, x := range b.ContainerSamples {
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO container_samples (container_id, ts, cpu, mem, mem_limit, net_rx, net_tx)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT DO NOTHING`),
				x.ContainerID, x.TS, x.CPU, x.Mem, x.MemLimit, x.NetRx, x.NetTx); err != nil {
				return fmt.Errorf("container sample: %w", err)
			}
		}
		for _, e := range b.Events {
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO events (ts, subject_kind, subject_id, kind, data_json) VALUES (?, ?, ?, ?, ?)`),
				e.TS, e.SubjectKind, e.SubjectID, e.Kind, defaultJSON(e.Data)); err != nil {
				return fmt.Errorf("event: %w", err)
			}
		}
		for _, x := range b.CheckResults {
			ok := 0
			if x.OK {
				ok = 1
			}
			var errText *string
			if x.Error != "" {
				errText = &x.Error
			}
			if _, err := tx.ExecContext(ctx, s.rebind(`
				INSERT INTO check_results (check_id, probe_id, ts, ok, latency_ms, status_code, cert_not_after, error)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT DO NOTHING`),
				x.CheckID, x.ProbeID, x.TS, ok, x.LatencyMS, x.StatusCode, x.CertNotAfter, errText); err != nil {
				return fmt.Errorf("check result: %w", err)
			}
		}
		for id, ts := range b.Seen {
			if _, err := tx.ExecContext(ctx, s.rebind(`
				UPDATE hosts SET last_seen = ? WHERE id = ? AND last_seen < ?`), ts, id, ts); err != nil {
				return fmt.Errorf("host seen: %w", err)
			}
		}
		return nil
	})
}

// Series is a time series in columns, as charts take it: Time holds the
// start of each step in Unix seconds, and each column one value per step,
// nil where nothing was measured.
type Series struct {
	Time    []int64               `json:"time"`
	Columns map[string][]*float64 `json:"columns"`
}

// What each kind of series reads: its table, the column that selects the
// object, and the columns it offers.
var seriesKinds = map[string]struct {
	table, key string
	columns    []string
}{
	"host": {"host_samples", "host_id", []string{"cpu", "iowait", "steal", "load1", "load5",
		"load15", "mem_used", "mem_available", "swap_used", "swap_total", "net_rx", "net_tx",
		"disk_read", "disk_write"}},
	"filesystem": {"fs_samples", "filesystem_id", []string{"used", "total", "inodes_used", "inodes_total"}},
	"container":  {"container_samples", "container_id", []string{"cpu", "mem", "mem_limit", "net_rx", "net_tx"}},
	// ok averages to the share of runs that passed.
	"check": {"check_results", "check_id", []string{"latency_ms", "ok"}},
}

// Series returns an object's samples from from to to (Unix seconds), as
// averages over steps of step seconds.
func (s *Store) Series(ctx context.Context, kind string, id, from, to, step int64) (Series, error) {
	k, ok := seriesKinds[kind]
	if !ok {
		return Series{}, fmt.Errorf("no series of kind %q", kind)
	}
	step = max(step, 1)
	avgs := make([]string, len(k.columns))
	for i, c := range k.columns {
		avgs[i] = "avg(" + c + ")"
	}
	// Both databases divide integers without a remainder.
	q := `SELECT (ts / ?) * ? AS t, ` + strings.Join(avgs, ", ") + `
		FROM ` + k.table + ` WHERE ` + k.key + ` = ? AND ts >= ? AND ts <= ?
		GROUP BY t ORDER BY t`
	rows, err := s.read.QueryContext(ctx, s.rebind(q), step, step, id, from, to)
	if err != nil {
		return Series{}, err
	}
	defer rows.Close()
	out := Series{Columns: make(map[string][]*float64, len(k.columns))}
	for _, c := range k.columns {
		out.Columns[c] = []*float64{}
	}
	out.Time = []int64{}
	dest := make([]any, len(k.columns)+1)
	values := make([]sql.NullFloat64, len(k.columns))
	var t int64
	dest[0] = &t
	for i := range values {
		dest[i+1] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return Series{}, err
		}
		out.Time = append(out.Time, t)
		for i, c := range k.columns {
			var v *float64
			if values[i].Valid {
				f := values[i].Float64
				v = &f
			}
			out.Columns[c] = append(out.Columns[c], v)
		}
	}
	return out, rows.Err()
}

// Events returns what happened to an object from from to to, oldest first.
func (s *Store) Events(ctx context.Context, kind string, id, from, to int64) ([]Event, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT id, ts, subject_kind, subject_id, kind, data_json FROM events
		WHERE subject_kind = ? AND subject_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts, id`), kind, id, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TS, &e.SubjectKind, &e.SubjectID, &e.Kind, &e.Data); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// DeleteSamplesBefore deletes samples older than before, object by object
// so that no single statement holds the database for long. It returns how
// many it deleted.
func (s *Store) DeleteSamplesBefore(ctx context.Context, before int64) (int64, error) {
	var total int64
	for _, k := range []struct{ table, key, parent string }{
		{"host_samples", "host_id", "hosts"},
		{"fs_samples", "filesystem_id", "filesystems"},
		{"container_samples", "container_id", "containers"},
		{"check_results", "check_id", "checks"},
	} {
		ids, err := s.ids(ctx, `SELECT id FROM `+k.parent)
		if err != nil {
			return total, err
		}
		for _, id := range ids {
			res, err := s.exec(ctx, `DELETE FROM `+k.table+` WHERE `+k.key+` = ? AND ts < ?`, id, before)
			if err != nil {
				return total, err
			}
			n, _ := res.RowsAffected()
			total += n
		}
	}
	return total, nil
}

func (s *Store) ids(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func defaultJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
