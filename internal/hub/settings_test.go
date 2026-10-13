package hub

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"lotse.sh/internal/agent"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

func TestSettingsAPI(t *testing.T) {
	h, server := newTestHub(t)
	h.hostInterval, h.containerInterval = 0, 0 // the built-in defaults
	ctx := t.Context()
	env, _ := h.db.EnsureEnvironment(ctx, ada(), [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
	host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "m", Hostname: "web1", LastSeen: 1})
	h.db.SetHostEnvironment(ctx, ada(), host.ID, &env.ID)
	project, _ := h.db.Scope(ctx, *env.ParentID)
	customer := *project.ParentID

	put := func(subject, key, value string) (int, string) {
		return do(t, server, "PUT", "/api/settings", fmt.Sprintf(`{"subject":%q,"key":%q,"value":%s}`, subject, key, value))
	}
	if status, body := put(fmt.Sprintf("customer:%d", customer), "agent.interval.host", `"60s"`); status != http.StatusNoContent {
		t.Fatalf("set on customer: %d %s", status, body)
	}
	if status, _ := put(fmt.Sprintf("environment:%d", env.ID), "agent.interval.host", `"20s"`); status != http.StatusNoContent {
		t.Fatal("set on environment")
	}
	for _, bad := range [][3]string{
		{fmt.Sprintf("host:%d", host.ID), "agent.interval.host", `"1s"`},
		{fmt.Sprintf("host:%d", host.ID), "agent.interval.host", `15`},
		{fmt.Sprintf("host:%d", host.ID), "no.such.key", `"1m"`},
		{"user:1", "agent.interval.host", `"1m"`},
	} {
		if status, body := put(bad[0], bad[1], bad[2]); status != http.StatusBadRequest {
			t.Errorf("%v: %d %s", bad, status, body)
		}
	}

	var values []api.ConfigValue
	getJSON(t, server, fmt.Sprintf("/api/config?subject=host:%d", host.ID), &values)
	hostInterval := values[0]
	if hostInterval.Key != "agent.interval.host" || hostInterval.Value != "20s" || hostInterval.From == nil ||
		hostInterval.From.Kind != "environment" || len(hostInterval.Overrides) != 1 || hostInterval.Overrides[0].Level.Kind != "customer" {
		t.Errorf("host interval: %+v", hostInterval)
	}
	if values[1].Value != "30s" || values[1].From != nil {
		t.Errorf("container interval: %+v", values[1])
	}

	// Unset on the environment: the customer's value shows through.
	put(fmt.Sprintf("environment:%d", env.ID), "agent.interval.host", `null`)
	getJSON(t, server, fmt.Sprintf("/api/config?subject=host:%d", host.ID), &values)
	if values[0].Value != "60s" || values[0].From.Kind != "customer" {
		t.Errorf("after unsetting: %+v", values[0])
	}
	if status, _ := do(t, server, "GET", "/api/config?subject=host:999", ""); status != http.StatusNotFound {
		t.Errorf("a missing host: %d", status)
	}
	if status, _ := do(t, server, "GET", fmt.Sprintf("/api/config?subject=project:%d", env.ID), ""); status != http.StatusNotFound {
		t.Errorf("an environment asked for as a project: %d", status)
	}
}

// A setting changed while an agent is connected reaches it at once.
func TestSettingsReachAgents(t *testing.T) {
	h, server := newTestHub(t)
	token := enrollmentToken(t, h)
	h.hostInterval = time.Second
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go agent.Run(ctx, agent.Config{Hub: server.URL, Token: token, DataDir: t.TempDir()}, slog.New(slog.DiscardHandler))
	waitFor(t, "the agent to connect", func() bool { return h.agents.count() == 1 })

	// The default interval here is a second; set a minute globally, then
	// count readings.
	if status, body := do(t, server, "PUT", "/api/settings", `{"subject":"global:1","key":"agent.interval.host","value":"1m"}`); status != http.StatusNoContent {
		t.Fatalf("%d %s", status, body)
	}
	hosts, _ := h.db.Hosts(ctx)
	time.Sleep(2 * time.Second) // what was already on its way
	before, _ := h.db.Series(ctx, "host", hosts[0].ID, 0, time.Now().Unix()+100, 1)
	time.Sleep(3 * time.Second)
	after, _ := h.db.Series(ctx, "host", hosts[0].ID, 0, time.Now().Unix()+100, 1)
	if len(after.Time) != len(before.Time) {
		t.Errorf("readings went on every second: %d, then %d", len(before.Time), len(after.Time))
	}
}

func ada() store.Actor {
	id := int64(1)
	return store.Actor{Kind: "user", ID: &id, Name: "admin@example.com", Via: "ui"}
}
