package hub

import (
	"fmt"
	"net/http"
	"testing"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

func TestHostsAPI(t *testing.T) {
	h, server := newTestHub(t)
	ctx := t.Context()
	host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", MemTotal: 1000, LastSeen: 1})
	ids, _ := h.db.SyncContainers(ctx, host.ID, []store.Container{
		{Slug: "app", DockerID: "a", State: "running", Health: "unhealthy"},
		{Slug: "db", DockerID: "d", State: "exited"},
	}, 1)
	if err := h.db.Write(ctx, store.Batch{
		HostSamples: []store.HostSample{
			{HostID: host.ID, TS: 100, CPU: ptr(10.0)},
			{HostID: host.ID, TS: 200, CPU: ptr(30.0)},
		},
		FSSamples: []store.FSSample{
			{HostID: host.ID, Mount: "/", TS: 200, Used: 40, Total: 100},
			{HostID: host.ID, Mount: "/data", TS: 200, Used: 90, Total: 100},
		},
		ContainerSamples: []store.ContainerSample{{ContainerID: ids["a"], TS: 200, Mem: ptr[int64](64)}},
	}); err != nil {
		t.Fatal(err)
	}

	var hosts []api.HostSummary
	getJSON(t, server, "/api/hosts", &hosts)
	if len(hosts) != 1 {
		t.Fatalf("%d hosts, want 1", len(hosts))
	}
	sum := hosts[0]
	if sum.Host.Online || *sum.Latest.Cpu != 30 || *sum.Disk != 90 ||
		sum.Containers != (api.ContainerCounts{Total: 2, Running: 1, Unhealthy: 1}) {
		t.Errorf("summary: %+v, latest %+v, disk %v", sum, sum.Latest, *sum.Disk)
	}

	var detail api.HostDetail
	getJSON(t, server, fmt.Sprintf("/api/hosts/%d", host.ID), &detail)
	if len(detail.Filesystems) != 2 || len(detail.Containers) != 2 ||
		detail.Containers[0].Latest == nil || *detail.Containers[0].Latest.Mem != 64 ||
		detail.Containers[0].HostName != "web1" {
		t.Errorf("detail: %+v", detail)
	}

	// Readings at 100 and 200 s, asked for in at most 2 points over 1000 s:
	// steps of 500 s put both into the first.
	var sr api.Series
	getJSON(t, server, fmt.Sprintf("/api/hosts/%d/series?from=0&to=1000&points=2", host.ID), &sr)
	if sr.Step != 500 || len(sr.Time) != 1 || *sr.Columns["cpu"][0] != 20 {
		t.Errorf("series: step %d, time %v, cpu %v", sr.Step, sr.Time, sr.Columns["cpu"])
	}

	for path, want := range map[string]int{
		"/api/hosts/999":               http.StatusNotFound,
		"/api/hosts/abc":               http.StatusBadRequest,
		"/api/events?subject=user:1":   http.StatusBadRequest,
		"/api/events?subject=host:1":   http.StatusOK,
		"/api/containers/999":          http.StatusNotFound,
		"/api/hosts/1/series?points=0": http.StatusBadRequest,
	} {
		if status, _ := do(t, server, "GET", path, ""); status != want {
			t.Errorf("GET %s: %d, want %d", path, status, want)
		}
	}
}
