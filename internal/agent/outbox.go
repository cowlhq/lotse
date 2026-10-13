package agent

import (
	"sync"
	"time"

	"lotse.sh/internal/protocol"
)

// outbox holds messages until they are sent. While the hub is away it keeps
// the last maxAge of them (decision 0008), in memory only.
type outbox struct {
	maxAge time.Duration
	now    func() time.Time

	mu    sync.Mutex
	items []outItem
	// ready gets a value when there is something to send.
	ready chan struct{}
}

type outItem struct {
	msg protocol.Message
	at  time.Time
}

func newOutbox(maxAge time.Duration) *outbox {
	return &outbox{maxAge: maxAge, now: time.Now, ready: make(chan struct{}, 1)}
}

func (o *outbox) add(m protocol.Message) {
	o.mu.Lock()
	now := o.now()
	o.items = append(o.items, outItem{m, now})
	drop := 0
	for drop < len(o.items) && now.Sub(o.items[drop].at) > o.maxAge {
		drop++
	}
	o.items = o.items[drop:]
	o.mu.Unlock()
	select {
	case o.ready <- struct{}{}:
	default:
	}
}

// pending returns what waits to be sent, oldest first.
func (o *outbox) pending() []protocol.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	msgs := make([]protocol.Message, len(o.items))
	for i, it := range o.items {
		msgs[i] = it.msg
	}
	return msgs
}

// sent removes msgs, which pending returned and which have gone out. Some
// of them may have been dropped for age meanwhile, always the oldest.
func (o *outbox) sent(msgs []protocol.Message) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, m := range msgs {
		if len(o.items) > 0 && sameMessage(o.items[0].msg, m) {
			o.items = o.items[1:]
		}
	}
}

// Messages are compared by identity: every message the outbox holds has a
// payload of its own.
func sameMessage(a, b protocol.Message) bool {
	return a.Type == b.Type && a.Host == b.Host
}

func (o *outbox) len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.items)
}
