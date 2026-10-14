package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"lotse.sh/internal/alert"
	"lotse.sh/internal/config"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// The alert loop (design 8.1): every 10 seconds it reads everything at
// once, works out where each rule stands on each object, records every
// transition, and queues the notifications that are due.

const (
	alertEvery = 10 * time.Second
	// startGrace is how long after the hub starts silence counts for
	// nothing: agents and probes need time to come back.
	startGrace = 2 * time.Minute
)

// alerting is the alert loop's state: where every rule stands, as in
// alert_states, read once at the start.
type alerting struct {
	mu      sync.Mutex
	states  map[store.AlertKey]store.AlertState
	values  map[store.AlertKey]*float64
	reasons map[store.AlertKey]string
	started time.Time
	wake    chan struct{}
	// failures counts the sends in a row that failed, by channel.
	failures map[int64]int
	// lastRun is when the loop last worked through everything; lastPing
	// when it last called the dead man's switch.
	lastRun, lastPing time.Time
	pinging           bool
}

// poke runs the loop now, as after a change of maintenance.
func (a *alerting) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (h *hub) alertLoop(ctx context.Context) {
	h.alerts.mu.Lock()
	h.alerts.started = time.Now()
	h.alerts.mu.Unlock()
	t := time.NewTicker(cmp.Or(h.alertEvery, alertEvery))
	defer t.Stop()
	for {
		if err := h.evaluate(ctx, time.Now()); err != nil {
			if ctx.Err() != nil {
				return
			}
			h.log.Error("cannot evaluate the alert rules", "err", err)
		} else {
			h.deadman(ctx, time.Now())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-h.alerts.wake:
		}
	}
}

// inputs is what the rules look at besides the tree.
type inputs struct {
	samples     map[int64]store.HostSample
	filesystems []store.HostFilesystem
	dies        map[int64]int
	revoked     map[int64]bool
	maintenance []store.Maintenance
}

func (h *hub) loadInputs(ctx context.Context, now time.Time) (inputs, error) {
	var in inputs
	var err error
	if in.samples, err = h.db.LatestHostSamples(ctx); err != nil {
		return in, err
	}
	if in.filesystems, err = h.db.AllFilesystems(ctx); err != nil {
		return in, err
	}
	if in.dies, err = h.db.CountEvents(ctx, "container.die", now.Add(-10*time.Minute).Unix()); err != nil {
		return in, err
	}
	if in.revoked, err = h.db.RevokedHosts(ctx); err != nil {
		return in, err
	}
	in.maintenance, err = h.db.Maintenances(ctx)
	return in, err
}

// reading is what a rule sees on an object now.
type reading struct {
	rule     config.Rule
	kind     string
	id       int64
	instance string
	// value is nil for no data, and 1 or 0 for a condition that holds or
	// not.
	value  *float64
	reason string
	// skip leaves the state as it is, as right after the hub started.
	skip bool
}

func yes(b bool) *float64 {
	if b {
		return ptr(1.0)
	}
	return ptr(0.0)
}

// hostEvery is how often a host's agent reads its machine.
func (h *hub) hostEvery(t *tree, chain []config.Level) time.Duration {
	v, from := t.value(chain, "agent.interval.host")
	if from == nil && h.hostInterval > 0 {
		return h.hostInterval
	}
	d, err := config.Duration(v)
	if err != nil || d <= 0 {
		return 15 * time.Second
	}
	return d
}

// readings are what every rule sees on every object now.
func (h *hub) readings(t *tree, in inputs, now time.Time) []reading {
	var out []reading
	rule := func(name string) config.Rule { r, _ := config.LookupRule(name); return r }
	h.alerts.mu.Lock()
	grace := now.Sub(h.alerts.started) < startGrace
	failures := map[int64]int{}
	for id, n := range h.alerts.failures {
		failures[id] = n
	}
	h.alerts.mu.Unlock()

	// Hosts: whether their agent reports, and their readings, if fresh.
	fresh := map[int64]bool{}
	for _, x := range t.hosts {
		if in.revoked[x.ID] {
			continue
		}
		window := 3*h.hostEvery(t, t.chain("host", x.ID)) + 30*time.Second
		connected := h.agents.connected(x.ID)
		fresh[x.ID] = connected && now.Sub(time.Unix(x.LastSeen, 0)) <= window
		r := reading{rule: rule("host.silent"), kind: "host", id: x.ID, value: yes(!fresh[x.ID]), skip: grace}
		switch {
		case !connected:
			r.reason = "the agent is not connected"
		case !fresh[x.ID]:
			r.reason = "no reading since " + time.Unix(x.LastSeen, 0).Format("2006-01-02 15:04 MST")
		}
		out = append(out, r)

		s, ok := in.samples[x.ID]
		ok = ok && fresh[x.ID] && now.Sub(time.Unix(s.TS, 0)) <= window
		metric := func(name string, v *float64) {
			r := reading{rule: rule(name), kind: "host", id: x.ID, value: v}
			if !ok {
				r.value = nil
			}
			if r.value == nil {
				r.skip = grace
			}
			out = append(out, r)
		}
		var mem, swap, load *float64
		if ok && s.MemUsed != nil && x.MemTotal > 0 {
			mem = ptr(100 * float64(*s.MemUsed) / float64(x.MemTotal))
		}
		if ok && s.SwapUsed != nil && s.SwapTotal != nil && *s.SwapTotal > 0 {
			swap = ptr(100 * float64(*s.SwapUsed) / float64(*s.SwapTotal))
		}
		if ok && s.Load5 != nil && x.Cores > 0 {
			load = ptr(*s.Load5 / float64(x.Cores))
		}
		metric("host.cpu", s.CPU)
		metric("host.iowait", s.IOWait)
		metric("host.memory", mem)
		metric("host.load", load)
		if !ok || s.SwapTotal != nil && *s.SwapTotal > 0 {
			metric("host.swap", swap)
		}
	}
	for _, f := range in.filesystems {
		x, known := t.hosts[f.HostID]
		if !known || in.revoked[f.HostID] {
			continue
		}
		window := 3*h.hostEvery(t, t.chain("host", x.ID)) + 30*time.Second
		r := reading{rule: rule("host.disk"), kind: "host", id: x.ID, instance: f.Mount}
		switch {
		case !fresh[x.ID]:
			r.skip = grace
		case now.Sub(time.Unix(f.LastSeen, 0)) > window:
			// The host reports, but not this filesystem: it is gone.
			continue
		case f.Used != nil && f.Total != nil && *f.Total > 0:
			r.value = ptr(100 * float64(*f.Used) / float64(*f.Total))
		}
		out = append(out, r)
	}

	// Containers.
	for _, c := range t.containers {
		if in.revoked[c.HostID] {
			continue
		}
		r := reading{rule: rule("container.unhealthy"), kind: "container", id: c.ID,
			value: yes(c.State == "running" && c.Health == "unhealthy")}
		if !fresh[c.HostID] {
			r.value, r.skip = nil, grace
		}
		out = append(out, r)
		out = append(out, reading{rule: rule("container.restarting"), kind: "container", id: c.ID,
			value: ptr(float64(in.dies[c.ID])), reason: fmt.Sprintf("%d stops in 10 minutes", in.dies[c.ID])})
	}

	// Checks.
	for _, c := range t.checks {
		if c.Type == "heartbeat" {
			r := reading{rule: rule("check.heartbeat"), kind: "check", id: c.ID, value: yes(c.State == "down")}
			if c.State == "down" {
				if _, reason := heartbeatState(c, now); reason != "" {
					r.reason = reason
				} else {
					r.reason = "the job said it failed"
				}
			}
			out = append(out, r)
			continue
		}
		st := h.checkStanding(c.ID, now)
		down := reading{rule: rule("check.down"), kind: "check", id: c.ID, reason: st.reason}
		if st.fresh {
			down.value = ptr(float64(st.failed))
		} else {
			down.skip, down.reason = grace, "no probe has a recent result"
		}
		out = append(out, down)
		if hasCert(c) {
			cert := reading{rule: rule("check.cert"), kind: "check", id: c.ID}
			switch {
			case st.certNotAfter > 0:
				cert.value = ptr(time.Unix(st.certNotAfter, 0).Sub(now).Hours() / 24)
				cert.reason = "the certificate expires " + time.Unix(st.certNotAfter, 0).Format("2006-01-02")
			case !st.fresh:
				cert.skip = grace
			default:
				// It ran, but saw no certificate, as when it cannot connect;
				// check.down covers that.
				continue
			}
			out = append(out, cert)
		}
	}

	// Probes and channels.
	for _, p := range t.probes {
		if p.Builtin || p.RevokedAt != nil {
			continue
		}
		online := h.probes.online(p.ID)
		r := reading{rule: rule("probe.silent"), kind: "probe", id: p.ID, value: yes(!online), skip: grace}
		if !online {
			r.reason = "the probe is not connected"
		}
		out = append(out, r)
	}
	for _, c := range t.channels {
		r := reading{rule: rule("channel.failing"), kind: "channel", id: c.ID, value: yes(failures[c.ID] >= 3)}
		if failures[c.ID] >= 3 {
			r.reason = fmt.Sprintf("the last %d attempts to send failed", failures[c.ID])
		}
		out = append(out, r)
	}
	return out
}

// hasCert says whether a check sees a certificate: a TLS check, or HTTP
// to an https address.
func hasCert(c store.Check) bool {
	if c.Type == "tls" {
		return true
	}
	var spec probe.HTTP
	json.Unmarshal(c.Spec, &spec)
	return c.Type == "http" && len(spec.URL) > 8 && spec.URL[:8] == "https://"
}

// ruleSetting is one setting of a rule as it applied, and where from.
type ruleSetting struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
	From  *config.Level   `json:"from,omitempty"`
}

// params are a rule's settings on an object, and how they came about.
func (h *hub) params(t *tree, r config.Rule, chain []config.Level) (alert.Params, []ruleSetting) {
	fields := []string{"enabled", "warn", "crit", "for", "recover"}
	if r.Condition() {
		fields = []string{"enabled", "level", "for", "recover"}
	}
	values := map[string]json.RawMessage{}
	var settings []ruleSetting
	for _, f := range fields {
		key := "rules." + r.Name + "." + f
		v, from := t.value(chain, key)
		values[f] = v
		settings = append(settings, ruleSetting{Key: key, Value: v, From: from})
	}
	for _, key := range []string{"notify.channels", "notify.repeat"} {
		v, from := t.value(chain, key)
		settings = append(settings, ruleSetting{Key: key, Value: v, From: from})
	}
	return alert.ParamsOf(r, values), settings
}

// evaluate runs every rule once.
func (h *hub) evaluate(ctx context.Context, now time.Time) error {
	if err := h.loadStates(ctx); err != nil {
		return err
	}
	t, err := h.loadTree(ctx)
	if err != nil {
		return err
	}
	in, err := h.loadInputs(ctx, now)
	if err != nil {
		return err
	}
	seen := map[store.AlertKey]bool{}
	changed := false
	for _, r := range h.readings(t, in, now) {
		key := store.AlertKey{Rule: r.rule.Name, SubjectKind: r.kind, SubjectID: r.id, Instance: r.instance}
		seen[key] = true
		chain := t.chain(r.kind, r.id)
		p, settings := h.params(t, r.rule, chain)
		h.alerts.mu.Lock()
		prev, had := h.alerts.states[key]
		h.alerts.mu.Unlock()
		if !had {
			prev = store.AlertState{AlertKey: key, State: alert.OK, NotifiedState: alert.OK}
		}
		next := toStep(prev)
		switch {
		case !p.Enabled:
			next = alert.State{State: alert.OK, Since: now}
			r.reason = "the rule is off"
		case !r.skip:
			next = alert.Step(r.rule, p, next, r.value, now)
		}
		if !r.skip || !p.Enabled {
			h.alerts.mu.Lock()
			h.alerts.values[key], h.alerts.reasons[key] = r.value, r.reason
			h.alerts.mu.Unlock()
		}
		c, err := h.advance(ctx, t, in, prev, next, r, p, settings, chain, now)
		if err != nil {
			return err
		}
		changed = changed || c
	}
	// What no rule saw is gone: a deleted check, a container that is no
	// more, a filesystem unmounted.
	h.alerts.mu.Lock()
	var gone []store.AlertState
	for key, st := range h.alerts.states {
		if !seen[key] {
			gone = append(gone, st)
		}
	}
	h.alerts.mu.Unlock()
	for _, st := range gone {
		r, _ := config.LookupRule(st.Rule)
		rd := reading{rule: r, kind: st.SubjectKind, id: st.SubjectID, instance: st.Instance, reason: "it is gone"}
		chain := t.chain(st.SubjectKind, st.SubjectID)
		p, settings := h.params(t, r, chain)
		if chain == nil {
			// Nobody to tell about something deleted.
			st.NotifiedState = alert.OK
		}
		if _, err := h.advance(ctx, t, in, st, alert.State{State: alert.OK, Since: now}, rd, p, settings, chain, now); err != nil {
			return err
		}
		h.alerts.mu.Lock()
		delete(h.alerts.states, st.AlertKey)
		delete(h.alerts.values, st.AlertKey)
		delete(h.alerts.reasons, st.AlertKey)
		h.alerts.mu.Unlock()
		changed = true
	}
	if changed {
		h.live.publish(LiveMessage{Type: "alerts"})
	}
	h.alerts.mu.Lock()
	h.alerts.lastRun = now
	h.alerts.mu.Unlock()
	return nil
}

// loadStates reads the states the first time.
func (h *hub) loadStates(ctx context.Context) error {
	h.alerts.mu.Lock()
	loaded := h.alerts.states != nil
	h.alerts.mu.Unlock()
	if loaded {
		return nil
	}
	list, err := h.db.AlertStates(ctx)
	if err != nil {
		return err
	}
	h.alerts.mu.Lock()
	defer h.alerts.mu.Unlock()
	h.alerts.states = map[store.AlertKey]store.AlertState{}
	h.alerts.values, h.alerts.reasons = map[store.AlertKey]*float64{}, map[store.AlertKey]string{}
	for _, st := range list {
		h.alerts.states[st.AlertKey] = st
		h.alerts.values[st.AlertKey] = st.Value
	}
	return nil
}

func toStep(st store.AlertState) alert.State {
	at := func(p *int64) time.Time {
		if p == nil {
			return time.Time{}
		}
		return time.Unix(*p, 0)
	}
	return alert.State{
		State: st.State, Since: time.Unix(st.Since, 0), Pending: ptrValue(st.Pending),
		PendingSince: at(st.PendingSince), RecoveringSince: at(st.RecoveringSince), Previous: ptrValue(st.Previous),
	}
}

// fromStep puts a step's result into a stored state.
func fromStep(st store.AlertState, next alert.State) store.AlertState {
	unix := func(t time.Time) *int64 {
		if t.IsZero() {
			return nil
		}
		return ptr(t.Unix())
	}
	text := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	st.State, st.Since = next.State, next.Since.Unix()
	st.Pending, st.PendingSince, st.RecoveringSince = text(next.Pending), unix(next.PendingSince), unix(next.RecoveringSince)
	st.Previous = text(next.Previous)
	return st
}

// advance saves where a rule stands now, and queues what is to be sent.
// It says whether the state changed.
func (h *hub) advance(ctx context.Context, t *tree, in inputs, prev store.AlertState, next alert.State,
	r reading, p alert.Params, settings []ruleSetting, chain []config.Level, now time.Time) (bool, error) {
	st := fromStep(prev, next)
	maintenance := inMaintenance(in.maintenance, chain, now)
	var transition *store.Alert
	if st.State != prev.State {
		effective, _ := json.Marshal(settings)
		transition = &store.Alert{
			SubjectPath: t.path(r.kind, r.id), From: prev.State, To: st.State, At: now.Unix(),
			Value: r.value, Threshold: alert.Threshold(p, prev.State, st.State), Reason: r.reason,
			Effective: effective, InMaintenance: maintenance,
		}
		st.Value, st.AckedBy, st.AckedAt = r.value, nil, nil
	}
	h.alerts.mu.Lock()
	if cur, ok := h.alerts.states[st.AlertKey]; ok && transition == nil && st.AckedAt == nil && ptrEqual(cur.AlertID, st.AlertID) {
		st.AckedBy, st.AckedAt = cur.AckedBy, cur.AckedAt
	}
	h.alerts.mu.Unlock()
	if transition != nil || !sameStep(prev, st) {
		saved, err := h.db.SaveAlertState(ctx, st, transition)
		if err != nil {
			return false, err
		}
		st = saved
		h.setState(st)
	}
	if err := h.queue(ctx, t, st, chain, maintenance, transition != nil, now); err != nil {
		return false, err
	}
	return transition != nil, nil
}

// sameStep says whether two states are the same but for their value.
func sameStep(a, b store.AlertState) bool {
	return a.State == b.State && a.Since == b.Since && ptrEqual(a.Pending, b.Pending) &&
		ptrEqual(a.PendingSince, b.PendingSince) && ptrEqual(a.RecoveringSince, b.RecoveringSince) &&
		ptrEqual(a.Previous, b.Previous)
}

func ptrEqual[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// plainlyOK says whether there is nothing to keep about a state.
func plainlyOK(st store.AlertState) bool {
	return st.State == alert.OK && st.Pending == nil && st.RecoveringSince == nil && st.NotifiedState == alert.OK
}

func (h *hub) setState(st store.AlertState) {
	h.alerts.mu.Lock()
	defer h.alerts.mu.Unlock()
	if plainlyOK(st) {
		delete(h.alerts.states, st.AlertKey)
		return
	}
	if cur, ok := h.alerts.states[st.AlertKey]; ok && st.AckedAt == nil && ptrEqual(cur.AlertID, st.AlertID) {
		st.AckedBy, st.AckedAt = cur.AckedBy, cur.AckedAt
	}
	h.alerts.states[st.AlertKey] = st
}
