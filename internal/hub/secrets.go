package hub

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"lotse.sh/internal/hub/api"
)

// Secrets of channels and the mail server (design 8.5): each field is
// either sealed, as entered in the UI, or a reference the hub resolves when
// it sends: an environment variable, or a file in the secrets directory
// (/run/secrets, where Docker puts them). Neither may reach what the hub
// keeps for itself: no LOTSE_ variable, no file outside the directory.

type secretRef struct {
	Env  string `json:"env,omitempty"`
	File string `json:"file,omitempty"`
}

// secrets are the secret fields of one channel, or of the mail server.
type secrets struct {
	sealed map[string]string
	refs   map[string]secretRef
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretsDir is where file references may point.
func secretsDir() string { return cmp.Or(os.Getenv("LOTSE_SECRETS_DIR"), "/run/secrets") }

// openSecrets reads the secrets as the store keeps them.
func (h *hub) openSecrets(sealed *string, refs json.RawMessage) (secrets, error) {
	s := secrets{sealed: map[string]string{}, refs: map[string]secretRef{}}
	if len(refs) > 0 {
		if err := json.Unmarshal(refs, &s.refs); err != nil {
			return s, err
		}
	}
	if sealed != nil {
		plain, err := h.box.Open(*sealed)
		if err != nil {
			return s, err
		}
		if err := json.Unmarshal([]byte(plain), &s.sealed); err != nil {
			return s, err
		}
	}
	return s, nil
}

// close turns the secrets into what the store keeps.
func (h *hub) closeSecrets(s secrets) (*string, json.RawMessage) {
	refs, _ := json.Marshal(s.refs)
	if len(s.sealed) == 0 {
		return nil, refs
	}
	b, _ := json.Marshal(s.sealed)
	sealed := h.box.Seal(string(b))
	return &sealed, refs
}

// apply changes the secrets as an input says: a value, a reference, or
// nothing (an empty object) for each field named. References read the
// hub's environment and secret files, which belong to the whole hub; refs
// says whether the one asking may make them (decision 0023). One that is
// already there, in known, may be given again by anyone.
func (s *secrets) apply(fields []string, in map[string]api.SecretInput, refs bool, known map[string]secretRef) error {
	for name, x := range in {
		if !slices.Contains(fields, name) {
			return fmt.Errorf("there is no secret %q here", name)
		}
		value, env, file := ptrValue(x.Value), strings.TrimSpace(ptrValue(x.Env)), strings.TrimSpace(ptrValue(x.File))
		n := 0
		for _, v := range []string{value, env, file} {
			if v != "" {
				n++
			}
		}
		if n > 1 {
			return fmt.Errorf("%s: a value, an environment variable or a file, not more than one", name)
		}
		if (env != "" || file != "") && !refs && known[name] != (secretRef{Env: env, File: file}) {
			return fmt.Errorf("%s: only an admin of the whole hub refers to the hub's environment or files; give the value", name)
		}
		delete(s.sealed, name)
		delete(s.refs, name)
		switch {
		case value != "":
			s.sealed[name] = value
		case env != "":
			if !envName.MatchString(env) || strings.HasPrefix(strings.ToUpper(env), "LOTSE_") {
				return fmt.Errorf("%s: %q cannot be used; the name of an environment variable, not one of lotse's own", name, env)
			}
			s.refs[name] = secretRef{Env: env}
		case file != "":
			if _, err := secretPath(file); err != nil {
				return fmt.Errorf("%s: %v", name, err)
			}
			s.refs[name] = secretRef{File: file}
		}
	}
	return nil
}

// secretPath checks that a file reference stays in the secrets directory.
func secretPath(file string) (string, error) {
	dir := secretsDir()
	p := file
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	p = filepath.Clean(p)
	if rel, err := filepath.Rel(dir, p); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("a file in %s, such as %s/chat-webhook", dir, dir)
	}
	return p, nil
}

// has says whether a field is set, one way or the other.
func (s secrets) has(name string) bool {
	_, sealed := s.sealed[name]
	_, ref := s.refs[name]
	return sealed || ref
}

// get resolves a field, when the hub needs it to send.
func (s secrets) get(name string) (string, error) {
	if v, ok := s.sealed[name]; ok {
		return v, nil
	}
	ref, ok := s.refs[name]
	switch {
	case !ok:
		return "", nil
	case ref.Env != "":
		v := os.Getenv(ref.Env)
		if v == "" {
			return "", fmt.Errorf("the environment variable %s for %s is not set", ref.Env, name)
		}
		return v, nil
	}
	p, err := secretPath(ref.File)
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("cannot read the file for %s: %w", name, errors.Unwrap(err))
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// info says of each field whether it is set and where from, for the API.
func (s secrets) info(fields []string) map[string]api.SecretInfo {
	out := map[string]api.SecretInfo{}
	for _, f := range fields {
		i := api.SecretInfo{Set: s.has(f)}
		if r, ok := s.refs[f]; ok {
			if r.Env != "" {
				i.Env = ptr(r.Env)
			}
			if r.File != "" {
				i.File = ptr(r.File)
			}
		}
		out[f] = i
	}
	return out
}

// refsStay says why a channel that sends with the hub's own secrets may not
// be pointed elsewhere by someone kept to a scope: its secret would go
// along (decision 0023). Empty when it may.
func refsStay(restricted bool, refs json.RawMessage, before, after json.RawMessage) string {
	var have map[string]secretRef
	json.Unmarshal(refs, &have)
	if !restricted || len(have) == 0 || sameJSON(before, after) {
		return ""
	}
	return "this channel sends with the hub's own secrets; only an admin of the whole hub changes where it sends"
}
