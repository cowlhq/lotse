package probe

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	for _, c := range []struct {
		typ, spec, want string
		ok              bool
	}{
		{"http", `{"url":"https://example.com","keyword":"Welcome"}`, `{"url":"https://example.com","keyword":"Welcome"}`, true},
		{"http", `{"url":"ftp://example.com"}`, "", false},
		{"http", `{"url":"https://example.com","status":"200-299"}`, "", true},
		{"http", `{"url":"https://example.com","status":"299-200"}`, "", false},
		{"http", `{"url":"https://example.com","other":1}`, "", false},
		{"tls", `{"address":"mail.example.com:465"}`, "", true},
		{"tcp", `{"address":"db.internal:5432"}`, `{"address":"db.internal:5432"}`, true},
		{"tcp", `{"address":"db.internal"}`, "", false},
		{"heartbeat", `{"period":"24h"}`, `{"period":"24h","grace":"2h24m0s"}`, true},
		{"heartbeat", `{"period":"10s"}`, "", false},
		{"ping", `{}`, "", false},
	} {
		got, err := Normalize(c.typ, json.RawMessage(c.spec))
		if (err == nil) != c.ok {
			t.Errorf("%s %s: %v", c.typ, c.spec, err)
			continue
		}
		if c.want != "" && string(got) != c.want {
			t.Errorf("%s %s: %s, want %s", c.typ, c.spec, got, c.want)
		}
	}
}

func TestHTTP(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte("<h1>Welcome</h1>"))
		case "/old":
			http.Redirect(w, r, "/", http.StatusFound)
		case "/slow":
			time.Sleep(2 * time.Second)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	r := &Runner{TLSConfig: &tls.Config{RootCAs: pool}}
	run := func(spec string, timeout time.Duration) Result {
		return r.Run(t.Context(), "http", json.RawMessage(spec), timeout)
	}

	res := run(`{"url":"`+srv.URL+`/old","keyword":"Welcome"}`, 5*time.Second)
	if !res.OK || res.StatusCode != 200 || res.CertNotAfter == 0 || res.LatencyMS <= 0 {
		t.Errorf("up: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/","keyword":"Goodbye"}`, 5*time.Second); res.OK || !strings.Contains(res.Error, "Goodbye") {
		t.Errorf("missing keyword: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/gone"}`, 5*time.Second); res.OK || res.StatusCode != 404 {
		t.Errorf("404: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/gone","status":"404"}`, 5*time.Second); !res.OK {
		t.Errorf("404 expected: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/slow"}`, 200*time.Millisecond); res.OK || res.Error != "no answer in time" {
		t.Errorf("timeout: %+v", res)
	}
	untrusted := (&Runner{}).Run(t.Context(), "http", json.RawMessage(`{"url":"`+srv.URL+`/"}`), 5*time.Second)
	if untrusted.OK || !strings.Contains(untrusted.Error, "certificate") {
		t.Errorf("an untrusted certificate: %+v", untrusted)
	}
}

func TestTLSAndTCP(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	r := &Runner{TLSConfig: &tls.Config{RootCAs: pool}}
	addr := strings.TrimPrefix(srv.URL, "https://")
	if res := r.Run(t.Context(), "tls", json.RawMessage(`{"address":"`+addr+`","server_name":"example.com"}`), 5*time.Second); !res.OK || res.CertNotAfter == 0 {
		t.Errorf("tls: %+v", res)
	}
	if res := r.Run(t.Context(), "tcp", json.RawMessage(`{"address":"`+addr+`"}`), 5*time.Second); !res.OK {
		t.Errorf("tcp up: %+v", res)
	}
	// A port nobody listens on.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().String()
	l.Close()
	if res := r.Run(t.Context(), "tcp", json.RawMessage(`{"address":"`+closed+`"}`), 5*time.Second); res.OK || !strings.Contains(res.Error, "refused") {
		t.Errorf("tcp down: %+v", res)
	}
}
