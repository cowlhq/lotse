package hub

import (
	"context"
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
	"lotse.sh/internal/version"
)

const (
	pingInterval = 20 * time.Second
	helloTimeout = 10 * time.Second
	// A boot time that moved by more than this is a reboot; less is the
	// clock being adjusted.
	rebootSlack = 60
)

// agents are the agents connected right now, by host.
type agents struct {
	mu   sync.Mutex
	conn map[*websocket.Conn]int64
}

func (a *agents) add(c *websocket.Conn, hostID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		a.conn = make(map[*websocket.Conn]int64)
	}
	a.conn[c] = hostID
}

func (a *agents) remove(c *websocket.Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.conn, c)
}

// closeAll tells every agent that the hub is going away, so that it
// reconnects instead of waiting for a dead connection to time out.
func (a *agents) closeAll() {
	a.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(a.conn))
	for c := range a.conn {
		conns = append(conns, c)
	}
	a.mu.Unlock()
	for _, c := range conns {
		go c.Close(websocket.StatusGoingAway, "hub stopping")
	}
}

func (a *agents) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.conn)
}

type agentConn struct {
	conn   *websocket.Conn
	hostID int64
}

// all returns every connected agent.
func (a *agents) all() []agentConn {
	a.mu.Lock()
	defer a.mu.Unlock()
	list := make([]agentConn, 0, len(a.conn))
	for c, id := range a.conn {
		list = append(list, agentConn{c, id})
	}
	return list
}

// of returns the connections of a host's agents.
func (a *agents) of(hostID int64) []*websocket.Conn {
	a.mu.Lock()
	defer a.mu.Unlock()
	var conns []*websocket.Conn
	for c, id := range a.conn {
		if id == hostID {
			conns = append(conns, c)
		}
	}
	return conns
}

// connected says whether the host has an agent connected now.
func (a *agents) connected(hostID int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range a.conn {
		if id == hostID {
			return true
		}
	}
	return false
}

// authorized checks the agent's bearer token. Until enrollment arrives with
// milestone 3, every agent shows the same key, LOTSE_AGENT_KEY; without it,
// no agent gets in.
func (h *hub) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && h.agentKey != "" &&
		subtle.ConstantTimeCompare([]byte(token), []byte(h.agentKey)) == 1
}

// connectAgent is the WebSocket an agent keeps open to the hub.
func (h *hub) connectAgent(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeError(w, http.StatusUnauthorized, "this agent's token is not accepted")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: protocol.Subprotocols(),
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	v, err := protocol.ParseSubprotocol(conn.Subprotocol())
	if err != nil || !slices.Contains(protocol.Versions, v) {
		conn.Close(websocket.StatusPolicyViolation, "no protocol version in common; update the agent or the hub")
		return
	}
	// Reads and writes do not use the request's context: one whose context
	// ends drops the connection without a word. The pings below and
	// closeAll end them instead.
	ctx := context.WithoutCancel(r.Context())

	helloCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	var m protocol.Message
	err = wsjson.Read(helloCtx, conn, &m)
	cancel()
	if err != nil || m.Type != protocol.TypeHello || m.Hello == nil {
		conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	hello := *m.Hello
	host, err := h.saveHost(ctx, hello)
	if err != nil {
		h.log.Error("cannot save a host", "hostname", hello.Hostname, "err", err)
		conn.Close(websocket.StatusInternalError, "the hub cannot save this host")
		return
	}
	log := h.log.With("host", host.Name, "host_id", host.ID, "agent_version", hello.Version, "protocol", v)
	if err := wsjson.Write(ctx, conn, protocol.Message{
		Type:    protocol.TypeWelcome,
		Welcome: &protocol.Welcome{HubVersion: version.Version, Config: h.agentConfig(ctx, host.ID)},
	}); err != nil {
		return
	}
	h.agents.add(conn, host.ID)
	h.event(host.ID, "agent.connected", "")
	h.publishState(host.ID)
	log.Info("agent connected")
	if slices.Contains(h.live.watchedHosts(), host.ID) {
		h.sendLive(ctx, host.ID, liveFor)
	}
	defer func() {
		h.agents.remove(conn)
		h.event(host.ID, "agent.disconnected", "")
		h.publishState(host.ID)
	}()

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, done := context.WithTimeout(ctx, pingInterval)
				err := conn.Ping(pingCtx)
				done()
				if err != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()
	ids := containerIDs{}
	for {
		var m protocol.Message
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			log.Info("agent disconnected", "reason", disconnectReason(err))
			return
		}
		var err error
		switch {
		case m.Type == protocol.TypeHost && m.Host != nil:
			h.hostSample(host.ID, *m.Host)
		case m.Type == protocol.TypeContainers && m.Containers != nil:
			err = h.containerList(ctx, host.ID, *m.Containers, ids)
		case m.Type == protocol.TypeContainerEvent && m.ContainerEvent != nil:
			err = h.containerEvent(ctx, host.ID, *m.ContainerEvent, ids)
		case m.Type == protocol.TypeContainerStats && m.ContainerStats != nil:
			h.containerStats(host.ID, *m.ContainerStats, ids)
		default:
			log.Debug("ignoring a message of unknown type", "type", m.Type)
		}
		if err != nil {
			log.Error("cannot save what the agent sent", "type", m.Type, "err", err)
		}
	}
}

// saveHost records the machine an agent says it is on, and its reboot.
func (h *hub) saveHost(ctx context.Context, hello protocol.Hello) (store.Host, error) {
	machineID := hello.MachineID
	if machineID == "" {
		machineID = "hostname:" + hello.Hostname
	}
	host, prevBoot, err := h.db.SaveHost(ctx, store.Host{
		MachineID: machineID, Hostname: hello.Hostname, OS: hello.OS, Platform: hello.Platform,
		Kernel: hello.Kernel, Arch: hello.Arch, CPUModel: hello.CPUModel, Cores: hello.Cores,
		MemTotal: hello.MemTotal, BootTime: hello.BootTime, AgentVersion: hello.Version,
		DockerVersion: hello.DockerVersion, LastSeen: time.Now().Unix(),
	})
	if err == nil && prevBoot != 0 && hello.BootTime > prevBoot+rebootSlack {
		h.writer.add(func(b *store.Batch) {
			b.Events = append(b.Events, store.Event{
				TS: hello.BootTime, SubjectKind: "host", SubjectID: host.ID, Kind: "host.reboot",
			})
		})
	}
	return host, err
}

// publishState tells the browsers whether a host's agent is connected.
func (h *hub) publishState(hostID int64) {
	online := h.agents.connected(hostID)
	h.live.publish(LiveMessage{Type: "host_state", HostID: hostID, Online: &online})
}

func (h *hub) hostSample(hostID int64, s protocol.HostSample) {
	h.live.publish(LiveMessage{Type: "host", HostID: hostID, Reading: &api.HostReading{
		Ts: s.TS, Cpu: s.CPU, Iowait: s.IOWait, Steal: s.Steal, Load1: s.Load1, Load5: s.Load5,
		Load15: s.Load15, MemUsed: s.MemUsed, MemAvailable: s.MemAvailable, SwapUsed: s.SwapUsed,
		SwapTotal: s.SwapTotal, NetRx: s.NetRx, NetTx: s.NetTx, DiskRead: s.DiskRead,
		DiskWrite: s.DiskWrite, Uptime: s.Uptime,
	}})
	h.writer.add(func(b *store.Batch) {
		b.HostSamples = append(b.HostSamples, store.HostSample{
			HostID: hostID, TS: s.TS, CPU: s.CPU, IOWait: s.IOWait, Steal: s.Steal,
			Load1: s.Load1, Load5: s.Load5, Load15: s.Load15,
			MemUsed: s.MemUsed, MemAvailable: s.MemAvailable,
			SwapUsed: s.SwapUsed, SwapTotal: s.SwapTotal,
			NetRx: s.NetRx, NetTx: s.NetTx, DiskRead: s.DiskRead, DiskWrite: s.DiskWrite,
			Uptime: s.Uptime,
		})
		for _, fs := range s.Filesystems {
			b.FSSamples = append(b.FSSamples, store.FSSample{
				HostID: hostID, Mount: fs.Mount, Device: fs.Device, FSType: fs.FSType, TS: s.TS,
				Used: fs.Used, Total: fs.Total, InodesUsed: fs.InodesUsed, InodesTotal: fs.InodesTotal,
			})
		}
		if b.Seen == nil {
			b.Seen = map[int64]int64{}
		}
		b.Seen[hostID] = max(b.Seen[hostID], s.TS)
	})
}

func (h *hub) event(hostID int64, kind, data string) {
	h.writer.add(func(b *store.Batch) {
		b.Events = append(b.Events, store.Event{
			TS: time.Now().Unix(), SubjectKind: "host", SubjectID: hostID, Kind: kind, Data: data,
		})
	})
}

func disconnectReason(err error) string {
	if s := websocket.CloseStatus(err); s != -1 {
		return s.String()
	}
	return err.Error()
}
