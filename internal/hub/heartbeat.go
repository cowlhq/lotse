package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// Heartbeats: a job, such as a backup, pings /ping/<token> when it ran, or
// /ping/<token>/fail when it failed. The check is down when the last ping
// is older than the period and the grace, or when the job said it failed.

const heartbeatEvery = 30 * time.Second

// pingGate keeps a heartbeat that is pinged in a loop, by a job gone wrong,
// from writing to the database each time: a few pings at once, then one a
// second (pingBurst, pingRate).
type pingGate struct {
	mu   sync.Mutex
	seen map[string]pingAllowance
}

type pingAllowance struct {
	left float64
	at   time.Time
}

const (
	pingBurst = 5
	pingRate  = 1 // per second
)

// allow says whether a ping for a token may go on now.
func (g *pingGate) allow(token string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seen == nil {
		g.seen = map[string]pingAllowance{}
	}
	a, ok := g.seen[token]
	if !ok {
		a = pingAllowance{left: pingBurst, at: now}
	}
	a.left = min(pingBurst, a.left+now.Sub(a.at).Seconds()*pingRate)
	a.at = now
	allowed := a.left >= 1
	if allowed {
		a.left--
	}
	g.seen[token] = a
	// Tokens that have their whole allowance back are forgotten.
	if len(g.seen) > 1000 {
		for k, x := range g.seen {
			if x.left+now.Sub(x.at).Seconds()*pingRate >= pingBurst {
				delete(g.seen, k)
			}
		}
	}
	return allowed
}

// ping takes a heartbeat's ping, by GET, POST or HEAD, without a login:
// the token in the address is the secret.
func (h *hub) ping(w http.ResponseWriter, r *http.Request) {
	token, failed := strings.CutSuffix(r.PathValue("token"), "/fail")
	if rest := r.PathValue("rest"); rest == "fail" {
		failed = true
	}
	if !h.pings.allow(token, time.Now()) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many pings; one a second is enough", http.StatusTooManyRequests)
		return
	}
	c, err := h.db.Ping(r.Context(), auth.HashToken(token), time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "no such heartbeat", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "the hub cannot take this ping", http.StatusInternalServerError)
		return
	}
	if failed {
		h.setCheckState(r.Context(), c.ID, "down", "the job said it failed")
	} else {
		h.setCheckState(r.Context(), c.ID, "up", "")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ok\n"))
}

// heartbeats looks for heartbeats that stopped, until ctx ends.
func (h *hub) heartbeats(ctx context.Context) {
	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		checks, err := h.db.Checks(ctx, 0)
		if err != nil {
			continue
		}
		now := time.Now()
		for _, c := range checks {
			if c.Type != "heartbeat" {
				continue
			}
			if state, reason := heartbeatState(c, now); state != "" {
				h.setCheckState(ctx, c.ID, state, reason)
			}
		}
	}
}

// heartbeatState is a heartbeat's state now, or "" to leave it as it is
// (up after a ping, or down after a failure, until the next ping).
func heartbeatState(c store.Check, now time.Time) (state, reason string) {
	var s probe.Heartbeat
	json.Unmarshal(c.Spec, &s)
	period, _ := time.ParseDuration(s.Period)
	grace, _ := time.ParseDuration(s.Grace)
	last := c.CreatedAt
	if c.LastPing != nil {
		last = *c.LastPing
	}
	if now.Sub(time.Unix(last, 0)) > period+grace {
		if c.LastPing == nil {
			return "down", "no ping since it was created"
		}
		// How long, not since when: the hub does not know the reader's time
		// zone, and a time in UTC reads wrong to most.
		return "down", fmt.Sprintf("no ping for %s, expected every %s", roughly(now.Sub(time.Unix(last, 0))), s.Period)
	}
	return "", ""
}

// roughly writes a duration to the minute, in at most two parts: 25m,
// 3h 10m, 2d 4h.
func roughly(d time.Duration) string {
	d = d.Round(time.Minute)
	two := func(a int, au string, b int, bu string) string {
		if b == 0 {
			return fmt.Sprintf("%d%s", a, au)
		}
		return fmt.Sprintf("%d%s %d%s", a, au, b, bu)
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return two(int(d/time.Hour), "h", int(d%time.Hour/time.Minute), "m")
	default:
		return two(int(d/(24*time.Hour)), "d", int(d%(24*time.Hour)/time.Hour), "h")
	}
}
