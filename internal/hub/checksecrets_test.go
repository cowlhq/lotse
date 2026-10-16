package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// probeSpecOf is the spec the hub hands a check's probes, and why it
// cannot, if it cannot.
func probeSpecOf(t *testing.T, h *hub, id int64) (map[string]any, string) {
	t.Helper()
	h.reassign(t.Context())
	h.checks.mu.Lock()
	a := h.checks.assigned[id]
	h.checks.mu.Unlock()
	var spec map[string]any
	json.Unmarshal(a.Spec, &spec)
	return spec, a.Fail
}

// A check's secrets are kept apart from its spec, shown only as where they
// come from, and given to its probes as headers.
func TestCheckSecrets(t *testing.T) {
	h, server, ids := accessHub(t)
	t.Setenv("ACME_API_TOKEN", "Bearer tk_env")
	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"API","type":"http",
		"spec":{"url":"https://api.acme.example/health","headers":{"X-Api-Version":"2"},"basic_auth":{"username":"monitor"}},
		"secrets":{"headers.X-Token":{"value":"tk_sealed"},"basic_auth.password":{"env":"ACME_API_TOKEN"}}}`, ids["acme-env"]))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	if strings.Contains(body, "tk_") {
		t.Errorf("a secret in the answer: %s", body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	if c.Secrets == nil || !(*c.Secrets)["headers.X-Token"].Set || ptrValue((*c.Secrets)["basic_auth.password"].Env) != "ACME_API_TOKEN" {
		t.Errorf("secrets: %+v", c.Secrets)
	}
	stored, _ := h.db.Check(t.Context(), c.Id)
	if strings.Contains(string(stored.Spec), "tk_") || stored.Secrets == nil || strings.Contains(*stored.Secrets, "tk_") {
		t.Errorf("stored: %s %v", stored.Spec, stored.Secrets)
	}

	spec, fail := probeSpecOf(t, h, c.Id)
	headers, _ := spec["headers"].(map[string]any)
	// monitor:Bearer tk_env
	if fail != "" || headers["X-Token"] != "tk_sealed" || headers["X-Api-Version"] != "2" || headers["Authorization"] != "Basic bW9uaXRvcjpCZWFyZXIgdGtfZW52" || spec["basic_auth"] != nil {
		t.Errorf("the probes' spec: %v %q", spec, fail)
	}

	// What does not fit.
	for _, bad := range []string{
		`"spec":{"url":"https://a.example","headers":{"X-Token":"plain"}},"secrets":{"headers.X-Token":{"value":"s"}}`,
		`"spec":{"url":"https://a.example","basic_auth":{"username":"u"}}`,
		`"spec":{"url":"https://a.example"},"secrets":{"basic_auth.password":{"value":"s"}}`,
		`"spec":{"url":"https://a.example","basic_auth":{"username":"u"}},"secrets":{"basic_auth.password":{"value":"s"},"headers.authorization":{"value":"s"}}`,
		`"spec":{"url":"https://a.example","basic_auth":{"username":"u","password":"p"}}`,
		`"spec":{"url":"https://a.example"},"secrets":{"headers.X Token":{"value":"s"}}`,
		`"spec":{"url":"https://a.example"},"secrets":{"url":{"value":"s"}}`,
	} {
		if status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Bad","type":"http",%s}`, ids["acme-env"], bad)); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, status, body)
		}
	}
	if status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Port","type":"tcp","spec":{"address":"db:5432"},"secrets":{"headers.X":{"value":"s"}}}`, ids["acme-env"])); status != http.StatusBadRequest {
		t.Errorf("a tcp check with a secret: %d %s", status, body)
	}

	// An update changes the secrets it names; an empty one removes one.
	status, body = do(t, server, "PATCH", fmt.Sprintf("/api/checks/%d", c.Id), `{"name":"API","spec":{"url":"https://api.acme.example/health","basic_auth":{"username":"monitor"}},"secrets":{"headers.X-Token":{}}}`)
	if status != 200 || strings.Contains(body, "X-Token") {
		t.Errorf("update: %d %s", status, body)
	}
	changes, _ := h.db.Changes(t.Context(), "check", c.Id, 10)
	if len(changes) == 0 || changes[0].Field != "secrets" || strings.Contains(fmt.Sprint(changes), "tk_") {
		t.Errorf("history: %+v", changes)
	}

	// A secret the hub cannot resolve: its own probe says so, and the
	// check goes down rather than quiet.
	t.Setenv("ACME_API_TOKEN", "")
	if _, fail := probeSpecOf(t, h, c.Id); !strings.Contains(fail, "ACME_API_TOKEN") {
		t.Errorf("an unset variable: %q", fail)
	}
	h.checks.mu.Lock()
	probes := h.checks.assigned[c.Id].probes
	h.checks.mu.Unlock()
	if len(probes) != 1 || probes[0] != store.BuiltinProbe {
		t.Errorf("who reports it: %v", probes)
	}

	// Viewers see no secrets.
	server.Client().Jar = joinAs(t, server, "v@acme.example", "viewer", ids["acme"])
	if _, body := do(t, server, "GET", fmt.Sprintf("/api/checks/%d", c.Id), ""); strings.Contains(body, "secrets") {
		t.Errorf("a viewer sees: %s", body)
	}
}

// An admin kept to a scope gives values, not the hub's own secrets, and
// does not point a check that sends those elsewhere.
func TestScopedCheckSecrets(t *testing.T) {
	_, server, ids := accessHub(t)
	t.Setenv("HUB_TOKEN", "tk_hub")
	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"API","type":"http","spec":{"url":"https://api.acme.example"},"secrets":{"headers.Authorization":{"env":"HUB_TOKEN"}}}`, ids["acme-env"]))
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var c api.Check
	json.Unmarshal([]byte(body), &c)
	server.Client().Jar = joinAs(t, server, "a@acme.example", "admin", ids["acme"])

	if status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Mine","type":"http","spec":{"url":"https://a.example"},"secrets":{"headers.Authorization":{"env":"HUB_TOKEN"}}}`, ids["acme-env"])); status != http.StatusBadRequest {
		t.Errorf("a reference to the hub's environment: %d %s", status, body)
	}
	if status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(`{"environment_id":%d,"name":"Mine","type":"http","spec":{"url":"https://a.example"},"secrets":{"headers.Authorization":{"value":"tk_acme"}}}`, ids["acme-env"])); status != http.StatusCreated {
		t.Errorf("a value of their own: %d %s", status, body)
	}
	if status, body := do(t, server, "PATCH", fmt.Sprintf("/api/checks/%d", c.Id), `{"name":"API v2","spec":{"url":"https://api.acme.example"}}`); status != 200 {
		t.Errorf("rename: %d %s", status, body)
	}
	if status, body := do(t, server, "PATCH", fmt.Sprintf("/api/checks/%d", c.Id), `{"name":"API","spec":{"url":"https://attacker.example"}}`); status != http.StatusBadRequest || !strings.Contains(body, "whole hub") {
		t.Errorf("pointing it elsewhere: %d %s", status, body)
	}
	_, out := do(t, server, "GET", "/api/v1/export", "")
	if status, body := do(t, server, "POST", "/api/v1/apply", yamlToJSON(t, out)); status != 200 {
		t.Errorf("applying its own export: %d %s\n%s", status, body, out)
	}
}

// lotse apply reads a check's secret where it runs, the hub seals it, and
// export writes where it comes from, or that the hub keeps it, which apply
// then leaves as it is.
func TestApplyCheckSecrets(t *testing.T) {
	h, server := newTestHub(t)
	doc := `
customers:
  - key: acme
    projects:
      - key: shop
        environments:
          - key: prod
            checks:
              - key: api
                http:
                  url: https://api.acme.example/health
                  headers:
                    X-Api-Version: "2"
                    X-Token: {from_env: API_TOKEN}
                  basic_auth: {username: monitor, password: {env: API_PASSWORD}}
`
	tok := token(t, server, "apply", 1)
	status, res, body := applyYAML(t, server, tok, doc, "")
	if status != 200 || len(res.Changes) == 0 {
		t.Fatalf("apply: %d %s", status, body)
	}
	checks, _ := h.db.Checks(t.Context(), 0)
	var c store.Check
	for _, x := range checks {
		if x.Path == "acme/shop/prod/api" {
			c = x
		}
	}
	sec, _ := h.openSecrets(c.Secrets, c.SecretRefs)
	if sec.sealed["headers.X-Token"] != "https://hooks.example/API_TOKEN" || sec.refs["basic_auth.password"].Env != "API_PASSWORD" || strings.Contains(string(c.Spec), "X-Token") {
		t.Errorf("stored: %s %+v", c.Spec, sec)
	}

	_, out := withToken(t, server, tok, "GET", "/api/v1/export", nil)
	for _, want := range []string{"X-Token:\n", "secret: api", "env: API_PASSWORD", "username: monitor", `X-Api-Version: "2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hooks.example") {
		t.Errorf("export has the secret:\n%s", out)
	}
	if status, res, body := applyYAML(t, server, tok, out, ""); status != 200 || len(res.Changes) != 0 {
		t.Errorf("its own export again: %d %s", status, body)
	}

	// Left out, a secret goes.
	status, res, body = applyYAML(t, server, tok, strings.Replace(doc, "                    X-Token: {from_env: API_TOKEN}\n", "", 1), "")
	if status != 200 || len(res.Changes) != 1 || res.Changes[0].Field == nil || *res.Changes[0].Field != "secrets" {
		t.Errorf("without the token: %d %s", status, body)
	}
}

// "Test now" runs a check once, with its secrets, and keeps nothing.
func TestTryCheck(t *testing.T) {
	h, server, ids := accessHub(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != "tk" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"status":"UP"}`))
	}))
	defer site.Close()
	try := func(body string) (int, api.CheckTryResult) {
		status, out := do(t, server, "POST", "/api/checks/try", fmt.Sprintf(`{"environment_id":%d,%s}`, ids["acme-env"], body))
		var r api.CheckTryResult
		json.Unmarshal([]byte(out), &r)
		return status, r
	}
	if status, r := try(`"type":"http","spec":{"url":"` + site.URL + `","json":{"path":"status","equals":"UP"}},"secrets":{"headers.X-Token":{"value":"tk"}}`); status != 200 || !r.Ok || ptrValue(r.StatusCode) != 200 {
		t.Errorf("up: %d %+v", status, r)
	}
	if status, r := try(`"type":"http","spec":{"url":"` + site.URL + `"}`); status != 200 || r.Ok || ptrValue(r.StatusCode) != 401 || r.Error == nil {
		t.Errorf("without the token: %d %+v", status, r)
	}
	if status, _ := try(`"type":"heartbeat","spec":{"period":"24h"}`); status != http.StatusBadRequest {
		t.Errorf("a heartbeat: %d", status)
	}
	if status, _ := try(`"type":"http","spec":{"url":"ftp://x"}`); status != http.StatusBadRequest {
		t.Errorf("a bad spec: %d", status)
	}
	if checks, _ := h.db.Checks(t.Context(), ids["acme-env"]); len(checks) != 1 {
		t.Errorf("trying added checks: %d", len(checks))
	}
	server.Client().Jar = joinAs(t, server, "v@acme.example", "viewer", ids["acme"])
	if status, _ := try(`"type":"tcp","spec":{"address":"127.0.0.1:1"}`); status != http.StatusForbidden {
		t.Errorf("a viewer: %d", status)
	}
}
