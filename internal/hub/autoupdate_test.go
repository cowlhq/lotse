package hub

import (
	"slices"
	"testing"
	"time"
)

func TestRolloutPlan(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var r rollouts
	// Environment 1 has hosts 1, 2, 3; environment 2 has host 4, which
	// does not update automatically; host 5 is outside its window.
	cands := []candidate{
		{hostID: 1, envID: 1, version: "1.0.0", auto: true, inWindow: true},
		{hostID: 2, envID: 1, version: "1.0.0", auto: true, inWindow: true},
		{hostID: 3, envID: 1, version: "1.0.0", auto: true, inWindow: true},
		{hostID: 4, envID: 2, version: "1.0.0", auto: false, inWindow: true},
		{hostID: 5, envID: 3, version: "1.0.0", auto: true, inWindow: false},
	}
	if got := r.plan(now, "1.1.0", cands); !slices.Equal(got, []int64{1}) {
		t.Fatalf("first round: %v, want host 1 alone", got)
	}
	// Five minutes on, host 1 runs 1.1.0, but not for long enough.
	cands[0].version = "1.1.0"
	if got := r.plan(now.Add(5*time.Minute), "1.1.0", cands); len(got) != 0 {
		t.Errorf("too early: %v", got)
	}
	// Ten minutes on: the rest of environment 1.
	if got := r.plan(now.Add(10*time.Minute), "1.1.0", cands); !slices.Equal(got, []int64{2, 3}) {
		t.Errorf("after the first: %v", got)
	}
	// They are not asked again while their update runs.
	if got := r.plan(now.Add(15*time.Minute), "1.1.0", cands); len(got) != 0 {
		t.Errorf("asked again: %v", got)
	}

	// A version that fails on the first host gets no further: host 1
	// stays on 1.1.0 (rolled back), the others wait.
	if got := r.plan(now.Add(time.Hour), "1.2.0", cands); !slices.Equal(got, []int64{1}) {
		t.Fatalf("1.2.0 first round: %v", got)
	}
	if got := r.plan(now.Add(2*time.Hour), "1.2.0", cands); !slices.Equal(got, []int64{1}) {
		t.Errorf("1.2.0, host 1 failed: %v, want host 1 to try again alone", got)
	}
}
