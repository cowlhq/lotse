package store_test

import (
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func ptr[T any](v T) *T { return &v }

func TestSaveHost(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, prev, err := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", BootTime: 100, LastSeen: 1000})
		if err != nil {
			t.Fatal(err)
		}
		if h.ID == 0 || h.Name != "web1" || prev != 0 || h.FirstSeen != 1000 {
			t.Errorf("new host: %+v, previous boot %d", h, prev)
		}
		again, prev, err := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1-renamed", BootTime: 200, LastSeen: 2000})
		if err != nil {
			t.Fatal(err)
		}
		if again.ID != h.ID || prev != 100 || again.BootTime != 200 {
			t.Errorf("same machine: %+v, previous boot %d", again, prev)
		}
		// The name stays what it was; the hostname follows the machine.
		if again.Name != "web1" || again.Hostname != "web1-renamed" || again.FirstSeen != 1000 {
			t.Errorf("same machine: name %q, hostname %q, first seen %d", again.Name, again.Hostname, again.FirstSeen)
		}
		hosts, err := s.Hosts(ctx)
		if err != nil || len(hosts) != 1 {
			t.Fatalf("hosts: %v, %v", hosts, err)
		}
		if _, err := s.Host(ctx, 999); err != store.ErrNotFound {
			t.Errorf("a missing host: %v", err)
		}
	})
}

func TestSyncContainers(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		ids, err := s.SyncContainers(ctx, h.ID, []store.Container{
			{Slug: "shop-app", DockerID: "a1", State: "running", Labels: map[string]string{"x": "y"}},
			{Slug: "shop-db", DockerID: "d1", State: "running"},
		}, 10)
		if err != nil {
			t.Fatal(err)
		}
		app := ids["a1"]
		// A redeploy: the app has a new Docker ID, the database is gone.
		ids, err = s.SyncContainers(ctx, h.ID, []store.Container{
			{Slug: "shop-app", DockerID: "a2", State: "running"},
		}, 20)
		if err != nil {
			t.Fatal(err)
		}
		if ids["a2"] != app {
			t.Errorf("the redeployed app got ID %d, want %d", ids["a2"], app)
		}
		list, err := s.Containers(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].Slug != "shop-app" || list[0].GoneAt != nil ||
			list[1].Slug != "shop-db" || list[1].GoneAt == nil || *list[1].GoneAt != 20 {
			t.Errorf("containers: %+v", list)
		}
		if list[0].DockerID != "a2" || list[0].FirstSeen != 10 || list[0].LastSeen != 20 {
			t.Errorf("app: %+v", list[0])
		}
		// Everything gone.
		if _, err := s.SyncContainers(ctx, h.ID, nil, 30); err != nil {
			t.Fatal(err)
		}
		c, err := s.Container(ctx, app)
		if err != nil || c.GoneAt == nil {
			t.Errorf("app after all went: %+v, %v", c, err)
		}
	})
}

func TestWriteAndSeries(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		batch := store.Batch{
			HostSamples: []store.HostSample{
				{HostID: h.ID, TS: 100, CPU: ptr(10.0), MemUsed: ptr[int64](1000)},
				{HostID: h.ID, TS: 115, CPU: ptr(20.0), MemUsed: ptr[int64](3000)},
				{HostID: h.ID, TS: 130, CPU: nil, MemUsed: ptr[int64](5000)},
			},
			FSSamples: []store.FSSample{
				{HostID: h.ID, Mount: "/", Device: "/dev/sda1", FSType: "ext4", TS: 100, Used: 50, Total: 100},
			},
			Events: []store.Event{{TS: 100, SubjectKind: "host", SubjectID: h.ID, Kind: "reboot"}},
			Seen:   map[int64]int64{h.ID: 130},
		}
		if err := s.Write(ctx, batch); err != nil {
			t.Fatal(err)
		}
		// Sent again after a reconnect: skipped, not an error.
		if err := s.Write(ctx, batch); err != nil {
			t.Fatalf("writing the same samples again: %v", err)
		}

		raw, err := s.Series(ctx, "host", store.Raw, h.ID, 0, 200, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw.Time) != 3 || *raw.Columns["cpu"][1] != 20 || raw.Columns["cpu"][2] != nil {
			t.Errorf("raw series: %v %v", raw.Time, raw.Columns["cpu"])
		}
		// Steps of 30 s: 100 and 115 fall into the step at 90, 130 into 120.
		avg, err := s.Series(ctx, "host", store.Raw, h.ID, 0, 200, 30)
		if err != nil {
			t.Fatal(err)
		}
		if len(avg.Time) != 2 || avg.Time[0] != 90 || avg.Time[1] != 120 {
			t.Fatalf("steps: %v", avg.Time)
		}
		if *avg.Columns["cpu"][0] != 15 || *avg.Columns["mem_used"][0] != 2000 || avg.Columns["cpu"][1] != nil {
			t.Errorf("averages: cpu %v, mem %v", avg.Columns["cpu"], avg.Columns["mem_used"])
		}

		events, err := s.Events(ctx, "host", h.ID, 0, 200)
		if err != nil || len(events) != 2 || events[0].Kind != "reboot" {
			t.Errorf("events: %+v, %v", events, err)
		}
		got, _ := s.Host(ctx, h.ID)
		if got.LastSeen != 130 {
			t.Errorf("last seen %d, want 130", got.LastSeen)
		}

		n, err := s.DeleteBefore(ctx, store.Raw, 120)
		if err != nil {
			t.Fatal(err)
		}
		if n != 3 { // two host samples, one filesystem sample
			t.Errorf("deleted %d samples, want 3", n)
		}
		left, _ := s.Series(ctx, "host", store.Raw, h.ID, 0, 200, 1)
		if len(left.Time) != 1 || left.Time[0] != 130 {
			t.Errorf("after deleting: %v", left.Time)
		}
	})
}

func TestLatest(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		if x, err := s.LatestHostSample(ctx, h.ID); x != nil || err != nil {
			t.Errorf("no readings yet: %+v, %v", x, err)
		}
		ids, _ := s.SyncContainers(ctx, h.ID, []store.Container{{Slug: "app", DockerID: "a"}}, 1)
		err := s.Write(ctx, store.Batch{
			HostSamples: []store.HostSample{{HostID: h.ID, TS: 10, CPU: ptr(1.0)}, {HostID: h.ID, TS: 20, CPU: ptr(2.0)}},
			FSSamples: []store.FSSample{
				{HostID: h.ID, Mount: "/", TS: 10, Used: 1, Total: 10},
				{HostID: h.ID, Mount: "/", TS: 20, Used: 2, Total: 10},
			},
			ContainerSamples: []store.ContainerSample{
				{ContainerID: ids["a"], TS: 10, Mem: ptr[int64](5)},
				{ContainerID: ids["a"], TS: 40, Mem: ptr[int64](7)},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		x, err := s.LatestHostSample(ctx, h.ID)
		if err != nil || x.TS != 20 || *x.CPU != 2 {
			t.Errorf("latest host reading: %+v, %v", x, err)
		}
		fs, err := s.Filesystems(ctx, h.ID)
		if err != nil || len(fs) != 1 || *fs[0].Used != 2 {
			t.Errorf("filesystems: %+v, %v", fs, err)
		}
		cs, err := s.LatestContainerSamples(ctx, h.ID)
		if err != nil || cs[ids["a"]].TS != 40 || *cs[ids["a"]].Mem != 7 {
			t.Errorf("latest container readings: %+v, %v", cs, err)
		}
	})
}
