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

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/version"
)

const (
	pingInterval = 20 * time.Second
	helloTimeout = 10 * time.Second
)

// agents are the agents connected right now.
type agents struct {
	mu   sync.Mutex
	conn map[*websocket.Conn]protocol.Hello
}

func (a *agents) add(c *websocket.Conn, h protocol.Hello) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		a.conn = make(map[*websocket.Conn]protocol.Hello)
	}
	a.conn[c] = h
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
	v, err := protocol.ParseSubprotocol(conn.Subprotocol())
	if err != nil || !slices.Contains(protocol.Versions, v) {
		conn.Close(websocket.StatusPolicyViolation, "no protocol version in common; update the agent or the hub")
		return
	}
	ctx := r.Context()

	helloCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	var m protocol.Message
	err = wsjson.Read(helloCtx, conn, &m)
	cancel()
	if err != nil || m.Type != protocol.TypeHello || m.Hello == nil {
		conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	hello := *m.Hello
	log := h.log.With("agent", hello.Hostname, "agent_version", hello.Version, "protocol", v)
	if err := wsjson.Write(ctx, conn, protocol.Message{
		Type:    protocol.TypeWelcome,
		Welcome: &protocol.Welcome{HubVersion: version.Version},
	}); err != nil {
		return
	}
	h.agents.add(conn, hello)
	defer h.agents.remove(conn)
	log.Info("agent connected")

	// Reading keeps the connection's pings and closes flowing; pinging
	// finds connections that hang without closing.
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
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
	for {
		var m protocol.Message
		// Not ctx: a read whose context ends drops the connection without
		// a word. The pings above and closeAll end the reads instead.
		if err := wsjson.Read(context.WithoutCancel(ctx), conn, &m); err != nil {
			log.Info("agent disconnected", "reason", disconnectReason(err))
			return
		}
		log.Debug("ignoring a message of unknown type", "type", m.Type)
	}
}

func disconnectReason(err error) string {
	if s := websocket.CloseStatus(err); s != -1 {
		return s.String()
	}
	return err.Error()
}
