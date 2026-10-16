package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// joinAs invites someone and accepts the invitation in a browser of their
// own, whose cookies it returns. The hub's client is left with the admin's.
func joinAs(t *testing.T, server *httptest.Server, email, role string, scopeID int64) http.CookieJar {
	t.Helper()
	admin := server.Client().Jar
	status, body := do(t, server, "POST", "/api/users", fmt.Sprintf(`{"email":%q,"role":%q,"scope_id":%d}`, email, role, scopeID))
	if status != http.StatusCreated {
		t.Fatalf("invite %s: %d %s", email, status, body)
	}
	var inv api.UserLink
	json.Unmarshal([]byte(body), &inv)
	token := inv.Link[strings.Index(inv.Link, "#")+1:]
	jar, _ := cookiejar.New(nil)
	server.Client().Jar = jar
	if status, body := do(t, server, "POST", "/api/auth/link/password", `{"token":"`+token+`","password":"a long password"}`); status != 200 {
		t.Fatalf("accept %s: %d %s", email, status, body)
	}
	server.Client().Jar = admin
	return jar
}

// accessHub has two customers, ACME and Globex, with a host and a check
// each, and a host in no environment whose name is ACME's key.
func accessHub(t *testing.T) (*hub, *httptest.Server, map[string]int64) {
	t.Helper()
	h, server := newTestHub(t)
	ctx := t.Context()
	ids := map[string]int64{}
	for _, c := range []string{"acme", "globex"} {
		env, err := h.db.EnsureEnvironment(ctx, ada(), [3]store.ScopeName{{Slug: c, Name: c}, {Slug: "web", Name: "Web"}, {Slug: "prod", Name: "Prod"}})
		if err != nil {
			t.Fatal(err)
		}
		host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: c, Hostname: c + "-1", LastSeen: 1})
		h.db.SetHostEnvironment(ctx, ada(), host.ID, &env.ID)
		check, err := h.db.CreateCheck(ctx, ada(), store.Check{EnvironmentID: env.ID, Slug: "home", Name: "Home", Type: "tcp", Spec: []byte(`{"address":"127.0.0.1:1"}`), CreatedAt: 1}, "")
		if err != nil {
			t.Fatal(err)
		}
		project, _ := h.db.Scope(ctx, *env.ParentID)
		ids[c], ids[c+"-env"], ids[c+"-host"], ids[c+"-check"] = *project.ParentID, env.ID, host.ID, check.ID
	}
	stray, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "stray", Hostname: "acme", LastSeen: 1})
	ids["stray"] = stray.ID
	return h, server, ids
}

// A viewer kept to ACME sees ACME, and nothing else: not Globex, not the
// host in no environment, not what belongs to the whole hub.
func TestScopedViewer(t *testing.T) {
	_, server, ids := accessHub(t)
	server.Client().Jar = joinAs(t, server, "v@acme.example", "viewer", ids["acme"])
	path := func(f string, a ...any) string { return fmt.Sprintf(f, a...) }

	var hosts []api.HostSummary
	getJSON(t, server, "/api/hosts", &hosts)
	if len(hosts) != 1 || hosts[0].Host.Id != ids["acme-host"] {
		t.Errorf("hosts: %+v", hosts)
	}
	var scopes []api.Scope
	getJSON(t, server, "/api/scopes", &scopes)
	if len(scopes) != 3 {
		t.Errorf("scopes: %+v", scopes)
	}
	var checks []api.Check
	getJSON(t, server, "/api/checks", &checks)
	if len(checks) != 1 || checks[0].Id != ids["acme-check"] {
		t.Errorf("checks: %+v", checks)
	}
	var v1 []api.HostSummary
	getJSON(t, server, "/api/v1/hosts", &v1)
	if len(v1) != 1 {
		t.Errorf("read API hosts: %+v", v1)
	}
	for _, p := range []string{
		path("/api/hosts/%d", ids["acme-host"]),
		path("/api/checks/%d", ids["acme-check"]),
		path("/api/scopes/%d", ids["acme"]),
		path("/api/config?subject=customer:%d", ids["acme"]),
		path("/api/config?subject=host:%d", ids["acme-host"]),
	} {
		if status, body := do(t, server, "GET", p, ""); status != 200 {
			t.Errorf("%s: %d %s", p, status, body)
		}
	}
	for _, p := range []string{
		path("/api/hosts/%d", ids["globex-host"]),
		path("/api/hosts/%d", ids["stray"]),
		path("/api/hosts/%d/series", ids["globex-host"]),
		path("/api/checks/%d", ids["globex-check"]),
		path("/api/checks/%d/series", ids["globex-check"]),
		path("/api/scopes/%d", ids["globex"]),
		path("/api/config?subject=customer:%d", ids["globex"]),
		"/api/config?subject=global:1",
		path("/api/v1/hosts/%d", ids["globex-host"]),
	} {
		if status, body := do(t, server, "GET", p, ""); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", p, status, body)
		}
	}
	for _, p := range []string{
		path("/api/changes?subject=customer:%d", ids["globex"]),
		"/api/changes?subject=global:1",
		path("/api/events?subject=host:%d", ids["globex-host"]),
		path("/api/checks/%d/results", ids["globex-check"]),
	} {
		if status, body := do(t, server, "GET", p, ""); status != 200 || body != "[]" {
			t.Errorf("%s: %d %s", p, status, body)
		}
	}
	for _, p := range []string{"/api/users", "/api/tokens", "/api/probes", "/api/mail-server"} {
		if status, _ := do(t, server, "GET", p, ""); status != http.StatusForbidden {
			t.Errorf("%s: %d", p, status)
		}
	}
	if status, _ := do(t, server, "PUT", "/api/settings", path(`{"subject":"host:%d","key":"agent.interval.host","value":"1m"}`, ids["acme-host"])); status != http.StatusForbidden {
		t.Errorf("a viewer changes a setting: %d", status)
	}
	if status, _ := do(t, server, "GET", "/api/v1/export", ""); status != http.StatusForbidden {
		t.Errorf("a viewer exports: %d", status)
	}
}

// An admin kept to ACME changes ACME, and only ACME.
func TestScopedAdmin(t *testing.T) {
	_, server, ids := accessHub(t)
	server.Client().Jar = joinAs(t, server, "a@acme.example", "admin", ids["acme"])
	path := func(f string, a ...any) string { return fmt.Sprintf(f, a...) }

	setting := `{"subject":"%s","key":"agent.interval.host","value":"1m"}`
	if status, body := do(t, server, "PUT", "/api/settings", path(setting, path("host:%d", ids["acme-host"]))); status != http.StatusNoContent {
		t.Errorf("a setting on ACME's host: %d %s", status, body)
	}
	for _, subject := range []string{path("host:%d", ids["globex-host"]), path("host:%d", ids["stray"]), "global:1"} {
		if status, _ := do(t, server, "PUT", "/api/settings", path(setting, subject)); status != http.StatusNotFound {
			t.Errorf("a setting on %s: %d", subject, status)
		}
	}
	check := `{"environment_id":%d,"name":"Shop","type":"tcp","spec":{"address":"127.0.0.1:2"}}`
	if status, body := do(t, server, "POST", "/api/checks", path(check, ids["acme-env"])); status != http.StatusCreated {
		t.Errorf("a check in ACME: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/checks", path(check, ids["globex-env"])); status != http.StatusBadRequest {
		t.Errorf("a check in Globex: %d", status)
	}
	if status, _ := do(t, server, "DELETE", path("/api/checks/%d", ids["globex-check"]), ""); status != http.StatusNotFound {
		t.Errorf("deleting Globex's check: %d", status)
	}
	env := `{"customer":{"slug":"%s","name":"X"},"project":{"slug":"web","name":"Web"},"environment":{"slug":"dev","name":"Dev"}}`
	if status, body := do(t, server, "POST", "/api/environments", path(env, "acme")); status != http.StatusCreated {
		t.Errorf("an environment in ACME: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/environments", path(env, "initech")); status != http.StatusForbidden {
		t.Errorf("a new customer: %d", status)
	}
	if status, _ := do(t, server, "PUT", path("/api/hosts/%d/environment", ids["acme-host"]), path(`{"environment_id":%d}`, ids["globex-env"])); status != http.StatusBadRequest {
		t.Errorf("moving a host to Globex: %d", status)
	}
	if status, _ := do(t, server, "PUT", path("/api/hosts/%d/environment", ids["acme-host"]), `{}`); status != http.StatusBadRequest {
		t.Errorf("moving a host out of every environment: %d", status)
	}
	channel := `{"scope_id":%d,"key":"ops","name":"Ops","type":"slack","spec":{},"secrets":{"url":{"value":"https://hooks.slack.com/services/x"}}}`
	if status, body := do(t, server, "POST", "/api/channels", path(channel, ids["acme"])); status != http.StatusCreated {
		t.Errorf("a channel for ACME: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/channels", path(channel, 1)); status != http.StatusBadRequest {
		t.Errorf("a global channel: %d", status)
	}
	// The hub's environment and secret files are the whole hub's.
	ref := `{"scope_id":%d,"key":"leak","name":"Leak","type":"slack","spec":{},"secrets":{"url":{"env":"GLOBEX_SLACK_URL"}}}`
	if status, body := do(t, server, "POST", "/api/channels", path(ref, ids["acme"])); status != http.StatusBadRequest || !strings.Contains(body, "whole hub") {
		t.Errorf("a channel reading the hub's environment: %d %s", status, body)
	}
	var channels []api.Channel
	getJSON(t, server, "/api/channels", &channels)
	if len(channels) != 1 {
		t.Errorf("channels: %+v", channels)
	}
	if status, _ := do(t, server, "POST", "/api/maintenance", `{"subject":"global:1","schedule":"sun 02:00-04:00"}`); status != http.StatusNotFound {
		t.Errorf("a global maintenance window: %d", status)
	}
	if status, _ := do(t, server, "GET", "/api/users", ""); status != http.StatusForbidden {
		t.Errorf("users: %d", status)
	}
	status, body := do(t, server, "GET", "/api/v1/export", "")
	if status != 200 || !strings.Contains(body, "acme") || strings.Contains(body, "globex") {
		t.Errorf("export: %d %s", status, body)
	}
	if status, _ := do(t, server, "DELETE", path("/api/scopes/%d", ids["globex"]), ""); status != http.StatusNotFound {
		t.Errorf("deleting Globex: %d", status)
	}
	// ACME's enrollment token does not take Globex's machine.
	_, body = do(t, server, "POST", path("/api/scopes/%d/enrollment", ids["acme-env"]), "")
	var e api.Enrollment
	json.Unmarshal([]byte(body), &e)
	if status, body := do(t, server, "POST", protocol.EnrollPath, `{"token":"`+e.Token+`","hostname":"x","machine_id":"globex"}`); status != http.StatusConflict {
		t.Errorf("enrolling Globex's machine: %d %s", status, body)
	}
	if status, _ := do(t, server, "GET", path("/api/hosts/%d", ids["globex-host"]), ""); status != http.StatusNotFound {
		t.Errorf("Globex's host afterwards: %d", status)
	}
}

// A customer that users are kept to stays; the hub keeps an admin of the
// whole hub.
func TestScopedUsersStay(t *testing.T) {
	h, server, ids := accessHub(t)
	joinAs(t, server, "v@acme.example", "viewer", ids["acme"])
	var users []api.User
	getJSON(t, server, "/api/users", &users)
	if len(users) != 2 || users[1].ScopeId != ids["acme"] || users[1].ScopePath != "acme" {
		t.Fatalf("users: %+v", users)
	}
	// Emptied, ACME still has a user kept to it.
	ctx := t.Context()
	h.db.DeleteCheck(ctx, ada(), ids["acme-check"])
	h.db.SetHostEnvironment(ctx, ada(), ids["acme-host"], nil)
	project, _ := h.db.Scope(ctx, ids["acme-env"])
	h.db.DeleteScope(ctx, ada(), ids["acme-env"])
	h.db.DeleteScope(ctx, ada(), *project.ParentID)
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/scopes/%d", ids["acme"]), ""); status != http.StatusConflict {
		t.Errorf("deleting a customer a user is kept to: %d", status)
	}
	if status, _ := do(t, server, "PATCH", fmt.Sprintf("/api/users/%d", users[1].Id), `{"scope_id":1}`); status != 200 {
		t.Errorf("give the whole hub: %d", status)
	}
	if status, body := do(t, server, "DELETE", fmt.Sprintf("/api/scopes/%d", ids["acme"]), ""); status != http.StatusNoContent {
		t.Errorf("then deleting it: %d %s", status, body)
	}
}

// The tree tells what is in a scope by what objects belong to, not by
// their paths: a host in no environment named like a customer is not in it.
func TestTreeSees(t *testing.T) {
	h, _, ids := accessHub(t)
	tr, err := h.loadTree(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind  string
		id    int64
		scope int64
		want  bool
	}{
		{"host", ids["acme-host"], ids["acme"], true},
		{"host", ids["globex-host"], ids["acme"], false},
		{"host", ids["stray"], ids["acme"], false},
		{"host", ids["stray"], store.GlobalScope, true},
		{"check", ids["acme-check"], ids["acme-env"], true},
		{"environment", ids["acme-env"], ids["acme"], true},
		{"customer", ids["acme"], ids["acme-env"], false},
		{"global", store.GlobalScope, ids["acme"], false},
		{"probe", 1, ids["acme"], false},
	} {
		if got := tr.sees(c.kind, c.id, c.scope); got != c.want {
			t.Errorf("%s:%d in scope %d: %v", c.kind, c.id, c.scope, got)
		}
	}
}

// A browser kept to a scope hears only about the hosts in it.
func TestLiveVisible(t *testing.T) {
	var l live
	c := newLiveClient()
	l.subscribe(c, []string{"hosts", "host:1", "host:2", "alerts"}, &liveAccess{hosts: map[int64]bool{1: true}, hidden: map[int64]bool{7: true}})
	if !c.topics["host:1"] || c.topics["host:2"] {
		t.Errorf("topics: %v", c.topics)
	}
	l.publish(LiveMessage{Type: "host", HostID: 2})
	l.publish(LiveMessage{Type: "container_stats", HostID: 1, Containers: []LiveContainerReading{{ContainerID: 6}, {ContainerID: 7}}})
	l.publish(LiveMessage{Type: "alerts"})
	if m := <-c.send; m.HostID != 1 || len(m.Containers) != 1 || m.Containers[0].ContainerID != 6 {
		t.Errorf("first message: %+v", m)
	}
	if m := <-c.send; m.Type != "alerts" {
		t.Errorf("second message: %+v", m)
	}
	if len(c.send) != 0 {
		t.Errorf("%d more messages", len(c.send))
	}
}

// An admin kept to a scope does not point a channel that sends with the
// hub's own secrets elsewhere, applies only within the scope, and can apply
// back what export gives, references and all.
func TestScopedChannelsAndApply(t *testing.T) {
	_, server, ids := accessHub(t)
	t.Setenv("ACME_CHAT", "https://chat.googleapis.com/v1/spaces/x/messages?key=k")
	status, body := do(t, server, "POST", "/api/channels", fmt.Sprintf(`{"scope_id":%d,"key":"chat","name":"Chat","type":"googlechat","spec":{},"secrets":{"url":{"env":"ACME_CHAT"}}}`, ids["acme"]))
	if status != http.StatusCreated {
		t.Fatalf("a channel with a reference: %d %s", status, body)
	}
	var ch api.Channel
	json.Unmarshal([]byte(body), &ch)
	t.Setenv("HUB_NTFY_TOKEN", "tk_hub")
	_, body = do(t, server, "POST", "/api/channels", fmt.Sprintf(`{"scope_id":%d,"key":"push","name":"Push","type":"ntfy","spec":{"server":"https://ntfy.sh"},"secrets":{"topic":{"value":"acme"},"token":{"env":"HUB_NTFY_TOKEN"}}}`, ids["acme"]))
	var push api.Channel
	json.Unmarshal([]byte(body), &push)
	scoped := joinAs(t, server, "a@acme.example", "admin", ids["acme-env"])
	server.Client().Jar = joinAs(t, server, "c@acme.example", "admin", ids["acme"])

	// The customer's admin may rename it, not point it elsewhere.
	if status, body := do(t, server, "PATCH", fmt.Sprintf("/api/channels/%d", ch.Id), `{"name":"Team chat","spec":{}}`); status != 200 {
		t.Errorf("rename: %d %s", status, body)
	}
	if status, body := do(t, server, "PATCH", fmt.Sprintf("/api/channels/%d", ch.Id), `{"name":"Chat","spec":{},"secrets":{"url":{"env":"OTHER_CHAT"}}}`); status != http.StatusBadRequest {
		t.Errorf("another reference: %d %s", status, body)
	}
	if status, body := do(t, server, "PATCH", fmt.Sprintf("/api/channels/%d", push.Id), `{"name":"Push","spec":{"server":"https://attacker.example"}}`); status != http.StatusBadRequest || !strings.Contains(body, "whole hub") {
		t.Errorf("pointing a channel with the hub's token elsewhere: %d %s", status, body)
	}
	_, out := do(t, server, "GET", "/api/v1/export", "")
	if status, body := do(t, server, "POST", "/api/v1/apply", yamlToJSON(t, out)); status != 200 {
		t.Errorf("applying its own export, with the reference: %d %s\n%s", status, body, out)
	}

	// An environment's admin names the customer above only to say where.
	server.Client().Jar = scoped
	doc := `{"customers":[{"key":"acme","settings":{"notify.repeat":"5h"}}]}`
	if status, body := do(t, server, "POST", "/api/v1/apply", doc); status != http.StatusBadRequest || !strings.Contains(body, "outside") {
		t.Errorf("a setting on the customer above: %d %s", status, body)
	}
	_, out = do(t, server, "GET", "/api/v1/export", "")
	if status, body := do(t, server, "POST", "/api/v1/apply", yamlToJSON(t, out)); status != 200 {
		t.Errorf("an environment applying its own export: %d %s\n%s", status, body, out)
	}
}

// yamlToJSON is a lotse.yaml as the apply API takes it.
func yamlToJSON(t *testing.T, doc string) string {
	t.Helper()
	d, err := apply.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	return string(b)
}
