package hub_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"lotse.sh/internal/db"
	"lotse.sh/internal/db/dbtest"
	"lotse.sh/internal/hub"
)

func TestHealth(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		h := hub.New(d, fstest.MapFS{"index.html": {Data: []byte("<app>")}})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want 200", rec.Code)
		}
		var body struct {
			OK      bool   `json:"ok"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.OK || body.Version == "" {
			t.Errorf("got %+v, want ok and a version", body)
		}
	})
}

// An unknown API path is a 404, not the web interface.
func TestUnknownAPI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		h := hub.New(d, fstest.MapFS{"index.html": {Data: []byte("<app>")}})
		for path, want := range map[string]int{"/api/nothing": 404, "/c/1": 200} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != want {
				t.Errorf("GET %s: status %d, want %d", path, rec.Code, want)
			}
		}
	})
}
