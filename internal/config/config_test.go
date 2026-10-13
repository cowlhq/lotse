package config

import (
	"encoding/json"
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
	if len(got) != 4 {
		t.Fatalf("%d values", len(got))
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
	for d, want := range map[string]string{"5s": "5s", "1h": "1h", "90m": "1h30m", "2m": "2m", "1h0m30s": "1h0m30s"} {
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
