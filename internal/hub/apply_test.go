package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

const shopYAML = `
settings:
  notify.channels: [ops]
channels:
  - key: ops
    webhook:
      url: {from_env: OPS_HOOK}
customers:
  - key: acme
    name: ACME GmbH
    channels:
      - key: acme-chat
        googlechat:
          webhook: {env: ACME_CHAT}
    projects:
      - key: shop
        name: Shop
        environments:
          - key: prod
            name: Production
            settings:
              rules.host.disk.warn: 80
            maintenance:
              - schedule: sun 02:00-04:00
                reason: backups
            checks:
              - key: homepage
                name: Homepage
                http: {url: "https://shop.acme.example", keyword: Willkommen}
              - key: nightly-backup
                heartbeat: {period: 24h, grace: 1h}
            hosts:
              - key: web9
`

// token makes an API token and returns its secret.
func token(t *testing.T, server *httptest.Server, kind string, scope int64) string {
	t.Helper()
	status, body := do(t, server, "POST", "/api/tokens", fmt.Sprintf(`{"name":"ci","kind":%q,"scope_id":%d}`, kind, scope))
	if status != http.StatusCreated {
		t.Fatalf("token: %d %s", status, body)
	}
	var nt api.NewToken
	json.Unmarshal([]byte(body), &nt)
	return nt.Secret
}

// withToken sends a request as lotse apply does: with a bearer token, no
// session.
func withToken(t *testing.T, server *httptest.Server, tok, method, path string, body []byte) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func applyYAML(t *testing.T, server *httptest.Server, tok, yaml, query string) (int, api.ApplyResult, string) {
	t.Helper()
	d, err := apply.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ResolveFromEnv(func(k string) string { return "https://hooks.example/" + k }); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	status, body := withToken(t, server, tok, "POST", "/api/v1/apply"+query, b)
	var res api.ApplyResult
	json.Unmarshal([]byte(body), &res)
	return status, res, body
}

func TestApply(t *testing.T) {
	h, server := newTestHub(t)
	tok := token(t, server, "apply", 1)

	// A dry run says what would happen, and changes nothing.
	status, res, body := applyYAML(t, server, tok, shopYAML, "?dry_run=true")
	if status != http.StatusOK || res.Applied || len(res.Changes) == 0 {
		t.Fatalf("dry run: %d %s", status, body)
	}
	if list, _ := h.db.Scopes(t.Context()); len(list) != 0 {
		t.Errorf("a dry run made %d scopes", len(list))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "web9 has not enrolled") {
		t.Errorf("warnings: %v", res.Warnings)
	}

	status, res, body = applyYAML(t, server, tok, shopYAML, "")
	if status != http.StatusOK || !res.Applied {
		t.Fatalf("apply: %d %s", status, body)
	}
	kinds := map[string]int{}
	for _, c := range res.Changes {
		kinds[string(c.Action)+" "+c.Kind]++
	}
	if kinds["create environment"] != 1 || kinds["create check"] != 2 || kinds["create channel"] != 2 || kinds["create setting"] != 2 || kinds["create maintenance"] != 1 {
		t.Errorf("changes: %v", kinds)
	}
	checks, _ := h.db.Checks(t.Context(), 0)
	if len(checks) != 2 || checks[0].Path != "acme/shop/prod/homepage" || checks[1].PingSealed == nil {
		t.Errorf("checks: %+v", checks)
	}
	channels, _ := h.db.Channels(t.Context())
	ops, _ := h.openSecrets(channels[0].Secrets, channels[0].SecretRefs)
	if v, _ := ops.get("url"); v != "https://hooks.example/OPS_HOOK" {
		t.Errorf("from_env, sealed: %q", v)
	}
	changes, _ := h.db.Changes(t.Context(), "environment", checks[0].EnvironmentID, 10)
	if len(changes) == 0 || changes[0].Via != "apply" || changes[0].ActorKind != "token" {
		t.Errorf("history: %+v", changes)
	}

	// Again: nothing to do.
	_, res, _ = applyYAML(t, server, tok, shopYAML, "")
	if len(res.Changes) != 0 {
		t.Errorf("again: %+v", res.Changes)
	}

	// Export, and apply what came out: nothing to do either. Sealed
	// secrets come out as placeholders.
	status, out := withToken(t, server, tok, "GET", "/api/v1/export", nil)
	if status != http.StatusOK || strings.Contains(out, "hooks.example") || !strings.Contains(out, "secret: ops") || !strings.Contains(out, "env: ACME_CHAT") {
		t.Fatalf("export: %d\n%s", status, out)
	}
	_, res, body = applyYAML(t, server, tok, out, "")
	if len(res.Changes) != 0 {
		t.Errorf("export applied: %s", body)
	}

	// A change, with who changed it last.
	changed := strings.Replace(shopYAML, "rules.host.disk.warn: 80", "rules.host.disk.warn: 75", 1)
	_, res, _ = applyYAML(t, server, tok, changed, "?dry_run=true")
	if len(res.Changes) != 1 || res.Changes[0].Action != "update" || res.Changes[0].Field == nil || *res.Changes[0].Field != "rules.host.disk.warn" {
		t.Errorf("an update: %+v", res.Changes)
	}

	// Prune within acme: what the document leaves out goes.
	less := strings.Replace(shopYAML, `              - key: nightly-backup
                heartbeat: {period: 24h, grace: 1h}
`, "", 1)
	_, res, body = applyYAML(t, server, tok, less, "?prune=true&scope=acme")
	deleted := []string{}
	for _, c := range res.Changes {
		if c.Action == "delete" {
			deleted = append(deleted, c.Kind+" "+c.Path)
		}
	}
	if len(deleted) != 1 || deleted[0] != "check acme/shop/prod/nightly-backup" {
		t.Errorf("pruned: %v (%s)", deleted, body)
	}

	// Mistakes are said in the document's terms.
	for doc, want := range map[string]string{
		"customers:\n  - key: acme\n    settings:\n      no.such: 1\n":                 "no setting",
		"customers:\n  - key: acme\n    settings:\n      rules.host.disk.warn: high\n": "rules.host.disk.warn",
		"customers:\n  - key: Acme Corp\n":                                             "not a key",
		"settings:\n  check.interval: 1s\n":                                            "check.interval",
	} {
		if status, _, body := applyYAML(t, server, tok, doc, ""); status != http.StatusBadRequest || !strings.Contains(body, want) {
			t.Errorf("%q: %d %s", doc, status, body)
		}
	}
}

func TestApplyToken(t *testing.T) {
	h, server := newTestHub(t)
	env := testEnvironment(t, h)
	acme := *env.ParentID
	project, _ := h.db.Scope(t.Context(), acme)
	customer := *project.ParentID
	tok := token(t, server, "apply", customer)

	// A token for acme cannot touch globex, nor the global settings.
	for doc, want := range map[string]string{
		"customers:\n  - key: globex\n":               "outside",
		"settings:\n  notify.repeat: 1h\n":            "outside",
		"customers:\n  - key: acme\n    name: ACME\n": "",
	} {
		status, _, body := applyYAML(t, server, tok, doc, "")
		if want == "" && status != http.StatusOK || want != "" && (status != http.StatusBadRequest || !strings.Contains(body, want)) {
			t.Errorf("%q: %d %s", doc, status, body)
		}
	}
	// It exports only acme.
	h.db.EnsureEnvironment(t.Context(), ada(), [3]store.ScopeName{{Slug: "globex", Name: "Globex"}, {Slug: "web", Name: "Web"}, {Slug: "prod", Name: "Prod"}})
	if _, out := withToken(t, server, tok, "GET", "/api/v1/export", nil); strings.Contains(out, "globex") || !strings.Contains(out, "acme") {
		t.Errorf("export:\n%s", out)
	}
	// A read token cannot apply; a wrong token does nothing.
	read := token(t, server, "read", 1)
	if status, _, _ := applyYAML(t, server, read, "customers:\n  - key: acme\n", ""); status != http.StatusForbidden {
		t.Errorf("a read token applied: %d", status)
	}
	if status, _, _ := applyYAML(t, server, "lotse_tok_wrong", "", ""); status != http.StatusUnauthorized {
		t.Errorf("a wrong token: %d", status)
	}
	// Tokens do not open the rest of the API.
	if status, _ := withToken(t, server, tok, "GET", "/api/hosts", nil); status != http.StatusUnauthorized {
		t.Errorf("a token on the UI's API: %d", status)
	}
}
