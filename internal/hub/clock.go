package hub

import "sync"

// Clocks: an agent stamps its readings with its machine's time. A machine
// whose clock is off puts its readings at the wrong time, and makes the hub
// think it went silent. The hub tells by how much from when readings
// arrive, and moves them to its own time.

const (
	// clockReadings is how many recent readings the estimate looks at.
	clockReadings = 5
	// clockTolerance is how far, in seconds, a clock may be off before
	// readings are moved: a reading takes a moment to arrive.
	clockTolerance = 30
	// clockSpread is how far the recent delays may differ and still say
	// the clock is off. They differ more when readings arrive late, sent
	// from the agent's buffer after a reconnect.
	clockSpread = 10
)

// clockEstimate follows one connection's readings.
type clockEstimate struct {
	delays [clockReadings]int64
	n      int
}

// observe notes when a reading stamped ts arrived. Once the recent delays
// agree, it says how far the agent's clock is behind the hub's, in
// seconds: negative when ahead, 0 within the tolerance.
func (c *clockEstimate) observe(ts, now int64) (offset int64, settled bool) {
	c.delays[c.n%clockReadings] = now - ts
	c.n++
	if c.n < clockReadings {
		return 0, false
	}
	lo, hi := c.delays[0], c.delays[0]
	for _, d := range c.delays {
		lo, hi = min(lo, d), max(hi, d)
	}
	if hi-lo > clockSpread {
		return 0, false
	}
	// The delay nearest zero: the way to the hub only adds to it.
	switch {
	case lo > 0:
		offset = lo
	case hi < 0:
		offset = hi
	}
	if offset >= -clockTolerance && offset <= clockTolerance {
		offset = 0
	}
	return offset, true
}

// clocks are the offsets of the agents whose clocks are off, by host.
type clocks struct {
	mu     sync.Mutex
	offset map[int64]int64
}

func (c *clocks) get(hostID int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offset[hostID]
}

// set keeps a host's offset and says whether it changed.
func (c *clocks) set(hostID, offset int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.offset[hostID] == offset {
		return false
	}
	if c.offset == nil {
		c.offset = map[int64]int64{}
	}
	if offset == 0 {
		delete(c.offset, hostID)
	} else {
		c.offset[hostID] = offset
	}
	return true
}
