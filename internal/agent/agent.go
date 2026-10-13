// Package agent watches a machine and reports to the hub. It opens one
// connection to the hub, listens on no port, and runs nothing it is told to.
package agent

import (
	"cmp"
	"context"
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
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/update"
	"lotse.sh/internal/version"
)

// Config is how the agent reaches the hub and where it finds the machine.
type Config struct {
	// Hub is the hub's address, such as https://lotse.example.com.
	Hub string
	// Token is the environment's enrollment token. It is needed once, to
	// enroll; after that the agent has a credential of its own.
	Token string
	// DataDir keeps the agent's credential (agent.json).
	DataDir string
	// Insecure allows plain HTTP to a hub that is not on this machine.
	Insecure bool
	// Root is where the machine's / is: "/" for an agent installed on the
	// machine, or where it is mounted into the agent's container.
	Root string
	// BinDir is where the starter finds versions; updates go there.
	BinDir string
	// ReleaseURL is where updates come from; empty means GitHub.
	ReleaseURL string
}

// Check says what is wrong with c, if anything.
func (c Config) Check() error {
	u, err := url.Parse(c.Hub)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not the address of a hub", c.Hub)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !c.Insecure && !isLocal(u.Hostname()) {
			return fmt.Errorf("refusing plain HTTP to %s; use https, or --insecure if you mean it", u.Host)
		}
	default:
		return fmt.Errorf("%q is not the address of a hub", c.Hub)
	}
	if c.DataDir == "" {
		return errors.New("no data directory given")
	}
	return nil
}

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

const (
	// bufferAge is how much the agent keeps while the hub is away.
	bufferAge = 15 * time.Minute
	// The intervals hold until the hub says otherwise.
	defaultHostInterval      = 15 * time.Second
	defaultContainerInterval = 30 * time.Second
	writeTimeout             = 10 * time.Second
)

type agent struct {
	cfg    Config
	log    *slog.Logger
	host   *hostCollector
	docker *docker
	out    *outbox

	hostInterval      atomic.Int64 // nanoseconds
	containerInterval atomic.Int64
	// liveUntil is when live reading ends, in Unix nanoseconds.
	liveUntil atomic.Int64
	// intervalChanged and containersWake wake the samplers when their
	// interval changes.
	intervalChanged, containersWake chan struct{}
	// updating is set while an update runs, so that a second one waits.
	updating atomic.Bool
	// restart ends Run with ErrRestart, after an update is installed.
	restart chan struct{}
}

// Enroll only enrolls: it gets the agent's credential with the enrollment
// token and keeps it, if it has none yet. The install script does this
// once, so that the service it sets up never sees the token.
func Enroll(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	cfg.Root = cmp.Or(cfg.Root, "/")
	a := &agent{cfg: cfg, log: log, host: newHostCollector(cfg.Root)}
	if s, err := a.loadState(); err == nil && s.Credential != "" {
		// Enrolled already: keep that, unless the hub revoked it and a
		// token to enroll again was given. Someone with root on the machine
		// and a current token wants the agent back.
		ok, err := a.checkCredential(ctx, s.Credential)
		switch {
		case err != nil:
			log.Warn("cannot ask the hub about the credential; keeping it", "err", err)
			return nil
		case ok:
			log.Info("already enrolled", "host", s.HostName)
			return nil
		case cfg.Token == "":
			return fmt.Errorf("%w, and no enrollment token is given to enroll again", errRevoked)
		}
		log.Info("the hub revoked this agent's credential; enrolling again")
		if err := os.Remove(a.statePath()); err != nil {
			return err
		}
	}
	_, err := a.credential(ctx)
	return err
}

// Run reads the machine and reports to the hub until ctx ends. Losing the
// connection is normal: it reconnects, waiting longer after each failure,
// and sends what it read meanwhile.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	cfg.Root = cmp.Or(cfg.Root, "/")
	cfg.BinDir = cmp.Or(cfg.BinDir, filepath.Join(cfg.DataDir, "bin"))
	a := &agent{
		cfg:             cfg,
		log:             log,
		host:            newHostCollector(cfg.Root),
		out:             newOutbox(bufferAge),
		intervalChanged: make(chan struct{}, 1),
		containersWake:  make(chan struct{}, 1),
		restart:         make(chan struct{}),
	}
	a.docker = newDocker(log, a.out.add, a.out.addLatest)
	a.hostInterval.Store(int64(defaultHostInterval))
	a.containerInterval.Store(int64(defaultContainerInterval))
	log.Info("lotse agent starting", "version", version.Version, "hub", cfg.Hub, "root", cfg.Root)

	go a.sampleHost(ctx)
	go a.docker.watch(ctx)
	go a.docker.sampleStats(ctx, func() time.Duration { return a.interval(&a.containerInterval) }, a.containersWake)

	wait := backoff{min: time.Second, max: time.Minute}
	for {
		started := time.Now()
		err := a.connect(ctx)
		if errors.Is(err, update.ErrRestart) {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		// A connection that lasted a while was a good one: start over
		// with short waits.
		if time.Since(started) > time.Minute {
			wait.reset()
		}
		d := wait.next()
		log.Warn("not connected to the hub", "err", err, "retry_in", d.Round(100*time.Millisecond), "buffered", a.out.len())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d):
		}
	}
}

// sampleHost reads the machine at the current interval, connected or not.
func (a *agent) sampleHost(ctx context.Context) {
	// Rates need two readings: a first one, kept to itself, a second
	// before the first one sent, so that one already has them.
	a.host.sample(ctx, time.Now())
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
	}
	for {
		s := a.host.sample(ctx, time.Now())
		a.out.add(protocol.Message{Type: protocol.TypeHost, Host: &s})
		select {
		case <-ctx.Done():
			return
		case <-a.intervalChanged:
		case <-time.After(a.interval(&a.hostInterval)):
		}
	}
}

// interval is the configured interval, or the live one while live.
func (a *agent) interval(configured *atomic.Int64) time.Duration {
	d := time.Duration(configured.Load())
	if time.Now().UnixNano() < a.liveUntil.Load() {
		return min(d, protocol.LiveInterval*time.Second)
	}
	return d
}

// live reads fast for the next seconds, or stops doing so for 0.
func (a *agent) live(seconds int) {
	until := time.Now().Add(time.Duration(seconds) * time.Second).UnixNano()
	if a.liveUntil.Swap(until) < time.Now().UnixNano() && seconds > 0 {
		// It was not live: wake the samplers instead of letting them
		// finish a long wait.
		for _, c := range []chan struct{}{a.intervalChanged, a.containersWake} {
			select {
			case c <- struct{}{}:
			default:
			}
		}
	}
}

func (a *agent) apply(c protocol.Config) {
	if c.HostInterval > 0 {
		d := time.Duration(c.HostInterval) * time.Second
		if a.hostInterval.Swap(int64(d)) != int64(d) {
			a.log.Info("reading the machine every " + d.String())
			select {
			case a.intervalChanged <- struct{}{}:
			default:
			}
		}
	}
	if c.ContainerInterval > 0 {
		d := time.Duration(c.ContainerInterval) * time.Second
		if a.containerInterval.Swap(int64(d)) != int64(d) {
			a.log.Info("reading containers every " + d.String())
		}
	}
}

func (a *agent) connect(ctx context.Context) error {
	credential, err := a.credential(ctx)
	if err != nil {
		return err
	}
	u := strings.TrimSuffix(a.cfg.Hub, "/") + protocol.ConnectPath
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(dialCtx, u, &websocket.DialOptions{
		Subprotocols: protocol.Subprotocols(),
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + credential}},
	})
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			// Revoked: enrolling again on its own would undo that. It takes
			// someone deleting the credential and a new token.
			return fmt.Errorf("%w; to enroll again, delete %s and start with a current enrollment token", errRevoked, a.statePath())
		}
		return err
	}
	defer conn.CloseNow()
	// Hub messages hold whole lists of containers; the default limit of
	// 32 KiB is too small for them.
	conn.SetReadLimit(4 << 20)

	// Reads and writes do not use ctx: one whose context ends drops the
	// connection without a word. When ctx ends, the connection is closed
	// properly instead, so the hub knows the agent left on purpose.
	bg := context.WithoutCancel(ctx)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusGoingAway, "agent stopping")
		case <-done:
		}
	}()

	hello := a.host.hello(ctx)
	hello.Version = version.Version
	hello.DockerVersion = a.docker.Version()
	hello.RolledBackFrom = update.TakeRolledBack(a.cfg.BinDir)
	if err := a.write(bg, conn, protocol.Message{Type: protocol.TypeHello, Hello: &hello}); err != nil {
		return err
	}
	var m protocol.Message
	if err := wsjson.Read(bg, conn, &m); err != nil {
		return err
	}
	if m.Type != protocol.TypeWelcome || m.Welcome == nil {
		return fmt.Errorf("the hub answered hello with %q", m.Type)
	}
	a.apply(m.Welcome.Config)
	a.log.Info("connected to the hub", "hub_version", m.Welcome.HubVersion, "protocol", conn.Subprotocol())
	// The hub learns the containers' identities from the full list.
	a.docker.Refresh()

	readErr := make(chan error, 1)
	go func() {
		for {
			var m protocol.Message
			if err := wsjson.Read(bg, conn, &m); err != nil {
				readErr <- err
				return
			}
			switch {
			case m.Type == protocol.TypeConfig && m.Config != nil:
				a.apply(*m.Config)
			case m.Type == protocol.TypeLive && m.Live != nil:
				a.live(m.Live.Seconds)
			case m.Type == protocol.TypeUpdate && m.Update != nil:
				go a.update(ctx, conn, m.Update.Version)
			default:
				a.log.Debug("ignoring a message of unknown type", "type", m.Type)
			}
		}
	}()

	for {
		msgs := a.out.pending()
		for _, m := range msgs {
			if err := a.write(bg, conn, m); err != nil {
				return err
			}
		}
		a.out.sent(msgs)
		select {
		case <-a.out.ready:
		case err := <-readErr:
			return err
		case <-a.restart:
			conn.Close(websocket.StatusGoingAway, "agent updating")
			return update.ErrRestart
		}
	}
}

// update installs version and has the starter run it (decision 0009). The
// hub only names the version; where it comes from and whether it is
// genuine, the agent decides.
func (a *agent) update(ctx context.Context, conn *websocket.Conn, v string) {
	if !a.updating.CompareAndSwap(false, true) {
		return
	}
	report := func(state string, err error) {
		s := protocol.UpdateStatus{Version: v, State: state}
		if err != nil {
			s.Error = err.Error()
		}
		a.write(context.WithoutCancel(ctx), conn, protocol.Message{Type: protocol.TypeUpdateStatus, UpdateStatus: &s})
	}
	if !update.Started() {
		a.updating.Store(false)
		report("failed", errors.New("the agent does not run under the starter (lotse start agent), which would run the new version"))
		return
	}
	a.log.Info("updating", "to", v)
	report("started", nil)
	in := &update.Installer{Dir: a.cfg.BinDir, ReleaseURL: a.cfg.ReleaseURL, Own: version.Version}
	if err := in.Install(ctx, v); err != nil {
		a.updating.Store(false)
		a.log.Error("update failed", "to", v, "err", err)
		report("failed", err)
		return
	}
	report("installed", nil)
	a.log.Info("update installed; restarting into it", "version", v)
	close(a.restart)
}

func (a *agent) write(ctx context.Context, conn *websocket.Conn, m protocol.Message) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, m)
}

// backoff doubles the wait after each failure, up to max, with jitter so that
// many agents losing the same hub do not all come back at the same moment.
type backoff struct {
	min, max, cur time.Duration
}

func (b *backoff) next() time.Duration {
	if b.cur == 0 {
		b.cur = b.min
	} else {
		b.cur = min(b.cur*2, b.max)
	}
	// Somewhere between half and all of the current wait.
	return b.cur/2 + rand.N(b.cur/2+1)
}

func (b *backoff) reset() { b.cur = 0 }
