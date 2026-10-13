package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/version"
)

// Config is how a probe in another place reaches the hub.
type Config struct {
	Hub string
	// Token is the probe token, needed once, to enroll.
	Token string
	// Slug names the probe, such as fra; Name and Location describe it.
	Slug, Name, Location string
	// DataDir keeps the probe's credential (probe.json).
	DataDir  string
	Insecure bool
}

func (c Config) check() error {
	u, err := url.Parse(c.Hub)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%q is not the address of a hub", c.Hub)
	}
	if u.Scheme == "http" && !c.Insecure {
		if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("refusing plain HTTP to %s; use https, or --insecure if you mean it", u.Host)
		}
	}
	if c.DataDir == "" {
		return errors.New("no data directory given")
	}
	return nil
}

type probeState struct {
	Credential string `json:"credential"`
	ProbeID    int64  `json:"probe_id"`
}

// Run runs the probe until ctx ends: it enrolls once, keeps one connection
// to the hub, runs the checks the hub gives it and sends their results.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := cfg.check(); err != nil {
		return err
	}
	log.Info("lotse probe starting", "version", version.Version, "hub", cfg.Hub, "probe", cfg.Slug)
	sched := &Scheduler{Runner: &Runner{}}
	defer sched.Stop()
	wait := time.Second
	for {
		started := time.Now()
		err := connect(ctx, cfg, sched, log)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > time.Minute {
			wait = time.Second
		}
		d := wait/2 + rand.N(wait/2+1)
		log.Warn("not connected to the hub", "err", err, "retry_in", d.Round(100*time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d):
		}
		wait = min(wait*2, time.Minute)
	}
}

func credential(ctx context.Context, cfg Config, log *slog.Logger) (string, error) {
	path := filepath.Join(cfg.DataDir, "probe.json")
	var st probeState
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &st) == nil && st.Credential != "" {
		return st.Credential, nil
	}
	if cfg.Token == "" {
		return "", errors.New("not enrolled yet, and no probe token given")
	}
	body, _ := json.Marshal(protocol.ProbeEnrollRequest{Token: cfg.Token, Slug: cfg.Slug, Name: cfg.Name, Location: cfg.Location})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(cfg.Hub, "/")+protocol.ProbeEnrollPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct{ Error string }
		json.NewDecoder(res.Body).Decode(&e)
		return "", fmt.Errorf("enrolling: %s %s", res.Status, e.Error)
	}
	var e protocol.ProbeEnrollResponse
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		return "", err
	}
	b, _ := json.Marshal(probeState{Credential: e.Credential, ProbeID: e.ProbeID})
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	log.Info("enrolled", "probe", cfg.Slug)
	return e.Credential, nil
}

func connect(ctx context.Context, cfg Config, sched *Scheduler, log *slog.Logger) error {
	cred, err := credential(ctx, cfg, log)
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(dialCtx, strings.TrimSuffix(cfg.Hub, "/")+protocol.ProbeConnectPath, &websocket.DialOptions{
		Subprotocols: protocol.Subprotocols(),
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + cred}, "X-Lotse-Version": {version.Version}},
	})
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			return errors.New("the hub does not accept this probe's credential")
		}
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	bg := context.WithoutCancel(ctx)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusGoingAway, "probe stopping")
		case <-done:
		}
	}()
	log.Info("connected to the hub")
	// Results go out on this connection; while there is none, they are
	// dropped, and the next ones tell.
	sched.Report = func(id int64, at time.Time, r Result) {
		wctx, cancel := context.WithTimeout(bg, 10*time.Second)
		defer cancel()
		wsjson.Write(wctx, conn, protocol.Message{Type: protocol.TypeResult, Result: &protocol.CheckResult{
			CheckID: id, TS: at.Unix(), OK: r.OK, LatencyMS: r.LatencyMS, StatusCode: r.StatusCode,
			CertNotAfter: r.CertNotAfter, Error: r.Error,
		}})
	}
	for {
		var m protocol.Message
		if err := wsjson.Read(bg, conn, &m); err != nil {
			return err
		}
		if m.Type != protocol.TypeChecks || m.Checks == nil {
			continue
		}
		list := make([]Assignment, len(*m.Checks))
		for i, c := range *m.Checks {
			list[i] = Assignment{CheckID: c.CheckID, Type: c.Type, Spec: c.Spec,
				Interval: time.Duration(c.Interval) * time.Second, Timeout: time.Duration(c.Timeout) * time.Second}
		}
		sched.Set(ctx, list)
		log.Info("running checks", "count", len(list))
	}
}
