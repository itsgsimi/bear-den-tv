// Hold leases: press-and-hold actions that repeat taps until stopped or the
// lease expires (spec contracts/actions.md).

package actions

import (
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// Hold lease states reported to phones (contracts/actions.md#holds).
const (
	HoldActive    = "active"
	HoldExpired   = "expired"
	HoldCancelled = "cancelled"
	HoldBusy      = "busy"
	HoldRejected  = "rejected"
)

// HoldStatus is the lease state returned for start/renew.
type HoldStatus struct {
	HoldID string
	State  string
	Reason string
}

// TapFunc delivers one repetition of a held action. A non-nil error ends the
// lease (for example a stale epoch after a target change) with reason.
type TapFunc func(deviceID, action string, epoch int64) (reason string, err error)

type lease struct {
	id        string
	deviceID  string
	action    string
	epoch     int64
	lastRenew time.Time
	stop      chan struct{}
	stopped   bool
}

// Holds runs at most one server-side hold lease at a time. The client sends the
// initial press itself as a normal action; the lease only produces repeats,
// starting RepeatDelayMs after Start at RepeatHz, so a short press moves once,
// never twice. The lease expires HoldExpiryMs after the last renew.
// Reconnecting never resumes a hold.
type Holds struct {
	clock    clock.Clock
	limits   contract.Limits
	tap      TapFunc
	onChange func()

	mu     sync.Mutex
	active *lease
	ended  map[string]HoldStatus // recent terminal states by hold id
	order  []string
}

// NewHolds builds a lease manager. onChange (may be nil) runs, without the lock
// held, whenever the active lease starts or ends so the state can republish.
func NewHolds(c clock.Clock, limits contract.Limits, tap TapFunc, onChange func()) *Holds {
	if c == nil {
		c = clock.Real{}
	}
	if onChange == nil {
		onChange = func() {}
	}
	return &Holds{clock: c, limits: limits, tap: tap, onChange: onChange, ended: map[string]HoldStatus{}}
}

// Start opens a lease for deviceID. A different device holding ⇒ busy; the
// same device starting a new hold replaces its previous one.
func (h *Holds) Start(deviceID, holdID, action string, epoch int64) HoldStatus {
	if !contract.IsNav(action) {
		return HoldStatus{HoldID: holdID, State: HoldRejected, Reason: "not_holdable"}
	}
	h.mu.Lock()
	if cur := h.active; cur != nil {
		if cur.deviceID != deviceID {
			h.mu.Unlock()
			return HoldStatus{HoldID: holdID, State: HoldBusy, Reason: "another device is holding"}
		}
		h.endLocked(cur, HoldCancelled, "replaced")
	}
	l := &lease{id: holdID, deviceID: deviceID, action: action, epoch: epoch, lastRenew: h.clock.Now(), stop: make(chan struct{})}
	h.active = l
	h.mu.Unlock()
	h.onChange()
	go h.run(l)
	return HoldStatus{HoldID: holdID, State: HoldActive}
}

// Renew extends the caller's lease; an unknown or ended lease reports its
// terminal state so the phone stops renewing.
func (h *Holds) Renew(deviceID, holdID string) HoldStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.active; l != nil && l.id == holdID && l.deviceID == deviceID {
		if h.clock.Since(l.lastRenew) > h.expiry() {
			h.endLocked(l, HoldExpired, "renew_late")
			go h.onChange()
			return h.ended[holdID]
		}
		l.lastRenew = h.clock.Now()
		return HoldStatus{HoldID: holdID, State: HoldActive}
	}
	if st, ok := h.ended[holdID]; ok {
		return st
	}
	return HoldStatus{HoldID: holdID, State: HoldExpired, Reason: "unknown_hold"}
}

// Stop ends the caller's lease if it is the active one.
func (h *Holds) Stop(deviceID, holdID, reason string) {
	h.mu.Lock()
	l := h.active
	if l == nil || l.id != holdID || l.deviceID != deviceID {
		h.mu.Unlock()
		return
	}
	h.endLocked(l, HoldCancelled, reason)
	h.mu.Unlock()
	h.onChange()
}

// CancelDevice ends any lease owned by deviceID (revocation, disconnect).
func (h *Holds) CancelDevice(deviceID, reason string) {
	h.mu.Lock()
	l := h.active
	if l == nil || l.deviceID != deviceID {
		h.mu.Unlock()
		return
	}
	h.endLocked(l, HoldCancelled, reason)
	h.mu.Unlock()
	h.onChange()
}

// CancelAll ends the active lease (session lock, target change).
func (h *Holds) CancelAll(reason string) {
	h.mu.Lock()
	l := h.active
	if l == nil {
		h.mu.Unlock()
		return
	}
	h.endLocked(l, HoldCancelled, reason)
	h.mu.Unlock()
	h.onChange()
}

// Active reports the current lease for the state snapshot.
func (h *Holds) Active() contract.HoldState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active == nil {
		return contract.HoldState{}
	}
	dev, act := h.active.deviceID, h.active.action
	return contract.HoldState{Active: true, DeviceID: &dev, Action: &act}
}

func (h *Holds) expiry() time.Duration {
	return time.Duration(h.limits.HoldExpiryMs) * time.Millisecond
}

func (h *Holds) endLocked(l *lease, state, reason string) {
	if l.stopped {
		return
	}
	l.stopped = true
	close(l.stop)
	if h.active == l {
		h.active = nil
	}
	h.ended[l.id] = HoldStatus{HoldID: l.id, State: state, Reason: reason}
	h.order = append(h.order, l.id)
	for len(h.order) > 64 {
		delete(h.ended, h.order[0])
		h.order = h.order[1:]
	}
}

// run waits the repeat delay, then taps at the repeat rate until the lease
// stops or expires. The initial press is the client's own action.
func (h *Holds) run(l *lease) {
	interval := time.Second / time.Duration(max(h.limits.RepeatHz, 1))
	t := h.clock.NewTimer(time.Duration(h.limits.RepeatDelayMs) * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C():
		}
		h.mu.Lock()
		if l.stopped {
			h.mu.Unlock()
			return
		}
		if h.clock.Since(l.lastRenew) > h.expiry() {
			h.endLocked(l, HoldExpired, "no_renew")
			h.mu.Unlock()
			h.onChange()
			return
		}
		h.mu.Unlock()
		if !h.fire(l) {
			return
		}
		t.Reset(interval)
	}
}

func (h *Holds) fire(l *lease) bool {
	reason, err := h.tap(l.deviceID, l.action, l.epoch)
	if err == nil {
		return true
	}
	h.mu.Lock()
	h.endLocked(l, HoldCancelled, reason)
	h.mu.Unlock()
	h.onChange()
	return false
}
