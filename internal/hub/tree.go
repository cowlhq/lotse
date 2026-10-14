package hub

import (
	"context"
	"encoding/json"

	"lotse.sh/internal/config"
	"lotse.sh/internal/store"
)

// tree is the hierarchy at one moment, read at once: the scopes, and the
// hosts, containers and checks in them. The alert loop works from it, so
// that it does not ask the database about every object.
type tree struct {
	scopes     map[int64]store.Scope
	hosts      map[int64]store.Host
	containers map[int64]store.Container
	checks     map[int64]store.Check
	probes     []store.Probe
	channels   []store.Channel
	settings   map[store.Subject]map[string]json.RawMessage
}

func (h *hub) loadTree(ctx context.Context) (*tree, error) {
	return h.loadTreeFrom(ctx, h.db)
}

// loadTreeFrom reads the tree from a store, which may be a transaction.
func (h *hub) loadTreeFrom(ctx context.Context, db *store.Store) (*tree, error) {
	t := &tree{scopes: map[int64]store.Scope{}, hosts: map[int64]store.Host{}, containers: map[int64]store.Container{}, checks: map[int64]store.Check{}}
	global, err := db.Scope(ctx, store.GlobalScope)
	if err != nil {
		return nil, err
	}
	t.scopes[global.ID] = global
	scopes, err := db.Scopes(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range scopes {
		t.scopes[x.ID] = x
	}
	hosts, err := db.Hosts(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range hosts {
		t.hosts[x.ID] = x
	}
	containers, err := db.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range containers {
		t.containers[x.ID] = x
	}
	checks, err := db.Checks(ctx, 0)
	if err != nil {
		return nil, err
	}
	for _, x := range checks {
		t.checks[x.ID] = x
	}
	if t.probes, err = db.Probes(ctx); err != nil {
		return nil, err
	}
	if t.channels, err = db.Channels(ctx); err != nil {
		return nil, err
	}
	if t.settings, err = db.AllSettings(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// chain is an object and everything above it, nearest first: a container,
// its host, the host's environment, project, customer, and the global
// scope. Settings are inherited along it, and maintenance covers it. It is
// nil for an object that is not there.
func (t *tree) chain(kind string, id int64) []config.Level {
	var levels []config.Level
	scopeID := int64(store.GlobalScope)
	switch kind {
	case "container":
		c, ok := t.containers[id]
		if !ok {
			return nil
		}
		levels = append(levels, config.Level{Kind: "container", ID: c.ID, Path: c.Name, Name: c.Name})
		if c.EnvironmentID == nil {
			return append(levels, t.chain("host", c.HostID)...)
		}
		// Put elsewhere by its lotse.env label: that environment is its.
		return append(levels, t.chain("environment", *c.EnvironmentID)...)
	case "host":
		x, ok := t.hosts[id]
		if !ok {
			return nil
		}
		levels = append(levels, config.Level{Kind: "host", ID: x.ID, Path: x.Name, Name: x.Name})
		if x.EnvironmentID != nil {
			scopeID = *x.EnvironmentID
		}
	case "check":
		c, ok := t.checks[id]
		if !ok {
			return nil
		}
		levels = append(levels, config.Level{Kind: "check", ID: c.ID, Path: c.Path, Name: c.Name})
		scopeID = c.EnvironmentID
	case "probe", "channel":
		// Their rules are set globally.
	default:
		x, ok := t.scopes[id]
		if !ok || x.Kind != kind {
			return nil
		}
		scopeID = id
	}
	for {
		x, ok := t.scopes[scopeID]
		if !ok {
			return levels
		}
		levels = append(levels, config.Level{Kind: x.Kind, ID: x.ID, Path: x.Path, Name: x.Name})
		if x.ParentID == nil {
			return levels
		}
		scopeID = *x.ParentID
	}
}

// value is a setting's effective value along a chain, and where it is set;
// nil for the default.
func (t *tree) value(chain []config.Level, key string) (json.RawMessage, *config.Level) {
	for i, l := range chain {
		if v, ok := t.settings[store.Subject{Kind: l.Kind, ID: l.ID}][key]; ok {
			return v, &chain[i]
		}
	}
	k, _ := config.Lookup(key)
	return k.Default, nil
}

// path names an object in alerts: hosts and containers by where they are,
// such as acme/shop/prod/web1/shop-db-1.
func (t *tree) path(kind string, id int64) string {
	switch kind {
	case "host":
		x := t.hosts[id]
		if x.EnvironmentID != nil {
			if env, ok := t.scopes[*x.EnvironmentID]; ok {
				return env.Path + "/" + x.Name
			}
		}
		return x.Name
	case "container":
		c := t.containers[id]
		if c.EnvironmentID != nil {
			if env, ok := t.scopes[*c.EnvironmentID]; ok {
				return env.Path + "/" + t.hosts[c.HostID].Name + "/" + c.Slug
			}
		}
		// By its key, which outlives redeploys, not Docker's name.
		return t.path("host", c.HostID) + "/" + c.Slug
	case "check":
		return t.checks[id].Path
	case "probe":
		for _, p := range t.probes {
			if p.ID == id {
				return "probe " + p.Slug
			}
		}
	case "channel":
		for _, c := range t.channels {
			if c.ID == id {
				return "channel " + channelKey(c)
			}
		}
	default:
		if x, ok := t.scopes[id]; ok {
			if x.Kind == "global" {
				return "global"
			}
			return x.Path
		}
	}
	return ""
}

// channelKey names a channel with its scope: acme/ops-chat, or ops-mail
// for one of the global scope.
func channelKey(c store.Channel) string {
	if c.ScopePath == "" {
		return c.Slug
	}
	return c.ScopePath + "/" + c.Slug
}
