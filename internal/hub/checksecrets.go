package hub

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// A check's secrets (decision 0024): an http check's headers, such as a
// token in Authorization, and its basic_auth password, kept as a channel's
// secrets are. The hub resolves them when it hands the check to its
// probes; the API and exports show only where they come from.

const basicAuthPassword = "basic_auth.password"

// checkSecretField says whether a check of type typ may have a secret of
// that name: headers.NAME or basic_auth.password.
func checkSecretField(typ, name string) bool {
	if typ != "http" {
		return false
	}
	h, ok := strings.CutPrefix(name, "headers.")
	return name == basicAuthPassword || ok && probe.ValidHeaderName(h)
}

// checkSecretFields are the secrets a check may have, of those it has and
// those asked for.
func checkSecretFields(typ string, have secrets, in map[string]api.SecretInput) []string {
	var out []string
	for _, name := range append(have.names(), slices.Collect(maps.Keys(in))...) {
		if checkSecretField(typ, name) {
			out = append(out, name)
		}
	}
	return out
}

// names are the fields that are set.
func (s secrets) names() []string {
	out := slices.Collect(maps.Keys(s.sealed))
	for k := range s.refs {
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// checkSecrets says what does not fit between a check's spec and its
// secrets.
func checkSecrets(typ string, spec json.RawMessage, sec secrets) error {
	names := sec.names()
	if typ != "http" {
		if len(names) > 0 {
			return errors.New("only http checks have secrets")
		}
		return nil
	}
	var s probe.HTTP
	json.Unmarshal(spec, &s)
	for _, name := range names {
		h, ok := strings.CutPrefix(name, "headers.")
		switch {
		case !ok:
		case s.HasHeader(h):
			return fmt.Errorf("the header %s is both plain and secret", h)
		case s.BasicAuth != nil && strings.EqualFold(h, "Authorization"):
			return errors.New("basic_auth or an Authorization header, not both")
		}
		for _, other := range names {
			if o, ok := strings.CutPrefix(other, "headers."); ok && other < name && strings.EqualFold(o, h) {
				return fmt.Errorf("the header %s is secret twice", h)
			}
		}
	}
	switch {
	case s.BasicAuth != nil && !sec.has(basicAuthPassword):
		return errors.New("basic_auth needs its password, as a secret")
	case s.BasicAuth == nil && sec.has(basicAuthPassword):
		return errors.New("a basic_auth password, but no basic_auth with the user name")
	}
	return nil
}

// probeSpec is the spec a check's probes get: with its secrets resolved
// into headers, basic_auth among them.
func (h *hub) probeSpec(c store.Check) (json.RawMessage, error) {
	if c.Secrets == nil && (len(c.SecretRefs) == 0 || string(c.SecretRefs) == "{}") {
		return c.Spec, nil
	}
	sec, err := h.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		return nil, fmt.Errorf("the hub cannot open the check's secrets: %w", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(c.Spec, &spec); err != nil {
		return nil, err
	}
	headers, _ := spec["headers"].(map[string]any)
	if headers == nil {
		headers = map[string]any{}
	}
	for _, name := range sec.names() {
		v, err := sec.get(name)
		if err != nil {
			return nil, err
		}
		if name == basicAuthPassword {
			var s probe.HTTP
			json.Unmarshal(c.Spec, &s)
			if s.BasicAuth == nil {
				continue
			}
			headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(s.BasicAuth.Username+":"+v))
			delete(spec, "basic_auth")
			continue
		}
		headers[strings.TrimPrefix(name, "headers.")] = v
	}
	if len(headers) > 0 {
		spec["headers"] = headers
	}
	return json.Marshal(spec)
}

// checkSecretInfo says which secrets a check has, for admins.
func (s apiServer) checkSecretInfo(c store.Check) *map[string]api.SecretInfo {
	sec, err := s.openSecrets(c.Secrets, c.SecretRefs)
	if err != nil {
		s.log.Error("cannot open a check's secrets", "check", c.Path, "err", err)
	}
	if len(sec.names()) == 0 {
		return nil
	}
	info := sec.info(sec.names())
	return &info
}
