// Live DPMS test against a real X server. By default it starts a throwaway
// Xvfb :N and never touches $DISPLAY or this machine's power settings; it
// skips with the reason when Xvfb is missing or offers no DPMS (Xvfb and the
// Xorg dummy driver in xorg-server 21 do not advertise the extension). To run
// it against a DPMS-capable display you chose (the TV, with nothing playing),
// set BDTV_DPMS_LIVE_DISPLAY=:0: the display goes dark for a moment and its
// DPMS settings are put back as they were.

package x11

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/dpms"
)

// startXvfb runs Xvfb on a free display number and returns ":N"; it is
// killed when the test ends.
func startXvfb(t *testing.T, bin string) string {
	t.Helper()
	for n := 170; n < 200; n++ {
		if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", fmt.Sprintf("X%d", n))); err == nil {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X%d-lock", n)); err == nil {
			continue
		}
		display := fmt.Sprintf(":%d", n)
		cmd := exec.Command(bin, display, "-nolisten", "tcp", "-screen", "0", "640x480x24")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start Xvfb: %v", err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		var last error
		for i := 0; i < 100; i++ {
			conn, err := xgb.NewConnDisplay(display)
			if err != nil {
				last = err
				time.Sleep(50 * time.Millisecond)
				continue
			}
			conn.Close()
			if ops, err := dialDPMS(display); err != nil {
				t.Skipf("the throwaway Xvfb on %s has no usable DPMS (%v); set BDTV_DPMS_LIVE_DISPLAY to a DPMS-capable display to run this", display, err)
			} else {
				_ = ops.Close()
			}
			return display
		}
		t.Skipf("Xvfb on %s never accepted a connection: %v", display, last)
	}
	t.Skip("no free X display number between :170 and :199")
	return ""
}

func readDPMS(t *testing.T, ops *xDPMS) (DPMSState, uint16) {
	t.Helper()
	level, enabled, err := ops.Info()
	if err != nil {
		t.Fatal(err)
	}
	s, su, o, err := ops.Timeouts()
	if err != nil {
		t.Fatal(err)
	}
	return DPMSState{Enabled: enabled, Standby: s, Suspend: su, OffSec: o}, level
}

// TestDPMSLiveRestoresTheServerState sets the reference TV's state (DPMS
// disabled, 600 s timeouts) and another one on a real X server, then checks
// Off, On and Close against what the server itself reports.
func TestDPMSLiveRestoresTheServerState(t *testing.T) {
	display := os.Getenv("BDTV_DPMS_LIVE_DISPLAY")
	if display == "" {
		bin, err := exec.LookPath("Xvfb")
		if err != nil {
			t.Skip("live DPMS test: Xvfb is not installed; TestDPMS* on the fake server cover the logic")
		}
		display = startXvfb(t, bin)
	}
	probe, err := dialDPMS(display)
	if err != nil {
		t.Skipf("the throwaway X server has no usable DPMS: %v", err)
	}
	defer probe.Close()
	ctx := context.Background()

	for _, start := range []DPMSState{
		{Enabled: false, Standby: 600, Suspend: 600, OffSec: 600}, // the reference TV
		{Enabled: true, Standby: 120, Suspend: 300, OffSec: 900},
	} {
		if err := probe.SetTimeouts(start.Standby, start.Suspend, start.OffSec); err != nil {
			t.Fatal(err)
		}
		if start.Enabled {
			err = probe.Enable()
		} else {
			err = probe.Disable()
		}
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := readDPMS(t, probe); got != start {
			t.Fatalf("could not set the start state: %+v", got)
		}

		d := NewDisplayPower(display)
		if cp := d.Capability(); !cp.Available {
			t.Fatalf("capability: %+v", cp)
		}
		if err := d.Off(ctx); err != nil {
			t.Fatal(err)
		}
		got, level := readDPMS(t, probe)
		if want := (DPMSState{Enabled: true}); got != want || level != dpms.DPMSModeOff {
			t.Fatalf("while off: %+v level %d; want %+v level off", got, level, want)
		}
		if st, err := d.Status(ctx); err != nil || st.On {
			t.Fatalf("status while off: %+v %v", st, err)
		}
		if err := d.Off(ctx); err != nil { // the timer firing after display.off
			t.Fatal(err)
		}
		if err := d.On(ctx); err != nil {
			t.Fatal(err)
		}
		if got, level := readDPMS(t, probe); got != start || level != dpms.DPMSModeOn {
			t.Fatalf("after On: %+v level %d; want %+v level on", got, level, start)
		}
		// The coordinator exiting while the display is off.
		if err := d.Off(ctx); err != nil {
			t.Fatal(err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if got, level := readDPMS(t, probe); got != start || level != dpms.DPMSModeOn {
			t.Fatalf("after Close: %+v level %d; want %+v level on", got, level, start)
		}
		t.Logf("start %+v: off, on and close restored it exactly", start)
	}
}
