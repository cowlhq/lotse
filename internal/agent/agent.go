// Package agent watches a machine and reports to the hub. It opens one
// connection to the hub, listens on no port, and runs nothing it is told to.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/version"
)

// Config is how the agent reaches the hub.
type Config struct {
	// Hub is the hub's address, such as https://lotse.example.com.
	Hub string
	// Token proves the agent to the hub.
	Token string
	// Insecure allows plain HTTP to a hub that is not on this machine.
	Insecure bool
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
	if c.Token == "" {
		return errors.New("no token given")
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

// Run keeps the agent connected to the hub until ctx ends. Losing the
// connection is normal: it reconnects, waiting longer after each failure.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	log.Info("lotse agent starting", "version", version.Version, "hub", cfg.Hub)
	wait := backoff{min: time.Second, max: time.Minute}
	for {
		started := time.Now()
		err := connect(ctx, cfg, log)
		if ctx.Err() != nil {
			return nil
		}
		// A connection that lasted a while was a good one: start over
		// with short waits.
		if time.Since(started) > time.Minute {
			wait.reset()
		}
		d := wait.next()
		log.Warn("not connected to the hub", "err", err, "retry_in", d.Round(100*time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d):
		}
	}
}

func connect(ctx context.Context, cfg Config, log *slog.Logger) error {
	u := strings.TrimSuffix(cfg.Hub, "/") + protocol.ConnectPath
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(dialCtx, u, &websocket.DialOptions{
		Subprotocols: protocol.Subprotocols(),
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + cfg.Token}},
	})
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			return errors.New("the hub does not accept this agent's token")
		}
		return err
	}
	defer conn.CloseNow()

	hostname, _ := os.Hostname()
	hello := protocol.Message{Type: protocol.TypeHello, Hello: &protocol.Hello{
		Version:   version.Version,
		Hostname:  hostname,
		MachineID: machineID(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}}
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		return err
	}

	// A read whose context ends drops the connection without a word, so
	// reads do not use ctx; when it ends, the connection is closed properly
	// and the hub knows the agent left on purpose.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusGoingAway, "agent stopping")
		case <-done:
		}
	}()
	for {
		var m protocol.Message
		if err := wsjson.Read(context.WithoutCancel(ctx), conn, &m); err != nil {
			return err
		}
		switch m.Type {
		case protocol.TypeWelcome:
			log.Info("connected to the hub", "hub_version", m.Welcome.HubVersion, "protocol", conn.Subprotocol())
		default:
			log.Debug("ignoring a message of unknown type", "type", m.Type)
		}
	}
}

// machineID identifies this machine across reinstalls of the agent. In a
// container, /etc/machine-id is the host's when it is mounted in.
func machineID() string {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
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
