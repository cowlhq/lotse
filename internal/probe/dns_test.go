package probe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDNS(t *testing.T) {
	r := &Runner{}
	run := func(spec string) Result {
		norm, err := Normalize("dns", json.RawMessage(spec))
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		return r.Run(t.Context(), "dns", norm, 5*time.Second)
	}
	// localhost resolves from /etc/hosts, without a network.
	if res := run(`{"name":"localhost","expect":["127.0.0.1"]}`); !res.OK {
		t.Errorf("localhost: %+v", res)
	}
	if res := run(`{"name":"localhost","expect":["10.0.0.1"]}`); res.OK || !strings.Contains(res.Error, "not 10.0.0.1") {
		t.Errorf("an address it does not have: %+v", res)
	}
	if res := run(`{"name":"example.com","server":"127.0.0.1:1"}`); res.OK || res.Error == "" {
		t.Errorf("a resolver that does not answer: %+v", res)
	}
	for _, bad := range []string{`{"name":""}`, `{"name":"x.com","type":"SRV"}`, `{"name":"x.com","server":"1.1.1.1"}`, `{"name":"https://x.com"}`} {
		if _, err := Normalize("dns", json.RawMessage(bad)); err == nil {
			t.Errorf("%s is a dns check", bad)
		}
	}
	if out, _ := Normalize("dns", json.RawMessage(`{"name":"Shop.Example.com.","type":"mx"}`)); string(out) != `{"name":"Shop.Example.com","type":"MX"}` {
		t.Errorf("normalized: %s", out)
	}
}

// A domain's expiry, from its registry's RDAP server, found through a list
// like IANA's.
func TestDomain(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dns.json":
			fmt.Fprintf(w, `{"services":[[["com","net"],["%s/com/"]],[["de"],["%s/de/"]]]}`, server.URL, server.URL)
		case "/com/domain/example.com":
			w.Write([]byte(`{"events":[{"eventAction":"registration","eventDate":"1995-08-14T04:00:00Z"},{"eventAction":"expiration","eventDate":"2027-08-13T04:00:00Z"}]}`))
		case "/de/domain/example.de":
			w.Write([]byte(`{"events":[{"eventAction":"last changed","eventDate":"2020-01-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	r := &Runner{RDAPBootstrap: server.URL + "/dns.json"}
	run := func(domain string) Result {
		return r.Run(t.Context(), "domain", json.RawMessage(`{"domain":"`+domain+`"}`), 5*time.Second)
	}
	if res := run("example.com"); !res.OK || time.Unix(res.CertNotAfter, 0).UTC().Format("2006-01-02") != "2027-08-13" {
		t.Errorf("example.com: %+v", res)
	}
	if res := run("example.de"); !res.OK || res.CertNotAfter != 0 {
		t.Errorf("a registry without a date: %+v", res)
	}
	if res := run("nothere.com"); res.OK || !strings.Contains(res.Error, "does not know") {
		t.Errorf("an unknown domain: %+v", res)
	}
	if res := run("example.zz"); res.OK || !strings.Contains(res.Error, "no RDAP") {
		t.Errorf("a registry without RDAP: %+v", res)
	}
}
