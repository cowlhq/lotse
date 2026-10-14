// Package alert works out where a rule stands on an object (design 8.1):
// a value is held against the rule's thresholds for a while before the
// alert fires, and must be back on the good side, by a margin, for a while
// before it clears.
package alert

import (
	"encoding/json"
	"time"

	"lotse.sh/internal/config"
)

// States, from good to bad. UNKNOWN is apart: there is no data.
const (
	OK      = "ok"
	Warn    = "warn"
	Crit    = "crit"
	Unknown = "unknown"
)

func rank(state string) int {
	switch state {
	case Warn:
		return 1
	case Crit:
		return 2
	}
	return 0
}

// Firing says whether a state is one that is sent.
func Firing(state string) bool { return state == Warn || state == Crit }

// Params are a rule's settings as they apply to one object.
type Params struct {
	Enabled      bool
	Warn, Crit   *float64
	Level        string
	For, Recover time.Duration
}

// ParamsOf reads a rule's settings from the effective values of its keys,
// by field: enabled, warn, crit, level, for, recover. What is missing or
// not valid keeps the rule's default.
func ParamsOf(r config.Rule, values map[string]json.RawMessage) Params {
	p := Params{Enabled: r.Enabled, Warn: r.Warn, Crit: r.Crit, Level: r.Level, For: r.For, Recover: r.Recover}
	if v, ok := values["enabled"]; ok {
		json.Unmarshal(v, &p.Enabled)
	}
	for field, t := range map[string]**float64{"warn": &p.Warn, "crit": &p.Crit} {
		if v, ok := values[field]; ok {
			if th, err := config.Threshold(v); err == nil {
				*t = th
			}
		}
	}
	if v, ok := values["level"]; ok {
		var level string
		if json.Unmarshal(v, &level) == nil && (level == Warn || level == Crit) {
			p.Level = level
		}
	}
	for field, d := range map[string]*time.Duration{"for": &p.For, "recover": &p.Recover} {
		if v, ok := values[field]; ok {
			if x, err := config.Duration(v); err == nil {
				*d = x
			}
		}
	}
	return p
}

// Level is the state a value calls for, given the state it is in: to
// leave a level, a value must be back over its threshold by the rule's
// margin. For a condition, a value other than 0 means it holds.
func Level(r config.Rule, p Params, v float64, current string) string {
	if r.Condition() {
		if v != 0 {
			return p.Level
		}
		return OK
	}
	bad := func(t *float64, level string) bool {
		if t == nil {
			return false
		}
		th := *t
		if rank(current) >= rank(level) {
			if r.Below {
				th += r.Margin
			} else {
				th -= r.Margin
			}
		}
		if r.Below {
			return v < th
		}
		return v >= th
	}
	switch {
	case bad(p.Crit, Crit):
		return Crit
	case bad(p.Warn, Warn):
		return Warn
	}
	return OK
}

// Threshold is the threshold that a transition between from and to
// crossed: the one of the worse of the two levels.
func Threshold(p Params, from, to string) *float64 {
	worse := to
	if rank(from) > rank(to) {
		worse = from
	}
	switch worse {
	case Crit:
		return p.Crit
	case Warn:
		return p.Warn
	}
	return nil
}

// State is where a rule stands on an object.
type State struct {
	State string
	Since time.Time
	// Pending is the worse state it is held at, since PendingSince, before
	// it fires; RecoveringSince is when it went back to the good side.
	Pending         string
	PendingSince    time.Time
	RecoveringSince time.Time
	// Previous is the state before UNKNOWN.
	Previous string
}

// Step takes a reading, nil for no data, and returns the new state.
func Step(r config.Rule, p Params, st State, value *float64, now time.Time) State {
	if st.State == "" {
		st.State = OK
	}
	if value == nil {
		if st.State == Unknown {
			return st
		}
		return State{State: Unknown, Since: now, Previous: st.State}
	}
	next := st
	if st.State == Unknown {
		// Data again: back where it was, and on from there.
		next = State{State: st.Previous, Since: now}
		if next.State == "" {
			next.State = OK
		}
	}
	target := Level(r, p, *value, next.State)
	switch {
	case rank(target) > rank(next.State):
		next.RecoveringSince = time.Time{}
		if next.PendingSince.IsZero() {
			next.PendingSince = now
		}
		next.Pending = target
		if now.Sub(next.PendingSince) >= p.For {
			next = State{State: target, Since: now}
		}
	case rank(target) < rank(next.State):
		next.Pending, next.PendingSince = "", time.Time{}
		if next.RecoveringSince.IsZero() {
			next.RecoveringSince = now
		}
		if now.Sub(next.RecoveringSince) >= p.Recover {
			next = State{State: target, Since: now}
		}
	default:
		next.Pending, next.PendingSince, next.RecoveringSince = "", time.Time{}, time.Time{}
	}
	return next
}
