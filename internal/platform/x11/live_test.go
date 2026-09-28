// Live X11 test against a real display; skipped without one.

package x11

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"bear-den-tv/internal/platform"
)

// TestLive exercises the adapter against a real X server. It is gated on
// BDTV_X11_LIVE=1 (observation only) and BDTV_X11_LIVE_INPUT=1 (creates a
// managed test window, activates it, and taps one key into it). A window
// manager that does not honor mapping/activation — the target while its seat
// is on the login greeter — is reported with t.Log, not treated as a failure;
// the pure units in props_test.go are the CI signal.
func TestLive(t *testing.T) {
	if os.Getenv("BDTV_X11_LIVE") != "1" {
		t.Skip("live X11 test: set BDTV_X11_LIVE=1 with a reachable $DISPLAY (and BDTV_X11_LIVE_INPUT=1 for injection)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a, err := New(ctx, Options{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer a.Close()

	info := a.Info(ctx)
	t.Logf("server vendor=%q protocol=%s wm=%q ewmh=%v xtest=%v %s keycodes=%d..%d", info.Vendor, info.ProtocolVersion, info.WMName, info.EWMH, info.XTest, info.XTestReason, info.MinKeycode, info.MaxKeycode)
	keys := make([]string, 0, len(info.Keys))
	for k := range info.Keys {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		b := info.Keys[platform.Key(k)]
		t.Logf("key %-10s keysym=%#x keycode=%d resolved=%v", k, b.Keysym, b.Keycode, b.Resolved)
	}
	caps := a.Capabilities()
	capNames := make([]string, 0, len(caps))
	for k := range caps {
		capNames = append(capNames, k)
	}
	sort.Strings(capNames)
	for _, k := range capNames {
		t.Logf("capability %-18s available=%v backend=%s reason=%q", k, caps[k].Available, caps[k].Backend, caps[k].Reason)
	}

	wins, err := a.ListWindows(ctx)
	if err != nil {
		t.Logf("REPORT: ListWindows: %v", err)
	}
	t.Logf("%d managed windows", len(wins))
	for _, w := range wins {
		t.Logf("window %#x pid=%d class=%q title=%q mapped=%v fullscreen=%v", uint64(w.ID), w.PID, w.Class, w.Title, w.Mapped, w.Fullscreen)
	}
	fg, err := a.ObserveForeground(ctx)
	if err != nil {
		t.Logf("REPORT: ObserveForeground: %v", err)
	}
	t.Logf("foreground known=%v window=%#x class=%q title=%q", fg.Known, uint64(fg.Window.ID), fg.Window.Class, fg.Window.Title)

	// Fail-closed proof without injecting anything: a window that is not the
	// active one must be refused before any XTEST request is issued.
	if caps[platform.CapInput].Available {
		if err := a.DeliverKey(ctx, platform.WindowID(0x7fffffff), platform.KeyRight); !errors.Is(err, platform.ErrNotForeground) {
			t.Errorf("DeliverKey to a non-foreground window: err=%v, want ErrNotForeground", err)
		} else {
			t.Logf("fail-closed check ok: %v", err)
		}
	}

	if os.Getenv("BDTV_X11_LIVE_INPUT") != "1" {
		t.Log("input phase skipped: set BDTV_X11_LIVE_INPUT=1 to create a test window and tap a key into it")
		return
	}
	if !caps[platform.CapInput].Available {
		t.Logf("REPORT: input phase skipped: %s", caps[platform.CapInput].Reason)
		return
	}

	// The test window lives on its own connection so its KeyPress events are
	// not consumed by the adapter's event pump.
	tc, err := xgb.NewConnDisplay("")
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer tc.Close()
	screen := xproto.Setup(tc).DefaultScreen(tc)
	wid, err := xproto.NewWindowId(tc)
	if err != nil {
		t.Fatal(err)
	}
	err = xproto.CreateWindowChecked(tc, xproto.WindowClassCopyFromParent, wid, screen.Root, 0, 0, 240, 160, 0,
		xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask,
		[]uint32{screen.BlackPixel, xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskStructureNotify | xproto.EventMaskFocusChange}).Check()
	if err != nil {
		t.Fatalf("create test window: %v", err)
	}
	defer xproto.DestroyWindow(tc, wid)
	class := []byte("bdtv-live-test\x00bdtv-live-test\x00")
	if err := xproto.ChangePropertyChecked(tc, xproto.PropModeReplace, wid, xproto.AtomWmClass, xproto.AtomString, 8, uint32(len(class)), class).Check(); err != nil {
		t.Fatalf("WM_CLASS: %v", err)
	}
	title := []byte("Bear Den TV live test")
	if err := xproto.ChangePropertyChecked(tc, xproto.PropModeReplace, wid, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), title).Check(); err != nil {
		t.Fatalf("WM_NAME: %v", err)
	}
	if err := xproto.MapWindowChecked(tc, wid).Check(); err != nil {
		t.Fatalf("map: %v", err)
	}
	t.Logf("test window %#x created and mapped", wid)

	watch, err := a.WatchForeground(ctx)
	if err != nil {
		t.Fatalf("WatchForeground: %v", err)
	}

	managed := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		ws, err := a.ListWindows(ctx)
		if err != nil {
			t.Fatalf("ListWindows: %v", err)
		}
		for _, w := range ws {
			if w.ID == platform.WindowID(wid) && w.Mapped {
				managed = true
			}
		}
		if managed {
			break
		}
	}
	if !managed {
		t.Logf("REPORT: the window manager did not manage/map the test window within 3s (seat on the greeter?); activation and input not exercised")
		return
	}
	if err := a.Activate(ctx, platform.WindowID(wid)); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	activated := false
	timeout := time.After(3 * time.Second)
wait:
	for {
		select {
		case fg, ok := <-watch:
			if !ok {
				break wait
			}
			t.Logf("watch: known=%v window=%#x class=%q", fg.Known, uint64(fg.Window.ID), fg.Window.Class)
			if fg.Known && fg.Window.ID == platform.WindowID(wid) {
				activated = true
				break wait
			}
		case <-timeout:
			break wait
		}
	}
	if !activated {
		fg, _ := a.ObserveForeground(ctx)
		t.Logf("REPORT: activation not honored within 3s; active window is %#x class=%q", uint64(fg.Window.ID), fg.Window.Class)
		return
	}
	t.Logf("OBSERVED: test window %#x became _NET_ACTIVE_WINDOW", wid)

	code, err := a.KeycodeFor(ctx, platform.KeyRight)
	if err != nil {
		t.Fatalf("KeycodeFor: %v", err)
	}
	if err := a.DeliverKey(ctx, platform.WindowID(wid), platform.KeyRight); err != nil {
		t.Fatalf("DeliverKey: %v", err)
	}
	t.Logf("DELIVERED: keycode %d to %#x", code, wid)
	pressed, released := false, false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) && !(pressed && released); {
		ev, xerr := tc.PollForEvent()
		if ev == nil && xerr == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		switch e := ev.(type) {
		case xproto.KeyPressEvent:
			if e.Detail == code {
				pressed = true
			}
		case xproto.KeyReleaseEvent:
			if e.Detail == code {
				released = true
			}
		}
	}
	if !pressed || !released {
		t.Errorf("key was delivered but the test window observed press=%v release=%v within 2s", pressed, released)
	} else {
		t.Logf("OBSERVED: KeyPress+KeyRelease keycode %d on the test window", code)
	}
}
