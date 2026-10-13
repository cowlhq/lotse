package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Container is a container on a host. It keeps its identity, and so its
// history, across redeploys that replace the Docker container.
type Container struct {
	ID             int64             `json:"id"`
	HostID         int64             `json:"host_id"`
	Slug           string            `json:"slug"`
	DockerID       string            `json:"docker_id"`
	Name           string            `json:"name"`
	Image          string            `json:"image"`
	ComposeProject string            `json:"compose_project"`
	ComposeService string            `json:"compose_service"`
	State          string            `json:"state"`
	Health         string            `json:"health"`
	ExitCode       int               `json:"exit_code"`
	Restarts       int               `json:"restarts"`
	StartedAt      int64             `json:"started_at"`
	Labels         map[string]string `json:"labels"`
	FirstSeen      int64             `json:"first_seen"`
	LastSeen       int64             `json:"last_seen"`
	// GoneAt is when the host last listed it, if it no longer does.
	GoneAt *int64 `json:"gone_at"`
}

const containerColumns = `id, host_id, slug, docker_id, name, image, compose_project,
	compose_service, state, health, exit_code, restarts, started_at, labels_json,
	first_seen, last_seen, gone_at`

func scanContainer(row interface{ Scan(...any) error }) (Container, error) {
	var c Container
	var labels string
	err := row.Scan(&c.ID, &c.HostID, &c.Slug, &c.DockerID, &c.Name, &c.Image, &c.ComposeProject,
		&c.ComposeService, &c.State, &c.Health, &c.ExitCode, &c.Restarts, &c.StartedAt, &labels,
		&c.FirstSeen, &c.LastSeen, &c.GoneAt)
	if err == nil {
		err = json.Unmarshal([]byte(labels), &c.Labels)
	}
	return c, err
}

// SaveContainer records a container as its host reports it now, matched by
// slug. It returns the container's ID.
func (s *Store) SaveContainer(ctx context.Context, c Container) (int64, error) {
	var id int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.saveContainer(ctx, tx, c)
		return err
	})
	return id, err
}

func (s *Store) saveContainer(ctx context.Context, tx *sql.Tx, c Container) (int64, error) {
	labels, err := json.Marshal(c.Labels)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, s.rebind(`
		INSERT INTO containers (host_id, slug, docker_id, name, image, compose_project,
			compose_service, state, health, exit_code, restarts, started_at, labels_json,
			first_seen, last_seen, gone_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT (host_id, slug) DO UPDATE SET
			docker_id = excluded.docker_id, name = excluded.name, image = excluded.image,
			compose_project = excluded.compose_project, compose_service = excluded.compose_service,
			state = excluded.state, health = excluded.health, exit_code = excluded.exit_code,
			restarts = excluded.restarts, started_at = excluded.started_at,
			labels_json = excluded.labels_json, last_seen = excluded.last_seen, gone_at = NULL
		RETURNING id`),
		c.HostID, c.Slug, c.DockerID, c.Name, c.Image, c.ComposeProject, c.ComposeService,
		c.State, c.Health, c.ExitCode, c.Restarts, c.StartedAt, string(labels),
		c.LastSeen, c.LastSeen).Scan(&id)
	return id, err
}

// SyncContainers records the complete list of a host's containers. Those it
// no longer lists are marked gone, but kept with their history. It returns
// the IDs of the listed containers by Docker ID.
func (s *Store) SyncContainers(ctx context.Context, hostID int64, list []Container, now int64) (map[string]int64, error) {
	ids := make(map[string]int64, len(list))
	err := s.tx(ctx, func(tx *sql.Tx) error {
		listed := make([]any, 0, len(list))
		for _, c := range list {
			c.HostID, c.LastSeen = hostID, now
			id, err := s.saveContainer(ctx, tx, c)
			if err != nil {
				return err
			}
			ids[c.DockerID] = id
			listed = append(listed, id)
		}
		q := `UPDATE containers SET gone_at = ? WHERE host_id = ? AND gone_at IS NULL`
		args := []any{now, hostID}
		if len(listed) > 0 {
			q += ` AND id NOT IN (` + placeholders(len(listed)) + `)`
			args = append(args, listed...)
		}
		_, err := tx.ExecContext(ctx, s.rebind(q), args...)
		return err
	})
	return ids, err
}

// Containers returns a host's containers, those it still has first.
func (s *Store) Containers(ctx context.Context, hostID int64) ([]Container, error) {
	rows, err := s.read.QueryContext(ctx, s.rebind(`SELECT `+containerColumns+`
		FROM containers WHERE host_id = ?
		ORDER BY gone_at IS NOT NULL, slug`), hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Container
	for rows.Next() {
		c, err := scanContainer(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// Container returns one container.
func (s *Store) Container(ctx context.Context, id int64) (Container, error) {
	c, err := scanContainer(s.queryRow(ctx, `SELECT `+containerColumns+` FROM containers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}
