package agent

import (
	"slices"
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

func (o *outbox) add(m protocol.Message) { o.put(m, false) }

// addLatest adds m and drops waiting messages of its type: for a full list
// of containers, only the newest matters.
func (o *outbox) addLatest(m protocol.Message) { o.put(m, true) }

func (o *outbox) put(m protocol.Message, latest bool) {
	o.mu.Lock()
	if latest {
		o.items = slices.DeleteFunc(o.items, func(it outItem) bool { return it.msg.Type == m.Type })
	}
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
// of them may have been dropped meanwhile, for age or by addLatest.
func (o *outbox) sent(msgs []protocol.Message) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, m := range msgs {
		if i := slices.IndexFunc(o.items, func(it outItem) bool { return sameMessage(it.msg, m) }); i >= 0 {
			o.items = slices.Delete(o.items, i, i+1)
		}
	}
}

// Messages are compared by identity: every message the outbox holds has a
// payload of its own.
func sameMessage(a, b protocol.Message) bool {
	return a.Type == b.Type && a.Host == b.Host && a.Containers == b.Containers &&
		a.ContainerEvent == b.ContainerEvent && a.ContainerStats == b.ContainerStats
}

func (o *outbox) len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.items)
}
