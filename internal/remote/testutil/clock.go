// Package testutil provides deterministic fakes for the remote server tests:
// a settable clock, an in-memory Backend, and an in-memory Devices store.
package testutil

import (
	"sync"
	"time"
)

// FakeClock is a settable Clock; time only moves through Advance/Set.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock starts at a fixed instant.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
