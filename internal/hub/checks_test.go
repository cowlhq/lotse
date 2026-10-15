package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

func testEnvironment(t *testing.T, h *hub) store.Scope {
	t.Helper()
	env, err := h.db.EnsureEnvironment(t.Context(), ada(), [3]store.ScopeName{
		{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func getCheck(t *testing.T, server *httptest.Server, id int64) api.CheckDetail {
	t.Helper()
	var c api.CheckDetail
	getJSON(t, server, fmt.Sprintf("/api/checks/%d", id), &c)
	return c
}

func TestHTTPCheck(t *testing.T) {
	var failing atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Welcome to the shop"))
	}))
	defer target.Close()
	h, server := newTestHub(t)
	h.checkInterval = 200 * time.Millisecond
	env := testEnvironment(t, h)

	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(
		`{"environment_id":%d,"name":"Shop homepage","type":"http","spec":{"url":%q,"keyword":"Welcome"}}`, env.ID, target.URL))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	if c.Path != "acme/shop/prod/shop-homepage" || c.State != "unknown" {
		t.Errorf("created: %+v", c)
	}
	for _, bad := range []string{
		fmt.Sprintf(`{"environment_id":%d,"name":"x","type":"http","spec":{"url":"ftp://x"}}`, env.ID),
		fmt.Sprintf(`{"environment_id":%d,"name":"x","type":"ping","spec":{}}`, env.ID),
		`{"environment_id":999,"name":"x","type":"tcp","spec":{"address":"x:1"}}`,
	} {
		if status, _ := do(t, server, "POST", "/api/checks", bad); status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, status)
		}
	}
	if status, _ := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Shop homepage","type":"tcp","spec":{"address":"x:1"}}`, env.ID)); status != http.StatusConflict {
		t.Errorf("the same key twice: %d", status)
	}

	waitFor(t, "the check to be up", func() bool { return getCheck(t, server, c.Id).State == "up" })
	failing.Store(true)
	waitFor(t, "the check to be down", func() bool { return getCheck(t, server, c.Id).State == "down" })
	// Results reach the database within a second.
	waitFor(t, "the results to be saved", func() bool { return len(getCheck(t, server, c.Id).Probes) == 1 })
	detail := getCheck(t, server, c.Id)
	if detail.Error == nil || !strings.Contains(*detail.Error, "500") || detail.Probes[0].Probe != "Hub" {
		t.Errorf("down: %+v", detail)
	}
	waitFor(t, "the events", func() bool {
		events, _ := h.db.Events(t.Context(), "check", c.Id, 0, time.Now().Unix()+10)
		return len(events) == 2 && events[0].Kind == "check.up" && events[1].Kind == "check.down"
	})
	var sr api.Series
	getJSON(t, server, fmt.Sprintf("/api/checks/%d/series?from=0", c.Id), &sr)
	if len(sr.Time) == 0 {
		t.Error("no series")
	}
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/checks/%d", c.Id), ""); status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
}

func TestHeartbeat(t *testing.T) {
	h, server := newTestHub(t)
	env := testEnvironment(t, h)
	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(
		`{"environment_id":%d,"name":"Nightly backup","type":"heartbeat","spec":{"period":"24h","grace":"1h"}}`, env.ID))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	if c.PingUrl == nil || !strings.HasPrefix(*c.PingUrl, "http://lotse.example/ping/") {
		t.Fatalf("ping url: %v", c.PingUrl)
	}
	path := strings.TrimPrefix(*c.PingUrl, "http://lotse.example")
	if status, _ := do(t, server, "GET", path, ""); status != http.StatusOK {
		t.Fatalf("ping: %d", status)
	}
	if got := getCheck(t, server, c.Id); got.State != "up" || got.LastPing == nil {
		t.Errorf("after a ping: %+v", got)
	}
	if status, _ := do(t, server, "POST", path+"/fail", ""); status != http.StatusOK {
		t.Fatalf("fail: %d", status)
	}
	if got := getCheck(t, server, c.Id); got.State != "down" {
		t.Errorf("after a failure: %s", got.State)
	}
	if status, _ := do(t, server, "GET", "/ping/not-a-token", ""); status != http.StatusNotFound {
		t.Errorf("a wrong token: %d", status)
	}
	// Pinged in a loop: a few go through, then they are turned away.
	limited := 0
	for range 20 {
		if status, _ := do(t, server, "GET", path, ""); status == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 || limited == 20 {
		t.Errorf("20 pings at once, %d turned away", limited)
	}

	// No ping in period and grace: down.
	created := time.Now().Add(-26 * time.Hour).Unix()
	stored := store.Check{Spec: json.RawMessage(`{"period":"24h","grace":"1h"}`), CreatedAt: created}
	if state, _ := heartbeatState(stored, time.Now()); state != "down" {
		t.Errorf("never pinged in 26 h: %q", state)
	}
	recent := time.Now().Add(-2 * time.Hour).Unix()
	stored.LastPing = &recent
	if state, _ := heartbeatState(stored, time.Now()); state != "" {
		t.Errorf("pinged 2 h ago: %q", state)
	}
}

func TestQuorum(t *testing.T) {
	ok, fail := probe.Result{OK: true}, probe.Result{Error: "refused"}
	for _, c := range []struct {
		results []probe.Result
		want    string
	}{
		{nil, "unknown"},
		{[]probe.Result{ok}, "up"},
		{[]probe.Result{fail}, "down"},
		{[]probe.Result{ok, fail}, "up"},
		{[]probe.Result{fail, fail, ok}, "down"},
	} {
		if got, _ := quorum(c.results); got != c.want {
			t.Errorf("%v: %s, want %s", c.results, got, c.want)
		}
	}
}

// An external probe enrolls with the probe token, gets the checks it is
// to run, and reports them.
func TestExternalProbe(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	h, server := newTestHub(t)
	h.checkInterval = 200 * time.Millisecond
	env := testEnvironment(t, h)

	status, body := do(t, server, "POST", "/api/probes/enrollment", "")
	if status != http.StatusOK {
		t.Fatalf("probe token: %d %s", status, body)
	}
	var e api.Enrollment
	json.Unmarshal([]byte(body), &e)

	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go probe.Run(ctx, probe.Config{Hub: server.URL, Token: e.Token, Slug: "fra", Name: "fra", Location: "Frankfurt", DataDir: t.TempDir()}, slog.New(slog.DiscardHandler))
	waitFor(t, "the probe to connect", func() bool { return len(h.probes.all()) == 1 })

	status, body = do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Port","type":"tcp","spec":{"address":%q}}`, env.ID, l.Addr().String()))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	// Only fra runs it.
	if status, body := do(t, server, "PUT", "/api/settings", fmt.Sprintf(`{"subject":"check:%d","key":"check.probes","value":["fra"]}`, c.Id)); status != http.StatusNoContent {
		t.Fatalf("check.probes: %d %s", status, body)
	}
	waitFor(t, "fra to report", func() bool {
		d := getCheck(t, server, c.Id)
		return d.State == "up" && len(d.Probes) > 0 && d.Probes[len(d.Probes)-1].Probe == "fra"
	})
	var probes []api.Probe
	getJSON(t, server, "/api/probes", &probes)
	if len(probes) != 2 || !probes[1].Online || probes[1].Location != "Frankfurt" {
		t.Errorf("probes: %+v", probes)
	}
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/probes/%d", probes[1].Id), ""); status != http.StatusNoContent {
		t.Errorf("revoke: %d", status)
	}
	waitFor(t, "fra to be gone", func() bool { return len(h.probes.all()) == 0 })
}

// Enroll alone keeps the credential; enrolling again under the same name,
// as running the install script again does, takes over the probe, even a
// revoked one. Run then needs no token.
func TestProbeEnrollOnly(t *testing.T) {
	h, server := newTestHub(t)
	status, body := do(t, server, "POST", "/api/probes/enrollment", "")
	if status != http.StatusOK {
		t.Fatalf("probe token: %d %s", status, body)
	}
	var e api.Enrollment
	json.Unmarshal([]byte(body), &e)
	dir := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	cfg := probe.Config{Hub: server.URL, Token: e.Token, Slug: "fra", Name: "fra", DataDir: dir}
	if err := probe.Enroll(t.Context(), probe.Config{Hub: server.URL, Slug: "fra", DataDir: dir}, log); err == nil {
		t.Error("enrolled without a token")
	}
	if err := probe.Enroll(t.Context(), cfg, log); err != nil {
		t.Fatal(err)
	}
	var probes []api.Probe
	getJSON(t, server, "/api/probes", &probes)
	if len(probes) != 2 {
		t.Fatalf("probes: %+v", probes)
	}
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/probes/%d", probes[1].Id), ""); status != http.StatusNoContent {
		t.Fatalf("revoke: %d", status)
	}
	if err := probe.Enroll(t.Context(), cfg, log); err != nil {
		t.Fatalf("enrolling again: %v", err)
	}
	getJSON(t, server, "/api/probes", &probes)
	if len(probes) != 2 || (probes[1].Revoked != nil && *probes[1].Revoked) {
		t.Errorf("after enrolling again: %+v", probes)
	}

	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go probe.Run(ctx, probe.Config{Hub: server.URL, DataDir: dir}, log)
	waitFor(t, "the probe to connect", func() bool { return len(h.probes.all()) == 1 })
}

// A probe runs the types of check it said it runs; one from before it
// could say, the old ones. Domains are the hub's own.
func TestCheckTypesPerProbe(t *testing.T) {
	var p probeConns
	p.add(nil, 7, []string{"http", "tls", "tcp", "dns"})
	for _, c := range []struct {
		id   int64
		typ  string
		runs bool
	}{
		{7, "dns", true}, {7, "domain", false}, {8, "http", true}, {8, "dns", false},
		{store.BuiltinProbe, "dns", true}, {store.BuiltinProbe, "domain", true},
	} {
		if got := p.runs(c.id, c.typ); got != c.runs {
			t.Errorf("probe %d runs %s: %v", c.id, c.typ, got)
		}
	}
}

// A domain's expiry, looked up by the hub's own probe.
func TestDomainCheck(t *testing.T) {
	var registry *httptest.Server
	registry = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dns.json" {
			fmt.Fprintf(w, `{"services":[[["com"],["%s/"]]]}`, registry.URL)
			return
		}
		fmt.Fprintf(w, `{"events":[{"eventAction":"expiration","eventDate":%q}]}`, time.Now().Add(5*24*time.Hour).UTC().Format(time.RFC3339))
	}))
	defer registry.Close()
	h, server := newTestHub(t)
	h.builtinProbe.Runner.RDAPBootstrap = registry.URL + "/dns.json"
	h.checkInterval = 200 * time.Millisecond
	env := testEnvironment(t, h)
	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Domain","type":"domain","spec":{"domain":"Example.com."}}`, env.ID))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	waitFor(t, "the expiry", func() bool {
		d := getCheck(t, server, c.Id)
		return d.State == "up" && d.CertNotAfter != nil
	})
}
