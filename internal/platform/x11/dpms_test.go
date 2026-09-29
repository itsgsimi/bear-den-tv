// Unit tests for DisplayPower (dpms.go) on a fake X server that behaves like
// the DPMS extension: the exact previous state (enabled flag and timeouts) is
// captured once and restored on On and Close, DPMS is never enabled with the
// old timeouts, and a failure part-way puts everything back.

package x11

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/dpms"

	"bear-den-tv/internal/platform"
)

// fakeDPMS models the server: ForceLevel needs DPMS enabled (BadMatch
// otherwise), enabling or disabling it turns the display on.
type fakeDPMS struct {
	enabled               bool
	level                 uint16
	standby, suspend, off uint16
	idle                  time.Duration
	calls                 []string
	failOn                string // a call name that fails
	enabledWithOldTimeout bool   // DPMS was ever enabled while the old non-zero timeouts were set
	closed                bool
}

// tvDefaults is what the reference TV reports: DPMS disabled, 600 s timeouts.
func tvDefaults() *fakeDPMS {
	return &fakeDPMS{enabled: false, level: dpms.DPMSModeOn, standby: 600, suspend: 600, off: 600, idle: 3 * time.Second}
}

func (f *fakeDPMS) state() DPMSState {
	return DPMSState{Enabled: f.enabled, Standby: f.standby, Suspend: f.suspend, OffSec: f.off}
}

func (f *fakeDPMS) call(name string) error {
	f.calls = append(f.calls, name)
	if f.failOn == name {
		return errors.New("fake: " + name + " failed")
	}
	return nil
}

func (f *fakeDPMS) Info() (uint16, bool, error) { return f.level, f.enabled, f.call("Info") }
func (f *fakeDPMS) Timeouts() (uint16, uint16, uint16, error) {
	return f.standby, f.suspend, f.off, f.call("Timeouts")
}

func (f *fakeDPMS) SetTimeouts(s, su, o uint16) error {
	if err := f.call("SetTimeouts"); err != nil {
		return err
	}
	f.standby, f.suspend, f.off = s, su, o
	return nil
}

func (f *fakeDPMS) Enable() error {
	if err := f.call("Enable"); err != nil {
		return err
	}
	if f.standby != 0 || f.suspend != 0 || f.off != 0 {
		f.enabledWithOldTimeout = true
	}
	f.enabled, f.level = true, dpms.DPMSModeOn
	return nil
}

func (f *fakeDPMS) Disable() error {
	if err := f.call("Disable"); err != nil {
		return err
	}
	f.enabled, f.level = false, dpms.DPMSModeOn
	return nil
}

func (f *fakeDPMS) ForceLevel(level uint16) error {
	if err := f.call("ForceLevel"); err != nil {
		return err
	}
	if !f.enabled {
		return errors.New("fake: BadMatch (DPMS disabled)")
	}
	f.level = level
	return nil
}

func (f *fakeDPMS) Idle() (time.Duration, bool, error) { return f.idle, true, f.call("Idle") }
func (f *fakeDPMS) Close() error                       { f.closed = true; return nil }

func TestDPMSOffCapturesAndOnRestoresExactly(t *testing.T) {
	ctx := context.Background()
	for _, before := range []*fakeDPMS{
		tvDefaults(), // disabled, 600 s: the reference TV
		{enabled: true, level: dpms.DPMSModeOn, standby: 120, suspend: 300, off: 900},
		{enabled: false, level: dpms.DPMSModeOn},
	} {
		f := before
		want := f.state()
		d := newDisplayPower(f)
		if err := d.Off(ctx); err != nil {
			t.Fatal(err)
		}
		if !f.enabled || f.level != dpms.DPMSModeOff || f.standby != 0 || f.suspend != 0 || f.off != 0 {
			t.Fatalf("off: enabled=%v level=%d timeouts=%d/%d/%d; want enabled, off, 0/0/0", f.enabled, f.level, f.standby, f.suspend, f.off)
		}
		if f.enabledWithOldTimeout {
			t.Fatal("DPMS was enabled while the old timeouts were still set")
		}
		if got := d.Saved(); got == nil || *got != want {
			t.Fatalf("captured %+v, want %+v", got, want)
		}
		st, err := d.Status(ctx)
		if err != nil || st.On || !st.IdleKnown || st.Idle != f.idle {
			t.Fatalf("status while off: %+v %v", st, err)
		}
		// A second Off (the timer firing after display.off) must not
		// capture Bear Den's own 0/0/0 state.
		if err := d.Off(ctx); err != nil {
			t.Fatal(err)
		}
		if err := d.On(ctx); err != nil {
			t.Fatal(err)
		}
		if f.state() != want || f.level != dpms.DPMSModeOn {
			t.Fatalf("restored %+v level %d, want %+v level on", f.state(), f.level, want)
		}
		if d.Saved() != nil {
			t.Fatal("capture kept after a successful restore")
		}
		if st, _ := d.Status(ctx); !st.On {
			t.Fatal("status after On is off")
		}
		// On with nothing captured changes nothing.
		f.calls = nil
		if err := d.On(ctx); err != nil || len(f.calls) != 0 {
			t.Fatalf("On without Off: %v calls %v", err, f.calls)
		}
	}
}

func TestDPMSCloseRestores(t *testing.T) {
	f := tvDefaults()
	d := newDisplayPower(f)
	if err := d.Off(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if f.state() != tvDefaults().state() || !f.closed {
		t.Fatalf("after Close: %+v closed=%v", f.state(), f.closed)
	}
	if err := d.Off(context.Background()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Off after Close: %v", err)
	}
}

func TestDPMSFailurePartWayRestores(t *testing.T) {
	for _, step := range []string{"Enable", "ForceLevel"} {
		f := tvDefaults()
		f.failOn = step
		d := newDisplayPower(f)
		err := d.Off(context.Background())
		if err == nil || !strings.Contains(err.Error(), step) {
			t.Fatalf("%s: err %v", step, err)
		}
		if step == "Enable" {
			// Enable failed, so DPMS stays disabled: restore sets the old
			// timeouts back and disables again.
			f.failOn = ""
		}
		if step == "ForceLevel" {
			// The restore's own ForceLevel(On) fails too; the capture is
			// kept and a later Close finishes the job.
			if d.Saved() == nil {
				t.Fatal("capture dropped after a failed restore")
			}
			f.failOn = ""
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if f.state() != tvDefaults().state() {
			t.Fatalf("%s: left %+v", step, f.state())
		}
	}
}

func TestDPMSReadFailureChangesNothing(t *testing.T) {
	f := tvDefaults()
	f.failOn = "Timeouts"
	d := newDisplayPower(f)
	if err := d.Off(context.Background()); err == nil {
		t.Fatal("Off succeeded without reading the timeouts")
	}
	if !reflect.DeepEqual(f.calls, []string{"Info", "Timeouts"}) || d.Saved() != nil {
		t.Fatalf("calls %v saved %+v", f.calls, d.Saved())
	}
}

func TestDPMSUnavailableSaysWhy(t *testing.T) {
	d := NewDisplayPower(":987") // no such local display; never the real one
	cp := d.Capability()
	if cp.Available || cp.Reason == "" || cp.Backend != DPMSBackend {
		t.Fatalf("capability %+v", cp)
	}
	if err := d.Off(context.Background()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Off: %v", err)
	}
	if _, err := d.Status(context.Background()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Status: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}
