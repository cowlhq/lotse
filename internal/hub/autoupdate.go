package hub

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"lotse.sh/internal/config"
)

// Automatic updates (decision 0009): agents whose effective
// agent.auto_update is on update in their agent.update_window. In each
// environment one host goes first; the others follow once it has run the
// new version for canaryTime. A version that fails on the first host gets
// no further.
const (
	autoUpdateEvery = 5 * time.Minute
	canaryTime      = 10 * time.Minute
	// askAgainAfter keeps a host that was asked from being asked every
	// round while its update runs.
	askAgainAfter = 30 * time.Minute
)

// candidate is a connected host that might update.
type candidate struct {
	hostID, envID int64
	version       string
	auto          bool
	inWindow      bool
}

// rollouts remembers, per environment, which host went first with which
// version, and when hosts were last asked.
type rollouts struct {
	first map[int64]firstHost // by environment
	asked map[int64]time.Time // by host
}

type firstHost struct {
	version string
	hostID  int64
	since   time.Time
}

// plan says which hosts to ask to update to latest now.
func (r *rollouts) plan(now time.Time, latest string, cands []candidate) []int64 {
	if r.first == nil {
		r.first, r.asked = map[int64]firstHost{}, map[int64]time.Time{}
	}
	var ask []int64
	for _, c := range cands {
		if !c.auto || !c.inWindow || !newer(latest, c.version) || now.Sub(r.asked[c.hostID]) < askAgainAfter {
			continue
		}
		f, ok := r.first[c.envID]
		switch {
		case !ok || f.version != latest:
			// The first of this environment for this version.
			r.first[c.envID] = firstHost{version: latest, hostID: c.hostID, since: now}
		case f.hostID == c.hostID:
			// Going first again: its last try did not take.
		case !r.firstDone(f, latest, cands) || now.Sub(f.since) < canaryTime:
			continue
		}
		r.asked[c.hostID] = now
		ask = append(ask, c.hostID)
	}
	return ask
}

// firstDone says whether the environment's first host runs the version.
func (r *rollouts) firstDone(f firstHost, latest string, cands []candidate) bool {
	for _, c := range cands {
		if c.hostID == f.hostID {
			return c.version == latest
		}
	}
	return false
}

// autoUpdate asks agents to update, every few minutes, until ctx ends.
func (h *hub) autoUpdate(ctx context.Context) {
	var r rollouts
	t := time.NewTicker(autoUpdateEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		latest, _, _ := h.releases.get()
		if latest == nil {
			continue
		}
		cands := h.candidates(ctx, time.Now())
		for _, id := range r.plan(time.Now(), latest.Version, cands) {
			h.log.Info("updating automatically", "host_id", id, "to", latest.Version)
			h.askUpdate(ctx, id, latest.Version)
		}
	}
}

func (h *hub) candidates(ctx context.Context, now time.Time) []candidate {
	seen := map[int64]bool{}
	var cands []candidate
	for _, a := range h.agents.all() {
		if seen[a.hostID] {
			continue
		}
		seen[a.hostID] = true
		host, err := h.db.Host(ctx, a.hostID)
		if err != nil {
			continue
		}
		values, err := h.effective(ctx, "host:"+strconv.FormatInt(host.ID, 10))
		if err != nil {
			continue
		}
		c := candidate{hostID: host.ID, version: host.AgentVersion}
		if host.EnvironmentID != nil {
			c.envID = *host.EnvironmentID
		}
		for _, v := range values {
			switch v.Key {
			case "agent.auto_update":
				json.Unmarshal(v.Value, &c.auto)
			case "agent.update_window":
				var w string
				json.Unmarshal(v.Value, &w)
				if start, end, err := config.ParseWindow(w); err == nil {
					c.inWindow = config.InWindow(now, start, end)
				}
			}
		}
		cands = append(cands, c)
	}
	return cands
}
