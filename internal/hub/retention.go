package hub

import (
	"context"
	"encoding/json"
	"time"

	"lotse.sh/internal/config"
	"lotse.sh/internal/store"
)

// Retention (milestone 9): readings are kept as they came for
// retention.raw, as 5-minute averages for retention.5m, and as hourly ones
// for retention.1h, which events and alerts are kept for as well.

const retentionEvery = 10 * time.Minute

// rollupLag is how old readings must be before they are averaged: an agent
// sends what it kept while the hub was away for up to 15 minutes late.
const rollupLag = 20 * time.Minute

// retention are the retention settings, as they are set globally.
func (h *hub) retention(ctx context.Context) map[string]time.Duration {
	out := map[string]time.Duration{}
	set, _ := h.db.Settings(ctx, []store.Subject{{Kind: "global", ID: store.GlobalScope}})
	for res, key := range map[string]string{store.Raw: "retention.raw", store.FiveMinutes: "retention.5m", store.Hour: "retention.1h"} {
		k, _ := config.Lookup(key)
		v := k.Default
		if len(set) > 0 {
			if x, ok := set[0][key]; ok {
				v = x
			}
		}
		d, err := config.Duration(json.RawMessage(v))
		if err != nil {
			d, _ = config.Duration(k.Default)
		}
		out[res] = d
	}
	return out
}

// retain averages and deletes, every few minutes, until ctx ends.
func (h *hub) retain(ctx context.Context) {
	t := time.NewTicker(retentionEvery)
	defer t.Stop()
	for {
		h.retainOnce(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (h *hub) retainOnce(ctx context.Context, now time.Time) {
	if err := h.db.DeleteExpiredSessions(ctx, now.Unix()); err != nil && ctx.Err() == nil {
		h.log.Error("cannot delete expired sessions", "err", err)
	}
	if err := h.db.Rollup(ctx, now.Add(-rollupLag).Unix()); err != nil && ctx.Err() == nil {
		h.log.Error("cannot average readings", "err", err)
		// Nothing raw is deleted that is not averaged yet.
		return
	}
	keep := h.retention(ctx)
	for _, res := range store.Resolutions {
		n, err := h.db.DeleteBefore(ctx, res, now.Add(-keep[res]).Unix())
		if err != nil && ctx.Err() == nil {
			h.log.Error("cannot delete old readings", "resolution", res, "err", err)
		} else if n > 0 {
			h.log.Debug("deleted old readings", "resolution", res, "count", n)
		}
	}
	if _, err := h.db.DeleteHistoryBefore(ctx, now.Add(-keep[store.Hour]).Unix()); err != nil && ctx.Err() == nil {
		h.log.Error("cannot delete old events and alerts", "err", err)
	}
}
