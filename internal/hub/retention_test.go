package hub

import (
	"fmt"
	"testing"
	"time"

	"lotse.sh/internal/store"
)

// Readings past their time are averaged, then deleted, and a long range
// still shows them.
func TestRetention(t *testing.T) {
	h, server := newTestHub(t)
	ctx := t.Context()
	now := time.Now()
	host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: now.Unix()})
	var b store.Batch
	cpu := 50.0
	for ts := now.Add(-72 * time.Hour).Unix(); ts < now.Unix(); ts += 60 {
		b.HostSamples = append(b.HostSamples, store.HostSample{HostID: host.ID, TS: ts, CPU: &cpu})
	}
	h.db.Write(ctx, b)
	h.retainOnce(ctx, now)

	if res, _ := h.db.Resolution(ctx, "host", host.ID, now.Add(-7*24*time.Hour).Unix()); res != store.FiveMinutes {
		t.Errorf("a week back is read from %q", res)
	}
	var sr struct {
		Time    []int64
		Step    int64
		Columns map[string][]*float64
	}
	getJSON(t, server, fmt.Sprintf("/api/hosts/%d/series?from=%d&points=100", host.ID, now.Add(-7*24*time.Hour).Unix()), &sr)
	if len(sr.Time) == 0 || sr.Time[0] > now.Add(-71*time.Hour).Unix() || *sr.Columns["cpu"][0] != 50 {
		t.Errorf("a week: step %d, from %v", sr.Step, sr.Time)
	}
	if last := sr.Time[len(sr.Time)-1]; last < now.Add(-2*time.Hour).Unix() {
		t.Errorf("a week ends %s ago", now.Sub(time.Unix(last, 0)))
	}
}
