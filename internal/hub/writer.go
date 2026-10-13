package hub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"lotse.sh/internal/store"
)

// writer gathers what agents send and writes it once a second, in one
// transaction, instead of one write per message.
type writer struct {
	db  *store.Store
	log *slog.Logger

	mu    sync.Mutex
	batch store.Batch
}

func (w *writer) add(f func(b *store.Batch)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f(&w.batch)
}

// run writes every second until ctx ends, and once more after that.
func (w *writer) run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flush(context.WithoutCancel(ctx))
			return
		case <-t.C:
			w.flush(ctx)
		}
	}
}

func (w *writer) flush(ctx context.Context) {
	w.mu.Lock()
	b := w.batch
	w.batch = store.Batch{}
	w.mu.Unlock()
	if b.Empty() {
		return
	}
	if err := w.db.Write(ctx, b); err != nil {
		// Lost, but the next second's batch may well succeed; holding on to
		// a batch that fails would only grow it.
		w.log.Error("cannot save what agents sent", "err", err,
			"host_samples", len(b.HostSamples), "events", len(b.Events))
	}
}
