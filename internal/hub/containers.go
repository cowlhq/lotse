package hub

import (
	"context"
	"encoding/json"
	"strconv"

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// containerSlug is how lotse knows a container across redeploys (design
// 5.1): its compose project and service, with the replica number from the
// second replica on; otherwise its name. The label lotse.name overrides
// both.
func containerSlug(c protocol.Container) string {
	if name := c.Labels["lotse.name"]; name != "" {
		return name
	}
	project, service := c.Labels["com.docker.compose.project"], c.Labels["com.docker.compose.service"]
	if project == "" || service == "" {
		return c.Name
	}
	slug := project + "-" + service
	if n, err := strconv.Atoi(c.Labels["com.docker.compose.container-number"]); err == nil && n > 1 {
		slug += "-" + strconv.Itoa(n)
	}
	return slug
}

func toStoreContainer(hostID int64, c protocol.Container, seen int64) store.Container {
	return store.Container{
		HostID: hostID, Slug: containerSlug(c), DockerID: c.DockerID, Name: c.Name,
		Image: c.Image, ComposeProject: c.Labels["com.docker.compose.project"],
		ComposeService: c.Labels["com.docker.compose.service"], State: c.State,
		Health: c.Health, ExitCode: c.ExitCode, Restarts: c.Restarts, StartedAt: c.StartedAt,
		Labels: c.Labels, LastSeen: seen,
	}
}

// containerIDs maps a host's Docker IDs to lotse's containers, for one
// agent connection.
type containerIDs map[string]int64

// containerList records a host's full list of containers.
func (h *hub) containerList(ctx context.Context, hostID int64, list protocol.ContainerList, ids containerIDs) error {
	cs := make([]store.Container, len(list.Containers))
	for i, c := range list.Containers {
		cs[i] = toStoreContainer(hostID, c, list.TS)
	}
	got, err := h.db.SyncContainers(ctx, hostID, cs, list.TS)
	if err != nil {
		return err
	}
	if err := h.db.SetDockerVersion(ctx, hostID, list.DockerVersion); err != nil {
		return err
	}
	clear(ids)
	for dockerID, id := range got {
		ids[dockerID] = id
	}
	h.live.publish(LiveMessage{Type: "containers", HostID: hostID})
	return nil
}

// Container events worth keeping in a container's history.
var recordedActions = map[string]bool{
	"start": true, "die": true, "oom": true, "restart": true, "destroy": true,
	"health_status: healthy": true, "health_status: unhealthy": true,
}

// containerEvent records a change of one container.
func (h *hub) containerEvent(ctx context.Context, hostID int64, e protocol.ContainerEvent, ids containerIDs) error {
	var id int64
	if e.Container != nil {
		var err error
		id, err = h.db.SaveContainer(ctx, toStoreContainer(hostID, *e.Container, e.TS))
		if err != nil {
			return err
		}
		ids[e.DockerID] = id
	} else {
		id = ids[e.DockerID]
		if err := h.db.ContainerGone(ctx, hostID, e.DockerID, e.TS); err != nil {
			return err
		}
		delete(ids, e.DockerID)
	}
	h.live.publish(LiveMessage{Type: "containers", HostID: hostID})
	if id == 0 || !recordedActions[e.Action] {
		return nil
	}
	data := map[string]any{"docker_id": e.DockerID}
	if e.Container != nil && e.Action == "die" {
		data["exit_code"] = e.Container.ExitCode
	}
	b, _ := json.Marshal(data)
	h.writer.add(func(batch *store.Batch) {
		batch.Events = append(batch.Events, store.Event{
			TS: e.TS, SubjectKind: "container", SubjectID: id, Kind: "container." + e.Action, Data: string(b),
		})
	})
	return nil
}

// containerStats records a reading of the running containers. Readings of
// containers the hub does not know yet are dropped; the next list names
// them.
func (h *hub) containerStats(hostID int64, s protocol.ContainerStats, ids containerIDs) {
	samples := make([]store.ContainerSample, 0, len(s.Stats))
	live := LiveMessage{Type: "container_stats", HostID: hostID}
	defer func() { h.live.publish(live) }()
	for _, st := range s.Stats {
		id, ok := ids[st.DockerID]
		if !ok {
			continue
		}
		x := store.ContainerSample{
			ContainerID: id, TS: s.TS, CPU: st.CPU, Mem: st.Mem, MemLimit: st.MemLimit,
			NetRx: st.NetRx, NetTx: st.NetTx,
		}
		samples = append(samples, x)
		live.Containers = append(live.Containers, LiveContainerReading{ContainerID: id, Reading: *toContainerReading(x)})
	}
	h.writer.add(func(b *store.Batch) {
		b.ContainerSamples = append(b.ContainerSamples, samples...)
	})
}
