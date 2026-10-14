package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	levels := []Level{
		{Kind: "host", ID: 7, Path: "web1"},
		{Kind: "environment", ID: 4, Path: "acme/shop/prod"},
		{Kind: "project", ID: 3, Path: "acme/shop"},
		{Kind: "customer", ID: 2, Path: "acme"},
		{Kind: "global", ID: 1},
	}
	set := []map[string]json.RawMessage{
		{},
		{"agent.interval.host": json.RawMessage(`"20s"`)},
		{},
		{"agent.interval.host": json.RawMessage(`"60s"`)},
		{"agent.interval.containers": json.RawMessage(`"45s"`)},
	}
	got := Resolve("host", levels, set, nil)
	for _, v := range got {
		if strings.HasPrefix(v.Key, "check.") || strings.HasPrefix(v.Key, "deadman.") || strings.HasPrefix(v.Key, "rules.probe.") {
			t.Errorf("a host has %s", v.Key)
		}
	}
	if byKey(got, "rules.container.unhealthy.enabled").Key == "" {
		t.Error("a host has no settings for its containers")
	}
	host := byKey(got, "agent.interval.host")
	if string(host.Value) != `"20s"` || host.From.Kind != "environment" || len(host.Overrides) != 1 || host.Overrides[0].Level.Kind != "customer" {
		t.Errorf("host interval: %+v", host)
	}
	containers := byKey(got, "agent.interval.containers")
	if string(containers.Value) != `"45s"` || containers.From.Kind != "global" {
		t.Errorf("container interval: %+v", containers)
	}

	none := Resolve("host", levels, make([]map[string]json.RawMessage, len(levels)), map[string]json.RawMessage{"agent.interval.host": json.RawMessage(`"1s"`)})
	if h, c := byKey(none, "agent.interval.host"), byKey(none, "agent.interval.containers"); string(h.Value) != `"1s"` || h.From != nil || string(c.Value) != `"30s"` {
		t.Errorf("defaults: %+v", none)
	}
}

func TestCheck(t *testing.T) {
	k, _ := Lookup("agent.interval.host")
	for v, ok := range map[string]bool{`"15s"`: true, `"2m"`: true, `"1s"`: false, `"2h"`: false, `"soon"`: false, `15`: false} {
		if err := k.Check(json.RawMessage(v)); (err == nil) != ok {
			t.Errorf("%s: %v", v, err)
		}
	}
	if !k.SettableOn("host") || !k.SettableOn("customer") || k.SettableOn("container") {
		t.Error("SettableOn")
	}
}

func TestShort(t *testing.T) {
	for d, want := range map[string]string{"5s": "5s", "1h": "1h", "90m": "1h30m", "2m": "2m", "1h0m30s": "1h0m30s", "24h": "24h", "48h": "2d", "720h": "30d", "50h": "50h"} {
		x, _ := time.ParseDuration(d)
		if got := short(x); got != want {
			t.Errorf("short(%s) = %s, want %s", d, got, want)
		}
	}
}

func byKey(values []Value, key string) Value {
	for _, v := range values {
		if v.Key == key {
			return v
		}
	}
	return Value{}
}

func TestWindow(t *testing.T) {
	for w, ok := range map[string]bool{"04:00-06:00": true, "22:00-02:00": true, "00:00-24:00": true, "4-6": false, "06:00-06:00": false, "25:00-26:00": false} {
		if _, _, err := ParseWindow(w); (err == nil) != ok {
			t.Errorf("%s: %v", w, err)
		}
	}
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	s, e, _ := ParseWindow("22:00-02:00")
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(23, 0), true}, {at(1, 59), true}, {at(2, 0), false}, {at(12, 0), false}} {
		if InWindow(c.t, s, e) != c.want {
			t.Errorf("%v in 22:00-02:00: %v", c.t, !c.want)
		}
	}
	k, _ := Lookup("agent.auto_update")
	if k.Check(json.RawMessage(`true`)) != nil || k.Check(json.RawMessage(`"yes"`)) == nil {
		t.Error("auto_update check")
	}
}

func TestRuleKeys(t *testing.T) {
	for _, r := range Rules {
		enabled, ok := Lookup("rules." + r.Name + ".enabled")
		if !ok || enabled.Check(enabled.Default) != nil {
			t.Errorf("%s: no enabled key", r.Name)
		}
		for _, k := range Keys {
			if strings.HasPrefix(k.Name, "rules."+r.Name+".") && k.Check(k.Default) != nil {
				t.Errorf("%s: the default %s is not valid", k.Name, k.Default)
			}
		}
	}
	warn, _ := Lookup("rules.host.disk.warn")
	for v, ok := range map[string]bool{`80`: true, `92.5`: true, `"off"`: true, `"80"`: false, `true`: false} {
		if err := warn.Check(json.RawMessage(v)); (err == nil) != ok {
			t.Errorf("warn %s: %v", v, err)
		}
	}
	if th, _ := Threshold(json.RawMessage(`"off"`)); th != nil {
		t.Error("off is a threshold")
	}
	probe, _ := Lookup("rules.probe.silent.enabled")
	if probe.SettableOn("customer") || !probe.SettableOn("global") {
		t.Error("probe rules are set globally only")
	}
	if _, ok := Lookup("rules.host.silent.warn"); ok {
		t.Error("a condition has thresholds")
	}
	repeat, _ := Lookup("notify.repeat")
	for v, ok := range map[string]bool{`"0s"`: true, `"1h"`: true, `"1m"`: false} {
		if err := repeat.Check(json.RawMessage(v)); (err == nil) != ok {
			t.Errorf("repeat %s: %v", v, err)
		}
	}
	channels, _ := Lookup("notify.channels")
	for v, ok := range map[string]bool{`[]`: true, `["ops-mail"]`: true, `["Ops Mail"]`: false, `"ops"`: false} {
		if err := channels.Check(json.RawMessage(v)); (err == nil) != ok {
			t.Errorf("channels %s: %v", v, err)
		}
	}
	deadman, _ := Lookup("deadman.url")
	for v, ok := range map[string]bool{`""`: true, `"https://hc-ping.com/abc"`: true, `"ftp://x"`: false, `"hc-ping.com"`: false} {
		if err := deadman.Check(json.RawMessage(v)); (err == nil) != ok {
			t.Errorf("deadman %s: %v", v, err)
		}
	}
}

func TestSchedule(t *testing.T) {
	for s, ok := range map[string]bool{"04:00-06:00": true, "daily 04:00-06:00": true, "sun 02:00-04:00": true,
		"mon,wed 22:00-02:00": true, "mon-fri 08:00-18:00": true, "fri-mon 00:00-24:00": true,
		"sunday 02:00-04:00": false, "sun": false, "sun 02:00": false, "mon- 01:00-02:00": false} {
		if _, err := ParseSchedule(s); (err == nil) != ok {
			t.Errorf("%q: %v", s, err)
		}
	}
	// 2026-10-11 is a Sunday.
	at := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, time.UTC) }
	for _, c := range []struct {
		schedule string
		t        time.Time
		want     bool
	}{
		{"sun 02:00-04:00", at(11, 3, 0), true},
		{"sun 02:00-04:00", at(12, 3, 0), false},
		{"sun 22:00-02:00", at(11, 23, 0), true},
		{"sun 22:00-02:00", at(12, 1, 0), true},
		{"sun 22:00-02:00", at(11, 1, 0), false},
		{"mon-fri 08:00-18:00", at(14, 9, 0), true},
		{"mon-fri 08:00-18:00", at(17, 9, 0), false},
		{"fri-mon 08:00-18:00", at(11, 9, 0), true},
		{"04:00-06:00", at(13, 5, 59), true},
	} {
		sc, _ := ParseSchedule(c.schedule)
		if got := sc.Contains(c.t); got != c.want {
			t.Errorf("%s contains %s: %v", c.schedule, c.t.Format("Mon 15:04"), got)
		}
	}
}

func TestDays(t *testing.T) {
	for v, want := range map[string]time.Duration{`"30d"`: 30 * 24 * time.Hour, `"2h"`: 2 * time.Hour} {
		if d, err := Duration(json.RawMessage(v)); err != nil || d != want {
			t.Errorf("%s: %v, %v", v, d, err)
		}
	}
	if _, err := Duration(json.RawMessage(`"xd"`)); err == nil {
		t.Error("xd is a duration")
	}
	k, _ := Lookup("retention.raw")
	if k.Check(json.RawMessage(`"1h"`)) == nil || k.Check(json.RawMessage(`"3d"`)) != nil || k.SettableOn("customer") {
		t.Error("retention.raw")
	}
}
