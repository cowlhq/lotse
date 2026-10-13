package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

func TestScopesAPI(t *testing.T) {
	h, server := newTestHub(t)
	status, body := do(t, server, "POST", "/api/environments",
		`{"customer":{"slug":"acme","name":"ACME"},"project":{"slug":"shop","name":"Shop"},"environment":{"slug":"prod","name":"Production"}}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var env api.Scope
	json.Unmarshal([]byte(body), &env)
	if env.Path != "acme/shop/prod" || env.Kind != "environment" {
		t.Errorf("environment: %+v", env)
	}
	for _, bad := range []string{
		`{"customer":{"slug":"ACME","name":"x"},"project":{"slug":"a","name":"x"},"environment":{"slug":"b","name":"x"}}`,
		`{"customer":{"slug":"-a","name":"x"},"project":{"slug":"a","name":"x"},"environment":{"slug":"b","name":"x"}}`,
		`{"customer":{"slug":"a","name":""},"project":{"slug":"a","name":"x"},"environment":{"slug":"b","name":"x"}}`,
	} {
		if status, _ := do(t, server, "POST", "/api/environments", bad); status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, status)
		}
	}

	var scopes []api.Scope
	getJSON(t, server, "/api/scopes", &scopes)
	if len(scopes) != 3 || scopes[0].Kind != "customer" || scopes[0].ParentId != nil ||
		scopes[2].ParentId == nil || *scopes[2].ParentId != scopes[1].Id {
		t.Errorf("scopes: %+v", scopes)
	}

	if status, _ := do(t, server, "PATCH", fmt.Sprintf("/api/scopes/%d", scopes[0].Id), `{"name":"ACME AG"}`); status != 200 {
		t.Errorf("rename: %d", status)
	}
	var changes []api.Change
	getJSON(t, server, fmt.Sprintf("/api/changes?subject=customer:%d", scopes[0].Id), &changes)
	if len(changes) != 2 || changes[0].Field != "name" || changes[0].ActorName != "admin@example.com" || changes[0].Via != "ui" {
		t.Errorf("history: %+v", changes)
	}
	raw, _ := json.Marshal(changes[1])
	if strings.Contains(string(raw), `"old"`) {
		t.Errorf("a creation has an old value: %s", raw)
	}

	host, _, _ := h.db.SaveHost(t.Context(), store.Host{MachineID: "m", Hostname: "web1", LastSeen: 1})
	if status, _ := do(t, server, "PUT", fmt.Sprintf("/api/hosts/%d/environment", host.ID), fmt.Sprintf(`{"environment_id":%d}`, env.Id)); status != http.StatusNoContent {
		t.Errorf("move host: %d", status)
	}
	if status, _ := do(t, server, "PUT", fmt.Sprintf("/api/hosts/%d/environment", host.ID), fmt.Sprintf(`{"environment_id":%d}`, scopes[0].Id)); status != http.StatusBadRequest {
		t.Errorf("move host into a customer: %d", status)
	}
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/scopes/%d", env.Id), ""); status != http.StatusConflict {
		t.Errorf("delete an environment with a host: %d", status)
	}

	// A viewer reads, but changes nothing.
	hash, _ := auth.HashPassword("viewer password")
	v, _ := h.db.CreateUser(t.Context(), store.User{Email: "viewer@example.com", PasswordHash: hash, Role: "viewer", CreatedAt: 1})
	cookie, _ := h.startSession(t.Context(), v.ID)
	c, _ := http.ParseSetCookie(cookie)
	server.Client().Jar.SetCookies(mustURL(server.URL), []*http.Cookie{c})
	if status, _ := do(t, server, "GET", "/api/scopes", ""); status != 200 {
		t.Errorf("viewer reads: %d", status)
	}
	if status, _ := do(t, server, "PATCH", fmt.Sprintf("/api/scopes/%d", scopes[0].Id), `{"name":"x"}`); status != http.StatusForbidden {
		t.Errorf("viewer renames: %d", status)
	}
}
