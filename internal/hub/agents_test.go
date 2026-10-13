package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/agent"
	"lotse.sh/internal/auth"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

func TestAgentReports(t *testing.T) {
	h, server := newTestHub(t)
	h.hostInterval = time.Second
	token := enrollmentToken(t, h)

	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error)
	go func() {
		done <- agent.Run(ctx, agent.Config{Hub: server.URL, Token: token, DataDir: t.TempDir()}, slog.New(slog.DiscardHandler))
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
	token := enrollmentToken(t, h)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + protocol.ConnectPath
	dial := func(credential string, subprotocols ...string) (*websocket.Conn, *http.Response, error) {
		return websocket.Dial(t.Context(), wsURL, &websocket.DialOptions{
			Subprotocols: subprotocols,
			HTTPHeader:   http.Header{"Authorization": {"Bearer " + credential}},
		})
	}
	if _, res, err := dial(""); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credential: %v", err)
	}
	if _, res, err := dial(token, protocol.Subprotocols()...); err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("the enrollment token as credential: %v", err)
	}

	// Enrolling: a wrong token is refused, the right one gives a credential.
	if status, _ := do(t, server, "POST", protocol.EnrollPath, `{"token":"lotse_enr_wrong","hostname":"web1","machine_id":"m1"}`); status != http.StatusUnauthorized {
		t.Errorf("enroll with a wrong token: %d", status)
	}
	status, body := do(t, server, "POST", protocol.EnrollPath, `{"token":"`+token+`","hostname":"web1","machine_id":"m1"}`)
	var e protocol.EnrollResponse
	json.Unmarshal([]byte(body), &e)
	if status != http.StatusOK || !strings.HasPrefix(e.Credential, "lotse_agt_") || e.Environment != "acme/shop/prod" {
		t.Fatalf("enroll: %d %s", status, body)
	}

	// No version in common: closed with a reason.
	conn, _, err := dial(e.Credential, "lotse.999")
	if err != nil {
		t.Fatal(err)
	}
	var m protocol.Message
	if err := wsjson.Read(t.Context(), conn, &m); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("an unknown version: %v", err)
	}

	// Anything but hello first: closed.
	conn, _, err = dial(e.Credential, protocol.Subprotocols()...)
	if err != nil {
		t.Fatal(err)
	}
	wsjson.Write(t.Context(), conn, protocol.Message{Type: "something"})
	if err := wsjson.Read(t.Context(), conn, &m); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("no hello: %v", err)
	}

	// The credential on another machine: closed.
	conn, _, err = dial(e.Credential, protocol.Subprotocols()...)
	if err != nil {
		t.Fatal(err)
	}
	wsjson.Write(t.Context(), conn, protocol.Message{Type: protocol.TypeHello, Hello: &protocol.Hello{Hostname: "other", MachineID: "m2"}})
	if err := wsjson.Read(t.Context(), conn, &m); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("another machine: %v", err)
	}
}

// A revoked agent is disconnected and does not enroll again by itself.
func TestRevokedAgent(t *testing.T) {
	h, server := newTestHub(t)
	token := enrollmentToken(t, h)
	dir := t.TempDir()
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go agent.Run(ctx, agent.Config{Hub: server.URL, Token: token, DataDir: dir}, slog.New(slog.DiscardHandler))
	waitFor(t, "the agent to connect", func() bool { return h.agents.count() == 1 })
	if _, err := os.Stat(filepath.Join(dir, "agent.json")); err != nil {
		t.Errorf("no credential kept: %v", err)
	}
	hosts, _ := h.db.Hosts(ctx)
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/hosts/%d/agent", hosts[0].ID), ""); status != http.StatusNoContent {
		t.Fatalf("revoke: %d", status)
	}
	waitFor(t, "the agent to be gone", func() bool { return h.agents.count() == 0 })
	time.Sleep(3 * time.Second) // it tries again, with the revoked credential
	if h.agents.count() != 0 {
		t.Error("the revoked agent came back")
	}
}

// enrollmentToken makes the environment acme/shop/prod and its enrollment
// token.
func enrollmentToken(t *testing.T, h *hub) string {
	t.Helper()
	env, err := h.db.EnsureEnvironment(t.Context(), ada(), [3]store.ScopeName{
		{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, hash := auth.NewToken("lotse_enr_")
	if err := h.db.NewEnrollmentToken(t.Context(), ada(), env.ID, hash, h.box.Seal(token), 1); err != nil {
		t.Fatal(err)
	}
	return token
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
