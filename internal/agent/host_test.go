package agent

import (
	"runtime"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestCPUShares(t *testing.T) {
	prev := cpu.TimesStat{User: 100, System: 50, Idle: 800, Iowait: 40, Steal: 10}
	// 100 s pass: 30 user, 10 system, 40 idle, 15 iowait, 5 steal.
	cur := cpu.TimesStat{User: 130, System: 60, Idle: 840, Iowait: 55, Steal: 15}
	busy, iowait, steal := cpuShares(prev, cur)
	if *busy != 45 || *iowait != 15 || *steal != 5 {
		t.Errorf("busy %v, iowait %v, steal %v; want 45, 15, 5", *busy, *iowait, *steal)
	}
	if b, _, _ := cpuShares(cur, cur); b != nil {
		t.Errorf("no time passed: busy %v, want nothing", *b)
	}
}

func TestRate(t *testing.T) {
	if r := rate(1000, 4000, 10); *r != 300 {
		t.Errorf("rate %v, want 300", *r)
	}
	if r := rate(4000, 1000, 10); r != nil {
		t.Errorf("a counter that went back: %v, want nothing", *r)
	}
}

// The collector reads the machine the test runs on.
func TestHostCollector(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads Linux")
	}
	c := newHostCollector("/")
	h := c.hello(t.Context())
	if h.Hostname == "" || h.Cores == 0 || h.MemTotal == 0 || h.BootTime == 0 {
		t.Errorf("hello: %+v", h)
	}
	first := c.sample(t.Context(), time.Now())
	if first.CPU != nil {
		t.Error("the first reading has a CPU share")
	}
	if first.MemUsed == nil || first.Load1 == nil || first.Uptime == nil {
		t.Errorf("first reading: %+v", first)
	}
	// The kernel counts CPU time in ticks: without time passing, there is
	// nothing to compare.
	time.Sleep(300 * time.Millisecond)
	second := c.sample(t.Context(), time.Now())
	if second.CPU == nil || second.NetRx == nil {
		t.Errorf("second reading: %+v", second)
	}
}
