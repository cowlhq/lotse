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
	if len(got) != 2 {
		t.Fatalf("%d values", len(got))
	}
	host := got[0]
	if string(host.Value) != `"20s"` || host.From.Kind != "environment" || len(host.Overrides) != 1 || host.Overrides[0].Level.Kind != "customer" {
		t.Errorf("host interval: %+v", host)
	}
	containers := got[1]
	if string(containers.Value) != `"45s"` || containers.From.Kind != "global" {
		t.Errorf("container interval: %+v", containers)
	}

	none := Resolve("host", levels, make([]map[string]json.RawMessage, len(levels)), map[string]json.RawMessage{"agent.interval.host": json.RawMessage(`"1s"`)})
	if string(none[0].Value) != `"1s"` || none[0].From != nil || string(none[1].Value) != `"30s"` {
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
