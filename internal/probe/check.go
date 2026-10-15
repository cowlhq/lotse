// Package probe runs checks from outside (design 8.3): HTTP, TLS, TCP and
// DNS, and, on the hub only, domains' expiry.
// The hub runs them itself, and so do probes in other places.
package probe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"lotse.sh/internal/version"
)

// HTTP checks a URL: its status after redirects, optionally a keyword in
// the body, how long it takes, and, over HTTPS, its certificate.
type HTTP struct {
	URL string `json:"url"`
	// Keyword must appear in the first MiB of the body, if set.
	Keyword string `json:"keyword,omitempty"`
	// Status is the range that counts as up, such as 200-299; default
	// 200-399.
	Status string `json:"status,omitempty"`
}

// TLS checks a TLS handshake and its certificate, for services that are
// not HTTPS, such as SMTP on 465.
type TLS struct {
	Address string `json:"address"`
	// ServerName defaults to the host of Address.
	ServerName string `json:"server_name,omitempty"`
}

// TCP checks that a port accepts connections.
type TCP struct {
	Address string `json:"address"`
}

// Heartbeat expects a ping every Period, give or take Grace. The hub
// checks it; no probe does.
type Heartbeat struct {
	Period string `json:"period"`
	Grace  string `json:"grace"`
}

// Types are the checks a probe runs, which it tells the hub when it
// connects.
var Types = []string{"http", "tls", "tcp", "dns"}

// HubTypes are the checks only the hub's own probe runs.
var HubTypes = []string{"domain"}

// Result is how one run went.
type Result struct {
	OK        bool
	LatencyMS float64
	// StatusCode is the HTTP status, if there was one.
	StatusCode int
	// CertNotAfter is when the certificate expires, in Unix seconds, if
	// there was one.
	CertNotAfter int64
	Error        string
}

// Normalize checks a check's spec and returns it as the hub keeps it.
func Normalize(typ string, spec json.RawMessage) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(spec))
	dec.DisallowUnknownFields()
	var v any
	switch typ {
	case "http":
		var s HTTP
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("http: %w", err)
		}
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("http: url must be an http or https address")
		}
		if _, _, err := statusRange(s.Status); err != nil {
			return nil, err
		}
		v = s
	case "tls", "tcp":
		var s TLS
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%s: %w", typ, err)
		}
		if _, port, err := net.SplitHostPort(s.Address); err != nil || port == "" {
			return nil, fmt.Errorf("%s: address must be host:port", typ)
		}
		if typ == "tcp" {
			if s.ServerName != "" {
				return nil, errors.New("tcp: server_name is for tls")
			}
			v = TCP{Address: s.Address}
		} else {
			v = s
		}
	case "dns":
		var s DNS
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("dns: %w", err)
		}
		if err := s.normalize(); err != nil {
			return nil, err
		}
		v = s
	case "domain":
		var s Domain
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("domain: %w", err)
		}
		if err := s.normalize(); err != nil {
			return nil, err
		}
		v = s
	case "heartbeat":
		var s Heartbeat
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("heartbeat: %w", err)
		}
		p, err := time.ParseDuration(s.Period)
		if err != nil || p < time.Minute {
			return nil, errors.New("heartbeat: period must be a duration of at least 1m, such as 24h")
		}
		if s.Grace == "" {
			s.Grace = (p / 10).String()
		}
		if g, err := time.ParseDuration(s.Grace); err != nil || g < 0 {
			return nil, errors.New("heartbeat: grace must be a duration, such as 1h")
		}
		v = s
	default:
		return nil, fmt.Errorf("there is no check of type %q", typ)
	}
	return json.Marshal(v)
}

func statusRange(s string) (lo, hi int, err error) {
	if s == "" {
		return 200, 399, nil
	}
	a, b, found := strings.Cut(s, "-")
	if !found {
		b = a
	}
	lo, err1 := strconv.Atoi(a)
	hi, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || lo < 100 || hi > 599 || lo > hi {
		return 0, 0, errors.New("http: status must be a code or a range, such as 200 or 200-299")
	}
	return lo, hi, nil
}

// Runner runs checks.
type Runner struct {
	// TLSConfig is for tests, to trust their own certificates.
	TLSConfig *tls.Config
	// RDAPBootstrap is where the list of registries is; tests change it.
	RDAPBootstrap string

	rdapOnce sync.Once
	rdap     *rdap
}

const maxBody = 1 << 20

// Run runs a check of type typ, giving up after timeout.
func (r *Runner) Run(ctx context.Context, typ string, spec json.RawMessage, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	var res Result
	switch typ {
	case "http":
		var s HTTP
		json.Unmarshal(spec, &s)
		res = r.http(ctx, s)
	case "tls":
		var s TLS
		json.Unmarshal(spec, &s)
		res = r.tls(ctx, s)
	case "tcp":
		var s TCP
		json.Unmarshal(spec, &s)
		res = r.tcp(ctx, s)
	case "dns":
		var s DNS
		json.Unmarshal(spec, &s)
		res = r.dns(ctx, s)
	case "domain":
		var s Domain
		json.Unmarshal(spec, &s)
		res = r.domain(ctx, s)
	default:
		return Result{Error: fmt.Sprintf("a probe does not run %s checks", typ)}
	}
	res.LatencyMS = float64(time.Since(start).Microseconds()) / 1000
	return res
}

func (r *Runner) http(ctx context.Context, s HTTP) Result {
	lo, hi, _ := statusRange(s.Status)
	client := &http.Client{
		// A new connection every time, so the time includes connecting,
		// as a visitor's would.
		Transport: &http.Transport{TLSClientConfig: r.TLSConfig, DisableKeepAlives: true, Proxy: http.ProxyFromEnvironment},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Result{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "lotse-probe/"+version.Version+" (+https://lotse.sh)")
	res, err := client.Do(req)
	if err != nil {
		return Result{Error: plainError(err)}
	}
	defer res.Body.Close()
	out := Result{StatusCode: res.StatusCode}
	if res.TLS != nil && len(res.TLS.PeerCertificates) > 0 {
		out.CertNotAfter = res.TLS.PeerCertificates[0].NotAfter.Unix()
	}
	if res.StatusCode < lo || res.StatusCode > hi {
		out.Error = fmt.Sprintf("status %d, not %d-%d", res.StatusCode, lo, hi)
		return out
	}
	if s.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
		if err != nil {
			out.Error = plainError(err)
			return out
		}
		if !bytes.Contains(body, []byte(s.Keyword)) {
			out.Error = fmt.Sprintf("the page does not contain %q", s.Keyword)
			return out
		}
	}
	out.OK = true
	return out
}

func (r *Runner) tls(ctx context.Context, s TLS) Result {
	cfg := &tls.Config{}
	if r.TLSConfig != nil {
		cfg = r.TLSConfig.Clone()
	}
	cfg.ServerName = s.ServerName
	if cfg.ServerName == "" {
		cfg.ServerName, _, _ = net.SplitHostPort(s.Address)
	}
	d := tls.Dialer{Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", s.Address)
	if err != nil {
		return Result{Error: plainError(err)}
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	out := Result{OK: true}
	if len(state.PeerCertificates) > 0 {
		out.CertNotAfter = state.PeerCertificates[0].NotAfter.Unix()
	}
	return out
}

func (r *Runner) tcp(ctx context.Context, s TCP) Result {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.Address)
	if err != nil {
		return Result{Error: plainError(err)}
	}
	conn.Close()
	return Result{OK: true}
}

// plainError says what went wrong without Go's wrapping: "connection
// refused", not "Get \"https://…\": dial tcp …: connect: connection
// refused".
func plainError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "no answer in time"
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Err != nil {
		return oe.Err.Error()
	}
	return err.Error()
}
