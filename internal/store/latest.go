package store

import (
	"context"
	"database/sql"
	"errors"
)

// LatestHostSample returns a host's newest reading, or nil if it has none.
func (s *Store) LatestHostSample(ctx context.Context, hostID int64) (*HostSample, error) {
	var x HostSample
	err := s.queryRow(ctx, `
		SELECT host_id, ts, cpu, iowait, steal, load1, load5, load15, mem_used, mem_available,
			swap_used, swap_total, net_rx, net_tx, disk_read, disk_write, uptime
		FROM host_samples WHERE host_id = ? ORDER BY ts DESC LIMIT 1`, hostID).Scan(
		&x.HostID, &x.TS, &x.CPU, &x.IOWait, &x.Steal, &x.Load1, &x.Load5, &x.Load15,
		&x.MemUsed, &x.MemAvailable, &x.SwapUsed, &x.SwapTotal, &x.NetRx, &x.NetTx,
		&x.DiskRead, &x.DiskWrite, &x.Uptime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}

// Filesystem is a host's filesystem with its newest usage.
type Filesystem struct {
	ID       int64
	Mount    string
	Device   string
	FSType   string
	LastSeen int64
	Used     *int64
	Total    *int64
}

// Filesystems returns a host's filesystems, by mount point.
func (s *Store) Filesystems(ctx context.Context, hostID int64) ([]Filesystem, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT f.id, f.mount, f.device, f.fstype, f.last_seen, x.used, x.total
		FROM filesystems f
		LEFT JOIN fs_samples x ON x.filesystem_id = f.id
			AND x.ts = (SELECT max(ts) FROM fs_samples WHERE filesystem_id = f.id)
		WHERE f.host_id = ? ORDER BY f.mount`), hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Filesystem{}
	for rows.Next() {
		var f Filesystem
		if err := rows.Scan(&f.ID, &f.Mount, &f.Device, &f.FSType, &f.LastSeen, &f.Used, &f.Total); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// LatestContainerSamples returns the newest reading of each of a host's
// containers that has one, by container ID.
func (s *Store) LatestContainerSamples(ctx context.Context, hostID int64) (map[int64]ContainerSample, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`
		SELECT x.container_id, x.ts, x.cpu, x.mem, x.mem_limit, x.net_rx, x.net_tx
		FROM containers c
		JOIN container_samples x ON x.container_id = c.id
			AND x.ts = (SELECT max(ts) FROM container_samples WHERE container_id = c.id)
		WHERE c.host_id = ?`), hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[int64]ContainerSample{}
	for rows.Next() {
		var x ContainerSample
		if err := rows.Scan(&x.ContainerID, &x.TS, &x.CPU, &x.Mem, &x.MemLimit, &x.NetRx, &x.NetTx); err != nil {
			return nil, err
		}
		latest[x.ContainerID] = x
	}
	return latest, rows.Err()
}

// LatestHostSamples returns every host's newest reading, by host ID.
func (s *Store) LatestHostSamples(ctx context.Context) (map[int64]HostSample, error) {
	rows, err := s.read.QueryContext(ctx, `
		SELECT x.host_id, x.ts, x.cpu, x.iowait, x.steal, x.load1, x.load5, x.load15, x.mem_used, x.mem_available,
			x.swap_used, x.swap_total, x.net_rx, x.net_tx, x.disk_read, x.disk_write, x.uptime
		FROM hosts h
		JOIN host_samples x ON x.host_id = h.id AND x.ts = (SELECT max(ts) FROM host_samples WHERE host_id = h.id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[int64]HostSample{}
	for rows.Next() {
		var x HostSample
		if err := rows.Scan(&x.HostID, &x.TS, &x.CPU, &x.IOWait, &x.Steal, &x.Load1, &x.Load5, &x.Load15,
			&x.MemUsed, &x.MemAvailable, &x.SwapUsed, &x.SwapTotal, &x.NetRx, &x.NetTx,
			&x.DiskRead, &x.DiskWrite, &x.Uptime); err != nil {
			return nil, err
		}
		latest[x.HostID] = x
	}
	return latest, rows.Err()
}

// HostFilesystem is a filesystem of some host, with its newest usage.
type HostFilesystem struct {
	HostID int64
	Filesystem
	// TS is when the usage was read.
	TS *int64
}

// AllFilesystems returns the filesystems of every host.
func (s *Store) AllFilesystems(ctx context.Context) ([]HostFilesystem, error) {
	rows, err := s.read.QueryContext(ctx, `
		SELECT f.host_id, f.id, f.mount, f.device, f.fstype, f.last_seen, x.used, x.total, x.ts
		FROM filesystems f
		LEFT JOIN fs_samples x ON x.filesystem_id = f.id
			AND x.ts = (SELECT max(ts) FROM fs_samples WHERE filesystem_id = f.id)
		ORDER BY f.host_id, f.mount`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []HostFilesystem{}
	for rows.Next() {
		var f HostFilesystem
		if err := rows.Scan(&f.HostID, &f.ID, &f.Mount, &f.Device, &f.FSType, &f.LastSeen, &f.Used, &f.Total, &f.TS); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// CountEvents counts the events of a kind since a time, by object.
func (s *Store) CountEvents(ctx context.Context, kind string, since int64) (map[int64]int, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`SELECT subject_id, count(*) FROM events
		WHERE kind = ? AND ts >= ? GROUP BY subject_id`), kind, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		counts[id] = n
	}
	return counts, rows.Err()
}
