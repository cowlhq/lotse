package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/secret"
	"lotse.sh/internal/store"
)

func testShared(t *testing.T) shared {
	box, _ := secret.NewBox("a test key, long enough")
	s := shared{
		box:         box,
		log:         slog.New(slog.DiscardHandler),
		cfg:         Config{DataDir: t.TempDir()},
		setup:       &setupState{},
		publicURL:   new(atomic.Pointer[string]),
		logins:      auth.NewLimiter(5, 15*time.Minute),
		chosen:      make(chan string, 1),
		releases:    &releases{api: "http://127.0.0.1:1/none"},
		restart:     make(chan struct{}),
		restartOnce: new(sync.Once),
	}
	s.setPublicURL("http://lotse.example")
	return s
}

// newTestHub runs a hub on SQLite with an admin, whose session the
// server's client carries.
func newTestHub(t *testing.T) (*hub, *httptest.Server) {
	t.Helper()
	h, server := newEmptyHub(t)
	hash, _ := auth.HashPassword("admin password")
	u, err := h.db.CreateUser(t.Context(), store.User{Email: "admin@example.com", PasswordHash: hash, Role: "admin", CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	h.setup.finish()
	cookie, err := h.startSession(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := http.ParseSetCookie(cookie)
	server.Client().Jar.SetCookies(mustURL(server.URL), []*http.Cookie{c})
	return h, server
}

// newEmptyHub runs a hub on SQLite without users: setup is at the admin
// step.
func newEmptyHub(t *testing.T) (*hub, *httptest.Server) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "lotse.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, err := newHub(t.Context(), testShared(t), db)
	if err != nil {
		t.Fatal(err)
	}
	// Checks the hub's own probe runs end with the test.
	t.Cleanup(h.builtinProbe.Stop)
	go h.writer.run(t.Context())
	server := httptest.NewServer(h.routes())
	t.Cleanup(server.Close)
	server.Client().Jar, _ = cookiejar.New(nil)
	return h, server
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestHealth(t *testing.T) {
	_, server := newEmptyHub(t)
	var got api.Health
	getJSON(t, server, "/api/health", &got)
	if got.Status != "ok" || got.Database == nil || *got.Database != "sqlite" || got.Setup == nil || *got.Setup != api.SetupStepAdmin {
		t.Errorf("got %+v, want ok on sqlite, waiting for the admin", got)
	}
}

// The database step: no database yet, so only health and setup answer.
func TestSetupDatabase(t *testing.T) {
	h := &hub{shared: testShared(t)}
	h.setup.start()
	server := httptest.NewServer(h.routes())
	defer server.Close()

	var health api.Health
	getJSON(t, server, "/api/health", &health)
	if health.Setup == nil || *health.Setup != api.SetupStepDatabase || health.Database != nil {
		t.Errorf("health: %+v", health)
	}
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != http.StatusServiceUnavailable {
		t.Errorf("hosts before setup: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/setup/check", `{"token":"`+h.setup.token()+`"}`); status != 200 || !strings.Contains(body, `"step":"database"`) {
		t.Errorf("check: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/setup/database", `{"token":"wrong","kind":"sqlite"}`); status != http.StatusForbidden {
		t.Errorf("a wrong token: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/setup/database", `{"token":"`+h.setup.token()+`","kind":"postgres","url":"mysql://x"}`); status != http.StatusBadRequest {
		t.Errorf("not a Postgres URL: %d %s", status, body)
	}
	if status, body := do(t, server, "POST", "/api/setup/database", `{"token":"`+h.setup.token()+`","kind":"sqlite"}`); status != http.StatusNoContent {
		t.Fatalf("sqlite: %d %s", status, body)
	}
	select {
	case got := <-h.chosen:
		if got != filepath.Join(h.cfg.DataDir, "lotse.db") {
			t.Errorf("chose %q", got)
		}
	default:
		t.Fatal("nothing chosen")
	}
	saved, _ := readHubFile(h.cfg.DataDir)
	if saved.DatabaseURL == "" {
		t.Error("hub.json has no database")
	}
	if status, _ := do(t, server, "POST", "/api/setup/database", `{"token":"`+h.setup.token()+`","kind":"sqlite"}`); status != http.StatusConflict {
		t.Errorf("choosing twice: %d", status)
	}
}

// The admin step logs the admin in and ends setup.
func TestSetupAdmin(t *testing.T) {
	h, server := newEmptyHub(t)
	token := h.setup.token()
	if status, _ := do(t, server, "GET", "/api/auth/me", ""); status != http.StatusUnauthorized {
		t.Errorf("me before setup: %d", status)
	}
	for _, bad := range []string{
		`{"token":"` + token + `","name":"A","email":"no address","password":"long enough pw","url":"http://x"}`,
		`{"token":"` + token + `","name":"A","email":"a@example.com","password":"short","url":"http://x"}`,
		`{"token":"` + token + `","name":"A","email":"a@example.com","password":"long enough pw","url":"ftp://x"}`,
	} {
		if status, body := do(t, server, "POST", "/api/setup/admin", bad); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, status, body)
		}
	}
	status, body := do(t, server, "POST", "/api/setup/admin",
		`{"token":"`+token+`","name":"Ada","email":"Ada@Example.com","password":"long enough pw","url":"https://lotse.example.com/"}`)
	if status != 200 || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("admin: %d %s", status, body)
	}
	if h.setup.pending() || h.URL() != "https://lotse.example.com" {
		t.Errorf("after setup: pending %v, URL %q", h.setup.pending(), h.URL())
	}
	if status, body := do(t, server, "GET", "/api/auth/me", ""); status != 200 || !strings.Contains(body, "ada@example.com") {
		t.Errorf("me after setup: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/setup/check", `{"token":"`+token+`"}`); status != http.StatusConflict {
		t.Errorf("check after setup: %d", status)
	}
}

func TestLogin(t *testing.T) {
	h, server := newTestHub(t)
	server.Client().Jar, _ = cookiejar.New(nil) // logged out
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != http.StatusUnauthorized {
		t.Errorf("hosts without a session: %d", status)
	}
	if status, _ := do(t, server, "POST", "/api/auth/login", `{"email":"admin@example.com","password":"wrong"}`); status != http.StatusUnauthorized {
		t.Errorf("a wrong password: %d", status)
	}
	if status, _ := do(t, server, "POST", "/api/auth/login", `{"email":"nobody@example.com","password":"admin password"}`); status != http.StatusUnauthorized {
		t.Errorf("no such user: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/auth/login", `{"email":"ADMIN@example.com","password":"admin password"}`); status != 200 {
		t.Fatalf("login: %d %s", status, body)
	}
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != 200 {
		t.Errorf("hosts with a session: %d", status)
	}
	if status, _ := do(t, server, "POST", "/api/auth/logout", ""); status != http.StatusNoContent {
		t.Errorf("logout: %d", status)
	}
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != http.StatusUnauthorized {
		t.Errorf("hosts after logout: %d", status)
	}

	// Guessing is slowed down: after five failures, even the right password
	// waits.
	h.logins.Succeeded("email:admin@example.com")
	for range 5 {
		do(t, server, "POST", "/api/auth/login", `{"email":"admin@example.com","password":"guess"}`)
	}
	if status, _ := do(t, server, "POST", "/api/auth/login", `{"email":"admin@example.com","password":"admin password"}`); status != http.StatusTooManyRequests {
		t.Errorf("after five failures: %d", status)
	}
}

func TestOtherSitesCannotPost(t *testing.T) {
	_, server := newTestHub(t)
	req, _ := http.NewRequest("POST", server.URL+"/api/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a post from another site: %d", res.StatusCode)
	}
}

func TestUnknownAPIPathIsNotTheApp(t *testing.T) {
	_, server := newTestHub(t)
	if status, _ := do(t, server, "GET", "/api/nothing", ""); status != http.StatusNotFound {
		t.Errorf("status %d, want 404", status)
	}
}

// do sends a request with the server's client, which carries its session,
// and returns the status and body.
func do(t *testing.T, server *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, server.URL+path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(bytes.TrimSpace(b))
}

func getJSON(t *testing.T, server *httptest.Server, path string, v any) {
	t.Helper()
	status, body := do(t, server, "GET", path, "")
	if status != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, status, body)
	}
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatal(err)
	}
}
