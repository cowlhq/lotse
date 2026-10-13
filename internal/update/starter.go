// Package update brings lotse to newer versions on the machines it runs on
// (decision 0009).
//
// The image, or the binary the install script puts in /usr/local/bin, does
// not run the hub or the agent itself. It runs "lotse start", the starter,
// which runs the newest version it finds in its directory, or its own code
// if that is newer. Versions live in Dir as <version>/lotse, with current
// and previous as symbolic links to version directories:
//
//	/var/lib/lotse/bin/
//	  1.2.0/lotse
//	  1.3.0/lotse
//	  current -> 1.3.0
//	  previous -> 1.2.0
//
// A version that exits again within QuickExit of starting, Retries times in
// a row, is marked bad (a file named bad in its directory) and never chosen
// again; the starter goes back to previous, or to its own code.
package update

import (
	"cmp"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/mod/semver"
)

// ExitRestart is the exit code with which a running version asks the
// starter to start the current version again, after it has installed a new
// one.
const ExitRestart = 75

// StartedEnv is set in the environment of what the starter runs, so that it
// knows a starter is there to restart it.
const StartedEnv = "LOTSE_STARTED"

// Starter runs the newest good version of lotse.
type Starter struct {
	// Dir holds the versions.
	Dir string
	// Own is the starter's own executable, and OwnVersion its version.
	Own        string
	OwnVersion string
	Log        *slog.Logger

	// QuickExit, Retries and RetryWait decide when a version is bad.
	QuickExit time.Duration
	Retries   int
	RetryWait time.Duration
	// StopWait is how long a version may take to stop before it is killed.
	StopWait time.Duration
}

func (s *Starter) defaults() {
	s.QuickExit = cmp.Or(s.QuickExit, time.Minute)
	s.Retries = cmp.Or(s.Retries, 3)
	s.RetryWait = cmp.Or(s.RetryWait, 2*time.Second)
	s.StopWait = cmp.Or(s.StopWait, 15*time.Second)
}

// Run runs lotse with args until ctx ends, and returns the exit code to
// leave with. When ctx ends, the running version gets SIGTERM.
func (s *Starter) Run(ctx context.Context, args []string) int {
	s.defaults()
	failures := 0
	for {
		path, v := s.choose()
		started := time.Now()
		code := s.run(ctx, path, args)
		switch {
		case ctx.Err() != nil:
			return code
		case code == ExitRestart:
			s.Log.Info("restarting to switch versions", "from", v)
			failures = 0
			continue
		case code == 0:
			// It stopped because it was meant to.
			return 0
		case path == s.Own:
			// Nothing to go back to: leave it to systemd or Docker to
			// start everything again.
			return code
		case time.Since(started) >= s.QuickExit:
			return code
		}
		failures++
		if failures < s.Retries {
			s.Log.Warn("version exited right after starting, trying again", "version", v, "code", code)
			select {
			case <-ctx.Done():
				return code
			case <-time.After(s.RetryWait):
			}
			continue
		}
		s.Log.Error("version keeps exiting right after starting, going back", "version", v, "code", code)
		s.markBad(v)
		failures = 0
	}
}

// choose returns the newest version that is not bad: current, previous, or
// the starter's own code.
func (s *Starter) choose() (path, version string) {
	path, version = s.Own, s.OwnVersion
	for _, link := range []string{"current", "previous"} {
		v, err := os.Readlink(filepath.Join(s.Dir, link))
		if err != nil {
			continue
		}
		v = filepath.Base(v)
		if s.isBad(v) || !newer(v, version) {
			continue
		}
		p := filepath.Join(s.Dir, v, "lotse")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		return p, v
	}
	return path, version
}

func (s *Starter) run(ctx context.Context, path string, args []string) int {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), StartedEnv+"=1")
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = s.StopWait
	// After a SIGTERM, Run reports the end of ctx even when the process
	// stopped cleanly, so the exit code comes from the process state.
	err := cmd.Run()
	if cmd.ProcessState == nil {
		s.Log.Error("cannot run lotse", "path", path, "err", err)
		return 1
	}
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return cmd.ProcessState.ExitCode()
}

func (s *Starter) isBad(v string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, v, "bad"))
	return err == nil
}

func (s *Starter) markBad(v string) {
	if err := os.WriteFile(filepath.Join(s.Dir, v, "bad"), nil, 0o644); err != nil {
		s.Log.Error("cannot mark a version bad", "version", v, "err", err)
	}
}

// newer says whether version a is newer than b. Anything that is not a
// release version, such as a development build, is never newer, and
// nothing is newer than a development build: a starter built by hand runs
// itself.
func newer(a, b string) bool {
	a, b = "v"+a, "v"+b
	return semver.IsValid(a) && semver.IsValid(b) && semver.Compare(a, b) > 0
}
