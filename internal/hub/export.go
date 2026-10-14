package hub

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/store"
)

// lotse export (design 9.2): a scope written as lotse.yaml, so that it can
// go from the UI to Git and back with lotse apply. Secrets are never
// written: references stay references, and a secret the hub keeps sealed
// becomes {secret: CHANNEL}, which apply leaves as it is.

func (h *hub) exportDocument(ctx context.Context, path string) (apply.Document, error) {
	var d apply.Document
	t, err := h.loadTree(ctx)
	if err != nil {
		return d, err
	}
	windows, err := h.db.Maintenances(ctx)
	if err != nil {
		return d, err
	}
	byPath := map[string]store.Scope{}
	for _, x := range t.scopes {
		byPath[x.Path] = x
	}
	root, ok := byPath[path]
	if !ok {
		return d, store.ErrNotFound
	}
	settingsOf := func(kind string, id int64) apply.Settings {
		set := t.settings[store.Subject{Kind: kind, ID: id}]
		if len(set) == 0 {
			return nil
		}
		out := apply.Settings{}
		for k, v := range set {
			var x any
			json.Unmarshal(v, &x)
			out[k] = x
		}
		return out
	}
	windowsOf := func(kind string, id int64) []apply.Window {
		var out []apply.Window
		for _, m := range windows {
			if m.SubjectKind == kind && m.SubjectID == id && m.Schedule != nil {
				out = append(out, apply.Window{Schedule: *m.Schedule, Reason: m.Reason})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Schedule < out[j].Schedule })
		return out
	}
	channelsOf := func(scopeID int64) []apply.Channel {
		var out []apply.Channel
		for _, c := range t.channels {
			if c.ScopeID == scopeID {
				out = append(out, h.exportChannel(c))
			}
		}
		return out
	}
	children := func(parent int64, kind string) []store.Scope {
		var out []store.Scope
		for _, x := range t.scopes {
			if x.ParentID != nil && *x.ParentID == parent && x.Kind == kind {
				out = append(out, x)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
		return out
	}
	// Only the root and what is below it is written in full; the scopes
	// above it are named by their keys, so that apply finds the way.
	full := func(x store.Scope) bool { return path == "" || x.Path == path || strings.HasPrefix(x.Path, path+"/") }
	env := func(x store.Scope) apply.Environment {
		e := apply.Environment{Key: x.Slug, Name: x.Name, Settings: settingsOf(x.Kind, x.ID), Channels: channelsOf(x.ID), Maintenance: windowsOf(x.Kind, x.ID)}
		var checks []store.Check
		for _, c := range t.checks {
			if c.EnvironmentID == x.ID && c.ContainerID == nil {
				checks = append(checks, c)
			}
		}
		sort.Slice(checks, func(i, j int) bool { return checks[i].Slug < checks[j].Slug })
		for _, c := range checks {
			var spec map[string]any
			json.Unmarshal(c.Spec, &spec)
			out := apply.Check{Key: c.Slug, Settings: settingsOf("check", c.ID), Maintenance: windowsOf("check", c.ID)}
			if c.Name != c.Slug {
				out.Name = c.Name
			}
			switch c.Type {
			case "http":
				out.HTTP = spec
			case "tls":
				out.TLS = spec
			case "tcp":
				out.TCP = spec
			case "heartbeat":
				out.Heartbeat = spec
			}
			e.Checks = append(e.Checks, out)
		}
		var hosts []store.Host
		for _, hst := range t.hosts {
			if hst.EnvironmentID != nil && *hst.EnvironmentID == x.ID {
				hosts = append(hosts, hst)
			}
		}
		sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
		for _, hst := range hosts {
			out := apply.Host{Key: hst.Name, Settings: settingsOf("host", hst.ID), Maintenance: windowsOf("host", hst.ID)}
			var containers []store.Container
			for _, c := range t.containers {
				if c.HostID == hst.ID {
					containers = append(containers, c)
				}
			}
			sort.Slice(containers, func(i, j int) bool { return containers[i].Slug < containers[j].Slug })
			for _, c := range containers {
				s, w := settingsOf("container", c.ID), windowsOf("container", c.ID)
				if s != nil || w != nil {
					out.Containers = append(out.Containers, apply.Container{Key: c.Slug, Settings: s, Maintenance: w})
				}
			}
			e.Hosts = append(e.Hosts, out)
		}
		return e
	}
	project := func(x store.Scope) apply.Project {
		p := apply.Project{Key: x.Slug}
		if full(x) {
			p.Name, p.Settings, p.Channels, p.Maintenance = x.Name, settingsOf(x.Kind, x.ID), channelsOf(x.ID), windowsOf(x.Kind, x.ID)
		}
		for _, e := range children(x.ID, "environment") {
			if full(e) {
				p.Environments = append(p.Environments, env(e))
			}
		}
		return p
	}
	customer := func(x store.Scope) apply.Customer {
		c := apply.Customer{Key: x.Slug}
		if full(x) {
			c.Name, c.Settings, c.Channels, c.Maintenance = x.Name, settingsOf(x.Kind, x.ID), channelsOf(x.ID), windowsOf(x.Kind, x.ID)
		}
		for _, p := range children(x.ID, "project") {
			if full(p) || strings.HasPrefix(path, p.Path+"/") {
				c.Projects = append(c.Projects, project(p))
			}
		}
		return c
	}
	if root.Kind == "global" {
		d.Settings, d.Channels, d.Maintenance = settingsOf("global", root.ID), channelsOf(root.ID), windowsOf("global", root.ID)
	}
	for _, c := range children(store.GlobalScope, "customer") {
		if full(c) || strings.HasPrefix(path, c.Path+"/") {
			d.Customers = append(d.Customers, customer(c))
		}
	}
	return d, nil
}

// exportChannel writes a channel, its secrets as references or
// placeholders.
func (h *hub) exportChannel(c store.Channel) apply.Channel {
	out := apply.Channel{Key: c.Slug}
	if c.Name != c.Slug {
		out.Name = c.Name
	}
	sec, _ := h.openSecrets(c.Secrets, c.SecretRefs)
	secret := func(field string) *apply.Secret {
		if r, ok := sec.refs[field]; ok {
			return &apply.Secret{Env: r.Env, File: r.File}
		}
		if _, ok := sec.sealed[field]; ok {
			return &apply.Secret{Keep: c.Slug}
		}
		return nil
	}
	var spec channelSpec
	json.Unmarshal(c.Spec, &spec)
	switch c.Type {
	case "email":
		out.Email = &apply.Email{To: spec.To}
		if s := spec.SMTP; s != nil {
			out.Email.SMTP = &apply.SMTP{Host: s.Host, Port: s.Port, Username: s.Username, From: s.From, TLS: s.TLS, Password: secret("password")}
		}
	case "googlechat":
		out.GoogleChat = &apply.GoogleChat{Webhook: secret("url")}
	case "webhook":
		out.Webhook = &apply.Webhook{URL: secret("url"), SigningKey: secret("signing_key")}
	}
	return out
}
