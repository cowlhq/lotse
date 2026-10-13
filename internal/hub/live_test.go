package hub

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/agent"
)

// A browser watching a host gets its readings, and its agent reads fast
// while it does.
func TestLive(t *testing.T) {
	h, server := newTestHub(t)
	token := enrollmentToken(t, h)
	h.hostInterval = time.Minute // without live, a minute between readings

	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go agent.Run(ctx, agent.Config{Hub: server.URL, Token: token, DataDir: t.TempDir()}, slog.New(slog.DiscardHandler))
	waitFor(t, "the agent to connect", func() bool { return h.agents.count() == 1 })
	hosts, _ := h.db.Hosts(ctx)

	browser, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/live",
		&websocket.DialOptions{HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.CloseNow()
	if err := wsjson.Write(ctx, browser, map[string]any{"subscribe": []string{"host:" + itoa(hosts[0].ID)}}); err != nil {
		t.Fatal(err)
	}
	// The first reading came right after connecting; live brings more
	// within seconds.
	readings := 0
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for readings < 2 {
		var m LiveMessage
		if err := wsjson.Read(deadline, browser, &m); err != nil {
			t.Fatalf("after %d readings: %v", readings, err)
		}
		if m.Type == "host" && m.HostID == hosts[0].ID {
			readings++
		}
	}
	if n := len(h.live.watchedHosts()); n != 1 {
		t.Errorf("%d hosts watched, want 1", n)
	}
	browser.Close(websocket.StatusNormalClosure, "")
	waitFor(t, "the host to be unwatched", func() bool { return len(h.live.watchedHosts()) == 0 })
}

func TestLiveDropsSlowClients(t *testing.T) {
	var l live
	c := newLiveClient()
	l.subscribe(c, []string{"host:1"})
	for range liveBuffer + 1 {
		l.publish(LiveMessage{Type: "host", HostID: 1})
	}
	if !c.isGone() || len(l.watchedHosts()) != 0 {
		t.Error("a client that fell behind is still there")
	}
	// A late subscription of a dropped client changes nothing.
	if started, _ := l.subscribe(c, []string{"host:1"}); started != nil || len(l.watchedHosts()) != 0 {
		t.Error("a dropped client subscribed again")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestLiveWatchCounts(t *testing.T) {
	var l live
	a, b := newLiveClient(), newLiveClient()
	if started, _ := l.subscribe(a, []string{"hosts", "host:1"}); len(started) != 1 || started[0] != 1 {
		t.Errorf("first watcher of host 1: started %v", started)
	}
	if started, _ := l.subscribe(b, []string{"host:1"}); started != nil {
		t.Errorf("second watcher of host 1: started %v", started)
	}
	// a moves on to host 2: host 1 is still watched by b.
	if started, stopped := l.subscribe(a, []string{"host:2"}); len(started) != 1 || started[0] != 2 || stopped != nil {
		t.Errorf("a moves to host 2: started %v, stopped %v", started, stopped)
	}
	if stopped := l.remove(b); len(stopped) != 1 || stopped[0] != 1 {
		t.Errorf("b leaves: stopped %v, want host 1", stopped)
	}
	if stopped := l.remove(a); len(stopped) != 1 || stopped[0] != 2 {
		t.Errorf("a leaves: stopped %v, want host 2", stopped)
	}
}
