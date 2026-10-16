// Package probe runs checks from outside (design 8.3): HTTP, TLS, TCP and
// DNS, and, on the hub only, domains' expiry.
// The hub runs them itself, and so do probes in other places.
package probe

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"lotse.sh/internal/version"
)

// HTTP checks a URL: its status after redirects, optionally what the body
// says, how long it takes, and, over HTTPS, its certificate.
type HTTP struct {
	URL string `json:"url"`
	// Method is GET unless set.
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	// FollowRedirects is true unless set to false: then a redirect is the
	// answer, and its status what counts.
	FollowRedirects *Bool `json:"follow_redirects,omitempty"`
	// IP is where to connect instead of where the URL's host points, such
	// as one server behind a load balancer. The host still names the site
	// and its certificate.
	IP string `json:"ip,omitempty"`
	// Keyword must appear in the first MiB of the body, if set.
	Keyword string `json:"keyword,omitempty"`
	// Absent must not appear in it, such as "Fatal error".
	Absent string `json:"absent,omitempty"`
	// JSON is a value the body must have, for health endpoints.
	JSON *JSONValue `json:"json,omitempty"`
	// MaxLatency is how long the answer may take, such as 800ms.
	MaxLatency string `json:"max_latency,omitempty"`
	// Status is the range that counts as up, such as 200-299; default
	// 200-399.
	Status string `json:"status,omitempty"`
}

// JSONValue says that the value at Path, such as status or
// checks.0.status, is Equals.
type JSONValue struct {
	Path   string `json:"path"`
	Equals Scalar `json:"equals"`
}

// Bool is a bool that may also come as "true" or "false", as from labels.
type Bool bool

func (b *Bool) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseBool(strings.Trim(string(data), `"`))
	if err != nil {
		return fmt.Errorf("%s is not true or false", data)
	}
	*b = Bool(v)
	return nil
}

// Scalar is a string, a number, true, false or null, kept as its text.
type Scalar string

func (s *Scalar) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case string:
		*s = Scalar(v)
	case float64, bool, nil:
		*s = Scalar(data)
	default:
		return errors.New("equals must be a string, a number, true, false or null")
	}
	return nil
}

var methods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}

func (s *HTTP) normalize() error {
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("http: url must be an http or https address")
	}
	s.Method = strings.ToUpper(s.Method)
	if s.Method == http.MethodGet {
		s.Method = ""
	}
	if s.Method != "" && !slices.Contains(methods, s.Method) {
		return fmt.Errorf("http: method must be one of %s", strings.Join(methods, ", "))
	}
	if s.Method == http.MethodHead && (s.Keyword != "" || s.Absent != "" || s.JSON != nil) {
		return errors.New("http: a HEAD request has no body to look into")
	}
	for k, v := range s.Headers {
		if !validHeaderName(k) {
			return fmt.Errorf("http: %q is not a header name", k)
		}
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("http: header %s has a line break", k)
		}
	}
	if len(s.Headers) == 0 {
		s.Headers = nil
	}
	if s.FollowRedirects != nil && *s.FollowRedirects {
		s.FollowRedirects = nil
	}
	if s.IP != "" && net.ParseIP(s.IP) == nil {
		return errors.New("http: ip must be an IP address, such as 203.0.113.10")
	}
	if s.JSON != nil && strings.Trim(s.JSON.Path, ".") == "" {
		return errors.New("http: json needs a path, such as status")
	}
	if s.MaxLatency != "" {
		if d, err := time.ParseDuration(s.MaxLatency); err != nil || d <= 0 {
			return errors.New("http: max_latency must be a duration, such as 800ms")
		}
	}
	_, _, err = statusRange(s.Status)
	return err
}

// validHeaderName says whether k is a token, as RFC 9110 has header names.
func validHeaderName(k string) bool {
	if k == "" {
		return false
	}
	for _, c := range k {
		if c > '~' || c <= ' ' || strings.ContainsRune(`"(),/:;<=>?@[\]{}`, c) {
			return false
		}
	}
	return true
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
// connects. http.v2 is an http check that uses what came after url,
// keyword and status; a probe from before would ignore it, and run
// another check than the one asked for.
var Types = []string{"http", "http.v2", "tls", "tcp", "dns"}

// Needs is what a probe must say it runs to run a check: its type, or
// http.v2.
func Needs(typ string, spec json.RawMessage) string {
	if typ != "http" {
		return typ
	}
	var s HTTP
	json.Unmarshal(spec, &s)
	if s.Method != "" || s.Headers != nil || s.Body != "" || s.FollowRedirects != nil || s.IP != "" ||
		s.Absent != "" || s.JSON != nil || s.MaxLatency != "" {
		return "http.v2"
	}
	return typ
}

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
		if err := s.normalize(); err != nil {
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
	transport := &http.Transport{TLSClientConfig: r.TLSConfig, DisableKeepAlives: true, Proxy: http.ProxyFromEnvironment}
	if s.IP != "" {
		// Only the URL's own host goes to the IP; a redirect elsewhere
		// goes where its host points.
		u, _ := url.Parse(s.URL)
		port := cmp.Or(u.Port(), map[string]string{"http": "80", "https": "443"}[u.Scheme])
		var d net.Dialer
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if h, p, err := net.SplitHostPort(addr); err == nil && strings.EqualFold(h, u.Hostname()) && p == port {
				addr = net.JoinHostPort(s.IP, port)
			}
			return d.DialContext(ctx, network, addr)
		}
	}
	// A new connection every time, so the time includes connecting, as a
	// visitor's would.
	client := &http.Client{Transport: transport}
	if s.FollowRedirects != nil && !*s.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	var body io.Reader
	if s.Body != "" {
		body = strings.NewReader(s.Body)
	}
	req, err := http.NewRequestWithContext(ctx, cmp.Or(s.Method, http.MethodGet), s.URL, body)
	if err != nil {
		return Result{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "lotse-probe/"+version.Version+" (+https://lotse.sh)")
	for k, v := range s.Headers {
		if strings.EqualFold(k, "Host") {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	start := time.Now()
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
	if s.Keyword != "" || s.Absent != "" || s.JSON != nil {
		b, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
		if err != nil {
			out.Error = plainError(err)
			return out
		}
		if out.Error = checkBody(s, b); out.Error != "" {
			return out
		}
	}
	if s.MaxLatency != "" {
		limit, _ := time.ParseDuration(s.MaxLatency)
		if took := time.Since(start); took > limit {
			out.Error = fmt.Sprintf("took %s, longer than %s", took.Round(time.Millisecond), limit)
			return out
		}
	}
	out.OK = true
	return out
}

// checkBody says what is wrong with a body, if anything.
func checkBody(s HTTP, body []byte) string {
	if s.Keyword != "" && !bytes.Contains(body, []byte(s.Keyword)) {
		return fmt.Sprintf("the page does not contain %q", s.Keyword)
	}
	if s.Absent != "" && bytes.Contains(body, []byte(s.Absent)) {
		return fmt.Sprintf("the page contains %q", s.Absent)
	}
	if s.JSON == nil {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "the answer is not JSON"
	}
	path := strings.Trim(s.JSON.Path, ".")
	for _, key := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			if v, ok = x[key]; !ok {
				return fmt.Sprintf("the answer has no %s", path)
			}
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(x) {
				return fmt.Sprintf("the answer has no %s", path)
			}
			v = x[i]
		default:
			return fmt.Sprintf("the answer has no %s", path)
		}
	}
	got := scalarText(v)
	if got == nil {
		return fmt.Sprintf("%s is not a single value", path)
	}
	want := string(s.JSON.Equals)
	if *got == want {
		return ""
	}
	if n, ok := v.(json.Number); ok {
		a, err1 := n.Float64()
		b, err2 := strconv.ParseFloat(want, 64)
		if err1 == nil && err2 == nil && a == b {
			return ""
		}
	}
	return fmt.Sprintf("%s is %s, not %s", path, *got, want)
}

// scalarText is a JSON value as text, as Scalar keeps it, or nil for an
// object or a list.
func scalarText(v any) *string {
	var s string
	switch v := v.(type) {
	case string:
		s = v
	case json.Number:
		s = v.String()
	case bool:
		s = strconv.FormatBool(v)
	case nil:
		s = "null"
	default:
		return nil
	}
	return &s
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
