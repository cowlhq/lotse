package store_test

import (
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestRollup(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		// Two hours of readings every 15 seconds, CPU rising from 0 to
		// 479 by one each time.
		var b store.Batch
		for i := 0; i < 480; i++ {
			cpu := float64(i)
			b.HostSamples = append(b.HostSamples, store.HostSample{HostID: h.ID, TS: int64(i * 15), CPU: &cpu})
		}
		if err := s.Write(ctx, b); err != nil {
			t.Fatal(err)
		}
		if err := s.Rollup(ctx, 7200); err != nil {
			t.Fatal(err)
		}
		// Again: nothing twice.
		if err := s.Rollup(ctx, 7200); err != nil {
			t.Fatal(err)
		}
		five, err := s.Series(ctx, "host", store.FiveMinutes, h.ID, 0, 7200, 300)
		if err != nil || len(five.Time) != 24 {
			t.Fatalf("5 minutes: %d steps, %v", len(five.Time), err)
		}
		// The first 5 minutes: readings 0 to 19, average 9.5.
		if v := *five.Columns["cpu"][0]; v != 9.5 {
			t.Errorf("first 5 minutes: %v", v)
		}
		hour, _ := s.Series(ctx, "host", store.Hour, h.ID, 0, 7200, 3600)
		if len(hour.Time) != 2 || *hour.Columns["cpu"][0] != 119.5 {
			t.Errorf("hours: %+v", hour)
		}
		// Raw readings go; the averages stay.
		if n, err := s.DeleteBefore(ctx, store.Raw, 7200); err != nil || n != 480 {
			t.Errorf("deleted %d raw, %v", n, err)
		}
		if again, _ := s.Series(ctx, "host", store.FiveMinutes, h.ID, 0, 7200, 300); len(again.Time) != 24 {
			t.Errorf("5 minutes after the raw readings went: %d", len(again.Time))
		}
		if n, _ := s.DeleteBefore(ctx, store.FiveMinutes, 3600); n != 12 {
			t.Errorf("deleted %d of 5 minutes", n)
		}
	})
}

// A long range reaches up to now: what is not averaged yet comes raw.
func TestSeriesUpToNow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		var b store.Batch
		for i := 0; i < 480; i++ {
			cpu := 10.0
			if i >= 240 {
				cpu = 30
			}
			b.HostSamples = append(b.HostSamples, store.HostSample{HostID: h.ID, TS: int64(i * 15), CPU: &cpu})
		}
		s.Write(ctx, b)
		// Averaged up to the first hour only.
		s.Rollup(ctx, 3600)
		sr, err := s.Series(ctx, "host", store.FiveMinutes, h.ID, 0, 7200, 600)
		if err != nil || len(sr.Time) != 12 {
			t.Fatalf("steps: %v, %v", sr.Time, err)
		}
		if *sr.Columns["cpu"][0] != 10 || *sr.Columns["cpu"][11] != 30 {
			t.Errorf("cpu: first %v, last %v", *sr.Columns["cpu"][0], *sr.Columns["cpu"][11])
		}
	})
}

// The finest resolution that reaches back far enough is read.
func TestResolution(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		var b store.Batch
		cpu := 1.0
		for i := 0; i < 3*24*240; i++ {
			b.HostSamples = append(b.HostSamples, store.HostSample{HostID: h.ID, TS: 100000 + int64(i*15), CPU: &cpu})
		}
		s.Write(ctx, b)
		for _, c := range []struct {
			from int64
			want string
		}{{0, store.Raw}, {150000, store.Raw}} {
			if got, _ := s.Resolution(ctx, "host", h.ID, c.from); got != c.want {
				t.Errorf("before deleting, from %d: %q, want %q", c.from, got, c.want)
			}
		}
		s.Rollup(ctx, 100000+3*86400)
		s.DeleteBefore(ctx, store.Raw, 100000+2*86400)
		s.DeleteBefore(ctx, store.FiveMinutes, 100000+86400)
		for _, c := range []struct {
			from int64
			want string
		}{{0, store.Hour}, {100000 + 86400, store.FiveMinutes}, {100000 + 2*86400 + 600, store.Raw}} {
			if got, _ := s.Resolution(ctx, "host", h.ID, c.from); got != c.want {
				t.Errorf("from %d: %q, want %q", c.from, got, c.want)
			}
		}
	})
}
