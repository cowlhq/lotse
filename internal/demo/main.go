// Command demo runs a hub full of made-up data, for screenshots of the
// interface: a small IT service provider with four customers, twelve
// machines with their containers, checks of every kind, a probe in
// Frankfurt, a week of history with a few incidents in it, and alerts that
// fire now.
//
//	pnpm --dir web build      # the interface, into internal/ui/dist
//	go run ./internal/demo -dir /tmp/lotse-demo -listen 127.0.0.1:18096
//
// It starts afresh each time: the data directory is emptied first (it
// refuses a directory that is not empty and not one of its own). The hub
// runs in this process as lotse hub would, and is set up and configured
// over its API, from a lotse.yaml (written to the data directory). Fake
// agents speak the agent protocol and send readings until the demo is
// stopped; the history before it started is written straight into the
// database. Nothing goes out to the internet: the web sites, the DNS
// resolver, the mail server and Slack the checks and channels reach are
// played by the demo, on six ports from -ports on (internet.go).
//
// Alerts that need a few minutes to fire, such as a disk filling up, are
// all there about seven minutes after the start.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
	"lotse.sh/internal/version"
)

type options struct {
	dir, listen string
	// ports is the first of the made-up services' ports; 0 for any.
	ports   int
	history time.Duration
	version string
	// log is the demo's own; the hub's goes to hub.log in dir.
	log *slog.Logger
}

func main() {
	var o options
	var days int
	flag.StringVar(&o.dir, "dir", filepath.Join(os.TempDir(), "lotse-demo"), "the data directory; emptied first")
	flag.StringVar(&o.listen, "listen", "127.0.0.1:18096", "where the hub listens")
	flag.IntVar(&o.ports, "ports", 18100, "the first of six ports for the made-up web sites, DNS, mail and so on; 0 for any free ones")
	flag.IntVar(&days, "days", 7, "days of history to make up")
	flag.StringVar(&o.version, "version", "", "the version hub and agents say they are; that of version.txt if not given")
	flag.Parse()
	o.history = time.Duration(days) * 24 * time.Hour
	o.log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	if o.version == "" {
		b, _ := os.ReadFile("version.txt")
		o.version = cmp.Or(strings.TrimSpace(string(b)), "0.1.0")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d, err := start(ctx, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
	fmt.Printf(`
lotse demo is running: http://%s
  log in as %s, password %s
  %d hosts, %d checks, %d days of history; alerts settle within about 7 minutes
  data in %s (lotse.yaml, hub.log)
Stop it with Ctrl-C.
`, o.listen, adminEmail, adminPassword, len(d.hosts), len(d.checks), days, o.dir)
	select {
	case <-ctx.Done():
	case err := <-d.failed:
		fmt.Fprintln(os.Stderr, "demo:", err)
		stop()
		<-d.hubDone
		os.Exit(1)
	}
	<-d.hubDone
}

// demo is a running demo.
type demo struct {
	client *client
	hosts  []placed
	checks []demoCheck
	failed chan error
	// hubDone is closed when the hub has stopped.
	hubDone chan struct{}
}

func start(ctx context.Context, o options) (*demo, error) {
	log := o.log
	startedAt := time.Now()
	version.Version = o.version
	if err := prepare(o.dir); err != nil {
		return nil, err
	}
	hubLogFile, err := os.OpenFile(filepath.Join(o.dir, "hub.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	hubLog := slog.New(slog.NewTextHandler(hubLogFile, nil))
	s := newStory(startedAt)

	inet, err := startInternet(ctx, o.ports, o.dir, startedAt, o.version, hubLog)
	if err != nil {
		return nil, fmt.Errorf("the made-up internet: %w", err)
	}

	d := &demo{failed: make(chan error, 1), hubDone: make(chan struct{})}
	hubURL := "http://" + o.listen
	hubDir := filepath.Join(o.dir, "hub")
	c, err := startHub(ctx, hub.Config{DataDir: hubDir, Listen: o.listen, ReleasesAPI: inet.web + "/releases/latest"},
		hubLog, d.failed, d.hubDone)
	if err != nil {
		return nil, err
	}
	d.client = c
	log.Info("hub set up", "url", hubURL)

	// The configuration, as lotse apply sends it, with a token for it.
	var token struct{ Secret string }
	if err := c.do(ctx, "POST", "/api/tokens", map[string]any{"name": "ci-deploy", "kind": "apply", "scope_id": store.GlobalScope}, &token); err != nil {
		return nil, err
	}
	yaml := configuration(inet.dns, inet.mx, inet.ssh)
	if err := os.WriteFile(filepath.Join(o.dir, "lotse.yaml"), []byte(yaml), 0o644); err != nil {
		return nil, err
	}
	doc, err := apply.Parse([]byte(yaml))
	if err != nil {
		return nil, fmt.Errorf("lotse.yaml: %w", err)
	}
	if err := doc.ResolveFromEnv(func(name string) string {
		if name == "DEMO_SLACK_WEBHOOK" {
			return inet.web + "/hooks/slack"
		}
		return ""
	}); err != nil {
		return nil, err
	}
	applier := &client{base: c.base, http: &http.Client{Timeout: 2 * time.Minute}, token: token.Secret}
	var applied struct{ Applied bool }
	if err := applier.do(ctx, "POST", "/api/v1/apply", doc, &applied); err != nil {
		return nil, err
	}
	if !applied.Applied {
		return nil, errors.New("lotse.yaml was not applied")
	}
	if err := c.do(ctx, "PUT", "/api/mail-server", map[string]any{
		"host": "localhost", "port": inet.smtp, "from": "lotse@ops.example", "tls": "none",
	}, nil); err != nil {
		return nil, err
	}
	log.Info("configured", "file", filepath.Join(o.dir, "lotse.yaml"))

	// The machines enroll with their environments' tokens.
	var scopes []struct {
		ID   int64
		Kind string
		Path string
	}
	if err := c.do(ctx, "GET", "/api/scopes", nil, &scopes); err != nil {
		return nil, err
	}
	ms := machines(s)
	tokens := map[string]string{}
	for _, env := range environments(ms) {
		for _, x := range scopes {
			if x.Kind == "environment" && x.Path == env {
				var e struct{ Token string }
				if err := c.do(ctx, "POST", fmt.Sprintf("/api/scopes/%d/enrollment", x.ID), nil, &e); err != nil {
					return nil, err
				}
				tokens[env] = e.Token
			}
		}
	}
	agents := make([]*fakeAgent, len(ms))
	for i := range ms {
		a := &fakeAgent{m: &ms[i], story: s, hub: hubURL, version: o.version, log: log}
		if err := a.enroll(ctx, tokens[ms[i].Env]); err != nil {
			return nil, err
		}
		agents[i] = a
		go a.run(ctx)
	}

	// The probe in Frankfurt.
	var probeToken struct{ Token string }
	if err := c.do(ctx, "POST", "/api/probes/enrollment", nil, &probeToken); err != nil {
		return nil, err
	}
	go probe.Run(ctx, probe.Config{
		Hub: hubURL, Token: probeToken.Token, Slug: "fra", Name: "fra", Location: "Frankfurt",
		DataDir: filepath.Join(o.dir, "probe-fra"),
	}, hubLog)

	// Wait until the hub knows every machine's containers, and the probe.
	for i, a := range agents {
		m := &ms[i]
		p := placed{m: m, hostID: a.hostID, containers: map[string]int64{}}
		err := waitFor(ctx, 30*time.Second, func() bool {
			var detail struct {
				Host       struct{ Online bool }
				Containers []struct {
					ID   int64
					Slug string
				}
			}
			if c.do(ctx, "GET", fmt.Sprintf("/api/hosts/%d", a.hostID), nil, &detail) != nil || !detail.Host.Online {
				return false
			}
			for _, x := range detail.Containers {
				p.containers[x.Slug] = x.ID
			}
			return len(p.containers) >= len(m.Containers)
		})
		if err != nil {
			return nil, fmt.Errorf("%s did not come online with its containers: %w", m.Name, err)
		}
		d.hosts = append(d.hosts, p)
	}
	var probeIDs []int64
	if err := waitFor(ctx, 30*time.Second, func() bool {
		var list []struct {
			ID      int64
			Builtin bool
			Online  bool
		}
		probeIDs = nil
		if c.do(ctx, "GET", "/api/probes", nil, &list) != nil {
			return false
		}
		for _, p := range list {
			if p.Online {
				probeIDs = append(probeIDs, p.ID)
			}
		}
		return len(probeIDs) == 2
	}); err != nil {
		return nil, fmt.Errorf("the probe did not come online: %w", err)
	}
	// Two checks come from the shop's containers' labels.
	if err := waitFor(ctx, 30*time.Second, func() bool {
		d.checks = nil
		if c.do(ctx, "GET", "/api/checks", nil, &d.checks) != nil {
			return false
		}
		labelled := 0
		for _, x := range d.checks {
			if strings.HasSuffix(x.Path, "/shop-app/web") {
				labelled++
			}
		}
		return labelled == 2
	}); err != nil {
		return nil, fmt.Errorf("the checks are not all there: %w", err)
	}
	log.Info("machines and probe online", "hosts", len(d.hosts), "checks", len(d.checks))

	db, err := store.OpenAsIs(filepath.Join(hubDir, "lotse.db"))
	if err != nil {
		return nil, err
	}
	go func() {
		<-d.hubDone
		db.Close()
		hubLogFile.Close()
	}()
	if err := heartbeats(ctx, c, db, s, d.checks); err != nil {
		return nil, fmt.Errorf("heartbeats: %w", err)
	}
	go acknowledge(ctx, c)
	took := time.Now()
	b := &backfill{db: db, s: s, net: inet, version: o.version, from: startedAt.Add(-o.history), hosts: d.hosts, checks: d.checks, probes: probeIDs}
	if err := b.run(ctx); err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	log.Info("history written", "days", o.history.Hours()/24, "took", time.Since(took).Round(time.Second))
	return d, nil
}

// heartbeats pings the heartbeat checks: the stock sync every five
// minutes, the nightly backup at night. The nightly export last came the
// night before yesterday, so it is late.
func heartbeats(ctx context.Context, c *client, db *store.Store, s *story, checks []demoCheck) error {
	at := func(daysAgo, hour, minute int) time.Time {
		d := s.start.Local().AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.Local)
	}
	urls := map[string]string{}
	for _, x := range checks {
		if x.Type != "heartbeat" {
			continue
		}
		var detail struct {
			PingURL string `json:"ping_url"`
		}
		if err := c.do(ctx, "GET", fmt.Sprintf("/api/checks/%d", x.ID), nil, &detail); err != nil {
			return err
		}
		urls[x.Path] = detail.PingURL
		if err := ping(ctx, detail.PingURL); err != nil {
			return err
		}
		var last time.Time
		switch x.Path {
		case "ops/infra/production/nightly-backup":
			last = at(0, 3, 12)
			if last.After(s.start) {
				last = at(1, 3, 12)
			}
		case "atlas/api/production/nightly-export":
			last = at(1, 0, 31)
			if s.start.Sub(last) < 25*time.Hour+10*time.Minute {
				last = at(2, 0, 31)
			}
		default:
			continue
		}
		// When the job last ran, before the demo: in the database, as the
		// hub would have kept it.
		_, token, _ := strings.Cut(detail.PingURL, "/ping/")
		if _, err := db.Ping(ctx, auth.HashToken(token), last.Unix()); err != nil {
			return err
		}
	}
	go func() {
		sync, backup := time.Now(), time.Time{}
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				if now.Sub(sync) >= 5*time.Minute {
					ping(ctx, urls["nordwind/erp/production/stock-sync"])
					sync = now
				}
				if inWindow(now, 3*time.Hour+12*time.Minute, 3*time.Hour+14*time.Minute) && now.Sub(backup) > time.Hour {
					ping(ctx, urls["ops/infra/production/nightly-backup"])
					backup = now
				}
			}
		}
	}()
	return nil
}

// acknowledge has the admin take on the bakery's full disk once it fires,
// as someone looking after it would.
func acknowledge(ctx context.Context, c *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
		var alerts []struct {
			AlertID     int64  `json:"alert_id"`
			Rule        string `json:"rule"`
			State       string `json:"state"`
			SubjectPath string `json:"subject_path"`
		}
		if c.do(ctx, "GET", "/api/alerts", nil, &alerts) != nil {
			continue
		}
		for _, a := range alerts {
			if a.Rule == "host.disk" && a.State == "warn" && a.AlertID != 0 && strings.HasSuffix(a.SubjectPath, "/www-01") {
				c.do(ctx, "POST", fmt.Sprintf("/api/alerts/%d/acknowledge", a.AlertID), nil, nil)
				return
			}
		}
	}
}

func ping(ctx context.Context, url string) error {
	if url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: %s", res.Status)
	}
	return nil
}

// marker says that a directory is a demo's, which may be emptied.
const marker = ".lotse-demo"

// prepare empties dir, if it is a demo's, or makes it.
func prepare(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
			return fmt.Errorf("%s is not empty and not a demo's; refusing to empty it", dir)
		}
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(filepath.Join(dir, marker), []byte("A lotse demo's data; the next demo deletes it.\n"), 0o644)
}
