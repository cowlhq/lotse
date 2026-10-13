package hub

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/agent"
	"lotse.sh/internal/protocol"
)

func TestAgentReports(t *testing.T) {
	h, server := newTestHub(t)
	h.agentKey = "key"
	h.hostInterval = time.Second

	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error)
	go func() {
		done <- agent.Run(ctx, agent.Config{Hub: server.URL, Token: "key"}, slog.New(slog.DiscardHandler))
	}()
	waitFor(t, "the agent to connect", func() bool { return h.agents.count() == 1 })

	var hostID int64
	waitFor(t, "the host to be saved", func() bool {
		hosts, err := h.db.Hosts(t.Context())
		if err != nil || len(hosts) != 1 {
			return false
		}
		hostID = hosts[0].ID
		return true
	})
	// The first reading has no CPU share yet; the second, a second later,
	// has.
	waitFor(t, "two readings", func() bool {
		s, err := h.db.Series(t.Context(), "host", hostID, 0, time.Now().Unix()+60, 1)
		return err == nil && len(s.Time) >= 2 && s.Columns["cpu"][1] != nil
	})

	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the agent to be gone", func() bool { return h.agents.count() == 0 })
	waitFor(t, "connect and disconnect events", func() bool {
		events, _ := h.db.Events(t.Context(), "host", hostID, 0, time.Now().Unix()+60)
		return len(events) == 2 && events[0].Kind == "agent.connected" && events[1].Kind == "agent.disconnected"
	})
}

func TestAgentRefused(t *testing.T) {
	h, server := newTestHub(t)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + protocol.ConnectPath
	dial := func(token string, subprotocols ...string) (*websocket.Conn, *http.Response, error) {
		return websocket.Dial(t.Context(), wsURL, &websocket.DialOptions{
			Subprotocols: subprotocols,
			HTTPHeader:   http.Header{"Authorization": {"Bearer " + token}},
		})
	}

	// No key on the hub: nobody gets in, not even with an empty token.
	if _, res, err := dial(""); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a key on the hub: %v", err)
	}
	h.agentKey = "key"
	if _, res, err := dial("wrong", protocol.Subprotocols()...); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a wrong token: %v", err)
	}

	// No version in common: closed with a reason.
	conn, _, err := dial("key", "lotse.999")
	if err != nil {
		t.Fatal(err)
	}
	var m protocol.Message
	if err := wsjson.Read(t.Context(), conn, &m); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("an unknown version: %v", err)
	}

	// Anything but hello first: closed.
	conn, _, err = dial("key", protocol.Subprotocols()...)
	if err != nil {
		t.Fatal(err)
	}
	wsjson.Write(t.Context(), conn, protocol.Message{Type: "something"})
	if err := wsjson.Read(t.Context(), conn, &m); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("no hello: %v", err)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
