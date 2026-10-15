package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sync"

	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// Services and restarts: an agent says every minute which systemd units
// its machine has and whether updates wait for a restart. The hub keeps
// the last word with the host, written when it changes.

// systems are what each host's agent said last, as kept.
type systems struct {
	mu   sync.Mutex
	last map[int64]string
}

func (h *hub) hostSystem(ctx context.Context, hostID int64, s protocol.System) error {
	// The time it was read changes every minute; what it says, seldom.
	stamp := s.TS
	s.TS = 0
	b, _ := json.Marshal(s)
	h.systems.mu.Lock()
	same := h.systems.last[hostID] == string(b)
	if h.systems.last == nil {
		h.systems.last = map[int64]string{}
	}
	h.systems.last[hostID] = string(b)
	h.systems.mu.Unlock()
	if same {
		return nil
	}
	s.TS = stamp
	b, _ = json.Marshal(s)
	return h.db.SetHostSystem(ctx, hostID, string(b))
}

// systemOf reads what a host's agent said, if it said anything.
func systemOf(h store.Host) (protocol.System, bool) {
	var s protocol.System
	if h.System == "" || json.Unmarshal([]byte(h.System), &s) != nil {
		return s, false
	}
	return s, true
}

func toSystem(s protocol.System) *api.HostSystem {
	out := &api.HostSystem{Ts: s.TS, Systemd: s.Systemd, RebootRequired: s.RebootRequired, Units: []api.SystemUnit{}}
	if s.RebootReason != "" {
		out.RebootReason = &s.RebootReason
	}
	for _, u := range s.Units {
		out.Units = append(out.Units, api.SystemUnit{Name: u.Name, Active: u.Active, Sub: u.Sub})
	}
	return out
}

// systemReadings are the readings of a host's services and restarts. A unit
// that failed raises host.unit; one that did, and is back, ends it. The
// services named in host.services must be active.
func (h *hub) systemReadings(t *tree, x store.Host, fresh, grace bool) []reading {
	s, ok := systemOf(x)
	if !ok {
		return nil
	}
	rule := func(name string) config.Rule { r, _ := config.LookupRule(name); return r }
	var out []reading
	if !fresh {
		// Nothing new is known: unknown, after the hub's first minutes.
		for _, inst := range h.alertInstances("host.unit", "host", x.ID) {
			out = append(out, reading{rule: rule("host.unit"), kind: "host", id: x.ID, instance: inst, skip: grace})
		}
		for _, name := range t.units(x.ID, "host.services") {
			out = append(out, reading{rule: rule("host.service"), kind: "host", id: x.ID, instance: name, skip: grace})
		}
		return append(out, reading{rule: rule("host.reboot"), kind: "host", id: x.ID, skip: grace})
	}
	reboot := reading{rule: rule("host.reboot"), kind: "host", id: x.ID, value: yes(s.RebootRequired)}
	if s.RebootRequired {
		reboot.reason = cmp.Or(s.RebootReason, "updates wait for a restart")
	}
	out = append(out, reboot)
	if !s.Systemd {
		return out
	}
	byName := map[string]protocol.Unit{}
	for _, u := range s.Units {
		byName[u.Name] = u
	}
	failed := map[string]bool{}
	ignore := t.units(x.ID, "host.ignore_units")
	for _, u := range s.Units {
		if u.Active == "failed" && !slices.Contains(ignore, u.Name) {
			failed[u.Name] = true
			// The instance names the unit; the alert's title says it failed.
			out = append(out, reading{rule: rule("host.unit"), kind: "host", id: x.ID, instance: u.Name, value: yes(true)})
		}
	}
	for _, inst := range h.alertInstances("host.unit", "host", x.ID) {
		if !failed[inst] {
			out = append(out, reading{rule: rule("host.unit"), kind: "host", id: x.ID, instance: inst, value: yes(false)})
		}
	}
	for _, name := range t.units(x.ID, "host.services") {
		r := reading{rule: rule("host.service"), kind: "host", id: x.ID, instance: name}
		switch u, known := byName[name]; {
		case !known:
			r.value, r.reason = yes(true), "not there"
		case u.Active != "active":
			r.value, r.reason = yes(true), u.Active+" ("+u.Sub+")"
		default:
			r.value = yes(false)
		}
		out = append(out, r)
	}
	return out
}

// units are the units a setting names for a host, such as host.services.
func (t *tree) units(hostID int64, key string) []string {
	v, _ := t.value(t.chain("host", hostID), key)
	var list []string
	json.Unmarshal(v, &list)
	return slices.Compact(slices.Sorted(slices.Values(list)))
}

// alertInstances are the instances of a rule on an object that have a
// state now.
func (h *hub) alertInstances(rule, kind string, id int64) []string {
	h.alerts.mu.Lock()
	defer h.alerts.mu.Unlock()
	var out []string
	for key := range h.alerts.states {
		if key.Rule == rule && key.SubjectKind == kind && key.SubjectID == id {
			out = append(out, key.Instance)
		}
	}
	return out
}
