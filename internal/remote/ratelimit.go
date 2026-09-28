// Per-key token-bucket rate limiting for the remote server.

package remote

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// keyedLimiter holds one token bucket per key (source address, device id).
// Idle entries are pruned once the map grows past pruneAbove.
type keyedLimiter struct {
	clock Clock
	limit rate.Limit
	burst int

	mu      sync.Mutex
	entries map[string]*limiterEntry
}

type limiterEntry struct {
	lim  *rate.Limiter
	last time.Time
}

const (
	pruneAbove = 1024
	pruneIdle  = 10 * time.Minute
)

func newKeyedLimiter(clock Clock, limit rate.Limit, burst int) *keyedLimiter {
	return &keyedLimiter{clock: clock, limit: limit, burst: burst, entries: map[string]*limiterEntry{}}
}

// allow consumes one token for key and reports whether it was available.
func (k *keyedLimiter) allow(key string) bool {
	now := k.clock.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.entries[key]
	if !ok {
		if len(k.entries) >= pruneAbove {
			for id, old := range k.entries {
				if now.Sub(old.last) > pruneIdle {
					delete(k.entries, id)
				}
			}
		}
		e = &limiterEntry{lim: rate.NewLimiter(k.limit, k.burst)}
		k.entries[key] = e
	}
	e.last = now
	return e.lim.AllowN(now, 1)
}
