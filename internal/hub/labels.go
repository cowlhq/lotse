package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/config"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// Docker labels (design 9.3, decision 0019): a container says how it wants
// to be watched, and the hub applies that like lotse apply limited to the
// container, whenever its lotse.* labels change, that is, on a deploy.
//
//	lotse.env: acme/shop/prod                     into that environment of its host's customer
//	lotse.name: shop-app                          its key (read where containers are saved)
//	lotse.ignore: "true"                          no alerts
//	lotse.check.web.http.url: https://…/health    a check of its own
//	lotse.rules.container.restarting.enabled: "false"
//
// Labels cannot touch routing, channels, secrets, other objects, or a scope
// of another customer.

const labelPrefix = "lotse."

// lotseLabels are a container's lotse.* labels.
func lotseLabels(all map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range all {
		if strings.HasPrefix(k, labelPrefix) {
			out[k] = v
		}
	}
	return out
}

// labelValue reads a label as a setting's value: JSON where it is JSON
// (80, true), a string otherwise (2m, off).
func labelValue(v string) json.RawMessage {
	v = strings.TrimSpace(v)
	if json.Valid([]byte(v)) {
		return json.RawMessage(v)
	}
	b, _ := json.Marshal(v)
	return b
}

// labelSettings are the settings labels set, by key, and what is wrong
// with the others.
func labelSettings(labels map[string]string) (map[string]json.RawMessage, []string) {
	out := map[string]json.RawMessage{}
	var problems []string
	for k, v := range labels {
		var key string
		switch {
		case k == "lotse.ignore":
			key = "ignore"
		case strings.HasPrefix(k, "lotse.rules."):
			key = strings.TrimPrefix(k, labelPrefix)
			if !strings.HasPrefix(key, "rules.container.") {
				problems = append(problems, fmt.Sprintf("%s: labels set only the container's own rules, rules.container.…", k))
				continue
			}
		default:
			continue
		}
		def, ok := config.Lookup(key)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: there is no such setting", k))
			continue
		}
		value := labelValue(v)
		if err := def.Check(value); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", k, err))
			continue
		}
		out[key] = value
	}
	return out, problems
}

// labelChecks are the checks labels ask for: lotse.check.<key>.<type>.<field>.
func labelChecks(labels map[string]string) (map[string]labelCheck, []string) {
	out := map[string]labelCheck{}
	var problems []string
	for k, v := range labels {
		rest, ok := strings.CutPrefix(k, "lotse.check.")
		if !ok {
			continue
		}
		parts := strings.SplitN(rest, ".", 3)
		if len(parts) != 3 || !slugPattern.MatchString(parts[0]) {
			problems = append(problems, fmt.Sprintf("%s: a check is lotse.check.<key>.<type>.<field>", k))
			continue
		}
		c := out[parts[0]]
		if c.typ != "" && c.typ != parts[1] {
			problems = append(problems, fmt.Sprintf("check %s: one type, not %s and %s", parts[0], c.typ, parts[1]))
			continue
		}
		c.typ = parts[1]
		if c.spec == nil {
			c.spec = map[string]any{}
		}
		c.spec[parts[2]] = v
		out[parts[0]] = c
	}
	return out, problems
}

type labelCheck struct {
	typ  string
	spec map[string]any
}

// knownLabel says whether a lotse.* label means something.
func knownLabel(k string) bool {
	return k == "lotse.env" || k == "lotse.name" || k == "lotse.ignore" ||
		strings.HasPrefix(k, "lotse.rules.") || strings.HasPrefix(k, "lotse.check.")
}

// applyLabels applies a container's lotse.* labels if they changed since
// they were last applied. What a label no longer there set is undone.
func (h *hub) applyLabels(ctx context.Context, c store.Container) error {
	want := lotseLabels(c.Labels)
	if maps.Equal(want, c.LabelsApplied) || (len(want) == 0 && len(c.LabelsApplied) == 0) {
		return nil
	}
	t, err := h.loadTree(ctx)
	if err != nil {
		return err
	}
	host, ok := t.hosts[c.HostID]
	if !ok {
		return nil
	}
	var problems []string
	for k := range want {
		if !knownLabel(k) {
			problems = append(problems, fmt.Sprintf("%s: lotse does not know this label", k))
		}
	}
	actor := store.Actor{Kind: "label", Name: c.Slug, Via: "label"}

	// Its environment: the host's, or another of the host's customer.
	var hostEnv *store.Scope
	if host.EnvironmentID != nil {
		if x, ok := t.scopes[*host.EnvironmentID]; ok {
			hostEnv = &x
		}
	}
	var envID *int64
	env := hostEnv
	if p, ok := want["lotse.env"]; ok {
		p = strings.Trim(strings.TrimSpace(p), "/")
		var target *store.Scope
		for _, x := range t.scopes {
			if x.Kind == "environment" && x.Path == p {
				x := x
				target = &x
			}
		}
		customer := func(path string) string { return strings.SplitN(path, "/", 2)[0] }
		switch {
		case target == nil:
			problems = append(problems, fmt.Sprintf("lotse.env: there is no environment %s; it stays in its host's", p))
		case hostEnv == nil || customer(target.Path) != customer(hostEnv.Path):
			problems = append(problems, fmt.Sprintf("lotse.env: %s is not of its host's customer; it stays in its host's", p))
		case target.ID != hostEnv.ID:
			envID, env = &target.ID, target
		}
	}
	path := c.Slug
	if env != nil {
		path = env.Path + "/" + host.Name + "/" + c.Slug
	}

	settings, bad := labelSettings(want)
	problems = append(problems, bad...)
	before, _ := labelSettings(c.LabelsApplied)
	checks, bad := labelChecks(want)
	problems = append(problems, bad...)
	if env == nil && len(checks) > 0 {
		problems = append(problems, "lotse.check: its host is in no environment, so it can have no checks")
		checks = nil
	}

	err = h.db.WithTx(ctx, func(tx *store.Store) error {
		sub := store.Subject{Kind: "container", ID: c.ID, Path: path}
		keys := make([]string, 0, len(settings))
		for k := range settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		now := time.Now().Unix()
		for _, k := range keys {
			if err := tx.SetSetting(ctx, actor, sub, k, settings[k], now); err != nil {
				return err
			}
		}
		for k := range before {
			if _, still := settings[k]; !still {
				if err := tx.SetSetting(ctx, actor, sub, k, nil, now); err != nil {
					return err
				}
			}
		}
		have, err := tx.Checks(ctx, 0)
		if err != nil {
			return err
		}
		own := map[string]store.Check{}
		for _, x := range have {
			if x.ContainerID != nil && *x.ContainerID == c.ID {
				own[x.Slug] = x
			}
		}
		for key, lc := range checks {
			b, _ := json.Marshal(lc.spec)
			spec, err := probe.Normalize(lc.typ, b)
			if err != nil {
				problems = append(problems, fmt.Sprintf("check %s: %v", key, err))
				continue
			}
			existing, ok := own[key]
			if ok && existing.Type != lc.typ {
				// Another type is another check.
				if err := tx.DeleteCheck(ctx, actor, existing.ID); err != nil {
					return err
				}
				ok = false
			}
			if !ok {
				id := c.ID
				x := store.Check{EnvironmentID: env.ID, ContainerID: &id, Slug: key, Path: path + "/" + key, Name: c.Slug + " " + key, Type: lc.typ, Spec: spec, CreatedAt: now}
				var pingHash string
				if lc.typ == "heartbeat" {
					token, hash := auth.NewToken("")
					sealed := h.box.Seal(token)
					x.PingSealed, pingHash = &sealed, hash
				}
				if _, err := tx.CreateCheck(ctx, actor, x, pingHash); err != nil {
					return err
				}
				continue
			}
			if !sameJSON(existing.Spec, spec) {
				if _, err := tx.UpdateCheck(ctx, actor, existing.ID, existing.Name, spec); err != nil {
					return err
				}
			}
		}
		// Its checks that the labels no longer name go; others' stay.
		for key, x := range own {
			if _, still := checks[key]; !still {
				if err := tx.DeleteCheck(ctx, actor, x.ID); err != nil {
					return err
				}
			}
		}
		return tx.SetLabelsApplied(ctx, actor, c.ID, want, envID, path)
	})
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		h.writer.add(func(b *store.Batch) {
			b.Events = append(b.Events, store.Event{
				TS: time.Now().Unix(), SubjectKind: "container", SubjectID: c.ID, Kind: "container.labels",
				Data: mustJSON(map[string]any{"problems": problems}),
			})
		})
	}
	h.reassign(context.WithoutCancel(ctx))
	h.alerts.poke()
	return nil
}
