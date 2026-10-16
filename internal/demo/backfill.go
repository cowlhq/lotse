package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"time"

	"lotse.sh/internal/config"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// The history before the demo started is written straight into the
// database, as the hub would have written it: readings, check results,
// events and alerts. Then the readings are averaged, as the hub's retention
// does every few minutes, so that long ranges have what they read.

// placed is what the hub calls a machine and its containers.
type placed struct {
	m          *machine
	hostID     int64
	containers map[string]int64 // by slug
}

type demoCheck struct {
	ID   int64
	Path string
	Type string
	Spec json.RawMessage
}

type backfill struct {
	db      *store.Store
	s       *story
	net     *internet
	version string
	from    time.Time
	hosts   []placed
	checks  []demoCheck
	probes  []int64 // the hub's own and the external one
	batch   store.Batch
	rows    int
}

func (b *backfill) flush(ctx context.Context) error {
	if b.rows == 0 {
		return nil
	}
	err := b.db.Write(ctx, b.batch)
	b.batch, b.rows = store.Batch{}, 0
	return err
}

func (b *backfill) add(ctx context.Context, n int, f func(*store.Batch)) error {
	f(&b.batch)
	b.rows += n
	if b.rows >= 5000 {
		return b.flush(ctx)
	}
	return nil
}

// step is how far apart readings are written: finer the nearer they are
// to now. Charts of a day or a week show no difference.
func (b *backfill) step(t time.Time, finest time.Duration) time.Duration {
	switch age := b.s.start.Sub(t); {
	case age > 24*time.Hour:
		return max(finest, 2*time.Minute)
	case age > time.Hour:
		return max(finest, 30*time.Second)
	}
	return finest
}

func (b *backfill) run(ctx context.Context) error {
	if err := b.readings(ctx); err != nil {
		return fmt.Errorf("readings: %w", err)
	}
	if err := b.results(ctx); err != nil {
		return fmt.Errorf("check results: %w", err)
	}
	if err := b.events(ctx); err != nil {
		return fmt.Errorf("events: %w", err)
	}
	if err := b.flush(ctx); err != nil {
		return err
	}
	if err := b.alerts(ctx); err != nil {
		return fmt.Errorf("alerts: %w", err)
	}
	// As the hub's retention does, readings older than 20 minutes.
	return b.db.Rollup(ctx, time.Now().Add(-20*time.Minute).Unix())
}

func (b *backfill) readings(ctx context.Context) error {
	for _, h := range b.hosts {
		m := h.m
		t0 := b.from.Truncate(2 * time.Minute)
		for t := t0; t.Before(b.s.start); t = t.Add(b.step(t, 15*time.Second)) {
			if m.down(b.s, t) {
				continue
			}
			x := m.sample(b.s, t)
			err := b.add(ctx, 1+len(x.Filesystems), func(batch *store.Batch) {
				batch.HostSamples = append(batch.HostSamples, store.HostSample{
					HostID: h.hostID, TS: x.TS, CPU: x.CPU, IOWait: x.IOWait, Steal: x.Steal,
					Load1: x.Load1, Load5: x.Load5, Load15: x.Load15, MemUsed: x.MemUsed, MemAvailable: x.MemAvailable,
					SwapUsed: x.SwapUsed, SwapTotal: x.SwapTotal, NetRx: x.NetRx, NetTx: x.NetTx,
					DiskRead: x.DiskRead, DiskWrite: x.DiskWrite, Uptime: x.Uptime,
				})
				for _, fs := range x.Filesystems {
					batch.FSSamples = append(batch.FSSamples, store.FSSample{
						HostID: h.hostID, Mount: fs.Mount, Device: fs.Device, FSType: fs.FSType, TS: x.TS,
						Used: fs.Used, Total: fs.Total, InodesUsed: fs.InodesUsed, InodesTotal: fs.InodesTotal,
					})
				}
			})
			if err != nil {
				return err
			}
		}
		for _, c := range m.Containers {
			id := h.containers[c.slug()]
			if id == 0 {
				continue
			}
			for t := t0; t.Before(b.s.start); t = t.Add(b.step(t, 30*time.Second)) {
				if m.down(b.s, t) {
					continue
				}
				st := c.stat(m, b.s, t)
				err := b.add(ctx, 1, func(batch *store.Batch) {
					batch.ContainerSamples = append(batch.ContainerSamples, store.ContainerSample{
						ContainerID: id, TS: t.Unix(), CPU: st.CPU, Mem: st.Mem, MemLimit: st.MemLimit, NetRx: st.NetRx, NetTx: st.NetTx,
					})
				})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// result is how a check went at t on a probe, as the made-up services
// answered then.
func (b *backfill) result(c demoCheck, probeID int64, t time.Time) store.CheckResult {
	ts := t.Unix()
	sd := seed(c.Path, fmt.Sprint(probeID))
	r := store.CheckResult{CheckID: c.ID, ProbeID: probeID, TS: ts, OK: true}
	ms := func(base, jitter float64) *float64 {
		v := base + jitter*smooth(sd, ts, 1800) + 0.5*jitter*white(sd, ts/60)
		if probeID != store.BuiltinProbe {
			// Frankfurt is a little further away.
			v += 6
		}
		return ptr(math.Round(math.Max(v, 0.1)*100) / 100)
	}
	switch c.Type {
	case "http":
		var spec probe.HTTP
		json.Unmarshal(c.Spec, &spec)
		u, _ := url.Parse(spec.URL)
		s := sites[u.Hostname()]
		r.LatencyMS = ms(float64(s.latency(t, 0).Milliseconds()), float64(s.Jitter.Milliseconds()))
		r.StatusCode = ptr(200)
		if u.Scheme == "https" {
			r.CertNotAfter = ptr(b.net.certNotAfter(u.Hostname()).Unix())
		}
		if u.Hostname() == "www.baeckerei-sommer.example" && !t.Before(b.s.siteDown) && t.Before(b.s.siteUp) {
			r.OK, r.Error, r.StatusCode, r.CertNotAfter = false, "no answer in time", nil, nil
			r.LatencyMS = ptr(10000.0)
		}
	case "tls":
		r.LatencyMS = ms(22, 6)
		r.CertNotAfter = ptr(b.net.certNotAfter("mx.ops.localhost").Unix())
	case "tcp":
		r.LatencyMS = ms(0.6, 0.3)
	case "dns":
		r.LatencyMS = ms(12, 4)
	}
	// Now and then a run fails on its own, as runs over the internet do.
	if r.OK && white(seed(c.Path, fmt.Sprint(probeID), "blip"), ts/60) > 0.9998 {
		r.OK, r.Error, r.StatusCode, r.CertNotAfter = false, "connection reset by peer", nil, nil
	}
	return r
}

func (b *backfill) results(ctx context.Context) error {
	for _, c := range b.checks {
		if c.Type == "heartbeat" {
			continue
		}
		for _, p := range b.probes {
			// Every minute, as checks run; older than a day, every two.
			for t := b.from.Truncate(time.Minute); t.Before(b.s.start); t = t.Add(b.step(t, time.Minute)) {
				r := b.result(c, p, t)
				if err := b.add(ctx, 1, func(batch *store.Batch) { batch.CheckResults = append(batch.CheckResults, r) }); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *backfill) host(name string) placed {
	for _, h := range b.hosts {
		if h.m.Name == name {
			return h
		}
	}
	return placed{}
}

func (b *backfill) event(t time.Time, kind string, id int64, what string, data map[string]any) {
	if t.Before(b.from) || !t.Before(b.s.start) || id == 0 {
		return
	}
	d, _ := json.Marshal(data)
	b.batch.Events = append(b.batch.Events, store.Event{TS: t.Unix(), SubjectKind: kind, SubjectID: id, Kind: what, Data: string(d)})
	b.rows++
}

// restart records a container stopping and starting again.
func (b *backfill) restart(h placed, c container, t time.Time, exit int, after time.Duration) {
	id := h.containers[c.slug()]
	docker := map[string]any{"docker_id": dockerID(h.m.Name, c)}
	b.event(t, "container", id, "container.die", map[string]any{"docker_id": docker["docker_id"], "exit_code": exit})
	b.event(t.Add(after), "container", id, "container.start", docker)
}

func (b *backfill) events(ctx context.Context) error {
	s := b.s
	// worker-02 hung, and came back after a reboot.
	w := b.host("worker-02")
	b.event(s.workerDown, "host", w.hostID, "agent.disconnected", nil)
	b.event(s.workerUp.Add(-50*time.Second), "host", w.hostID, "host.reboot", nil)
	b.event(s.workerUp, "host", w.hostID, "agent.connected", map[string]any{"version": b.version})
	for _, c := range w.m.Containers {
		b.event(s.workerUp.Add(-30*time.Second), "container", w.containers[c.slug()], "container.start", map[string]any{"docker_id": dockerID(w.m.Name, c)})
	}
	// The shop's deploys, one web server after the other.
	for _, d := range s.deploys {
		for i, name := range []string{"web-01", "web-02"} {
			h := b.host(name)
			for _, c := range h.m.Containers {
				if c.Service == "app" || c.Service == "worker" {
					b.restart(h, c, d.Add(time.Duration(i)*95*time.Second), 0, 4*time.Second)
				}
			}
		}
	}
	// staging got a release candidate this morning, whose queue worker
	// crashes.
	st := b.host("staging-01")
	for _, c := range st.m.Containers {
		switch {
		case c.Service == "app":
			b.restart(st, c, s.start.Add(-time.Duration(c.StartedDays*24*float64(time.Hour))), 0, 5*time.Second)
		case c.Restarting:
			for t := s.crashingSince; t.Before(s.start); t = t.Add(3 * time.Minute) {
				b.restart(st, c, t, 1, 6*time.Second)
			}
		}
	}
	return b.flush(ctx)
}

// transition is a past change of an alert's state.
type transition struct {
	at        time.Time
	to        string
	value     float64
	threshold *float64
	reason    string
}

func (b *backfill) alerts(ctx context.Context) error {
	s := b.s
	w, db, st := b.host("worker-02"), b.host("db-01"), b.host("staging-01")
	site := b.check("sommer/website/production/website")
	type incident struct {
		rule, kind, path, instance string
		id                         int64
		steps                      []transition
	}
	list := []incident{
		{"host.silent", "host", w.m.Env + "/worker-02", "", w.hostID, []transition{
			{s.workerDown.Add(2 * time.Minute), "crit", 1, nil, "the agent is not connected"},
			{s.workerUp, "ok", 0, nil, ""},
		}},
		{"host.cpu", "host", db.m.Env + "/db-01", "", db.hostID, []transition{
			{s.spikeFrom.Add(10 * time.Minute), "warn", 96.4, ptr(90.0), ""},
			{s.spikeFrom.Add(19 * time.Minute), "crit", 98.2, ptr(98.0), ""},
			{s.spikeTo.Add(5 * time.Minute), "ok", 27.9, ptr(98.0), ""},
		}},
		{"host.disk", "host", st.m.Env + "/staging-01", "/", st.hostID, []transition{
			{s.diskWarn.Add(5 * time.Minute), "warn", 85.6, ptr(85.0), ""},
			{s.diskCrit.Add(5 * time.Minute), "crit", 95.3, ptr(95.0), ""},
			{s.diskCleaned.Add(5 * time.Minute), "ok", 52.1, ptr(95.0), ""},
		}},
		{"check.down", "check", site.Path, "", site.ID, []transition{
			{s.siteDown.Add(2 * time.Minute), "crit", 2, ptr(2.0), "no answer in time"},
			{s.siteUp.Add(time.Minute), "ok", 0, ptr(2.0), ""},
		}},
	}
	for _, x := range list {
		if x.id == 0 || x.steps[0].at.Before(b.from) {
			continue
		}
		effective := b.effective(x.rule)
		from := "ok"
		for _, step := range x.steps {
			at := step.at.Unix()
			state := store.AlertState{
				AlertKey: store.AlertKey{Rule: x.rule, SubjectKind: x.kind, SubjectID: x.id, Instance: x.instance},
				State:    step.to, Since: at, Value: ptr(step.value), NotifiedState: step.to, NotifiedAt: ptr(at),
			}
			_, err := b.db.SaveAlertState(ctx, state, &store.Alert{
				SubjectPath: x.path, From: from, To: step.to, At: at, Value: ptr(step.value),
				Threshold: step.threshold, Reason: step.reason, Effective: effective,
			})
			if err != nil {
				return err
			}
			from = step.to
		}
	}
	return nil
}

func (b *backfill) check(path string) demoCheck {
	for _, c := range b.checks {
		if c.Path == path {
			return c
		}
	}
	return demoCheck{}
}

// effective is a rule's settings as they applied: the defaults, but for
// host.cpu, which the demo's configuration turns on.
func (b *backfill) effective(rule string) json.RawMessage {
	r, _ := config.LookupRule(rule)
	fields := []string{"enabled", "warn", "crit", "for", "recover"}
	if r.Condition() {
		fields = []string{"enabled", "level", "for", "recover"}
	}
	type setting struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
		From  *config.Level   `json:"from,omitempty"`
	}
	var list []setting
	for _, f := range fields {
		key := "rules." + rule + "." + f
		k, ok := config.Lookup(key)
		if !ok {
			continue
		}
		x := setting{Key: key, Value: k.Default}
		if key == "rules.host.cpu.enabled" {
			x.Value, x.From = json.RawMessage("true"), &config.Level{Kind: "global", ID: store.GlobalScope}
		}
		list = append(list, x)
	}
	out, _ := json.Marshal(list)
	return out
}
