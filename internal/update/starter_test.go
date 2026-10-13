package update

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The test binary also plays the versions the starter runs. Which version
// it plays follows from the directory it is started from; what it does is
// set per version in fakeBehavior.
const (
	fakeBehavior = "LOTSE_TEST_FAKE"     // version=action,…
	fakeLog      = "LOTSE_TEST_FAKE_LOG" // file that gets one line per start
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeBehavior) != "" {
		os.Exit(fake())
	}
	os.Exit(m.Run())
}

func fake() int {
	version := filepath.Base(filepath.Dir(os.Args[0]))
	if _, err := strconv.Atoi(version[:1]); err != nil {
		version = "own"
	}
	f, _ := os.OpenFile(os.Getenv(fakeLog), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	fmt.Fprintln(f, version)
	f.Close()
	action := ""
	for _, rule := range strings.Split(os.Getenv(fakeBehavior), ",") {
		if v, a, _ := strings.Cut(rule, "="); v == version {
			action = a
		}
	}
	switch {
	case action == "wait":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM)
		<-stop
		return 0
	case action == "restart-once":
		marker := os.Getenv(fakeLog) + ".restarted"
		if _, err := os.Stat(marker); err != nil {
			os.WriteFile(marker, nil, 0o644)
			return ExitRestart
		}
		return 0
	case strings.HasPrefix(action, "exit:"):
		code, _ := strconv.Atoi(strings.TrimPrefix(action, "exit:"))
		return code
	}
	return 0
}

type setup struct {
	t   *testing.T
	dir string
	log string
	s   *Starter
}

func newSetup(t *testing.T, ownVersion, behavior string) *setup {
	own, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log := filepath.Join(t.TempDir(), "runs")
	t.Setenv(fakeBehavior, behavior)
	t.Setenv(fakeLog, log)
	return &setup{t: t, dir: dir, log: log, s: &Starter{
		Dir: dir, Own: own, OwnVersion: ownVersion,
		Log:       slog.New(slog.DiscardHandler),
		QuickExit: 5 * time.Second, RetryWait: time.Millisecond,
	}}
}

// install puts version v into the directory, as a link to the test binary.
func (s *setup) install(v string) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Join(s.dir, v), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Symlink(s.s.Own, filepath.Join(s.dir, v, "lotse")); err != nil {
		s.t.Fatal(err)
	}
}

func (s *setup) link(name, v string) {
	s.t.Helper()
	if err := os.Symlink(v, filepath.Join(s.dir, name)); err != nil {
		s.t.Fatal(err)
	}
}

func (s *setup) runs() string {
	b, _ := os.ReadFile(s.log)
	return strings.Join(strings.Fields(string(b)), " ")
}

func TestChoose(t *testing.T) {
	for _, c := range []struct {
		name, own, want string
		prepare         func(s *setup)
	}{
		{"nothing installed", "1.0.0", "1.0.0", func(s *setup) {}},
		{"current is newer", "1.0.0", "1.2.0", func(s *setup) {
			s.install("1.2.0")
			s.link("current", "1.2.0")
		}},
		{"own is newer", "1.3.0", "1.3.0", func(s *setup) {
			s.install("1.2.0")
			s.link("current", "1.2.0")
		}},
		{"current is bad", "1.0.0", "1.1.0", func(s *setup) {
			s.install("1.1.0")
			s.install("1.2.0")
			s.link("current", "1.2.0")
			s.link("previous", "1.1.0")
			os.WriteFile(filepath.Join(s.dir, "1.2.0", "bad"), nil, 0o644)
		}},
		{"current has no binary", "1.0.0", "1.0.0", func(s *setup) {
			os.MkdirAll(filepath.Join(s.dir, "1.2.0"), 0o755)
			s.link("current", "1.2.0")
		}},
		{"own is a development build", "dev", "dev", func(s *setup) {
			s.install("1.2.0")
			s.link("current", "1.2.0")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSetup(t, c.own, "")
			c.prepare(s)
			if _, got := s.s.choose(); got != c.want {
				t.Errorf("chose %s, want %s", got, c.want)
			}
		})
	}
}

func TestRunGoesBackFromABadVersion(t *testing.T) {
	s := newSetup(t, "1.0.0", "1.2.0=exit:1,own=exit:0")
	s.install("1.2.0")
	s.link("current", "1.2.0")
	if code := s.s.Run(t.Context(), nil); code != 0 {
		t.Errorf("exit code %d, want 0 from the own version", code)
	}
	if got, want := s.runs(), "1.2.0 1.2.0 1.2.0 own"; got != want {
		t.Errorf("ran %q, want %q", got, want)
	}
	if !s.s.isBad("1.2.0") {
		t.Error("1.2.0 is not marked bad")
	}
}

func TestRunRestartsOnRequest(t *testing.T) {
	s := newSetup(t, "1.0.0", "1.2.0=restart-once")
	s.install("1.2.0")
	s.link("current", "1.2.0")
	if code := s.s.Run(t.Context(), nil); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if got, want := s.runs(), "1.2.0 1.2.0"; got != want {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestRunPassesOnTheOwnExitCode(t *testing.T) {
	s := newSetup(t, "1.0.0", "own=exit:3")
	if code := s.s.Run(t.Context(), nil); code != 3 {
		t.Errorf("exit code %d, want 3", code)
	}
	if got := s.runs(); got != "own" {
		t.Errorf("ran %q, want own once", got)
	}
}

func TestRunStopsTheVersionWhenStopped(t *testing.T) {
	s := newSetup(t, "1.0.0", "1.2.0=wait")
	s.install("1.2.0")
	s.link("current", "1.2.0")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int)
	go func() { done <- s.s.Run(ctx, nil) }()
	for s.runs() == "" {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d, want 0 after SIGTERM", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the starter did not stop")
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.2.0", "1.1.9", true},
		{"1.10.0", "1.9.0", true},
		{"1.2.0", "1.2.0", false},
		{"1.1.0", "1.2.0", false},
		{"1.2.0", "dev", false},
		{"dev", "1.2.0", false},
		{"1.2.0", "dev-abc123", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRunPassesOnACleanStop(t *testing.T) {
	s := newSetup(t, "1.0.0", "1.2.0=exit:0")
	s.install("1.2.0")
	s.link("current", "1.2.0")
	if code := s.s.Run(t.Context(), nil); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if got := s.runs(); got != "1.2.0" {
		t.Errorf("ran %q, want 1.2.0 once", got)
	}
}
