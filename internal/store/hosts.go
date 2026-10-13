package store

import (
	"context"
	"database/sql"
	"errors"
)

// Host is a machine with an agent.
type Host struct {
	ID            int64  `json:"id"`
	MachineID     string `json:"machine_id"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Platform      string `json:"platform"`
	Kernel        string `json:"kernel"`
	Arch          string `json:"arch"`
	CPUModel      string `json:"cpu_model"`
	Cores         int    `json:"cores"`
	MemTotal      int64  `json:"mem_total"`
	BootTime      int64  `json:"boot_time"`
	AgentVersion  string `json:"agent_version"`
	DockerVersion string `json:"docker_version"`
	FirstSeen     int64  `json:"first_seen"`
	LastSeen      int64  `json:"last_seen"`
}

const hostColumns = `id, machine_id, name, hostname, os, platform, kernel, arch, cpu_model,
	cores, mem_total, boot_time, agent_version, docker_version, first_seen, last_seen`

func scanHost(row interface{ Scan(...any) error }) (Host, error) {
	var h Host
	err := row.Scan(&h.ID, &h.MachineID, &h.Name, &h.Hostname, &h.OS, &h.Platform, &h.Kernel,
		&h.Arch, &h.CPUModel, &h.Cores, &h.MemTotal, &h.BootTime, &h.AgentVersion,
		&h.DockerVersion, &h.FirstSeen, &h.LastSeen)
	return h, err
}

// SaveHost records what an agent says about its machine when it connects:
// a new host, or an update of the one with the same machine ID. It returns
// the host and the boot time it had before (0 for a new host), which tells
// whether the machine rebooted.
func (s *Store) SaveHost(ctx context.Context, h Host) (Host, int64, error) {
	var prevBoot int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, s.rebind(`SELECT boot_time FROM hosts WHERE machine_id = ?`),
			h.MachineID).Scan(&prevBoot)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		h.Name = h.Hostname // only taken for a new host
		row := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO hosts (machine_id, name, hostname, os, platform, kernel, arch, cpu_model,
				cores, mem_total, boot_time, agent_version, docker_version, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (machine_id) DO UPDATE SET
				hostname = excluded.hostname, os = excluded.os, platform = excluded.platform,
				kernel = excluded.kernel, arch = excluded.arch, cpu_model = excluded.cpu_model,
				cores = excluded.cores, mem_total = excluded.mem_total,
				boot_time = excluded.boot_time, agent_version = excluded.agent_version,
				docker_version = excluded.docker_version, last_seen = excluded.last_seen
			RETURNING `+hostColumns),
			h.MachineID, h.Name, h.Hostname, h.OS, h.Platform, h.Kernel, h.Arch, h.CPUModel,
			h.Cores, h.MemTotal, h.BootTime, h.AgentVersion, h.DockerVersion, h.LastSeen, h.LastSeen)
		h, err = scanHost(row)
		return err
	})
	return h, prevBoot, err
}

// Hosts returns all hosts, by name.
func (s *Store) Hosts(ctx context.Context) ([]Host, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+hostColumns+` FROM hosts ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// ErrNotFound is returned for an object that does not exist.
var ErrNotFound = errors.New("not found")

// Host returns one host.
func (s *Store) Host(ctx context.Context, id int64) (Host, error) {
	h, err := scanHost(s.queryRow(ctx, `SELECT `+hostColumns+` FROM hosts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	return h, err
}

// SetDockerVersion records the version of Docker on a host.
func (s *Store) SetDockerVersion(ctx context.Context, hostID int64, version string) error {
	_, err := s.exec(ctx, `UPDATE hosts SET docker_version = ? WHERE id = ?`, version, hostID)
	return err
}
