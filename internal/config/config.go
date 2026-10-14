// Package config is the schema of settings and how they are inherited
// (design 5.2): a value set on an object wins over one set on its
// environment, which wins over its project's, its customer's, the global
// one, and the built-in default.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Key is one setting.
type Key struct {
	Name string
	// Type tells the UI how to show and edit it: duration, bool, window,
	// probes, channels, threshold, level or url.
	Type        string
	Description string
	// Default is the built-in value, as JSON.
	Default json.RawMessage
	// On says which kinds of objects use it. It can be set on those and on
	// every scope above them.
	On []string
	// GlobalOnly keys are set on the global scope and nowhere else.
	GlobalOnly bool
	// Check says what is wrong with a value, if anything.
	Check func(json.RawMessage) error
}

// Keys are the settings there are, the rules' last.
var Keys = append(keys, ruleKeys()...)

var keys = []Key{
	{
		Name:        "agent.interval.host",
		Type:        "duration",
		Description: "How often the agent reads the machine.",
		Default:     json.RawMessage(`"15s"`),
		On:          []string{"host"},
		Check:       durationBetween(5*time.Second, time.Hour),
	},
	{
		Name:        "agent.auto_update",
		Type:        "bool",
		Description: "Whether the agent updates itself to new releases, in its update window.",
		Default:     json.RawMessage(`false`),
		On:          []string{"host"},
		Check:       isBool,
	},
	{
		Name:        "agent.update_window",
		Type:        "window",
		Description: "When automatic updates may happen, daily, in the hub's time zone, such as 04:00-06:00.",
		Default:     json.RawMessage(`"04:00-06:00"`),
		On:          []string{"host"},
		Check: func(v json.RawMessage) error {
			var s string
			if json.Unmarshal(v, &s) != nil {
				return fmt.Errorf("a window such as 04:00-06:00")
			}
			_, _, err := ParseWindow(s)
			return err
		},
	},
	{
		Name:        "check.interval",
		Type:        "duration",
		Description: "How often a check runs, on each probe.",
		Default:     json.RawMessage(`"1m"`),
		On:          []string{"check"},
		Check:       durationBetween(10*time.Second, 24*time.Hour),
	},
	{
		Name:        "check.timeout",
		Type:        "duration",
		Description: "How long a check waits for an answer.",
		Default:     json.RawMessage(`"10s"`),
		On:          []string{"check"},
		Check:       durationBetween(time.Second, time.Minute),
	},
	{
		Name:        "check.probes",
		Type:        "probes",
		Description: "Which probes run a check: all, or a list of their names.",
		Default:     json.RawMessage(`"all"`),
		On:          []string{"check"},
		Check: func(v json.RawMessage) error {
			var all string
			if json.Unmarshal(v, &all) == nil && all == "all" {
				return nil
			}
			var list []string
			if json.Unmarshal(v, &list) != nil || len(list) == 0 {
				return fmt.Errorf("\"all\", or a list of probe names such as [\"hub\", \"fra\"]")
			}
			return nil
		},
	},
	{
		Name:        "agent.interval.containers",
		Type:        "duration",
		Description: "How often the agent reads its containers' usage.",
		Default:     json.RawMessage(`"30s"`),
		On:          []string{"host"},
		Check:       durationBetween(5*time.Second, time.Hour),
	},
	{
		Name:        "ignore",
		Type:        "bool",
		Description: "Whether lotse leaves the container alone: no alerts, and kept out of the way.",
		Default:     json.RawMessage(`false`),
		On:          []string{"container", "host"},
		Check:       isBool,
	},
	{
		Name:        "notify.channels",
		Type:        "channels",
		Description: "Where alerts go: the keys of channels defined here or further up.",
		Default:     json.RawMessage(`[]`),
		On:          []string{"host", "container", "check"},
		Check: func(v json.RawMessage) error {
			var list []string
			if json.Unmarshal(v, &list) != nil {
				return fmt.Errorf("a list of channel keys such as [\"ops-mail\"]")
			}
			for _, k := range list {
				if !slugPattern.MatchString(k) {
					return fmt.Errorf("%q is not a channel key", k)
				}
			}
			return nil
		},
	},
	{
		Name:        "notify.repeat",
		Type:        "duration",
		Description: "How often an alert that still fires and is not acknowledged is sent again; 0s for never.",
		Default:     json.RawMessage(`"0s"`),
		On:          []string{"host", "container", "check"},
		Check: func(v json.RawMessage) error {
			if d, err := Duration(v); err == nil && d == 0 {
				return nil
			}
			return durationBetween(5*time.Minute, 7*24*time.Hour)(v)
		},
	},
	{
		Name:        "deadman.url",
		Type:        "url",
		Description: "An address the hub calls while its alerting works, such as a healthchecks.io check, which raises the alarm when the calls stop.",
		Default:     json.RawMessage(`""`),
		GlobalOnly:  true,
		Check: func(v json.RawMessage) error {
			var s string
			if json.Unmarshal(v, &s) != nil {
				return fmt.Errorf("an address such as https://hc-ping.com/…")
			}
			if s == "" {
				return nil
			}
			u, err := url.Parse(s)
			if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
				return fmt.Errorf("an http or https address")
			}
			return nil
		},
	},
	{
		Name:        "deadman.interval",
		Type:        "duration",
		Description: "How often the hub calls the dead man's switch.",
		Default:     json.RawMessage(`"1m"`),
		GlobalOnly:  true,
		Check:       durationBetween(30*time.Second, time.Hour),
	},
}

// slugPattern is what keys of channels and the like look like.
var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

func Lookup(name string) (Key, bool) {
	for _, k := range Keys {
		if k.Name == name {
			return k, true
		}
	}
	return Key{}, false
}

// Levels in the order they are inherited, from the object to the top.
var scopeKinds = []string{"environment", "project", "customer", "global"}

// SettableOn says whether key can be set on an object of kind: on objects
// that use it, and on any scope above them.
func (k Key) SettableOn(kind string) bool {
	if k.GlobalOnly {
		return kind == "global"
	}
	for _, on := range k.On {
		if on == kind {
			return true
		}
	}
	for _, s := range scopeKinds {
		if s == kind {
			return true
		}
	}
	return false
}

// Duration reads a duration value.
func Duration(v json.RawMessage) (time.Duration, error) {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return 0, fmt.Errorf("a duration is a string such as \"15s\"")
	}
	return time.ParseDuration(s)
}

func durationBetween(lo, hi time.Duration) func(json.RawMessage) error {
	return func(v json.RawMessage) error {
		d, err := Duration(v)
		if err != nil {
			return fmt.Errorf("a duration such as 15s or 2m")
		}
		if d < lo || d > hi {
			return fmt.Errorf("between %s and %s", short(lo), short(hi))
		}
		return nil
	}
}

// Level is an object settings can be set on: the object itself and the
// scopes above it, in the order they are inherited.
type Level struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// Value is the value a setting has for an object, and where it comes from.
type Value struct {
	Key string
	// Value is the effective value, as JSON.
	Value json.RawMessage
	// From is where it was set; nil means the built-in default.
	From *Level
	// Overrides are the values set further up that it overrides, nearest
	// first.
	Overrides []Set
}

// Set is a value set on a level.
type Set struct {
	Level Level
	Value json.RawMessage
}

// Resolve works out every setting that objects of kind use, given the
// levels from the object up and what is set on each level (by level index).
// defaults replaces built-in defaults where it has a value.
func Resolve(kind string, levels []Level, set []map[string]json.RawMessage, defaults map[string]json.RawMessage) []Value {
	var out []Value
	for _, k := range Keys {
		if !k.SettableOn(kind) {
			continue
		}
		v := Value{Key: k.Name, Value: k.Default}
		if d, ok := defaults[k.Name]; ok {
			v.Value = d
		}
		found := false
		for i, l := range levels {
			x, ok := set[i][k.Name]
			if !ok {
				continue
			}
			if !found {
				v.Value, v.From, found = x, &levels[i], true
			} else {
				v.Overrides = append(v.Overrides, Set{Level: l, Value: x})
			}
		}
		out = append(out, v)
	}
	return out
}

// short writes a duration without zero parts: 1h, not 1h0m0s.
func short(d time.Duration) string {
	s := d.String()
	for _, zero := range []string{"m0s", "h0m"} {
		if strings.HasSuffix(s, zero) {
			s = strings.TrimSuffix(s, zero[1:])
		}
	}
	return s
}

func isBool(v json.RawMessage) error {
	var b bool
	if json.Unmarshal(v, &b) != nil {
		return fmt.Errorf("true or false")
	}
	return nil
}

// ParseWindow reads a daily window, HH:MM-HH:MM, as minutes after
// midnight. The end may be earlier than the start: 22:00-02:00 runs
// over midnight.
func ParseWindow(s string) (start, end int, err error) {
	var h1, m1, h2, m2 int
	if _, err := fmt.Sscanf(s, "%d:%d-%d:%d", &h1, &m1, &h2, &m2); err != nil ||
		h1 > 23 || h2 > 24 || m1 > 59 || m2 > 59 || h1 < 0 || h2 < 0 || m1 < 0 || m2 < 0 {
		return 0, 0, fmt.Errorf("a window such as 04:00-06:00")
	}
	start, end = h1*60+m1, h2*60+m2
	if start == end {
		return 0, 0, fmt.Errorf("a window needs a start and a different end")
	}
	return start, end, nil
}

// InWindow says whether t falls in the window start-end.
func InWindow(t time.Time, start, end int) bool {
	m := t.Hour()*60 + t.Minute()
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end
}
