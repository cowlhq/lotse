// Package apply is the format of lotse.yaml (design 9.2): the tree of
// customers, projects and environments with their settings, channels,
// checks, hosts and maintenance, as `lotse apply` sends it to the hub and
// `lotse export` writes it.
//
// Objects are matched by key. Secrets are never in the file: a secret is a
// reference the hub resolves when it sends ({env: NAME}, {file: PATH}), one
// that `lotse apply` resolves on its own machine ({from_env: NAME}), or, in
// an export, a placeholder for a secret the hub keeps sealed
// ({secret: CHANNEL} or {secret: CHECK}), which apply leaves as it is.
package apply

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Document is a lotse.yaml.
type Document struct {
	Settings    Settings   `yaml:"settings,omitempty" json:"settings,omitempty"`
	Channels    []Channel  `yaml:"channels,omitempty" json:"channels,omitempty"`
	Maintenance []Window   `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Customers   []Customer `yaml:"customers,omitempty" json:"customers,omitempty"`
}

// Settings are settings by key, such as rules.host.disk.warn: 80.
type Settings map[string]any

type Customer struct {
	Key         string    `yaml:"key" json:"key"`
	Name        string    `yaml:"name,omitempty" json:"name,omitempty"`
	Settings    Settings  `yaml:"settings,omitempty" json:"settings,omitempty"`
	Channels    []Channel `yaml:"channels,omitempty" json:"channels,omitempty"`
	Maintenance []Window  `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Projects    []Project `yaml:"projects,omitempty" json:"projects,omitempty"`
}

type Project struct {
	Key          string        `yaml:"key" json:"key"`
	Name         string        `yaml:"name,omitempty" json:"name,omitempty"`
	Settings     Settings      `yaml:"settings,omitempty" json:"settings,omitempty"`
	Channels     []Channel     `yaml:"channels,omitempty" json:"channels,omitempty"`
	Maintenance  []Window      `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Environments []Environment `yaml:"environments,omitempty" json:"environments,omitempty"`
}

type Environment struct {
	Key         string    `yaml:"key" json:"key"`
	Name        string    `yaml:"name,omitempty" json:"name,omitempty"`
	Settings    Settings  `yaml:"settings,omitempty" json:"settings,omitempty"`
	Channels    []Channel `yaml:"channels,omitempty" json:"channels,omitempty"`
	Maintenance []Window  `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Checks      []Check   `yaml:"checks,omitempty" json:"checks,omitempty"`
	Hosts       []Host    `yaml:"hosts,omitempty" json:"hosts,omitempty"`
}

// Check is a check, of exactly one type: what to check is its spec.
type Check struct {
	Key         string         `yaml:"key" json:"key"`
	Name        string         `yaml:"name,omitempty" json:"name,omitempty"`
	HTTP        map[string]any `yaml:"http,omitempty" json:"http,omitempty"`
	TLS         map[string]any `yaml:"tls,omitempty" json:"tls,omitempty"`
	TCP         map[string]any `yaml:"tcp,omitempty" json:"tcp,omitempty"`
	DNS         map[string]any `yaml:"dns,omitempty" json:"dns,omitempty"`
	Domain      map[string]any `yaml:"domain,omitempty" json:"domain,omitempty"`
	Heartbeat   map[string]any `yaml:"heartbeat,omitempty" json:"heartbeat,omitempty"`
	Settings    Settings       `yaml:"settings,omitempty" json:"settings,omitempty"`
	Maintenance []Window       `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	// Secrets are an http check's secrets, by field: headers.NAME and
	// basic_auth.password. In a file they stand where they go, such as
	// headers: {Authorization: {from_env: API_TOKEN}}; LiftSecrets takes
	// them out of the spec, and Marshal puts them back.
	Secrets map[string]*Secret `yaml:"-" json:"secrets,omitempty"`
}

// Type returns a check's type and spec, or an error when it has none or
// more than one.
func (c Check) Type() (string, map[string]any, error) {
	var typ string
	var spec map[string]any
	n := 0
	for t, s := range map[string]map[string]any{"http": c.HTTP, "tls": c.TLS, "tcp": c.TCP, "dns": c.DNS, "domain": c.Domain, "heartbeat": c.Heartbeat} {
		if s != nil {
			typ, spec = t, s
			n++
		}
	}
	if n != 1 {
		return "", nil, fmt.Errorf("check %s: exactly one of http, tls, tcp, dns, domain and heartbeat", c.Key)
	}
	return typ, spec, nil
}

// Host is a machine, by its name. It may be named before its agent
// enrolls.
type Host struct {
	Key         string      `yaml:"key" json:"key"`
	Settings    Settings    `yaml:"settings,omitempty" json:"settings,omitempty"`
	Maintenance []Window    `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Containers  []Container `yaml:"containers,omitempty" json:"containers,omitempty"`
}

// Container is a container on a host, by its key: compose project and
// service, or its name.
type Container struct {
	Key         string   `yaml:"key" json:"key"`
	Settings    Settings `yaml:"settings,omitempty" json:"settings,omitempty"`
	Maintenance []Window `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
}

// Channel is where alerts go, of exactly one type.
type Channel struct {
	Key        string      `yaml:"key" json:"key"`
	Name       string      `yaml:"name,omitempty" json:"name,omitempty"`
	Email      *Email      `yaml:"email,omitempty" json:"email,omitempty"`
	GoogleChat *GoogleChat `yaml:"googlechat,omitempty" json:"googlechat,omitempty"`
	Webhook    *Webhook    `yaml:"webhook,omitempty" json:"webhook,omitempty"`
	Slack      *Hook       `yaml:"slack,omitempty" json:"slack,omitempty"`
	Discord    *Hook       `yaml:"discord,omitempty" json:"discord,omitempty"`
	Telegram   *Telegram   `yaml:"telegram,omitempty" json:"telegram,omitempty"`
	Ntfy       *Ntfy       `yaml:"ntfy,omitempty" json:"ntfy,omitempty"`
	Pushover   *Pushover   `yaml:"pushover,omitempty" json:"pushover,omitempty"`
}

type Email struct {
	To   []string `yaml:"to" json:"to"`
	SMTP *SMTP    `yaml:"smtp,omitempty" json:"smtp,omitempty"`
}

// SMTP is a mail server of a channel's own.
type SMTP struct {
	Host     string  `yaml:"host" json:"host"`
	Port     int     `yaml:"port,omitempty" json:"port,omitempty"`
	Username string  `yaml:"username,omitempty" json:"username,omitempty"`
	From     string  `yaml:"from" json:"from"`
	TLS      string  `yaml:"tls,omitempty" json:"tls,omitempty"`
	Password *Secret `yaml:"password,omitempty" json:"password,omitempty"`
}

type GoogleChat struct {
	Webhook *Secret `yaml:"webhook" json:"webhook"`
}

type Webhook struct {
	URL        *Secret `yaml:"url" json:"url"`
	SigningKey *Secret `yaml:"signing_key,omitempty" json:"signing_key,omitempty"`
}

// Hook is a Slack or Discord channel: its incoming webhook.
type Hook struct {
	Webhook *Secret `yaml:"webhook" json:"webhook"`
}

type Telegram struct {
	ChatID   string  `yaml:"chat_id" json:"chat_id"`
	BotToken *Secret `yaml:"bot_token" json:"bot_token"`
}

type Ntfy struct {
	// Server is https://ntfy.sh unless set.
	Server string  `yaml:"server,omitempty" json:"server,omitempty"`
	Topic  *Secret `yaml:"topic" json:"topic"`
	Token  *Secret `yaml:"token,omitempty" json:"token,omitempty"`
}

type Pushover struct {
	Token *Secret `yaml:"token" json:"token"`
	User  *Secret `yaml:"user" json:"user"`
}

// Secret is where a secret comes from: exactly one of these.
type Secret struct {
	// Env and File are read by the hub when it sends.
	Env  string `yaml:"env,omitempty" json:"env,omitempty"`
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// FromEnv is read by lotse apply, on the machine it runs on, and sent
	// as Value, which the hub keeps sealed. Value is never in a file.
	FromEnv string `yaml:"from_env,omitempty" json:"from_env,omitempty"`
	Value   string `yaml:"-" json:"value,omitempty"`
	// Keep, written {secret: CHANNEL} by export, keeps the secret the hub
	// has.
	Keep string `yaml:"secret,omitempty" json:"secret,omitempty"`
}

// Window is a maintenance window that comes back, such as
// sun 02:00-04:00. One-off windows are not in files.
type Window struct {
	Schedule string `yaml:"schedule" json:"schedule"`
	Reason   string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// Parse reads a lotse.yaml. Unknown fields are mistakes.
func Parse(b []byte) (Document, error) {
	var d Document
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	// An empty file is an empty document.
	if err := dec.Decode(&d); err != nil && !errors.Is(err, io.EOF) {
		return d, err
	}
	if err := d.LiftSecrets(); err != nil {
		return d, err
	}
	var err error
	d.eachCheck(func(c *Check) {
		for field, s := range c.Secrets {
			if s.Value != "" && err == nil {
				err = fmt.Errorf("check %s: %s: a secret's value is never in a file; {from_env: NAME} reads it where lotse apply runs", c.Key, field)
			}
		}
	})
	return d, err
}

// LiftSecrets moves the secrets that stand in checks' specs to their
// Secrets: a header whose value is a secret, and basic_auth's password.
func (d *Document) LiftSecrets() error {
	var err error
	d.eachCheck(func(c *Check) {
		if err == nil {
			err = c.liftSecrets()
		}
	})
	return err
}

func (c *Check) liftSecrets() error {
	if c.HTTP == nil {
		return nil
	}
	lift := func(field string, v any) error {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		b, _ := json.Marshal(m)
		var s Secret
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return fmt.Errorf("check %s: %s is a secret, {from_env: NAME}, {env: NAME} or {file: PATH}", c.Key, field)
		}
		if c.Secrets == nil {
			c.Secrets = map[string]*Secret{}
		}
		c.Secrets[field] = &s
		return nil
	}
	if headers, ok := c.HTTP["headers"].(map[string]any); ok {
		for k, v := range headers {
			if _, ok := v.(map[string]any); ok {
				if err := lift("headers."+k, v); err != nil {
					return err
				}
				delete(headers, k)
			}
		}
		if len(headers) == 0 {
			delete(c.HTTP, "headers")
		}
	}
	if auth, ok := c.HTTP["basic_auth"].(map[string]any); ok {
		if p, ok := auth["password"]; ok {
			if _, ok := p.(map[string]any); !ok {
				return fmt.Errorf("check %s: basic_auth.password is a secret, {from_env: NAME}, {env: NAME} or {file: PATH}", c.Key)
			}
			if err := lift("basic_auth.password", p); err != nil {
				return err
			}
			delete(auth, "password")
		}
	}
	return nil
}

// inlineSecrets puts a check's secrets back where they go in its spec.
func (c *Check) inlineSecrets() {
	if len(c.Secrets) == 0 || c.HTTP == nil {
		return
	}
	spec := maps.Clone(c.HTTP)
	sub := func(key string) map[string]any {
		m, _ := spec[key].(map[string]any)
		m = maps.Clone(m)
		if m == nil {
			m = map[string]any{}
		}
		spec[key] = m
		return m
	}
	for field, s := range c.Secrets {
		if h, ok := strings.CutPrefix(field, "headers."); ok {
			sub("headers")[h] = s
		} else if field == "basic_auth.password" {
			sub("basic_auth")["password"] = s
		}
	}
	c.HTTP = spec
}

// eachCheck calls f with every check in the document.
func (d *Document) eachCheck(f func(*Check)) {
	for i := range d.Customers {
		for j := range d.Customers[i].Projects {
			envs := d.Customers[i].Projects[j].Environments
			for k := range envs {
				for l := range envs[k].Checks {
					f(&envs[k].Checks[l])
				}
			}
		}
	}
}

// Marshal writes a document as YAML.
func Marshal(d Document) ([]byte, error) {
	// Checks' secrets go back into their specs, in copies: d shares its
	// lists with the caller's.
	d.Customers = slices.Clone(d.Customers)
	for i := range d.Customers {
		c := &d.Customers[i]
		c.Projects = slices.Clone(c.Projects)
		for j := range c.Projects {
			p := &c.Projects[j]
			p.Environments = slices.Clone(p.Environments)
			for k := range p.Environments {
				e := &p.Environments[k]
				e.Checks = slices.Clone(e.Checks)
				for l := range e.Checks {
					e.Checks[l].inlineSecrets()
				}
			}
		}
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}

// ResolveFromEnv reads every {from_env: NAME} secret from this machine's
// environment, as lotse apply does before it sends a document.
func (d *Document) ResolveFromEnv(getenv func(string) string) error {
	var missing []string
	d.EachSecret(func(s *Secret) {
		if s.FromEnv == "" {
			return
		}
		v := getenv(s.FromEnv)
		if v == "" {
			missing = append(missing, s.FromEnv)
			return
		}
		s.Value, s.FromEnv = v, ""
	})
	if len(missing) > 0 {
		return fmt.Errorf("not set here: %s", strings.Join(missing, ", "))
	}
	return nil
}

// EachSecret calls f with every secret in the document.
func (d *Document) EachSecret(f func(*Secret)) {
	each := func(list []Channel) {
		for i := range list {
			for _, s := range list[i].secrets() {
				f(s)
			}
		}
	}
	each(d.Channels)
	d.eachCheck(func(c *Check) {
		for _, k := range slices.Sorted(maps.Keys(c.Secrets)) {
			if c.Secrets[k] != nil {
				f(c.Secrets[k])
			}
		}
	})
	for i := range d.Customers {
		c := &d.Customers[i]
		each(c.Channels)
		for j := range c.Projects {
			p := &c.Projects[j]
			each(p.Channels)
			for k := range p.Environments {
				each(p.Environments[k].Channels)
			}
		}
	}
}

// secrets are a channel's secrets, of whichever type it is.
func (c *Channel) secrets() []*Secret {
	var all []*Secret
	if c.Email != nil && c.Email.SMTP != nil {
		all = append(all, c.Email.SMTP.Password)
	}
	if c.GoogleChat != nil {
		all = append(all, c.GoogleChat.Webhook)
	}
	if c.Webhook != nil {
		all = append(all, c.Webhook.URL, c.Webhook.SigningKey)
	}
	for _, h := range []*Hook{c.Slack, c.Discord} {
		if h != nil {
			all = append(all, h.Webhook)
		}
	}
	if c.Telegram != nil {
		all = append(all, c.Telegram.BotToken)
	}
	if c.Ntfy != nil {
		all = append(all, c.Ntfy.Topic, c.Ntfy.Token)
	}
	if c.Pushover != nil {
		all = append(all, c.Pushover.Token, c.Pushover.User)
	}
	return slices.DeleteFunc(all, func(s *Secret) bool { return s == nil })
}

// ScopeRef names a scope by its key, and optionally its name.
type ScopeRef struct{ Key, Name string }
