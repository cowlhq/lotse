package hub

import (
	"context"
	"time"
)

// rawRetention is how long readings are kept as they came. Milestone 9
// keeps averages for longer; until then, older readings are deleted.
const rawRetention = 48 * time.Hour

const retentionEvery = 10 * time.Minute

// retain deletes readings older than rawRetention, every few minutes, until
// ctx ends.
func (h *hub) retain(ctx context.Context) {
	t := time.NewTicker(retentionEvery)
	defer t.Stop()
	for {
		if err := h.db.DeleteExpiredSessions(ctx, time.Now().Unix()); err != nil && ctx.Err() == nil {
			h.log.Error("cannot delete expired sessions", "err", err)
		}
		n, err := h.db.DeleteSamplesBefore(ctx, time.Now().Add(-rawRetention).Unix())
		if err != nil && ctx.Err() == nil {
			h.log.Error("cannot delete old readings", "err", err)
		} else if n > 0 {
			h.log.Debug("deleted old readings", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
