package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"lotse.sh/internal/version"
)

// Domain checks when a domain's registration expires, as its registry
// publishes it over RDAP. Some registries, such as DENIC for .de, publish
// no date: domains there renew until they are cancelled. The hub runs these
// checks itself, a few times a day.
type Domain struct {
	Domain string `json:"domain"`
}

func (s *Domain) normalize() error {
	s.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s.Domain), "."))
	if !strings.Contains(s.Domain, ".") || strings.ContainsAny(s.Domain, " /:") {
		return errors.New("domain: domain must be a registered name, such as example.com")
	}
	return nil
}

// DefaultRDAPBootstrap is where IANA lists the RDAP servers of top-level
// domains.
const DefaultRDAPBootstrap = "https://data.iana.org/rdap/dns.json"

// rdap knows which server answers for which top-level domain, from IANA's
// list, fetched once a day.
type rdap struct {
	mu      sync.Mutex
	servers map[string]string
	fetched time.Time
}

func (r *Runner) rdapServer(ctx context.Context, client *http.Client, tld string) (string, error) {
	r.rdapOnce.Do(func() { r.rdap = &rdap{} })
	c := r.rdap
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers == nil || time.Since(c.fetched) > 24*time.Hour {
		bootstrap := r.RDAPBootstrap
		if bootstrap == "" {
			bootstrap = DefaultRDAPBootstrap
		}
		var list struct {
			Services [][][]string `json:"services"`
		}
		if err := getJSON(ctx, client, bootstrap, &list); err != nil {
			if c.servers == nil {
				return "", fmt.Errorf("cannot read the list of registries: %s", err)
			}
		} else {
			c.servers, c.fetched = map[string]string{}, time.Now()
			for _, s := range list.Services {
				if len(s) == 2 && len(s[1]) > 0 {
					for _, t := range s[0] {
						c.servers[strings.ToLower(t)] = s[1][0]
					}
				}
			}
		}
	}
	server, ok := c.servers[tld]
	if !ok {
		return "", fmt.Errorf("the registry of .%s offers no RDAP", tld)
	}
	return server, nil
}

func (r *Runner) domain(ctx context.Context, s Domain) Result {
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: r.TLSConfig, Proxy: http.ProxyFromEnvironment}}
	tld := s.Domain[strings.LastIndex(s.Domain, ".")+1:]
	server, err := r.rdapServer(ctx, client, tld)
	if err != nil {
		return Result{Error: err.Error()}
	}
	var answer struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if err := getJSON(ctx, client, strings.TrimSuffix(server, "/")+"/domain/"+s.Domain, &answer); err != nil {
		return Result{Error: err.Error()}
	}
	for _, e := range answer.Events {
		if e.Action != "expiration" {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			return Result{Error: "the registry gave an expiry date that is not one: " + e.Date}
		}
		// The registration's end, kept where a certificate's would be.
		return Result{OK: true, CertNotAfter: t.Unix()}
	}
	return Result{OK: true}
}

// getJSON reads a JSON document, and says plainly why not.
func getJSON(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", "lotse-probe/"+version.Version+" (+https://lotse.sh)")
	res, err := client.Do(req)
	if err != nil {
		return errors.New(plainError(err))
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return errors.New("the registry does not know this domain")
	case res.StatusCode/100 != 2:
		return fmt.Errorf("the registry answered %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(v)
}
