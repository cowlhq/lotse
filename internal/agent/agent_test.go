package agent

import (
	"testing"
	"time"
)

func TestConfigCheck(t *testing.T) {
	for _, c := range []struct {
		cfg Config
		ok  bool
	}{
		{Config{Hub: "https://lotse.example.com", DataDir: "d"}, true},
		{Config{Hub: "http://localhost:8080", DataDir: "d"}, true},
		{Config{Hub: "http://127.0.0.1:8080", DataDir: "d"}, true},
		{Config{Hub: "http://[::1]:8080", DataDir: "d"}, true},
		{Config{Hub: "http://hub:8080", DataDir: "d"}, false},
		{Config{Hub: "http://hub:8080", DataDir: "d", Insecure: true}, true},
		{Config{Hub: "https://lotse.example.com"}, false},
		{Config{Hub: "lotse.example.com", DataDir: "d"}, false},
		{Config{Hub: "ftp://lotse.example.com", DataDir: "d"}, false},
		{Config{DataDir: "d"}, false},
	} {
		if err := c.cfg.Check(); (err == nil) != c.ok {
			t.Errorf("%+v: got %v, want ok=%v", c.cfg, err, c.ok)
		}
	}
}

func TestBackoff(t *testing.T) {
	b := backoff{min: time.Second, max: 8 * time.Second}
	limits := []time.Duration{1, 2, 4, 8, 8, 8}
	for i, limit := range limits {
		d := b.next()
		if d < limit*time.Second/2 || d > limit*time.Second {
			t.Errorf("wait %d: %v, want between %v and %v", i, d, limit*time.Second/2, limit*time.Second)
		}
	}
	b.reset()
	if d := b.next(); d > time.Second {
		t.Errorf("after reset: %v, want at most 1s", d)
	}
}
