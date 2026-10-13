package hub

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

func newTestHub(t *testing.T) (*hub, *httptest.Server) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "lotse.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, err := newHub(t.Context(), db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	go h.writer.run(t.Context())
	server := httptest.NewServer(h.routes())
	t.Cleanup(server.Close)
	return h, server
}

func TestHealth(t *testing.T) {
	_, server := newTestHub(t)
	var got api.Health
	getJSON(t, server.URL+"/api/health", &got)
	if got.Status != "ok" || got.Database != "sqlite" || !got.Setup {
		t.Errorf("got %+v, want ok on sqlite, waiting for setup", got)
	}
}

func TestCheckSetup(t *testing.T) {
	h, server := newTestHub(t)
	if h.setupToken == "" {
		t.Fatal("a hub without users has no setup token")
	}
	check := func(token string) (int, string) {
		res, err := http.Post(server.URL+"/api/setup/check", "application/json",
			strings.NewReader(`{"token":"`+token+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, strings.TrimSpace(string(body))
	}
	if status, body := check(h.setupToken); status != 200 || body != `{"valid":true}` {
		t.Errorf("the right token: %d %s", status, body)
	}
	if status, body := check("wrong"); status != 200 || body != `{"valid":false}` {
		t.Errorf("a wrong token: %d %s", status, body)
	}
	h.setupToken = ""
	if status, _ := check("anything"); status != http.StatusConflict {
		t.Errorf("after setup: status %d, want 409", status)
	}
}

func TestUnknownAPIPathIsNotTheApp(t *testing.T) {
	_, server := newTestHub(t)
	res, err := http.Get(server.URL + "/api/nothing")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", res.StatusCode)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", url, res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}
