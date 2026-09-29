// DisplayPower over the X11 DPMS extension (platform.DisplayPower; the sleep
// timer and display.off, contracts/actions.md). Many TV desktops ship DPMS
// disabled with timeouts configured (600 s on the reference box), so forcing
// the display off means enabling DPMS. While it is enabled the standby,
// suspend and off timeouts are 0 (never blank on its own), and the exact
// previous state (enabled flag and the three timeouts) is restored when the
// display comes back on and on Close. If the coordinator is killed while the
// display is off, DPMS stays enabled with no timeouts: nothing blanks later.
// Idle time comes from MIT-SCREEN-SAVER when the server has it.

package x11

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/dpms"
	"github.com/jezek/xgb/screensaver"
	"github.com/jezek/xgb/xproto"

	"bear-den-tv/internal/platform"
)

// DPMSBackend is the capability backend name for display power.
const DPMSBackend = "x11-dpms"

// DPMSState is the part of the X server's DPMS state Bear Den changes and
// restores: the enabled flag and the three timeouts in seconds (0 = never).
type DPMSState struct {
	Enabled                  bool
	Standby, Suspend, OffSec uint16
}

// dpmsOps is the narrow set of X requests DisplayPower needs; the real one
// speaks DPMS and MIT-SCREEN-SAVER on its own connection, tests use a fake.
type dpmsOps interface {
	// Info returns the power level (dpms.DPMSMode*) and whether DPMS is enabled.
	Info() (level uint16, enabled bool, err error)
	Timeouts() (standby, suspend, off uint16, err error)
	SetTimeouts(standby, suspend, off uint16) error
	Enable() error
	Disable() error
	ForceLevel(level uint16) error
	// Idle is the time since the last user input; ok false without MIT-SCREEN-SAVER.
	Idle() (d time.Duration, ok bool, err error)
	Close() error
}

// DisplayPower implements platform.DisplayPower. It is safe for concurrent use.
type DisplayPower struct {
	mu    sync.Mutex
	ops   dpmsOps
	cap   platform.Capability
	saved *DPMSState // the state before the first Off; nil while nothing is changed
}

var _ platform.DisplayPower = (*DisplayPower)(nil)

// NewDisplayPower connects to display ("" = $DISPLAY) on a connection of its
// own and checks for DPMS. It never fails: without a display or without DPMS
// the result reports the capability unavailable with the reason.
func NewDisplayPower(display string) *DisplayPower {
	ops, err := dialDPMS(display)
	if err != nil {
		return &DisplayPower{cap: platform.Capability{Backend: DPMSBackend, Reason: err.Error()}}
	}
	return newDisplayPower(ops)
}

func newDisplayPower(ops dpmsOps) *DisplayPower {
	return &DisplayPower{ops: ops, cap: platform.Capability{Available: true, Backend: DPMSBackend}}
}

// Capability implements platform.DisplayPower.
func (d *DisplayPower) Capability() platform.Capability {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cap
}

// Saved returns the state captured before the display was turned off, or
// nil when nothing is changed (tests, doctor).
func (d *DisplayPower) Saved() *DPMSState {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.saved == nil {
		return nil
	}
	s := *d.saved
	return &s
}

// Off implements platform.DisplayPower: capture (first time only), timeouts
// to 0, enable, force off. A failure part-way restores what was captured.
func (d *DisplayPower) Off(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ops == nil {
		return fmt.Errorf("%w: %s", platform.ErrUnsupported, d.cap.Reason)
	}
	if d.saved == nil {
		_, enabled, err := d.ops.Info()
		if err != nil {
			return fmt.Errorf("x11: read DPMS state: %w", err)
		}
		s, su, o, err := d.ops.Timeouts()
		if err != nil {
			return fmt.Errorf("x11: read DPMS timeouts: %w", err)
		}
		d.saved = &DPMSState{Enabled: enabled, Standby: s, Suspend: su, OffSec: o}
	}
	// Timeouts first, so DPMS is never enabled with the old timeouts.
	err := d.ops.SetTimeouts(0, 0, 0)
	if err == nil {
		err = d.ops.Enable()
	}
	if err == nil {
		err = d.ops.ForceLevel(dpms.DPMSModeOff)
	}
	if err != nil {
		if rerr := d.restoreLocked(); rerr != nil {
			return fmt.Errorf("x11: turn the display off: %w (restoring: %v)", err, rerr)
		}
		return fmt.Errorf("x11: turn the display off: %w", err)
	}
	return nil
}

// On implements platform.DisplayPower.
func (d *DisplayPower) On(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.restoreLocked()
}

// restoreLocked turns the display on and puts back the captured state. On
// any error the capture is kept so a later On or Close tries again.
func (d *DisplayPower) restoreLocked() error {
	if d.saved == nil || d.ops == nil {
		return nil
	}
	s := *d.saved
	var errs []error
	// ForceLevel needs DPMS enabled (BadMatch otherwise); it is not when Off
	// failed before enabling it.
	if _, enabled, err := d.ops.Info(); err != nil {
		errs = append(errs, fmt.Errorf("read state: %w", err))
	} else if enabled {
		if err := d.ops.ForceLevel(dpms.DPMSModeOn); err != nil {
			errs = append(errs, fmt.Errorf("force on: %w", err))
		}
	}
	if err := d.ops.SetTimeouts(s.Standby, s.Suspend, s.OffSec); err != nil {
		errs = append(errs, fmt.Errorf("timeouts: %w", err))
	}
	var err error
	if s.Enabled {
		err = d.ops.Enable()
	} else {
		err = d.ops.Disable()
	}
	if err != nil {
		errs = append(errs, fmt.Errorf("enabled=%v: %w", s.Enabled, err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("x11: restore DPMS: %w", errors.Join(errs...))
	}
	d.saved = nil
	return nil
}

// Status implements platform.DisplayPower. With DPMS disabled the display is on.
func (d *DisplayPower) Status(context.Context) (platform.DisplayStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ops == nil {
		return platform.DisplayStatus{}, fmt.Errorf("%w: %s", platform.ErrUnsupported, d.cap.Reason)
	}
	level, enabled, err := d.ops.Info()
	if err != nil {
		return platform.DisplayStatus{}, fmt.Errorf("x11: read DPMS state: %w", err)
	}
	st := platform.DisplayStatus{On: !enabled || level == dpms.DPMSModeOn}
	if idle, ok, err := d.ops.Idle(); err == nil && ok {
		st.Idle, st.IdleKnown = idle, true
	}
	return st, nil
}

// Close implements platform.DisplayPower: restore, then disconnect.
func (d *DisplayPower) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	err := d.restoreLocked()
	if d.ops != nil {
		_ = d.ops.Close()
		d.ops = nil
		d.cap = platform.Capability{Backend: DPMSBackend, Reason: "closed"}
	}
	return err
}

// xDPMS is dpmsOps on a real X connection.
type xDPMS struct {
	conn  *xgb.Conn
	root  xproto.Window
	saver bool // MIT-SCREEN-SAVER is present
}

// dialDPMS connects and checks that the server has DPMS and the display is capable.
func dialDPMS(display string) (*xDPMS, error) {
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("the X display %q cannot be reached", displayLabel(display))
	}
	if err := dpms.Init(conn); err != nil {
		conn.Close()
		return nil, errors.New("the X server has no DPMS extension")
	}
	if _, err := dpms.GetVersion(conn, 1, 1).Reply(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("the X server's DPMS extension did not answer: %v", err)
	}
	c, err := dpms.Capable(conn).Reply()
	if err != nil || !c.Capable {
		conn.Close()
		return nil, errors.New("this display is not DPMS capable")
	}
	x := &xDPMS{conn: conn, root: xproto.Setup(conn).DefaultScreen(conn).Root}
	x.saver = screensaver.Init(conn) == nil
	return x, nil
}

func (x *xDPMS) Info() (uint16, bool, error) {
	r, err := dpms.Info(x.conn).Reply()
	if err != nil {
		return 0, false, err
	}
	return r.PowerLevel, r.State, nil
}

func (x *xDPMS) Timeouts() (uint16, uint16, uint16, error) {
	r, err := dpms.GetTimeouts(x.conn).Reply()
	if err != nil {
		return 0, 0, 0, err
	}
	return r.StandbyTimeout, r.SuspendTimeout, r.OffTimeout, nil
}

func (x *xDPMS) SetTimeouts(s, su, o uint16) error {
	return dpms.SetTimeoutsChecked(x.conn, s, su, o).Check()
}

func (x *xDPMS) Enable() error  { return dpms.EnableChecked(x.conn).Check() }
func (x *xDPMS) Disable() error { return dpms.DisableChecked(x.conn).Check() }

func (x *xDPMS) ForceLevel(level uint16) error {
	return dpms.ForceLevelChecked(x.conn, level).Check()
}

func (x *xDPMS) Idle() (time.Duration, bool, error) {
	if !x.saver {
		return 0, false, nil
	}
	r, err := screensaver.QueryInfo(x.conn, xproto.Drawable(x.root)).Reply()
	if err != nil {
		return 0, false, err
	}
	return time.Duration(r.MsSinceUserInput) * time.Millisecond, true, nil
}

func (x *xDPMS) Close() error {
	x.conn.Close()
	return nil
}
