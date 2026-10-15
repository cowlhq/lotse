package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Rule is an alert rule (design 8.2). Its settings are
// rules.<name>.enabled, .for and .recover, and either .warn and .crit, for
// a rule on a value, or .level, for a rule on a condition.
type Rule struct {
	Name        string
	Description string
	// On is the kind of object the rule watches: host, container, check,
	// probe or channel.
	On string
	// Unit is what the value is in: percent, count, days or load (per
	// core). A rule without one watches a condition, which holds or not.
	Unit string
	// Below says that low values are bad, as with the days a certificate
	// has left.
	Below bool
	// Warn and Crit are the thresholds; nil turns a level off.
	Warn, Crit *float64
	// Level is how bad a condition is while it holds: warn or crit.
	Level string
	// Margin is how far back over the threshold a value must be to count
	// as good again, so that a value near it does not flap.
	Margin float64
	// For is how long it must be bad before it fires, and Recover how long
	// good again before it clears.
	For, Recover time.Duration
	Enabled      bool
}

// Condition says whether the rule watches a condition rather than a value.
func (r Rule) Condition() bool { return r.Unit == "" }

func num(f float64) *float64 { return &f }

// Rules are the rules there are, active from the first minute where they
// are enabled (design 8.2).
var Rules = []Rule{
	{Name: "host.silent", On: "host", Description: "The agent sends nothing.", Level: "crit", For: 2 * time.Minute, Enabled: true},
	{Name: "host.disk", On: "host", Unit: "percent", Description: "A filesystem is full.", Warn: num(85), Crit: num(95), Margin: 2, For: 5 * time.Minute, Recover: 5 * time.Minute, Enabled: true},
	{Name: "host.cpu", On: "host", Unit: "percent", Description: "The CPUs are busy.", Warn: num(90), Crit: num(98), Margin: 5, For: 10 * time.Minute, Recover: 5 * time.Minute},
	{Name: "host.memory", On: "host", Unit: "percent", Description: "Memory is used up.", Warn: num(90), Crit: num(95), Margin: 3, For: 10 * time.Minute, Recover: 5 * time.Minute},
	{Name: "host.swap", On: "host", Unit: "percent", Description: "Swap is used up.", Warn: num(50), Crit: num(80), Margin: 5, For: 10 * time.Minute, Recover: 5 * time.Minute},
	{Name: "host.load", On: "host", Unit: "load", Description: "The load over 5 minutes, per core, is high.", Warn: num(2), Crit: num(4), Margin: 0.25, For: 10 * time.Minute, Recover: 5 * time.Minute},
	{Name: "host.iowait", On: "host", Unit: "percent", Description: "The CPUs wait for disks.", Warn: num(20), Crit: num(40), Margin: 5, For: 10 * time.Minute, Recover: 5 * time.Minute},
	{Name: "host.unit", On: "host", Description: "A systemd unit failed.", Level: "warn", Enabled: true},
	{Name: "host.service", On: "host", Description: "A service in host.services does not run.", Level: "crit", For: 2 * time.Minute, Enabled: true},
	{Name: "host.reboot", On: "host", Description: "Updates have waited this long for a restart.", Level: "warn", For: 24 * time.Hour, Enabled: true},
	{Name: "container.unhealthy", On: "container", Description: "A running container's health check fails.", Level: "crit", For: 2 * time.Minute, Enabled: true},
	{Name: "container.restarting", On: "container", Unit: "count", Description: "A container stopped this many times in 10 minutes.", Crit: num(3), Margin: 1, Recover: 10 * time.Minute, Enabled: true},
	{Name: "check.down", On: "check", Unit: "count", Description: "A check failed this many runs in a row, on most of its probes.", Crit: num(2), Enabled: true},
	{Name: "check.cert", On: "check", Unit: "days", Below: true, Description: "A certificate expires in fewer days than this.", Warn: num(14), Crit: num(7), Enabled: true},
	{Name: "check.domain", On: "check", Unit: "days", Below: true, Description: "A domain's registration expires in fewer days than this.", Warn: num(30), Crit: num(7), Enabled: true},
	{Name: "check.heartbeat", On: "check", Description: "A heartbeat did not come in time, or said its job failed.", Level: "crit", Enabled: true},
	{Name: "probe.silent", On: "probe", Description: "An external probe is gone.", Level: "warn", For: 2 * time.Minute, Enabled: true},
	{Name: "channel.failing", On: "channel", Description: "A channel cannot send notifications.", Level: "crit", Enabled: true},
}

// LookupRule returns the rule of a name.
func LookupRule(name string) (Rule, bool) {
	for _, r := range Rules {
		if r.Name == name {
			return r, true
		}
	}
	return Rule{}, false
}

// ruleKeys are the settings of every rule.
func ruleKeys() []Key {
	var keys []Key
	for _, r := range Rules {
		on, global := []string{r.On}, false
		switch r.On {
		case "container":
			// A host's settings apply to its containers.
			on = []string{"container", "host"}
		case "probe", "channel":
			global = true
		}
		key := func(field, typ, desc string, def any, check func(json.RawMessage) error) Key {
			b, _ := json.Marshal(def)
			return Key{Name: "rules." + r.Name + "." + field, Type: typ, Description: desc, Default: b, On: on, GlobalOnly: global, Check: check}
		}
		keys = append(keys, key("enabled", "bool", r.Description+" Whether the rule is on.", r.Enabled, isBool))
		if r.Condition() {
			keys = append(keys, key("level", "level", "How bad it is: warn or crit.", r.Level, isLevel))
		} else {
			for _, t := range []struct {
				field, level string
				def          *float64
			}{{"warn", "WARN", r.Warn}, {"crit", "CRIT", r.Crit}} {
				var def any = "off"
				if t.def != nil {
					def = *t.def
				}
				keys = append(keys, key(t.field, "threshold", fmt.Sprintf("%s at this value (%s), or off.", t.level, r.Unit), def, isThreshold))
			}
		}
		keys = append(keys,
			key("for", "duration", "How long it must hold before the alert fires.", short(r.For), durationBetween(0, 24*time.Hour)),
			key("recover", "duration", "How long it must be good again before the alert clears.", short(r.Recover), durationBetween(0, 24*time.Hour)))
	}
	return keys
}

// Threshold reads a threshold: a number, or nil for off.
func Threshold(v json.RawMessage) (*float64, error) {
	var f float64
	if json.Unmarshal(v, &f) == nil {
		return &f, nil
	}
	var s string
	if json.Unmarshal(v, &s) == nil && s == "off" {
		return nil, nil
	}
	return nil, fmt.Errorf("a number, or \"off\"")
}

func isThreshold(v json.RawMessage) error {
	_, err := Threshold(v)
	return err
}

func isLevel(v json.RawMessage) error {
	var s string
	if json.Unmarshal(v, &s) != nil || s != "warn" && s != "crit" {
		return fmt.Errorf("warn or crit")
	}
	return nil
}
