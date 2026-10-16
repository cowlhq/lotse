package probe

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

func TestScheduler(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	var mu sync.Mutex
	runs := map[int64]int{}
	s := &Scheduler{Runner: &Runner{}, Report: func(id int64, at time.Time, r Result) {
		mu.Lock()
		runs[id]++
		mu.Unlock()
	}}
	spec := json.RawMessage(`{"address":"` + l.Addr().String() + `"}`)
	a := Assignment{CheckID: 1, Type: "tcp", Spec: spec, Interval: 100 * time.Millisecond, Timeout: time.Second}
	b := Assignment{CheckID: 2, Type: "tcp", Spec: spec, Interval: 100 * time.Millisecond, Timeout: time.Second}
	s.Set(t.Context(), []Assignment{a, b})
	time.Sleep(450 * time.Millisecond)
	s.Set(t.Context(), []Assignment{a})
	mu.Lock()
	two := runs[2]
	mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	s.Stop()
	mu.Lock()
	defer mu.Unlock()
	if runs[1] < 5 || two < 3 {
		t.Errorf("runs: %v", runs)
	}
	if runs[2] > two+1 {
		t.Errorf("check 2 ran on after it was removed: %d, then %d", two, runs[2])
	}
}

// An assignment that fails says so every run, and runs nothing.
func TestSchedulerFail(t *testing.T) {
	got := make(chan Result, 1)
	s := &Scheduler{Runner: &Runner{}, Report: func(_ int64, _ time.Time, r Result) {
		select {
		case got <- r:
		default:
		}
	}}
	s.Set(t.Context(), []Assignment{{CheckID: 1, Type: "http", Spec: []byte(`{"url":"http://127.0.0.1:1"}`), Interval: 100 * time.Millisecond, Timeout: time.Second, Fail: "the environment variable X is not set"}})
	defer s.Stop()
	if r := <-got; r.OK || r.Error != "the environment variable X is not set" {
		t.Errorf("%+v", r)
	}
}
