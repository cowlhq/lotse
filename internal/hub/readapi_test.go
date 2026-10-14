package hub

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lotse.sh/client"
	"lotse.sh/internal/store"
)

// The read API through the client, with a token for ACME only.
func TestReadAPI(t *testing.T) {
	h, server := newTestHub(t)
	ctx := t.Context()
	acmeEnv := testEnvironment(t, h)
	globex, _ := h.db.EnsureEnvironment(ctx, ada(), [3]store.ScopeName{{Slug: "globex", Name: "Globex"}, {Slug: "web", Name: "Web"}, {Slug: "prod", Name: "Prod"}})
	for i, env := range []store.Scope{acmeEnv, globex} {
		host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: []string{"a", "b"}[i], Hostname: []string{"web1", "gx1"}[i], LastSeen: 1})
		h.db.SetHostEnvironment(ctx, ada(), host.ID, &env.ID)
		cpu := 42.0
		h.db.Write(ctx, store.Batch{HostSamples: []store.HostSample{{HostID: host.ID, TS: time.Now().Unix() - 30, CPU: &cpu}}})
		h.db.CreateCheck(ctx, ada(), store.Check{EnvironmentID: env.ID, Slug: "home", Name: "Home", Type: "tcp", Spec: []byte(`{"address":"127.0.0.1:1"}`), CreatedAt: 1}, "")
	}
	project, _ := h.db.Scope(ctx, *acmeEnv.ParentID)
	tok := token(t, server, "read", *project.ParentID)
	hub, err := client.New(server.URL, tok)
	if err != nil {
		t.Fatal(err)
	}

	hosts, err := hub.Hosts(ctx)
	if err != nil || len(hosts) != 1 || hosts[0].Host.Name != "web1" || hosts[0].Latest == nil || *hosts[0].Latest.Cpu != 42 {
		t.Fatalf("hosts: %+v, %v", hosts, err)
	}
	checks, _ := hub.Checks(ctx)
	if len(checks) != 1 || checks[0].Path != "acme/shop/prod/home" {
		t.Errorf("checks: %+v", checks)
	}
	scopes, _ := hub.Scopes(ctx)
	if len(scopes) != 3 {
		t.Errorf("scopes: %+v", scopes)
	}
	status, err := hub.Status(ctx)
	if err != nil || status.Hosts != 1 || status.Checks != 1 || status.Level != client.StatusLevelOk {
		t.Errorf("status: %+v, %v", status, err)
	}
	series, err := hub.HostSeries(ctx, hosts[0].Host.Id, client.Range{From: time.Now().Add(-time.Hour)})
	if err != nil || len(series.Time) == 0 {
		t.Errorf("series: %+v, %v", series, err)
	}

	// Globex's host is not there for this token, as if it did not exist.
	all, _ := h.db.Hosts(ctx)
	var gx int64
	for _, x := range all {
		if x.Name == "gx1" {
			gx = x.ID
		}
	}
	var e *client.APIError
	if _, err := hub.Host(ctx, gx); !errors.As(err, &e) || e.Status != 404 {
		t.Errorf("another customer's host: %v", err)
	}
	if out, err := hub.Export(ctx, ""); err != nil || len(out) == 0 {
		t.Errorf("export: %v", err)
	}

	// A token that does not work says so.
	bad, _ := client.New(server.URL, "lotse_tok_nope")
	if _, err := bad.Hosts(context.Background()); !errors.As(err, &e) || e.Status != 401 {
		t.Errorf("a wrong token: %v", err)
	}
}

// The API's description is served, to anyone.
func TestOpenAPI(t *testing.T) {
	_, server := newEmptyHub(t)
	status, body := do(t, server, "GET", "/api/openapi.yaml", "")
	if status != 200 || !strings.Contains(body, "operationId: readHosts") {
		t.Errorf("openapi: %d", status)
	}
}
