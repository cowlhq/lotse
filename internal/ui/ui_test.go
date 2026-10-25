package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	files := fstest.MapFS{
		"index.html":      {Data: []byte("<app>")},
		"favicon.svg":     {Data: []byte("<svg>")},
		"assets/app-1.js": {Data: []byte("js")},
	}
	h := Handler(files)
	for _, c := range []struct {
		path   string
		status int
		body   string
	}{
		{"/", 200, "<app>"},
		{"/c/12", 200, "<app>"},
		{"/favicon.svg", 200, "<svg>"},
		{"/assets/app-1.js", 200, "js"},
		{"/assets/gone.js", 404, ""},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.status {
			t.Errorf("GET %s: status %d, want %d", c.path, rec.Code, c.status)
		}
		if c.body != "" && rec.Body.String() != c.body {
			t.Errorf("GET %s: body %q, want %q", c.path, rec.Body.String(), c.body)
		}
	}
}

func TestHandlerWithoutUI(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if !strings.Contains(rec.Body.String(), "without its web interface") {
		t.Errorf("got %q, want the page that explains the missing UI", rec.Body.String())
	}
}
