package hub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"lotse.sh/internal/probe"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// labelHub is a hub with a host web1 in acme/shop/prod, beside acme/shop/
// staging and globex/web/prod.
func labelHub(t *testing.T) (*hub, store.Host, map[string]store.Scope) {
	t.Helper()
	h, _ := newTestHub(t)
	ctx := t.Context()
	envs := map[string]store.Scope{}
	for _, l := range [][3]store.ScopeName{
		{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}},
		{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "staging", Name: "Staging"}},
		{{Slug: "globex", Name: "Globex"}, {Slug: "web", Name: "Web"}, {Slug: "prod", Name: "Prod"}},
	} {
		env, err := h.db.EnsureEnvironment(ctx, ada(), l)
		if err != nil {
			t.Fatal(err)
		}
		envs[env.Path] = env
	}
	host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
	prod := envs["acme/shop/prod"].ID
	h.db.SetHostEnvironment(ctx, ada(), host.ID, &prod)
	return h, host, envs
}

// deploy lists one container with these labels, as the agent does after a
// deploy.
func deploy(t *testing.T, h *hub, host store.Host, labels map[string]string) store.Container {
	t.Helper()
	all := map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "app"}
	for k, v := range labels {
		all[k] = v
	}
	list := protocol.ContainerList{TS: time.Now().Unix(), Containers: []protocol.Container{{DockerID: "d1", Name: "shop-app-1", State: "running", Labels: all}}}
	if err := h.containerList(t.Context(), host.ID, list, containerIDs{}); err != nil {
		t.Fatal(err)
	}
	cs, _ := h.db.Containers(t.Context(), host.ID)
	return cs[0]
}

func containerChecks(t *testing.T, h *hub, id int64) []store.Check {
	all, _ := h.db.Checks(t.Context(), 0)
	var out []store.Check
	for _, c := range all {
		if c.ContainerID != nil && *c.ContainerID == id {
			out = append(out, c)
		}
	}
	return out
}

func TestLabels(t *testing.T) {
	h, host, envs := labelHub(t)
	ctx := t.Context()
	c := deploy(t, h, host, map[string]string{
		"lotse.check.web.http.url":                 "https://shop.acme.example/health",
		"lotse.check.web.http.keyword":             "ok",
		"lotse.rules.container.restarting.enabled": "false",
		"lotse.rules.container.unhealthy.for":      "5m",
		"lotse.ignore":                             "false",
	})
	set, _ := h.db.Settings(ctx, []store.Subject{{Kind: "container", ID: c.ID}})
	if string(set[0]["rules.container.restarting.enabled"]) != "false" || string(set[0]["rules.container.unhealthy.for"]) != `"5m"` || string(set[0]["ignore"]) != "false" {
		t.Errorf("settings: %v", set[0])
	}
	checks := containerChecks(t, h, c.ID)
	if len(checks) != 1 || checks[0].Path != "acme/shop/prod/web1/shop-app/web" || checks[0].Type != "http" || !strings.Contains(string(checks[0].Spec), "Welcome") && !strings.Contains(string(checks[0].Spec), `"keyword":"ok"`) {
		t.Errorf("checks: %+v", checks)
	}
	changes, _ := h.db.Changes(ctx, "container", c.ID, 10)
	if len(changes) == 0 || changes[0].Via != "label" {
		t.Errorf("history: %+v", changes)
	}

	// The same labels again: nothing happens.
	before, _ := h.db.Changes(ctx, "container", c.ID, 100)
	deploy(t, h, host, map[string]string{
		"lotse.check.web.http.url":                 "https://shop.acme.example/health",
		"lotse.check.web.http.keyword":             "ok",
		"lotse.rules.container.restarting.enabled": "false",
		"lotse.rules.container.unhealthy.for":      "5m",
		"lotse.ignore":                             "false",
	})
	if after, _ := h.db.Changes(ctx, "container", c.ID, 100); len(after) != len(before) {
		t.Errorf("the same labels changed things: %d → %d", len(before), len(after))
	}

	// Another deploy, without the check and one rule, into staging: what
	// went is undone, and the container moves.
	c = deploy(t, h, host, map[string]string{
		"lotse.env": "acme/shop/staging",
		"lotse.rules.container.restarting.enabled": "false",
	})
	set, _ = h.db.Settings(ctx, []store.Subject{{Kind: "container", ID: c.ID}})
	if _, ok := set[0]["rules.container.unhealthy.for"]; ok || len(set[0]) != 1 {
		t.Errorf("settings after: %v", set[0])
	}
	if checks := containerChecks(t, h, c.ID); len(checks) != 0 {
		t.Errorf("checks after: %+v", checks)
	}
	if c.EnvironmentID == nil || *c.EnvironmentID != envs["acme/shop/staging"].ID {
		t.Errorf("environment: %v", c.EnvironmentID)
	}
	tr, _ := h.loadTree(ctx)
	if p := tr.path("container", c.ID); p != "acme/shop/staging/web1/shop-app" {
		t.Errorf("path: %s", p)
	}

	// Not another customer's, and only the container's own rules; what is
	// wrong is said in its events.
	c = deploy(t, h, host, map[string]string{
		"lotse.env":                  "globex/web/prod",
		"lotse.rules.host.disk.warn": "50",
		"lotse.colour":               "blue",
	})
	if c.EnvironmentID != nil {
		t.Errorf("moved to another customer: %v", *c.EnvironmentID)
	}
	if set, _ := h.db.Settings(ctx, []store.Subject{{Kind: "container", ID: c.ID}}); len(set[0]) != 0 {
		t.Errorf("settings from bad labels: %v", set[0])
	}
	waitFor(t, "the problems as an event", func() bool {
		events, _ := h.db.Events(ctx, "container", c.ID, 0, time.Now().Unix()+10)
		for _, e := range events {
			if e.Kind == "container.labels" && strings.Contains(e.Data, "globex") && strings.Contains(e.Data, "rules.container") && strings.Contains(e.Data, "lotse.colour") {
				return true
			}
		}
		return false
	})
}

// An ignored container raises no alerts.
func TestIgnoredContainer(t *testing.T) {
	h, host, _ := labelHub(t)
	h.alerts.started = time.Unix(0, 0)
	h.agents.add(nil, host.ID)
	h.db.Write(t.Context(), store.Batch{Seen: map[int64]int64{host.ID: time.Now().Unix()}})
	list := protocol.ContainerList{TS: time.Now().Unix(), Containers: []protocol.Container{{DockerID: "d1", Name: "cron", State: "running", Health: "unhealthy", Labels: map[string]string{"lotse.ignore": "true", "lotse.rules.container.unhealthy.for": "0s"}}}}
	if err := h.containerList(t.Context(), host.ID, list, containerIDs{}); err != nil {
		t.Fatal(err)
	}
	h.evaluate(t.Context(), time.Now())
	h.alerts.mu.Lock()
	defer h.alerts.mu.Unlock()
	for k, st := range h.alerts.states {
		if k.SubjectKind == "container" {
			t.Errorf("an alert on an ignored container: %+v", st)
		}
	}
}

func TestLabelChecksNested(t *testing.T) {
	checks, problems := labelChecks(map[string]string{
		"lotse.check.api.http.url":              "https://api.acme.example/health",
		"lotse.check.api.http.headers.X-Token":  "abc",
		"lotse.check.api.http.json.path":        "status",
		"lotse.check.api.http.json.equals":      "UP",
		"lotse.check.api.http.follow_redirects": "false",
	})
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	b, _ := json.Marshal(checks["api"].spec)
	spec, err := probe.Normalize(checks["api"].typ, b)
	if err != nil || string(spec) != `{"url":"https://api.acme.example/health","headers":{"X-Token":"abc"},"follow_redirects":false,"json":{"path":"status","equals":"UP"}}` {
		t.Errorf("%s %v", spec, err)
	}
}
