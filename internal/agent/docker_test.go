package agent

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestMemoryUsed(t *testing.T) {
	for _, c := range []struct {
		name string
		m    container.MemoryStats
		want uint64
	}{
		{"cgroup v2", container.MemoryStats{Usage: 1000, Stats: map[string]uint64{"inactive_file": 300}}, 700},
		{"cgroup v1", container.MemoryStats{Usage: 1000, Stats: map[string]uint64{"total_inactive_file": 200}}, 800},
		{"no stats", container.MemoryStats{Usage: 1000}, 1000},
		{"more cache than usage", container.MemoryStats{Usage: 100, Stats: map[string]uint64{"inactive_file": 300}}, 100},
	} {
		if got := memoryUsed(c.m); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
