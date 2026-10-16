package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
)

// fakeAgent plays the agent of one machine: it enrolls and connects as
// lotse agent does and sends what the machine would, made up.
type fakeAgent struct {
	m       *machine
	story   *story
	hub     string
	version string
	log     *slog.Logger

	credential string
	hostID     int64
	// restarts counts the crashes of a restarting container.
	restarts int
	// liveUntil is when reading fast ends, in Unix nanoseconds.
	liveUntil atomic.Int64
	connected atomic.Bool
}

func (a *fakeAgent) machineID() string {
	return fmt.Sprintf("%016x%016x", mix(seed("machine", a.m.Name)), mix(seed("machine id", a.m.Name)))
}

// enroll gets the agent's credential with the environment's token.
func (a *fakeAgent) enroll(ctx context.Context, token string) error {
	body, _ := json.Marshal(protocol.EnrollRequest{Token: token, Hostname: a.m.Name, MachineID: a.machineID()})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.hub+protocol.EnrollPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("enrolling %s: %s", a.m.Name, res.Status)
	}
	var e protocol.EnrollResponse
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		return err
	}
	a.credential, a.hostID = e.Credential, e.HostID
	return nil
}

// run stays connected until ctx ends.
func (a *fakeAgent) run(ctx context.Context) {
	for {
		err := a.connect(ctx)
		a.connected.Store(false)
		if ctx.Err() != nil {
			return
		}
		a.log.Warn("agent not connected", "host", a.m.Name, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *fakeAgent) hello() protocol.Hello {
	return protocol.Hello{
		Version: a.version, Hostname: a.m.Name, MachineID: a.machineID(), OS: "linux",
		Platform: a.m.Platform, Kernel: a.m.Kernel, Arch: "x86_64", CPUModel: a.m.CPU, Cores: a.m.Cores,
		MemTotal: int64(a.m.MemGiB * gib), BootTime: a.m.bootTime(a.story), DockerVersion: a.m.Docker,
	}
}

func (a *fakeAgent) connect(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, a.hub+protocol.ConnectPath, &websocket.DialOptions{
		Subprotocols: protocol.Subprotocols(),
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + a.credential}},
	})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	bg := context.WithoutCancel(ctx)
	go func() {
		<-ctx.Done()
		conn.Close(websocket.StatusGoingAway, "agent stopping")
	}()
	send := func(m protocol.Message) error {
		wctx, cancel := context.WithTimeout(bg, 10*time.Second)
		defer cancel()
		return wsjson.Write(wctx, conn, m)
	}
	hello := a.hello()
	if err := send(protocol.Message{Type: protocol.TypeHello, Hello: &hello}); err != nil {
		return err
	}
	var welcome protocol.Message
	if err := wsjson.Read(bg, conn, &welcome); err != nil {
		return err
	}
	if welcome.Type != protocol.TypeWelcome {
		return fmt.Errorf("the hub answered hello with %q", welcome.Type)
	}
	a.connected.Store(true)
	readErr := make(chan error, 1)
	go func() {
		for {
			var m protocol.Message
			if err := wsjson.Read(bg, conn, &m); err != nil {
				readErr <- err
				return
			}
			if m.Type == protocol.TypeLive && m.Live != nil {
				a.liveUntil.Store(time.Now().Add(time.Duration(m.Live.Seconds) * time.Second).UnixNano())
			}
		}
	}()

	now := time.Now()
	if err := send(a.containerList(now)); err != nil {
		return err
	}
	if err := send(a.system(now)); err != nil {
		return err
	}
	for _, c := range a.m.Containers {
		if c.Health == "unhealthy" {
			x := a.container(c, now)
			if err := send(protocol.Message{Type: protocol.TypeContainerEvent, ContainerEvent: &protocol.ContainerEvent{
				TS: now.Unix(), DockerID: x.DockerID, Action: "health_status: unhealthy", Container: &x,
			}}); err != nil {
				return err
			}
		}
	}
	var lastHost, lastStats, lastSystem, lastList time.Time
	lastSystem, lastList = now, now
	// The restarting container crashes every three minutes, and comes back
	// a few seconds later.
	nextCrash := now.Add(20 * time.Second)
	var comeBack time.Time
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-readErr:
			return err
		case <-tick.C:
		}
		now := time.Now()
		hostEvery, statsEvery := 15*time.Second, 30*time.Second
		if now.UnixNano() < a.liveUntil.Load() {
			hostEvery, statsEvery = protocol.LiveInterval*time.Second, protocol.LiveInterval*time.Second
		}
		var msgs []protocol.Message
		if now.Sub(lastHost) >= hostEvery {
			s := a.m.sample(a.story, now)
			msgs = append(msgs, protocol.Message{Type: protocol.TypeHost, Host: &s})
			lastHost = now
		}
		if len(a.m.Containers) > 0 && now.Sub(lastStats) >= statsEvery {
			msgs = append(msgs, a.stats(now, !comeBack.IsZero()))
			lastStats = now
		}
		if now.Sub(lastSystem) >= time.Minute {
			msgs = append(msgs, a.system(now))
			lastSystem = now
		}
		if now.Sub(lastList) >= 5*time.Minute {
			msgs = append(msgs, a.containerList(now))
			lastList = now
		}
		for _, c := range a.m.Containers {
			if !c.Restarting {
				continue
			}
			switch {
			case comeBack.IsZero() && now.After(nextCrash):
				a.restarts++
				x := a.container(c, now)
				x.State, x.ExitCode = "restarting", 1
				msgs = append(msgs, protocol.Message{Type: protocol.TypeContainerEvent, ContainerEvent: &protocol.ContainerEvent{
					TS: now.Unix(), DockerID: x.DockerID, Action: "die", Container: &x,
				}})
				comeBack = now.Add(6 * time.Second)
				nextCrash = now.Add(3 * time.Minute)
			case !comeBack.IsZero() && now.After(comeBack):
				x := a.container(c, now)
				msgs = append(msgs, protocol.Message{Type: protocol.TypeContainerEvent, ContainerEvent: &protocol.ContainerEvent{
					TS: now.Unix(), DockerID: x.DockerID, Action: "start", Container: &x,
				}})
				comeBack = time.Time{}
			}
		}
		for _, m := range msgs {
			if err := send(m); err != nil {
				return err
			}
		}
	}
}

// container is a container as Docker would describe it now.
func (a *fakeAgent) container(c container, now time.Time) protocol.Container {
	labels := map[string]string{
		"com.docker.compose.project":          c.Project,
		"com.docker.compose.service":          c.Service,
		"com.docker.compose.container-number": fmt.Sprint(max(c.Replica, 1)),
		"com.docker.compose.oneoff":           "False",
	}
	for k, v := range c.Labels {
		labels[k] = v
	}
	started := a.story.start.Add(-time.Duration(c.StartedDays * 24 * float64(time.Hour)))
	x := protocol.Container{
		DockerID: dockerID(a.m.Name, c), Name: c.name(), Image: c.Image, State: "running",
		Health: c.Health, StartedAt: started.Unix(), Labels: labels,
	}
	if c.Restarting {
		x.Restarts = 11 + a.restarts
		x.StartedAt = now.Unix()
	}
	return x
}

func (a *fakeAgent) containerList(now time.Time) protocol.Message {
	list := protocol.ContainerList{TS: now.Unix(), DockerVersion: a.m.Docker, Containers: []protocol.Container{}}
	for _, c := range a.m.Containers {
		list.Containers = append(list.Containers, a.container(c, now))
	}
	return protocol.Message{Type: protocol.TypeContainers, Containers: &list}
}

// stats reads the running containers; a crashed one is not running.
func (a *fakeAgent) stats(now time.Time, crashed bool) protocol.Message {
	s := protocol.ContainerStats{TS: now.Unix()}
	for _, c := range a.m.Containers {
		if c.Restarting && crashed {
			continue
		}
		s.Stats = append(s.Stats, c.stat(a.m, a.story, now))
	}
	return protocol.Message{Type: protocol.TypeContainerStats, ContainerStats: &s}
}

func (a *fakeAgent) system(now time.Time) protocol.Message {
	s := protocol.System{TS: now.Unix(), Systemd: true}
	for _, name := range a.m.Services {
		s.Units = append(s.Units, protocol.Unit{Name: name, Active: "active", Sub: "running"})
	}
	slices.SortFunc(s.Units, func(a, b protocol.Unit) int { return strings.Compare(a.Name, b.Name) })
	if a.m.RebootReason != "" {
		s.RebootRequired, s.RebootReason = true, a.m.RebootReason
	}
	return protocol.Message{Type: protocol.TypeSystem, System: &s}
}
