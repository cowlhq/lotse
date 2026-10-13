package auth

import (
	"sync"
	"time"
)

// Limiter slows down guessing: after Max failures within Window for the
// same key (an address, an email), that key is refused until the window
// has passed. A success clears the key.
type Limiter struct {
	Max    int
	Window time.Duration
	Now    func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, Now: time.Now, failures: map[string][]time.Time{}}
}

// Allowed says whether all keys may try now.
func (l *Limiter) Allowed(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		if len(l.recent(k)) >= l.Max {
			return false
		}
	}
	return true
}

// Failed records a failure for each key.
func (l *Limiter) Failed(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		l.failures[k] = append(l.recent(k), l.Now())
	}
}

// Succeeded forgets the failures of each key.
func (l *Limiter) Succeeded(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.failures, k)
	}
}

// recent drops failures older than the window; l.mu is held.
func (l *Limiter) recent(k string) []time.Time {
	cutoff := l.Now().Add(-l.Window)
	f := l.failures[k]
	i := 0
	for i < len(f) && f[i].Before(cutoff) {
		i++
	}
	if i == len(f) {
		delete(l.failures, k)
		return nil
	}
	l.failures[k] = f[i:]
	return f[i:]
}
