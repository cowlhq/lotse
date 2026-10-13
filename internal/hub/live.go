package hub

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
)

// The browser's live connection (decision 0003): one WebSocket per tab at
// /api/live. The page says what it shows by subscribing to topics:
//
//	hosts     every host's readings and state, for the overview
//	host:<id> one host's readings, its containers' readings and changes
//
// The hub sends what happens to those, as LiveMessage. While anyone
// watches a host, its agent reads every few seconds.

// LiveMessage is what the hub sends to the browser.
type LiveMessage struct {
	Type string `json:"type"` // host, host_state, container_stats, containers
	// HostID is the host the message is about.
	HostID     int64                  `json:"host_id"`
	Reading    *api.HostReading       `json:"reading,omitempty"`
	Online     *bool                  `json:"online,omitempty"`
	Containers []LiveContainerReading `json:"containers,omitempty"`
}

type LiveContainerReading struct {
	ContainerID int64                `json:"container_id"`
	Reading     api.ContainerReading `json:"reading"`
}

const (
	// liveRenew is how often the hub tells an agent again to stay live;
	// liveFor is how long each time lasts.
	liveRenew = 20 * time.Second
	liveFor   = 60
	// A browser that cannot keep up loses its connection rather than
	// holding up the hub; it reconnects.
	liveBuffer = 64
)

type liveClient struct {
	send   chan LiveMessage
	topics map[string]bool
	// gone is closed when the client is done, by either side.
	gone     chan struct{}
	goneOnce sync.Once
}

func newLiveClient() *liveClient {
	return &liveClient{send: make(chan LiveMessage, liveBuffer), gone: make(chan struct{})}
}

func (c *liveClient) close() { c.goneOnce.Do(func() { close(c.gone) }) }

func (c *liveClient) isGone() bool {
	select {
	case <-c.gone:
		return true
	default:
		return false
	}
}

// live keeps the browsers' subscriptions and the hosts being watched.
type live struct {
	mu      sync.Mutex
	clients map[*liveClient]bool
	// watched counts the clients watching each host.
	watched map[int64]int
}

func (l *live) publish(m LiveMessage) {
	topic := "host:" + strconv.FormatInt(m.HostID, 10)
	l.mu.Lock()
	defer l.mu.Unlock()
	for c := range l.clients {
		if !c.topics[topic] && !(c.topics["hosts"] && m.Type != "container_stats") {
			continue
		}
		select {
		case c.send <- m:
		default:
			c.close()
			l.drop(c)
		}
	}
}

// subscribe replaces a client's topics. It returns the hosts that are
// watched now and were not before, and those no longer watched at all.
func (l *live) subscribe(c *liveClient, topics []string) (started, stopped []int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c.isGone() {
		return nil, nil
	}
	if l.clients == nil {
		l.clients, l.watched = map[*liveClient]bool{}, map[int64]int{}
	}
	l.clients[c] = true
	before := l.unwatch(c)
	c.topics = map[string]bool{}
	for _, t := range topics {
		c.topics[t] = true
		if id, ok := hostTopic(t); ok {
			if l.watched[id] == 0 && !slices.Contains(before, id) {
				started = append(started, id)
			}
			l.watched[id]++
		}
	}
	for _, id := range before {
		if l.watched[id] == 0 {
			stopped = append(stopped, id)
		}
	}
	return started, stopped
}

// remove forgets a client and returns the hosts no longer watched.
func (l *live) remove(c *liveClient) []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.clients[c] {
		return nil
	}
	return l.drop(c)
}

// drop forgets a client; l.mu is held.
func (l *live) drop(c *liveClient) []int64 {
	delete(l.clients, c)
	return l.unwatch(c)
}

// unwatch takes the client's hosts off the watched ones and returns those
// that no one watches any more.
func (l *live) unwatch(c *liveClient) (unwatched []int64) {
	for t := range c.topics {
		if id, ok := hostTopic(t); ok {
			if l.watched[id]--; l.watched[id] <= 0 {
				delete(l.watched, id)
				unwatched = append(unwatched, id)
			}
		}
	}
	c.topics = nil
	return unwatched
}

func (l *live) watchedHosts() []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]int64, 0, len(l.watched))
	for id := range l.watched {
		ids = append(ids, id)
	}
	return ids
}

func hostTopic(t string) (int64, bool) {
	s, ok := strings.CutPrefix(t, "host:")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

// connectLive is the browser's WebSocket.
func (h *hub) connectLive(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.originPatterns()})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := context.WithoutCancel(r.Context())
	c := newLiveClient()
	defer func() {
		for _, id := range h.live.remove(c) {
			h.sendLive(ctx, id, 0)
		}
	}()
	defer c.close()

	go func() {
		defer c.close()
		for {
			var req struct {
				Subscribe []string `json:"subscribe"`
			}
			if err := wsjson.Read(ctx, conn, &req); err != nil {
				return
			}
			started, stopped := h.live.subscribe(c, req.Subscribe)
			for _, id := range started {
				h.sendLive(ctx, id, liveFor)
			}
			for _, id := range stopped {
				h.sendLive(ctx, id, 0)
			}
		}
	}()
	for {
		select {
		case m := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, conn, m)
			cancel()
			if err != nil {
				return
			}
		case <-c.gone:
			// The browser left, or fell behind and was dropped.
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

const writeTimeout = 10 * time.Second

// keepLive tells the agents of watched hosts, again and again, to stay
// live, until ctx ends.
func (h *hub) keepLive(ctx context.Context) {
	t := time.NewTicker(liveRenew)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, id := range h.live.watchedHosts() {
				h.sendLive(ctx, id, liveFor)
			}
		}
	}
}

// sendLive tells a host's agents to read fast for the next seconds, or to
// stop for 0.
func (h *hub) sendLive(ctx context.Context, hostID int64, seconds int) {
	m := protocol.Message{Type: protocol.TypeLive, Live: &protocol.Live{Seconds: seconds}}
	for _, conn := range h.agents.of(hostID) {
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		wsjson.Write(wctx, conn, m)
		cancel()
	}
}

// originPatterns are the hosts the browser's WebSocket may be opened from
// besides the hub's own: the address people use, which in development is
// the web dev server.
func (h *hub) originPatterns() []string {
	u, err := url.Parse(h.URL())
	if err != nil || u.Host == "" {
		return nil
	}
	return []string{u.Host}
}
