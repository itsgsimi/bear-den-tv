// Fake: a manually advanced clock for deterministic tests.

package clock

import (
	"sort"
	"sync"
	"time"
)

// Fake is a manually advanced clock. Timers fire in deadline order during
// Advance; AfterFunc callbacks run synchronously on the advancing goroutine
// (outside the fake's lock), so a test observes every effect of an Advance
// call once it returns. Fake is safe for concurrent use.
type Fake struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	seq    uint64
}

// NewFake returns a fake clock starting at start.
func NewFake(start time.Time) *Fake { return &Fake{now: start} }

// Now implements Clock.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Since implements Clock.
func (f *Fake) Since(t time.Time) time.Duration { return f.Now().Sub(t) }

// After implements Clock.
func (f *Fake) After(d time.Duration) <-chan time.Time { return f.NewTimer(d).C() }

// NewTimer implements Clock.
func (f *Fake) NewTimer(d time.Duration) Timer {
	return f.add(d, nil)
}

// AfterFunc implements Clock.
func (f *Fake) AfterFunc(d time.Duration, fn func()) Timer {
	return f.add(d, fn)
}

func (f *Fake) add(d time.Duration, fn func()) *fakeTimer {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	t := &fakeTimer{clock: f, when: f.now.Add(d), fn: fn, seq: f.seq, active: true}
	if fn == nil {
		t.ch = make(chan time.Time, 1)
	}
	f.timers = append(f.timers, t)
	return t
}

// Advance moves the clock forward by d, firing every timer whose deadline is
// reached, in deadline order. Timers armed by callbacks during the advance
// fire too when their deadline falls inside the window.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	target := f.now.Add(d)
	f.mu.Unlock()
	for {
		f.mu.Lock()
		var next *fakeTimer
		for _, t := range f.timers {
			if !t.active || t.when.After(target) {
				continue
			}
			if next == nil || t.when.Before(next.when) || (t.when.Equal(next.when) && t.seq < next.seq) {
				next = t
			}
		}
		if next == nil {
			f.now = target
			f.mu.Unlock()
			return
		}
		if next.when.After(f.now) {
			f.now = next.when
		}
		next.active = false
		f.remove(next)
		if next.fn == nil {
			// Sent under the lock, so Stop and Reset (which drain under the
			// same lock) never leave a stale tick behind.
			select {
			case next.ch <- next.when:
			default:
			}
		}
		f.mu.Unlock()
		if next.fn != nil {
			next.fn()
		}
	}
}

// Pending reports how many timers are armed; tests use it to assert that a
// lease or retry was scheduled or cancelled.
func (f *Fake) Pending() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.timers)
}

// NextDeadline returns the earliest armed deadline and whether any timer exists.
func (f *Fake) NextDeadline() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.timers) == 0 {
		return time.Time{}, false
	}
	ts := append([]*fakeTimer(nil), f.timers...)
	sort.Slice(ts, func(i, j int) bool { return ts[i].when.Before(ts[j].when) })
	return ts[0].when, true
}

func (f *Fake) remove(t *fakeTimer) {
	for i, x := range f.timers {
		if x == t {
			f.timers = append(f.timers[:i], f.timers[i+1:]...)
			return
		}
	}
}

type fakeTimer struct {
	clock  *Fake
	when   time.Time
	fn     func()
	ch     chan time.Time
	seq    uint64
	active bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

// Stop and Reset drop a tick that fired but was never received, like
// time.Timer since Go 1.23: after they return, no stale value arrives.
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := t.active
	t.active = false
	t.clock.remove(t)
	t.drain()
	return was
}

func (t *fakeTimer) drain() {
	if t.ch == nil {
		return
	}
	select {
	case <-t.ch:
	default:
	}
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := t.active
	t.clock.remove(t)
	t.drain()
	t.clock.seq++
	t.seq = t.clock.seq
	t.when = t.clock.now.Add(d)
	t.active = true
	t.clock.timers = append(t.clock.timers, t)
	return was
}
