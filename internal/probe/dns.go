package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
)

// DNS checks that a name resolves, and, if Expect is set, to what. It
// catches a record changed by mistake, or one that a move left behind.
type DNS struct {
	Name string `json:"name"`
	// Type is A (the default), AAAA, CNAME, MX, NS or TXT.
	Type string `json:"type,omitempty"`
	// Expect are values that must all be among the answers, such as
	// 203.0.113.10, or mail.example.com for MX.
	Expect []string `json:"expect,omitempty"`
	// Server is a resolver to ask, host:port; the system's otherwise.
	Server string `json:"server,omitempty"`
}

var dnsTypes = []string{"A", "AAAA", "CNAME", "MX", "NS", "TXT"}

func (s *DNS) normalize() error {
	s.Name = strings.TrimSuffix(strings.TrimSpace(s.Name), ".")
	s.Type = strings.ToUpper(strings.TrimSpace(s.Type))
	if s.Type == "" {
		s.Type = "A"
	}
	if s.Name == "" || strings.ContainsAny(s.Name, " /:") {
		return errors.New("dns: name must be a domain name, such as shop.example.com")
	}
	if !slices.Contains(dnsTypes, s.Type) {
		return fmt.Errorf("dns: type is one of %s", strings.Join(dnsTypes, ", "))
	}
	if s.Server != "" {
		if _, port, err := net.SplitHostPort(s.Server); err != nil || port == "" {
			return errors.New("dns: server must be host:port, such as 1.1.1.1:53")
		}
	}
	for i, e := range s.Expect {
		s.Expect[i] = strings.TrimSpace(e)
	}
	s.Expect = slices.DeleteFunc(s.Expect, func(e string) bool { return e == "" })
	return nil
}

func (r *Runner) dns(ctx context.Context, s DNS) Result {
	res := &net.Resolver{PreferGo: true}
	if s.Server != "" {
		res.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, s.Server)
		}
	}
	var got []string
	var err error
	switch s.Type {
	case "A", "AAAA":
		network := "ip4"
		if s.Type == "AAAA" {
			network = "ip6"
		}
		var ips []net.IP
		if ips, err = res.LookupIP(ctx, network, s.Name); err == nil {
			for _, ip := range ips {
				got = append(got, ip.String())
			}
		}
	case "CNAME":
		var c string
		if c, err = res.LookupCNAME(ctx, s.Name); err == nil {
			got = []string{c}
		}
	case "MX":
		var mx []*net.MX
		if mx, err = res.LookupMX(ctx, s.Name); err == nil {
			for _, m := range mx {
				got = append(got, m.Host)
			}
		}
	case "NS":
		var ns []*net.NS
		if ns, err = res.LookupNS(ctx, s.Name); err == nil {
			for _, n := range ns {
				got = append(got, n.Host)
			}
		}
	case "TXT":
		got, err = res.LookupTXT(ctx, s.Name)
	}
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return Result{Error: fmt.Sprintf("no %s record for %s", s.Type, s.Name)}
		}
		return Result{Error: plainError(err)}
	}
	if len(got) == 0 {
		return Result{Error: fmt.Sprintf("no %s record for %s", s.Type, s.Name)}
	}
	for i, g := range got {
		got[i] = strings.TrimSuffix(g, ".")
	}
	for _, e := range s.Expect {
		want := strings.TrimSuffix(e, ".")
		if !slices.ContainsFunc(got, func(g string) bool { return strings.EqualFold(g, want) || sameIP(g, want) }) {
			return Result{Error: fmt.Sprintf("%s has %s, not %s", s.Name, strings.Join(got, ", "), e)}
		}
	}
	return Result{OK: true}
}

// sameIP compares addresses as addresses, so that 2001:db8::1 is
// 2001:0db8:0:0:0:0:0:1.
func sameIP(a, b string) bool {
	x, y := net.ParseIP(a), net.ParseIP(b)
	return x != nil && y != nil && x.Equal(y)
}
