package alert

import (
	"encoding/json"
	"testing"
	"time"

	"lotse.sh/internal/config"
)

func v(f float64) *float64 { return &f }

func TestDisk(t *testing.T) {
	r, _ := config.LookupRule("host.disk") // WARN 85, CRIT 95, margin 2, for 5m, recover 5m
	p := ParamsOf(r, nil)
	start := time.Unix(1000, 0)
	at := func(m int) time.Time { return start.Add(time.Duration(m) * time.Minute) }

	st := Step(r, p, State{}, v(90), at(0))
	if st.State != OK || st.Pending != Warn {
		t.Fatalf("over WARN: %+v", st)
	}
	st = Step(r, p, st, v(96), at(3))
	if st.State != OK || st.Pending != Crit || !st.PendingSince.Equal(at(0)) {
		t.Fatalf("over CRIT while pending: %+v", st)
	}
	st = Step(r, p, st, v(96), at(5))
	if st.State != Crit || !st.Since.Equal(at(5)) || st.Pending != "" {
		t.Fatalf("held for 5 minutes: %+v", st)
	}
	// 94 is under CRIT, but not by the margin: still CRIT, not recovering.
	st = Step(r, p, st, v(94), at(6))
	if st.State != Crit || !st.RecoveringSince.IsZero() {
		t.Fatalf("within the margin: %+v", st)
	}
	st = Step(r, p, st, v(80), at(7))
	if st.State != Crit || !st.RecoveringSince.Equal(at(7)) {
		t.Fatalf("recovering: %+v", st)
	}
	// Back up before it recovered: it starts over.
	st = Step(r, p, st, v(97), at(8))
	if st.State != Crit || !st.RecoveringSince.IsZero() {
		t.Fatalf("bad again: %+v", st)
	}
	st = Step(r, p, st, v(80), at(9))
	st = Step(r, p, st, v(80), at(14))
	if st.State != OK || !st.Since.Equal(at(14)) {
		t.Fatalf("recovered: %+v", st)
	}

	// A threshold turned off, another changed.
	p = ParamsOf(r, map[string]json.RawMessage{"warn": json.RawMessage(`"off"`), "crit": json.RawMessage(`99`), "for": json.RawMessage(`"0s"`)})
	if p.Warn != nil || *p.Crit != 99 || p.For != 0 {
		t.Fatalf("params: %+v", p)
	}
	if st := Step(r, p, State{}, v(98), at(0)); st.State != OK || st.Pending != "" {
		t.Errorf("98 with WARN off, CRIT 99: %+v", st)
	}
	if st := Step(r, p, State{}, v(99), at(0)); st.State != Crit {
		t.Errorf("99 with no wait: %+v", st)
	}
}

func TestUnknown(t *testing.T) {
	r, _ := config.LookupRule("host.disk")
	p := ParamsOf(r, map[string]json.RawMessage{"for": json.RawMessage(`"5m"`)})
	now := time.Unix(1000, 0)
	crit := State{State: Crit, Since: now}
	st := Step(r, p, crit, nil, now.Add(time.Minute))
	if st.State != Unknown || st.Previous != Crit {
		t.Fatalf("no data: %+v", st)
	}
	if again := Step(r, p, st, nil, now.Add(2*time.Minute)); again != st {
		t.Errorf("still no data: %+v", again)
	}
	// Data again, still bad: CRIT at once, not after another 5 minutes.
	if back := Step(r, p, st, v(97), now.Add(3*time.Minute)); back.State != Crit {
		t.Errorf("back, still full: %+v", back)
	}
	// Data again, and fine: recovering from CRIT.
	if back := Step(r, p, st, v(50), now.Add(3*time.Minute)); back.State != Crit || back.RecoveringSince.IsZero() {
		t.Errorf("back, fine: %+v", back)
	}
}

func TestBelow(t *testing.T) {
	r, _ := config.LookupRule("check.cert") // WARN under 14 days, CRIT under 7, at once
	p := ParamsOf(r, nil)
	now := time.Unix(1000, 0)
	for days, want := range map[float64]string{30: OK, 14: OK, 13.5: Warn, 6: Crit} {
		if st := Step(r, p, State{}, v(days), now); st.State != want {
			t.Errorf("%v days: %s, want %s", days, st.State, want)
		}
	}
	if th := Threshold(p, OK, Crit); *th != 7 {
		t.Errorf("threshold to CRIT: %v", *th)
	}
	if th := Threshold(p, Warn, OK); *th != 14 {
		t.Errorf("threshold back from WARN: %v", *th)
	}
}

func TestCondition(t *testing.T) {
	r, _ := config.LookupRule("host.silent") // CRIT after 2 minutes
	p := ParamsOf(r, map[string]json.RawMessage{"level": json.RawMessage(`"warn"`)})
	now := time.Unix(1000, 0)
	st := Step(r, p, State{}, v(1), now)
	if st.State != OK || st.Pending != Warn {
		t.Fatalf("silent: %+v", st)
	}
	if st = Step(r, p, st, v(1), now.Add(2*time.Minute)); st.State != Warn {
		t.Fatalf("silent for 2 minutes, level warn: %+v", st)
	}
	if st = Step(r, p, st, v(0), now.Add(3*time.Minute)); st.State != OK {
		t.Fatalf("back: %+v", st)
	}
	if Threshold(p, OK, Warn) != nil {
		t.Error("a condition has a threshold")
	}
}
