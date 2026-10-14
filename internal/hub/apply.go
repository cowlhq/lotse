package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/auth"
	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// lotse apply (design 9.2): a document is matched against what the hub has,
// by key, and what differs is changed, all in one transaction. A dry run
// takes the same way and rolls back at the end, so that what it shows is
// exactly what applying would do.

// change is one difference between the document and the hub.
type change struct {
	Action string      `json:"action"` // create, update, delete
	Kind   string      `json:"kind"`   // customer, project, environment, setting, channel, check, maintenance
	Path   string      `json:"path"`
	Field  string      `json:"field,omitempty"`
	Old    any         `json:"old,omitempty"`
	New    any         `json:"new,omitempty"`
	Last   *lastChange `json:"last,omitempty"`
	// of is the object it is about, for its last change.
	of store.Subject
}

// lastChange says who changed an object last, and how: "changed
// 2026-10-12 by andreas via ui".
type lastChange struct {
	At  int64  `json:"at"`
	By  string `json:"by"`
	Via string `json:"via"`
}

// applyOptions are how a document is applied.
type applyOptions struct {
	dryRun bool
	// prune deletes, within pruneScope, what the document does not name.
	prune      bool
	pruneScope string
	// within is the path of the scope the token may change; "" for all.
	within string
	all    bool
}

type applyResult struct {
	Changes  []change `json:"changes"`
	Warnings []string `json:"warnings"`
	Applied  bool     `json:"applied"`
}

var errDryRun = errors.New("dry run")

// applier works through one document in one transaction.
type applier struct {
	h        *hub
	tx       *store.Store
	ctx      context.Context
	actor    store.Actor
	opts     applyOptions
	now      int64
	changes  []change
	warnings []string
	// seen are the objects the document names, for pruning.
	seen map[string]bool
	// scopes by path, as they are now in the transaction.
	scopes map[string]store.Scope
}

// applyDocument applies a document, or shows what it would change.
func (h *hub) applyDocument(ctx context.Context, actor store.Actor, doc apply.Document, opts applyOptions) (applyResult, error) {
	a := &applier{h: h, ctx: ctx, actor: actor, opts: opts, now: time.Now().Unix(), seen: map[string]bool{}}
	err := h.db.WithTx(ctx, func(tx *store.Store) error {
		a.tx = tx
		if err := a.loadScopes(); err != nil {
			return err
		}
		if err := a.document(doc); err != nil {
			return err
		}
		if opts.prune {
			if err := a.prune(); err != nil {
				return err
			}
		}
		if opts.dryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return applyResult{}, err
	}
	res := applyResult{Changes: a.changes, Warnings: a.warnings, Applied: !opts.dryRun}
	if res.Changes == nil {
		res.Changes = []change{}
	}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	if !opts.dryRun && len(a.changes) > 0 {
		h.reassign(context.WithoutCancel(ctx))
		h.pushConfig(context.WithoutCancel(ctx))
		h.alerts.poke()
	}
	return res, nil
}

// applyError is a mistake in the document, said in its terms.
type applyError struct{ msg string }

func (e applyError) Error() string { return e.msg }

func fail(format string, args ...any) error { return applyError{fmt.Sprintf(format, args...)} }

func (a *applier) loadScopes() error {
	list, err := a.tx.Scopes(a.ctx)
	if err != nil {
		return err
	}
	global, err := a.tx.Scope(a.ctx, store.GlobalScope)
	if err != nil {
		return err
	}
	a.scopes = map[string]store.Scope{"": global}
	for _, x := range list {
		a.scopes[x.Path] = x
	}
	return nil
}

// may says whether the token may change what is at path.
func (a *applier) may(path string) error {
	if a.opts.all || a.opts.within == "" || path == a.opts.within || strings.HasPrefix(path, a.opts.within+"/") {
		return nil
	}
	where := path
	if where == "" {
		where = "the global settings"
	}
	return fail("the token is for %s; %s is outside it", a.opts.within, where)
}

func (a *applier) note(c change) {
	if c.Action != "create" {
		if id, kind := a.subjectOf(c); id != 0 {
			if list, err := a.tx.Changes(a.ctx, kind, id, 1); err == nil && len(list) > 0 {
				c.Last = &lastChange{At: list[0].At, By: list[0].ActorName, Via: list[0].Via}
			}
		}
	}
	a.changes = append(a.changes, c)
}

// subjectOf finds the object a change is about, for its last change.
func (a *applier) subjectOf(c change) (int64, string) {
	if c.of.ID != 0 {
		return c.of.ID, c.of.Kind
	}
	switch c.Kind {
	case "customer", "project", "environment":
		if x, ok := a.scopes[c.Path]; ok {
			return x.ID, x.Kind
		}
	}
	return 0, ""
}

func nameOr(name, key string) string {
	if strings.TrimSpace(name) == "" {
		return key
	}
	return strings.TrimSpace(name)
}

func (a *applier) document(d apply.Document) error {
	if len(d.Settings) > 0 || len(d.Channels) > 0 || len(d.Maintenance) > 0 {
		if err := a.may(""); err != nil {
			return err
		}
	}
	global := a.scopes[""]
	if err := a.level(global, d.Settings, d.Channels, d.Maintenance); err != nil {
		return err
	}
	for _, c := range d.Customers {
		cs, err := a.scope([]apply.ScopeRef{{Key: c.Key, Name: c.Name}})
		if err != nil {
			return err
		}
		if err := a.level(cs, c.Settings, c.Channels, c.Maintenance); err != nil {
			return err
		}
		for _, p := range c.Projects {
			ps, err := a.scope([]apply.ScopeRef{{Key: c.Key}, {Key: p.Key, Name: p.Name}})
			if err != nil {
				return err
			}
			if err := a.level(ps, p.Settings, p.Channels, p.Maintenance); err != nil {
				return err
			}
			for _, e := range p.Environments {
				es, err := a.scope([]apply.ScopeRef{{Key: c.Key}, {Key: p.Key}, {Key: e.Key, Name: e.Name}})
				if err != nil {
					return err
				}
				if err := a.level(es, e.Settings, e.Channels, e.Maintenance); err != nil {
					return err
				}
				if err := a.checks(es, e.Checks); err != nil {
					return err
				}
				if err := a.hosts(es, e.Hosts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// scope finds or creates a scope from its keys, the last one named.
func (a *applier) scope(refs []apply.ScopeRef) (store.Scope, error) {
	keys := make([]string, len(refs))
	for i, r := range refs {
		if !slugPattern.MatchString(r.Key) {
			return store.Scope{}, fail("%q is not a key: lowercase letters, digits and dashes, up to 40", r.Key)
		}
		keys[i] = r.Key
	}
	path := strings.Join(keys, "/")
	if err := a.may(path); err != nil {
		return store.Scope{}, err
	}
	kinds := []string{"customer", "project", "environment"}
	last := refs[len(refs)-1]
	a.seen["scope:"+path] = true
	if x, ok := a.scopes[path]; ok {
		if last.Name != "" && strings.TrimSpace(last.Name) != x.Name {
			a.note(change{Action: "update", Kind: x.Kind, Path: path, Field: "name", Old: x.Name, New: strings.TrimSpace(last.Name)})
			if err := a.tx.RenameScope(a.ctx, a.actor, x.ID, strings.TrimSpace(last.Name)); err != nil {
				return x, err
			}
			x.Name = strings.TrimSpace(last.Name)
			a.scopes[path] = x
		}
		return x, nil
	}
	names := make([]store.ScopeName, len(refs))
	for i, r := range refs {
		p := strings.Join(keys[:i+1], "/")
		name := nameOr(r.Name, r.Key)
		if x, ok := a.scopes[p]; ok {
			name = x.Name
		}
		names[i] = store.ScopeName{Slug: r.Key, Name: name}
	}
	x, err := a.tx.EnsureScope(a.ctx, a.actor, names)
	if err != nil {
		return x, err
	}
	a.note(change{Action: "create", Kind: kinds[len(refs)-1], Path: path, New: x.Name})
	return x, a.loadScopes()
}

// level is what every scope has: settings, channels, maintenance.
func (a *applier) level(x store.Scope, s apply.Settings, channels []apply.Channel, windows []apply.Window) error {
	sub := store.Subject{Kind: x.Kind, ID: x.ID, Path: x.Path}
	if err := a.settings(sub, s); err != nil {
		return err
	}
	if err := a.channels(x, channels); err != nil {
		return err
	}
	return a.windows(sub, windows)
}

func shown(path string) string {
	if path == "" {
		return "global"
	}
	return path
}

// settings makes what is set on a subject what the document says.
func (a *applier) settings(sub store.Subject, s apply.Settings) error {
	if len(s) == 0 {
		return nil
	}
	have, err := a.tx.Settings(a.ctx, []store.Subject{sub})
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		k, ok := config.Lookup(key)
		if !ok {
			return fail("%s: there is no setting %q", shown(sub.Path), key)
		}
		if !k.SettableOn(sub.Kind) {
			return fail("%s: %s cannot be set on a %s", shown(sub.Path), key, sub.Kind)
		}
		value, err := json.Marshal(s[key])
		if err != nil {
			return err
		}
		if err := k.Check(value); err != nil {
			return fail("%s: %s: %v", shown(sub.Path), key, err)
		}
		a.seen[settingKey(sub, key)] = true
		old, set := have[0][key]
		if set && sameJSON(old, value) {
			continue
		}
		c := change{Action: "create", Kind: "setting", Path: shown(sub.Path), Field: key, New: json.RawMessage(value), of: sub}
		if set {
			c.Action, c.Old = "update", json.RawMessage(old)
		}
		a.note(c)
		if err := a.tx.SetSetting(a.ctx, a.actor, sub, key, value, a.now); err != nil {
			return err
		}
	}
	return nil
}

func settingKey(sub store.Subject, key string) string {
	return fmt.Sprintf("setting:%s:%d:%s", sub.Kind, sub.ID, key)
}

func sameJSON(a, b []byte) bool {
	var x, y bytes.Buffer
	if json.Compact(&x, a) != nil || json.Compact(&y, b) != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(x.Bytes(), y.Bytes())
}

// channels makes a scope's channels what the document says.
func (a *applier) channels(x store.Scope, list []apply.Channel) error {
	if len(list) == 0 {
		return nil
	}
	all, err := a.tx.Channels(a.ctx)
	if err != nil {
		return err
	}
	for _, c := range list {
		if !slugPattern.MatchString(c.Key) {
			return fail("channel %q: a key has lowercase letters, digits and dashes", c.Key)
		}
		path := channelPath(x, c.Key)
		a.seen["channel:"+path] = true
		var have *store.Channel
		for i := range all {
			if all[i].ScopeID == x.ID && all[i].Slug == c.Key {
				have = &all[i]
			}
		}
		typ, raw, wanted, err := channelOf(c)
		if err != nil {
			return fail("channel %s: %v", path, err)
		}
		sec := secrets{sealed: map[string]string{}, refs: map[string]secretRef{}}
		var old secrets
		if have != nil {
			if have.Type != typ {
				return fail("channel %s is %s, not %s; delete it first to change its type", path, have.Type, typ)
			}
			if old, err = a.h.openSecrets(have.Secrets, have.SecretRefs); err != nil {
				return err
			}
		}
		inputs := map[string]api.SecretInput{}
		for field, s := range wanted {
			switch {
			case s.FromEnv != "":
				return fail("channel %s: from_env %s must be read by lotse apply", path, s.FromEnv)
			case s.Keep != "":
				if have == nil || !old.has(field) {
					return fail("channel %s: there is no secret %s to keep", path, field)
				}
				if v, ok := old.sealed[field]; ok {
					sec.sealed[field] = v
				}
				if r, ok := old.refs[field]; ok {
					sec.refs[field] = r
				}
			default:
				inputs[field] = api.SecretInput{Value: nonEmpty(s.Value), Env: nonEmpty(s.Env), File: nonEmpty(s.File)}
			}
		}
		if err := sec.apply(secretFields[typ], inputs); err != nil {
			return fail("channel %s: %v", path, err)
		}
		spec, err := checkChannel(typ, raw, sec)
		if err != nil {
			return fail("channel %s: %v", path, err)
		}
		name := nameOr(c.Name, c.Key)
		if have == nil {
			a.note(change{Action: "create", Kind: "channel", Path: path, New: typ})
			sealed, refs := a.h.closeSecrets(sec)
			if _, err := a.tx.CreateChannel(a.ctx, a.actor, store.Channel{ScopeID: x.ID, Slug: c.Key, Name: name, Type: typ, Spec: spec, Secrets: sealed, SecretRefs: refs, CreatedAt: a.now}); err != nil {
				return err
			}
			continue
		}
		sameSecrets := sameSecretSets(old, sec)
		if have.Name == name && sameJSON(have.Spec, spec) && sameSecrets {
			continue
		}
		ch := change{Action: "update", Kind: "channel", Path: path, of: store.Subject{Kind: "channel", ID: have.ID}}
		switch {
		case have.Name != name:
			ch.Field, ch.Old, ch.New = "name", have.Name, name
		case !sameJSON(have.Spec, spec):
			ch.Field, ch.Old, ch.New = "spec", json.RawMessage(have.Spec), json.RawMessage(spec)
		default:
			ch.Field = "secrets"
		}
		a.note(ch)
		updated := *have
		updated.Name, updated.Spec = name, spec
		updated.Secrets, updated.SecretRefs = a.h.closeSecrets(sec)
		if sameSecrets {
			updated.Secrets, updated.SecretRefs = have.Secrets, have.SecretRefs
		}
		if _, err := a.tx.UpdateChannel(a.ctx, a.actor, updated); err != nil {
			return err
		}
	}
	return nil
}

func channelPath(x store.Scope, key string) string {
	if x.Path == "" {
		return key
	}
	return x.Path + "/" + key
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func sameSecretSets(a, b secrets) bool {
	if len(a.sealed) != len(b.sealed) || len(a.refs) != len(b.refs) {
		return false
	}
	for k, v := range a.sealed {
		if b.sealed[k] != v {
			return false
		}
	}
	for k, v := range a.refs {
		if b.refs[k] != v {
			return false
		}
	}
	return true
}

// channelOf reads a channel of the document: its type, its spec as the
// hub checks it, and its secrets by field.
func channelOf(c apply.Channel) (string, map[string]any, map[string]*apply.Secret, error) {
	n := 0
	for _, set := range []bool{c.Email != nil, c.GoogleChat != nil, c.Webhook != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return "", nil, nil, errors.New("exactly one of email, googlechat and webhook")
	}
	secrets := map[string]*apply.Secret{}
	switch {
	case c.Email != nil:
		raw := map[string]any{"to": c.Email.To}
		if s := c.Email.SMTP; s != nil {
			raw["smtp"] = map[string]any{"host": s.Host, "port": s.Port, "username": s.Username, "from": s.From, "tls": s.TLS}
			if s.Password != nil {
				secrets["password"] = s.Password
			}
		}
		return "email", raw, secrets, nil
	case c.GoogleChat != nil:
		if c.GoogleChat.Webhook != nil {
			secrets["url"] = c.GoogleChat.Webhook
		}
		return "googlechat", map[string]any{}, secrets, nil
	default:
		if c.Webhook.URL != nil {
			secrets["url"] = c.Webhook.URL
		}
		if c.Webhook.SigningKey != nil {
			secrets["signing_key"] = c.Webhook.SigningKey
		}
		return "webhook", map[string]any{}, secrets, nil
	}
}

// windows makes the maintenance that comes back on a subject what the
// document says. One-off windows are left alone.
func (a *applier) windows(sub store.Subject, list []apply.Window) error {
	if len(list) == 0 {
		return nil
	}
	all, err := a.tx.Maintenances(a.ctx)
	if err != nil {
		return err
	}
	for _, w := range list {
		schedule := strings.Join(strings.Fields(w.Schedule), " ")
		if _, err := config.ParseSchedule(schedule); err != nil {
			return fail("%s: maintenance %q: %v", shown(sub.Path), w.Schedule, err)
		}
		a.seen[windowKey(sub, schedule)] = true
		var have *store.Maintenance
		for i := range all {
			m := all[i]
			if m.SubjectKind == sub.Kind && m.SubjectID == sub.ID && m.Schedule != nil && *m.Schedule == schedule {
				have = &all[i]
			}
		}
		if have != nil && have.Reason == w.Reason {
			continue
		}
		c := change{Action: "create", Kind: "maintenance", Path: shown(sub.Path), Field: schedule, New: w.Reason, of: sub}
		if have != nil {
			c.Action, c.Old = "update", have.Reason
			if err := a.tx.DeleteMaintenance(a.ctx, a.actor, have.ID, sub.Path); err != nil {
				return err
			}
		}
		a.note(c)
		if _, err := a.tx.AddMaintenance(a.ctx, a.actor, store.Maintenance{SubjectKind: sub.Kind, SubjectID: sub.ID, Schedule: &schedule, Reason: w.Reason, CreatedBy: a.actor.Name, CreatedAt: a.now}, sub.Path); err != nil {
			return err
		}
	}
	return nil
}

func windowKey(sub store.Subject, schedule string) string {
	return fmt.Sprintf("window:%s:%d:%s", sub.Kind, sub.ID, schedule)
}

// checks makes an environment's checks what the document says.
func (a *applier) checks(env store.Scope, list []apply.Check) error {
	have, err := a.tx.Checks(a.ctx, env.ID)
	if err != nil {
		return err
	}
	for _, c := range list {
		if !slugPattern.MatchString(c.Key) {
			return fail("check %q: a key has lowercase letters, digits and dashes", c.Key)
		}
		path := env.Path + "/" + c.Key
		a.seen["check:"+path] = true
		typ, raw, err := c.Type()
		if err != nil {
			return fail("%s", err)
		}
		b, _ := json.Marshal(raw)
		spec, err := probe.Normalize(typ, b)
		if err != nil {
			return fail("check %s: %v", path, err)
		}
		name := nameOr(c.Name, c.Key)
		var existing *store.Check
		for i := range have {
			if have[i].Slug == c.Key && have[i].ContainerID == nil {
				existing = &have[i]
			}
		}
		var check store.Check
		switch {
		case existing == nil:
			a.note(change{Action: "create", Kind: "check", Path: path, New: typ})
			check = store.Check{EnvironmentID: env.ID, Slug: c.Key, Name: name, Type: typ, Spec: spec, CreatedAt: a.now}
			var pingHash string
			if typ == "heartbeat" {
				token, hash := auth.NewToken("")
				sealed := a.h.box.Seal(token)
				check.PingSealed, pingHash = &sealed, hash
			}
			if check, err = a.tx.CreateCheck(a.ctx, a.actor, check, pingHash); err != nil {
				return err
			}
		case existing.Type != typ:
			return fail("check %s is %s, not %s; delete it first to change its type", path, existing.Type, typ)
		default:
			check = *existing
			if existing.Name != name || !sameJSON(existing.Spec, spec) {
				ch := change{Action: "update", Kind: "check", Path: path, Field: "spec", Old: json.RawMessage(existing.Spec), New: json.RawMessage(spec), of: store.Subject{Kind: "check", ID: existing.ID}}
				if existing.Name != name {
					ch.Field, ch.Old, ch.New = "name", existing.Name, name
				}
				a.note(ch)
				if check, err = a.tx.UpdateCheck(a.ctx, a.actor, existing.ID, name, spec); err != nil {
					return err
				}
			}
		}
		sub := store.Subject{Kind: "check", ID: check.ID, Path: path}
		if err := a.settings(sub, c.Settings); err != nil {
			return err
		}
		if err := a.windows(sub, c.Maintenance); err != nil {
			return err
		}
	}
	return nil
}

// hosts sets what the document says on an environment's hosts and their
// containers. Hosts come from agents: one that has not enrolled yet is
// named in a warning, and nothing is set on it.
func (a *applier) hosts(env store.Scope, list []apply.Host) error {
	if len(list) == 0 {
		return nil
	}
	all, err := a.tx.Hosts(a.ctx)
	if err != nil {
		return err
	}
	for _, h := range list {
		var host *store.Host
		for i := range all {
			if all[i].EnvironmentID != nil && *all[i].EnvironmentID == env.ID && all[i].Name == h.Key {
				host = &all[i]
			}
		}
		path := env.Path + "/" + h.Key
		if host == nil {
			a.warnings = append(a.warnings, fmt.Sprintf("%s has not enrolled yet; what is set on it waits until it has", path))
			continue
		}
		sub := store.Subject{Kind: "host", ID: host.ID, Path: path}
		if err := a.settings(sub, h.Settings); err != nil {
			return err
		}
		if err := a.windows(sub, h.Maintenance); err != nil {
			return err
		}
		containers, err := a.tx.Containers(a.ctx, host.ID)
		if err != nil {
			return err
		}
		for _, c := range h.Containers {
			var found *store.Container
			for i := range containers {
				if containers[i].Slug == c.Key && containers[i].GoneAt == nil {
					found = &containers[i]
				}
			}
			cpath := path + "/" + c.Key
			if found == nil {
				a.warnings = append(a.warnings, fmt.Sprintf("%s does not run on %s; what is set on it waits until it does", c.Key, path))
				continue
			}
			csub := store.Subject{Kind: "container", ID: found.ID, Path: cpath}
			if err := a.settings(csub, c.Settings); err != nil {
				return err
			}
			if err := a.windows(csub, c.Maintenance); err != nil {
				return err
			}
		}
	}
	return nil
}

// prune deletes, within the prune scope, what the document does not name:
// scopes, channels, checks, settings and maintenance that comes back.
// Hosts and containers come from agents and are never deleted.
func (a *applier) prune() error {
	root := a.opts.pruneScope
	if err := a.may(root); err != nil {
		return err
	}
	if _, ok := a.scopes[root]; !ok {
		return fail("there is no scope %s to prune", root)
	}
	under := func(path string) bool {
		return root == "" || path == root || strings.HasPrefix(path, root+"/")
	}

	// Settings and maintenance on scopes, hosts, containers and checks.
	settings, err := a.tx.AllSettings(a.ctx)
	if err != nil {
		return err
	}
	t, err := a.h.loadTreeFrom(a.ctx, a.tx)
	if err != nil {
		return err
	}
	pathOf := func(kind string, id int64) (string, bool) {
		switch kind {
		case "global", "customer", "project", "environment":
			x, ok := t.scopes[id]
			return x.Path, ok
		}
		chain := t.chain(kind, id)
		if chain == nil {
			return "", false
		}
		for _, l := range chain {
			if l.Kind == "environment" {
				return objectPath(t, kind, id, l.Path), true
			}
		}
		return "", false
	}
	subjects := make([]store.Subject, 0, len(settings))
	for sub := range settings {
		subjects = append(subjects, sub)
	}
	sort.Slice(subjects, func(i, j int) bool {
		return subjects[i].Kind+fmt.Sprint(subjects[i].ID) < subjects[j].Kind+fmt.Sprint(subjects[j].ID)
	})
	for _, sub := range subjects {
		path, ok := pathOf(sub.Kind, sub.ID)
		if !ok || !under(path) {
			continue
		}
		keys := make([]string, 0, len(settings[sub]))
		for k := range settings[sub] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if a.seen[settingKey(sub, key)] {
				continue
			}
			a.note(change{Action: "delete", Kind: "setting", Path: shown(path), Field: key, Old: json.RawMessage(settings[sub][key]), of: sub})
			if err := a.tx.SetSetting(a.ctx, a.actor, store.Subject{Kind: sub.Kind, ID: sub.ID, Path: path}, key, nil, a.now); err != nil {
				return err
			}
		}
	}
	windows, err := a.tx.Maintenances(a.ctx)
	if err != nil {
		return err
	}
	for _, m := range windows {
		if m.Schedule == nil {
			continue
		}
		path, ok := pathOf(m.SubjectKind, m.SubjectID)
		sub := store.Subject{Kind: m.SubjectKind, ID: m.SubjectID}
		if !ok || !under(path) || a.seen[windowKey(sub, *m.Schedule)] {
			continue
		}
		a.note(change{Action: "delete", Kind: "maintenance", Path: shown(path), Field: *m.Schedule, Old: m.Reason})
		if err := a.tx.DeleteMaintenance(a.ctx, a.actor, m.ID, path); err != nil {
			return err
		}
	}

	// Channels and checks.
	channels, err := a.tx.Channels(a.ctx)
	if err != nil {
		return err
	}
	for _, c := range channels {
		path := channelKey(c)
		if !under(c.ScopePath) || a.seen["channel:"+path] {
			continue
		}
		a.note(change{Action: "delete", Kind: "channel", Path: path, Old: c.Type, of: store.Subject{Kind: "channel", ID: c.ID}})
		if err := a.tx.DeleteChannel(a.ctx, a.actor, c.ID); err != nil {
			return err
		}
	}
	checks, err := a.tx.Checks(a.ctx, 0)
	if err != nil {
		return err
	}
	for _, c := range checks {
		if c.ContainerID != nil || !under(c.Path) || a.seen["check:"+c.Path] {
			continue
		}
		a.note(change{Action: "delete", Kind: "check", Path: c.Path, Old: c.Type, of: store.Subject{Kind: "check", ID: c.ID}})
		if err := a.tx.DeleteCheck(a.ctx, a.actor, c.ID); err != nil {
			return err
		}
	}

	// Scopes, the deepest first, so that a project goes after its
	// environments. One that still has hosts cannot go.
	paths := make([]string, 0, len(a.scopes))
	for p := range a.scopes {
		if p != "" && under(p) && p != root && !a.seen["scope:"+p] {
			paths = append(paths, p)
		}
	}
	slices.SortFunc(paths, func(x, y string) int { return strings.Count(y, "/") - strings.Count(x, "/") })
	for _, p := range paths {
		x := a.scopes[p]
		a.note(change{Action: "delete", Kind: x.Kind, Path: p})
		err := a.tx.DeleteScope(a.ctx, a.actor, x.ID)
		if errors.Is(err, store.ErrNotEmpty) {
			return fail("%s is not in the document, but cannot go: it still has hosts", p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// objectPath names a host, container or check by where it is, as the
// document does: environment, host, container.
func objectPath(t *tree, kind string, id int64, envPath string) string {
	switch kind {
	case "host":
		return envPath + "/" + t.hosts[id].Name
	case "container":
		c := t.containers[id]
		return envPath + "/" + t.hosts[c.HostID].Name + "/" + c.Slug
	case "check":
		return t.checks[id].Path
	}
	return envPath
}
