package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"lotse.sh/internal/protocol"
)

// docker watches the machine's containers through the Docker API, reading
// only. Changes arrive as Docker events; the full list is sent again every
// few minutes in case one was missed.
type docker struct {
	log *slog.Logger
	// emit hands a message to the outbox; emitLatest does too, and drops
	// older messages of the same type that are still waiting.
	emit, emitLatest func(protocol.Message)

	mu      sync.Mutex
	client  *client.Client
	version string
	running map[string]bool // Docker IDs of running containers
	prev    map[string]containerCounters
	// refresh asks for the full list now, after connecting to the hub.
	refresh chan struct{}
}

type containerCounters struct {
	at     time.Time
	cpu    uint64 // nanoseconds
	rx, tx uint64
}

const (
	listInterval   = 5 * time.Minute
	dockerRetry    = 30 * time.Second
	statsParallel  = 8
	defaultStatsAt = 30 * time.Second
)

// Actions that change what a container is. exec_* and the like are left
// out: health checks alone would fire them every few seconds.
var watchedActions = []string{
	"create", "start", "restart", "stop", "die", "kill", "oom",
	"pause", "unpause", "rename", "update", "destroy", "health_status",
}

func newDocker(log *slog.Logger, emit, emitLatest func(protocol.Message)) *docker {
	return &docker{
		log: log, emit: emit, emitLatest: emitLatest,
		running: map[string]bool{},
		prev:    map[string]containerCounters{},
		refresh: make(chan struct{}, 1),
	}
}

// Version is Docker's version, or empty while Docker cannot be reached.
func (d *docker) Version() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version
}

// Refresh asks for the full list to be sent soon.
func (d *docker) Refresh() {
	select {
	case d.refresh <- struct{}{}:
	default:
	}
}

// watch follows Docker until ctx ends. Without Docker, it tries again every
// half minute: a machine without containers is fine, and Docker may come.
func (d *docker) watch(ctx context.Context) {
	reported := false
	for {
		err := d.watchOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		d.mu.Lock()
		d.version = ""
		clear(d.running)
		d.mu.Unlock()
		if !reported {
			d.log.Warn("cannot watch Docker, trying again every 30 s", "err", err)
			reported = true
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(dockerRetry):
		}
	}
}

func (d *docker) watchOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer c.Close()
	v, err := c.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.client, d.version = c, v.Version
	d.mu.Unlock()
	d.log.Info("watching Docker", "version", v.Version)

	// Subscribe before listing, so nothing that happens in between is lost.
	stream := c.Events(ctx, client.EventsListOptions{
		Filters: make(client.Filters).Add("type", "container").Add("event", watchedActions...),
	})
	if err := d.list(ctx); err != nil {
		return err
	}
	t := time.NewTicker(listInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-stream.Err:
			return fmt.Errorf("docker events: %w", err)
		case <-t.C:
			if err := d.list(ctx); err != nil {
				return err
			}
		case <-d.refresh:
			if err := d.list(ctx); err != nil {
				return err
			}
		case ev := <-stream.Messages:
			if err := d.event(ctx, ev); err != nil {
				return err
			}
		}
	}
}

// list sends every container on the machine.
func (d *docker) list(ctx context.Context) error {
	c := d.dockerClient()
	res, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	list := protocol.ContainerList{TS: time.Now().Unix(), DockerVersion: d.Version(), Containers: []protocol.Container{}}
	running := map[string]bool{}
	for _, item := range res.Items {
		ct, err := d.inspect(ctx, item.ID)
		if errdefs.IsNotFound(err) {
			continue // removed in the meantime
		}
		if err != nil {
			return err
		}
		list.Containers = append(list.Containers, ct)
		if ct.State == "running" {
			running[ct.DockerID] = true
		}
	}
	d.mu.Lock()
	d.running = running
	d.mu.Unlock()
	d.emitLatest(protocol.Message{Type: protocol.TypeContainers, Containers: &list})
	return nil
}

// event sends what Docker reported, along with the container as it is now:
// events can arrive out of order, inspecting says what is true.
func (d *docker) event(ctx context.Context, ev events.Message) error {
	e := protocol.ContainerEvent{
		TS:       ev.Time,
		DockerID: ev.Actor.ID,
		Action:   string(ev.Action),
	}
	if ev.Action != events.ActionDestroy {
		ct, err := d.inspect(ctx, ev.Actor.ID)
		switch {
		case errdefs.IsNotFound(err):
		case err != nil:
			return err
		default:
			e.Container = &ct
		}
	}
	d.mu.Lock()
	if e.Container != nil && e.Container.State == "running" {
		d.running[e.DockerID] = true
	} else {
		delete(d.running, e.DockerID)
		delete(d.prev, e.DockerID)
	}
	d.mu.Unlock()
	d.emit(protocol.Message{Type: protocol.TypeContainerEvent, ContainerEvent: &e})
	return nil
}

func (d *docker) dockerClient() *client.Client {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.client
}

func (d *docker) inspect(ctx context.Context, id string) (protocol.Container, error) {
	res, err := d.dockerClient().ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return protocol.Container{}, err
	}
	return fromInspect(res.Container), nil
}

func fromInspect(in container.InspectResponse) protocol.Container {
	c := protocol.Container{
		DockerID: in.ID,
		Name:     strings.TrimPrefix(in.Name, "/"),
		Restarts: in.RestartCount,
	}
	if in.Config != nil {
		c.Image = in.Config.Image
		c.Labels = in.Config.Labels
	}
	if s := in.State; s != nil {
		c.State = string(s.Status)
		c.ExitCode = s.ExitCode
		if t, err := time.Parse(time.RFC3339Nano, s.StartedAt); err == nil && t.Year() > 1 {
			c.StartedAt = t.Unix()
		}
		if s.Health != nil {
			c.Health = string(s.Health.Status)
		}
	}
	return c
}

// sampleStats reads the running containers' usage every interval.
func (d *docker) sampleStats(ctx context.Context, interval func() time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval()):
		}
		if s, ok := d.stats(ctx); ok {
			d.emit(protocol.Message{Type: protocol.TypeContainerStats, ContainerStats: &s})
		}
	}
}

func (d *docker) stats(ctx context.Context) (protocol.ContainerStats, bool) {
	c := d.dockerClient()
	d.mu.Lock()
	ids := make([]string, 0, len(d.running))
	for id := range d.running {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	if c == nil || len(ids) == 0 {
		return protocol.ContainerStats{}, false
	}
	out := protocol.ContainerStats{TS: time.Now().Unix()}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(statsParallel)
	for _, id := range ids {
		g.Go(func() error {
			st, err := d.statsOf(gctx, c, id)
			if err != nil {
				// Stopped in the meantime, most likely; its event follows.
				return nil
			}
			mu.Lock()
			out.Stats = append(out.Stats, st)
			mu.Unlock()
			return nil
		})
	}
	g.Wait()
	return out, len(out.Stats) > 0
}

// statsOf reads one container once, without waiting for Docker's second
// sample, and works out rates against the previous reading.
func (d *docker) statsOf(ctx context.Context, c *client.Client, id string) (protocol.ContainerStat, error) {
	res, err := c.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		return protocol.ContainerStat{}, err
	}
	defer res.Body.Close()
	var raw container.StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return protocol.ContainerStat{}, err
	}
	if raw.Read.IsZero() {
		return protocol.ContainerStat{}, errors.New("no reading")
	}
	cur := containerCounters{at: raw.Read, cpu: raw.CPUStats.CPUUsage.TotalUsage}
	for _, n := range raw.Networks {
		cur.rx += n.RxBytes
		cur.tx += n.TxBytes
	}
	st := protocol.ContainerStat{DockerID: id}
	if mem := memoryUsed(raw.MemoryStats); mem > 0 {
		m := int64(mem)
		st.Mem = &m
	}
	if raw.MemoryStats.Limit > 0 {
		l := int64(raw.MemoryStats.Limit)
		st.MemLimit = &l
	}
	d.mu.Lock()
	prev, ok := d.prev[id]
	d.prev[id] = cur
	d.mu.Unlock()
	if ok {
		if secs := cur.at.Sub(prev.at).Seconds(); secs > 0 {
			if cur.cpu >= prev.cpu {
				p := float64(cur.cpu-prev.cpu) / (secs * 1e9) * 100
				st.CPU = &p
			}
			st.NetRx, st.NetTx = rate(prev.rx, cur.rx, secs), rate(prev.tx, cur.tx, secs)
		}
	}
	return st, nil
}

// memoryUsed is the memory a container really uses, as docker stats counts
// it: without the page cache it could give back (cgroup v2, then v1).
func memoryUsed(m container.MemoryStats) uint64 {
	cache, ok := m.Stats["inactive_file"]
	if !ok {
		cache = m.Stats["total_inactive_file"]
	}
	if cache > m.Usage {
		return m.Usage
	}
	return m.Usage - cache
}
