package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// Heartbeats: a job, such as a backup, pings /ping/<token> when it ran, or
// /ping/<token>/fail when it failed. The check is down when the last ping
// is older than the period and the grace, or when the job said it failed.

const heartbeatEvery = 30 * time.Second

// ping takes a heartbeat's ping, by GET, POST or HEAD, without a login:
// the token in the address is the secret.
func (h *hub) ping(w http.ResponseWriter, r *http.Request) {
	token, failed := strings.CutSuffix(r.PathValue("token"), "/fail")
	if rest := r.PathValue("rest"); rest == "fail" {
		failed = true
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
		return "down", "no ping since " + time.Unix(last, 0).UTC().Format(time.RFC3339)
	}
	return "", ""
}
