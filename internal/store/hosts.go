package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
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
	// EnvironmentID is nil until the host is put into an environment.
	EnvironmentID *int64 `json:"environment_id"`
	// System is what the agent last said of the machine's services and
	// restarts (protocol.System) as JSON, or empty. It is not part of the
	// history of changes.
	System string `json:"-"`
}

// waitingPrefix marks the machine ID of a host that lotse.yaml named before
// its agent enrolled.
const waitingPrefix = "waiting:"

// Waiting says whether a host waits for its agent: named in lotse.yaml,
// never heard from.
func (h Host) Waiting() bool { return strings.HasPrefix(h.MachineID, waitingPrefix) }

// CreateWaitingHost names a host in an environment before its agent
// enrolls; the first machine of that name to enroll there becomes it.
func (s *Store) CreateWaitingHost(ctx context.Context, actor Actor, envID int64, name string) (Host, error) {
	var h Host
	err := s.tx(ctx, func(tx *sql.Tx) error {
		b := make([]byte, 8)
		rand.Read(b)
		if err := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO hosts (machine_id, name, hostname, first_seen, last_seen, environment_id)
			VALUES (?, ?, ?, 0, 0, ?) RETURNING `+hostColumns),
			waitingPrefix+hex.EncodeToString(b), name, name, envID).Scan(hostDest(&h)...); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "host", h.ID, name, "created", nil, map[string]string{"name": name})
	})
	return h, err
}

// DeleteWaitingHost deletes a host that never heard from its agent. Hosts
// that did are never deleted.
func (s *Store) DeleteWaitingHost(ctx context.Context, actor Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		h, err := scanHost(tx.QueryRowContext(ctx, s.rebind(`SELECT `+hostColumns+` FROM hosts WHERE id = ?`), id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !h.Waiting() {
			return ErrNotEmpty
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM hosts WHERE id = ?`), id); err != nil {
			return err
		}
		if err := s.forget(ctx, tx, "host", id); err != nil {
			return err
		}
		return s.recorder(tx, actor).record(ctx, "host", id, h.Name, "deleted", map[string]string{"name": h.Name}, nil)
	})
}

const hostColumns = `id, machine_id, name, hostname, os, platform, kernel, arch, cpu_model,
	cores, mem_total, boot_time, agent_version, docker_version, first_seen, last_seen, environment_id,
	COALESCE(system_json, '')`

func scanHost(row interface{ Scan(...any) error }) (Host, error) {
	var h Host
	err := row.Scan(hostDest(&h)...)
	return h, err
}

func hostDest(h *Host) []any {
	return []any{&h.ID, &h.MachineID, &h.Name, &h.Hostname, &h.OS, &h.Platform, &h.Kernel,
		&h.Arch, &h.CPUModel, &h.Cores, &h.MemTotal, &h.BootTime, &h.AgentVersion,
		&h.DockerVersion, &h.FirstSeen, &h.LastSeen, &h.EnvironmentID, &h.System}
}

// SetHostSystem keeps what an agent says of its machine's services and
// restarts.
func (s *Store) SetHostSystem(ctx context.Context, id int64, system string) error {
	_, err := s.exec(ctx, `UPDATE hosts SET system_json = ? WHERE id = ?`, system, id)
	return err
}

// UpdateHost records what an agent says about its machine when it
// connects. It returns the host and the boot time it had before (0 if it
// never said), which tells whether the machine rebooted.
func (s *Store) UpdateHost(ctx context.Context, h Host) (Host, int64, error) {
	var prevBoot int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, s.rebind(`SELECT boot_time FROM hosts WHERE id = ?`), h.ID).Scan(&prevBoot); err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, s.rebind(`
			UPDATE hosts SET hostname = ?, os = ?, platform = ?, kernel = ?, arch = ?, cpu_model = ?,
				cores = ?, mem_total = ?, boot_time = ?, agent_version = ?, docker_version = ?, last_seen = ?
			WHERE id = ?
			RETURNING `+hostColumns),
			h.Hostname, h.OS, h.Platform, h.Kernel, h.Arch, h.CPUModel, h.Cores, h.MemTotal,
			h.BootTime, h.AgentVersion, h.DockerVersion, h.LastSeen, h.ID)
		var err error
		h, err = scanHost(row)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return h, 0, ErrNotFound
	}
	return h, prevBoot, err
}

// SaveHost records a machine by its machine ID, as a new host or an update
// of the one it is. Agents enroll instead; this is for tests and tools.
func (s *Store) SaveHost(ctx context.Context, h Host) (Host, int64, error) {
	var id int64
	err := s.queryRow(ctx, `SELECT id FROM hosts WHERE machine_id = ?`, h.MachineID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, s.rebind(`INSERT INTO hosts (machine_id, name, hostname, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?) RETURNING id`), h.MachineID, h.Hostname, h.Hostname, h.LastSeen, h.LastSeen).Scan(&id)
	}
	if err != nil {
		return h, 0, err
	}
	h.ID = id
	return s.UpdateHost(ctx, h)
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
