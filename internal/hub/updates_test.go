package hub

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lotse.sh/internal/agent"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
	"lotse.sh/internal/update"
	"lotse.sh/internal/update/updatetest"
	"lotse.sh/internal/version"
)

func setVersion(t *testing.T, v string) {
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func TestReleases(t *testing.T) {
	rel := updatetest.New(t, "1.0.0", "1.1.0")
	r := &releases{api: rel.URL + "/latest"}
	r.check(t.Context())
	latest, checked, errText := r.get()
	if latest == nil || latest.Version != "1.1.0" || latest.Notes != "notes of 1.1.0" || checked == 0 || errText != "" {
		t.Errorf("latest %+v, checked %d, error %q", latest, checked, errText)
	}
	bad := &releases{api: rel.URL + "/nothing"}
	bad.check(t.Context())
	if _, _, errText := bad.get(); errText == "" {
		t.Error("a failed check has no error")
	}
}

// An admin asks a connected agent to update: it downloads the release,
// checks it, installs it, reports so, and ends to be restarted into it.
func TestAgentUpdate(t *testing.T) {
	setVersion(t, "1.0.0")
	t.Setenv(update.StartedEnv, "1")
	rel := updatetest.New(t, "1.1.0")
	h, server := newTestHub(t)
	h.releases = &releases{api: rel.URL + "/latest"}
	h.releases.check(t.Context())
	token := enrollmentToken(t, h)

	dir := t.TempDir()
	done := make(chan error, 1)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go func() {
		done <- agent.Run(ctx, agent.Config{Hub: server.URL, Token: token, DataDir: dir, ReleaseURL: rel.URL}, slog.New(slog.DiscardHandler))
	}()
	waitFor(t, "the agent to connect", func() bool { return h.agents.count() == 1 })
	hosts, _ := h.db.Hosts(t.Context())

	var u api.Updates
	getJSON(t, server, "/api/updates", &u)
	if u.Latest == nil || u.Latest.Version != "1.1.0" || u.HubVersion != "1.0.0" {
		t.Errorf("updates: %+v", u)
	}
	if status, body := do(t, server, "POST", fmt.Sprintf("/api/hosts/%d/update", hosts[0].ID), ""); status != http.StatusAccepted {
		t.Fatalf("update: %d %s", status, body)
	}
	select {
	case err := <-done:
		if err != update.ErrRestart {
			t.Fatalf("the agent ended with %v, want a restart", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the agent did not restart")
	}
	target, err := os.Readlink(filepath.Join(dir, "bin", "current"))
	if err != nil || target != "1.1.0" {
		t.Errorf("current: %q, %v", target, err)
	}
	waitFor(t, "the update events", func() bool {
		events, _ := h.db.Events(t.Context(), "host", hosts[0].ID, 0, time.Now().Unix()+10)
		var kinds []string
		for _, e := range events {
			kinds = append(kinds, e.Kind)
		}
		return fmt.Sprint(kinds) == "[agent.connected agent.update.started agent.update.installed agent.disconnected]"
	})

	// Now the agent has it: nothing to update.
	h.db.UpdateHost(t.Context(), hostWithVersion(hosts[0], "1.1.0"))
	if status, _ := do(t, server, "POST", fmt.Sprintf("/api/hosts/%d/update", hosts[0].ID), ""); status != http.StatusConflict {
		t.Errorf("updating an up-to-date agent: %d", status)
	}
}

func hostWithVersion(h store.Host, v string) store.Host {
	h.AgentVersion = v
	return h
}

// The hub updates itself: a copy of the database first, then the release,
// then it restarts.
func TestHubUpdate(t *testing.T) {
	setVersion(t, "1.0.0")
	rel := updatetest.New(t, "1.1.0")
	h, server := newTestHub(t)
	h.releases = &releases{api: rel.URL + "/latest"}
	h.releases.check(t.Context())
	h.cfg.ReleaseURL = rel.URL

	if status, _ := do(t, server, "POST", "/api/hub/update", ""); status != http.StatusConflict {
		t.Errorf("not under a starter: %d", status)
	}
	t.Setenv(update.StartedEnv, "1")
	t.Setenv("LOTSE_BIN_DIR", filepath.Join(h.cfg.DataDir, "bin"))
	if status, body := do(t, server, "POST", "/api/hub/update", ""); status != http.StatusAccepted {
		t.Fatalf("update: %d %s", status, body)
	}
	select {
	case <-h.restart:
	case <-time.After(5 * time.Second):
		t.Fatal("the hub did not restart")
	}
	if target, _ := os.Readlink(filepath.Join(h.cfg.DataDir, "bin", "current")); target != "1.1.0" {
		t.Errorf("current: %q", target)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.DataDir, "backups", "before-1.1.0.db")); err != nil {
		t.Errorf("no backup: %v", err)
	}
}
