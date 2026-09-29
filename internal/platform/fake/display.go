// Fake display power: an in-memory platform.DisplayPower for session tests
// and `bear-den-tv dev` (nothing touches a real display). It models what the
// X11 backend guarantees: Off changes the desktop's power settings (Changed
// reports it) and On or Close puts them back; Wake plays a key pressed on
// the TV, which turns the display on by itself; SetIdle sets the time since
// the last TV input.

package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"bear-den-tv/internal/platform"
)

// DisplayBackend is the fake's capability backend name.
const DisplayBackend = "fake-dpms"

// Display is a scriptable platform.DisplayPower.
type Display struct {
	mu      sync.Mutex
	cap     platform.Capability
	on      bool
	changed bool // settings changed by Off and not yet restored
	idle    time.Duration
	calls   []string
	failOff error
	closed  bool
	// OnOff, when set, runs inside Off after the display went dark (tests
	// record what else had happened by then).
	OnOff func()
}

var _ platform.DisplayPower = (*Display)(nil)

// NewDisplay returns a display that is on, can be turned off, and has seen
// no input for an hour.
func NewDisplay() *Display {
	return &Display{cap: platform.Capability{Available: true, Backend: DisplayBackend}, on: true, idle: time.Hour}
}

// SetCapability replaces what Capability reports.
func (d *Display) SetCapability(cp platform.Capability) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cap = cp
}

// FailOff makes Off return err (nil restores).
func (d *Display) FailOff(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failOff = err
}

// SetIdle sets the time since the last input on the TV.
func (d *Display) SetIdle(idle time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idle = idle
}

// Wake plays a key pressed on the TV: the display turns itself on (as DPMS
// does), the idle time restarts, and the settings stay changed until On.
func (d *Display) Wake() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.on, d.idle = true, 0
}

// IsOn reports whether the display is on.
func (d *Display) IsOn() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.on
}

// Changed reports whether Off changed the settings and nothing restored them.
func (d *Display) Changed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.changed
}

// Calls lists Off, On and Close in order.
func (d *Display) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

// Capability implements platform.DisplayPower.
func (d *Display) Capability() platform.Capability {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cap
}

// Off implements platform.DisplayPower.
func (d *Display) Off(context.Context) error {
	d.mu.Lock()
	d.calls = append(d.calls, "Off")
	switch {
	case d.closed || !d.cap.Available:
		d.mu.Unlock()
		return platform.ErrUnsupported
	case d.failOff != nil:
		err := d.failOff
		d.mu.Unlock()
		return err
	}
	d.on, d.changed = false, true
	hook := d.OnOff
	d.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

// On implements platform.DisplayPower.
func (d *Display) On(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, "On")
	if d.changed {
		d.on, d.changed = true, false
	}
	return nil
}

// Status implements platform.DisplayPower.
func (d *Display) Status(context.Context) (platform.DisplayStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return platform.DisplayStatus{}, errors.New("fake display closed")
	}
	return platform.DisplayStatus{On: d.on, Idle: d.idle, IdleKnown: true}, nil
}

// Close implements platform.DisplayPower: restore, then refuse further use.
func (d *Display) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, "Close")
	if d.changed {
		d.on, d.changed = true, false
	}
	d.closed = true
	return nil
}
