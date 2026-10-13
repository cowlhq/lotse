// Package config is the schema of settings and how they are inherited
// (design 5.2): a value set on an object wins over one set on its
// environment, which wins over its project's, its customer's, the global
// one, and the built-in default.
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Key is one setting.
type Key struct {
	Name        string
	Type        string // duration, bool
	Description string
	// Default is the built-in value, as JSON.
	Default json.RawMessage
	// On says which kinds of objects use it. It can be set on those and on
	// every scope above them.
	On []string
	// Check says what is wrong with a value, if anything.
	Check func(json.RawMessage) error
}

// Keys are the settings there are. More come with the features that use
// them: notifications, rules, checks, updates.
var Keys = []Key{
	{
		Name:        "agent.interval.host",
		Type:        "duration",
		Description: "How often the agent reads the machine.",
		Default:     json.RawMessage(`"15s"`),
		On:          []string{"host"},
		Check:       durationBetween(5*time.Second, time.Hour),
	},
	{
		Name:        "agent.interval.containers",
		Type:        "duration",
		Description: "How often the agent reads its containers' usage.",
		Default:     json.RawMessage(`"30s"`),
		On:          []string{"host"},
		Check:       durationBetween(5*time.Second, time.Hour),
	},
}

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
