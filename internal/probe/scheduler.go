package probe

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sync"
	"time"
)

// Assignment is a check a probe runs: what, how often, how long to wait.
type Assignment struct {
	CheckID  int64           `json:"check_id"`
	Type     string          `json:"type"`
	Spec     json.RawMessage `json:"spec"`
	Interval time.Duration   `json:"interval"`
	Timeout  time.Duration   `json:"timeout"`
}

func (a Assignment) same(b Assignment) bool {
	return a.Type == b.Type && string(a.Spec) == string(b.Spec) && a.Interval == b.Interval && a.Timeout == b.Timeout
}

// Scheduler runs each assignment at its interval, starting at a random
// point within the first interval, so that checks spread out.
type Scheduler struct {
	Runner *Runner
	// Report gets every result.
	Report func(checkID int64, at time.Time, r Result)

	mu      sync.Mutex
	running map[int64]running
	loops   sync.WaitGroup
}

type running struct {
	a    Assignment
	stop context.CancelFunc
}

// Set makes the scheduler run exactly these assignments: new ones start,
// changed ones restart, missing ones stop.
func (s *Scheduler) Set(ctx context.Context, list []Assignment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		s.running = map[int64]running{}
	}
	want := map[int64]Assignment{}
	for _, a := range list {
		want[a.CheckID] = a
	}
	for id, r := range s.running {
		if a, ok := want[id]; !ok || !a.same(r.a) {
			r.stop()
			delete(s.running, id)
		}
	}
	for id, a := range want {
		if _, ok := s.running[id]; ok {
			continue
		}
		cctx, cancel := context.WithCancel(ctx)
		s.running[id] = running{a: a, stop: cancel}
		s.loops.Go(func() { s.loop(cctx, a) })
	}
}

// Stop stops everything, and returns when no check runs any more.
func (s *Scheduler) Stop() {
	s.Set(context.Background(), nil)
	s.loops.Wait()
}

func (s *Scheduler) loop(ctx context.Context, a Assignment) {
	// A floor against a busy loop; settings keep checks at 10 s or more.
	interval := max(a.Interval, 100*time.Millisecond)
	wait := rand.N(interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		at := time.Now()
		r := s.Runner.Run(ctx, a.Type, a.Spec, a.Timeout)
		if ctx.Err() != nil {
			return
		}
		s.Report(a.CheckID, at, r)
		wait = interval - time.Since(at)
	}
}
