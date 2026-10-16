package probe

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
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
		{"http", `{"url":"https://example.com","method":"get","follow_redirects":"true","headers":{}}`, `{"url":"https://example.com"}`, true},
		{"http", `{"url":"https://example.com","method":"post","headers":{"X-Token":"a"},"body":"{}","follow_redirects":"false","ip":"203.0.113.10"}`,
			`{"url":"https://example.com","method":"POST","headers":{"X-Token":"a"},"body":"{}","follow_redirects":false,"ip":"203.0.113.10"}`, true},
		{"http", `{"url":"https://example.com","json":{"path":"status","equals":"UP"},"absent":"Error","max_latency":"800ms"}`, "", true},
		{"http", `{"url":"https://example.com","json":{"path":"count","equals":3}}`, `{"url":"https://example.com","json":{"path":"count","equals":"3"}}`, true},
		{"http", `{"url":"https://example.com","json":{"path":"a","equals":[1]}}`, "", false},
		{"http", `{"url":"https://example.com","json":{"equals":"UP"}}`, "", false},
		{"http", `{"url":"https://example.com","method":"TRACE"}`, "", false},
		{"http", `{"url":"https://example.com","method":"HEAD","keyword":"x"}`, "", false},
		{"http", `{"url":"https://example.com","headers":{"X Token":"a"}}`, "", false},
		{"http", `{"url":"https://example.com","headers":{"X-Token":"a\r\nX-Other: b"}}`, "", false},
		{"http", `{"url":"https://example.com","ip":"web1"}`, "", false},
		{"http", `{"url":"https://example.com","max_latency":"fast"}`, "", false},
		{"http", `{"url":"https://example.com","follow_redirects":"maybe"}`, "", false},
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
	if res := run(`{"url":"`+srv.URL+`/","absent":"Welcome"}`, 5*time.Second); res.OK || res.Error != `the page contains "Welcome"` {
		t.Errorf("absent: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/old","follow_redirects":false}`, 5*time.Second); !res.OK || res.StatusCode != 302 {
		t.Errorf("no redirects: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/old","follow_redirects":false,"status":"200"}`, 5*time.Second); res.OK {
		t.Errorf("a redirect where 200 is expected: %+v", res)
	}
	if res := run(`{"url":"`+srv.URL+`/slow","max_latency":"100ms"}`, 5*time.Second); res.OK || !strings.HasPrefix(res.Error, "took 2") {
		t.Errorf("slow: %+v", res)
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

func TestHTTPRequest(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"UP","checks":[{"name":"db","status":"DOWN"}],"count":3,"ok":true}`))
	}))
	defer srv.Close()
	r := &Runner{}
	run := func(spec string) Result {
		return r.Run(t.Context(), "http", json.RawMessage(spec), 5*time.Second)
	}

	res := run(`{"url":"` + srv.URL + `/hook","method":"POST","headers":{"X-Token":"abc","Host":"shop.example.com","User-Agent":"mine"},"body":"{\"a\":1}"}`)
	if !res.OK || got.Method != "POST" || got.Header.Get("X-Token") != "abc" || got.Host != "shop.example.com" || got.UserAgent() != "mine" || gotBody != `{"a":1}` {
		t.Errorf("request: %+v %s %v %s %q", res, got.Method, got.Header, got.Host, gotBody)
	}
	for spec, want := range map[string]string{
		`{"path":"status","equals":"UP"}`:          "",
		`{"path":"count","equals":3}`:              "",
		`{"path":"count","equals":"3.0"}`:          "",
		`{"path":"ok","equals":true}`:              "",
		`{"path":"checks.0.status","equals":"UP"}`: "checks.0.status is DOWN, not UP",
		`{"path":"checks.1.status","equals":"UP"}`: "the answer has no checks.1.status",
		`{"path":"version","equals":"1"}`:          "the answer has no version",
		`{"path":"checks","equals":"UP"}`:          "checks is not a single value",
	} {
		res := run(`{"url":"` + srv.URL + `","json":` + spec + `}`)
		if res.Error != want || res.OK != (want == "") {
			t.Errorf("json %s: %+v, want %q", spec, res, want)
		}
	}
	if res := run(`{"url":"` + srv.URL + `","method":"HEAD"}`); !res.OK || got.Method != "HEAD" {
		t.Errorf("head: %+v", res)
	}

	// The URL names a host that does not exist; the IP says where it is.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if res := run(`{"url":"http://shop.invalid:` + port + `/","ip":"127.0.0.1"}`); !res.OK || got.Host != "shop.invalid:"+port {
		t.Errorf("ip: %+v %s", res, got.Host)
	}
}

func TestCheckBodyNotJSON(t *testing.T) {
	if got := checkBody(HTTP{JSON: &JSONValue{Path: "status", Equals: "UP"}}, []byte("<html>")); got != "the answer is not JSON" {
		t.Errorf("got %q", got)
	}
}

func TestNeeds(t *testing.T) {
	for spec, want := range map[string]string{
		`{"url":"https://example.com","keyword":"a","status":"200"}`:     "http",
		`{"url":"https://example.com","method":"POST"}`:                  "http.v2",
		`{"url":"https://example.com","json":{"path":"a","equals":"b"}}`: "http.v2",
		`{"url":"https://example.com","follow_redirects":false}`:         "http.v2",
	} {
		if got := Needs("http", json.RawMessage(spec)); got != want {
			t.Errorf("%s: %s, want %s", spec, got, want)
		}
	}
	if got := Needs("dns", json.RawMessage(`{}`)); got != "dns" {
		t.Errorf("dns: %s", got)
	}
}
