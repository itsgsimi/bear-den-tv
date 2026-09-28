// Package clock abstracts monotonic time so every timeout, lease, backoff, and
// observation window in the coordinator can be driven deterministically in
// tests. Production code uses Real; tests use Fake and advance it explicitly.
package clock

import "time"

// Timer is a one-shot timer. C fires once unless the timer is stopped or reset.
type Timer interface {
	// C delivers the fire time. A timer created by AfterFunc never sends on C.
	C() <-chan time.Time
	// Stop prevents the timer from firing; it reports whether the call stopped it.
	Stop() bool
	// Reset re-arms the timer for d from now; it reports whether it was active.
	Reset(d time.Duration) bool
}

// Clock is the time source used by every package that measures durations.
type Clock interface {
	// Now returns the current time. Real clocks include a monotonic reading;
	// only differences between two Now() values are meaningful for ordering.
	Now() time.Time
	// Since is Now().Sub(t).
	Since(t time.Time) time.Duration
	// NewTimer returns a timer that fires on C after d.
	NewTimer(d time.Duration) Timer
	// AfterFunc runs f in its own goroutine after d unless stopped first.
	AfterFunc(d time.Duration, f func()) Timer
	// After is NewTimer(d).C().
	After(d time.Duration) <-chan time.Time
}

// Real is the wall/monotonic clock backed by package time.
type Real struct{}

// Now implements Clock.
func (Real) Now() time.Time { return time.Now() }

// Since implements Clock.
func (Real) Since(t time.Time) time.Duration { return time.Since(t) }

// NewTimer implements Clock.
func (Real) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

// AfterFunc implements Clock.
func (Real) AfterFunc(d time.Duration, f func()) Timer { return realTimer{time.AfterFunc(d, f)} }

// After implements Clock.
func (Real) After(d time.Duration) <-chan time.Time { return time.After(d) }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
