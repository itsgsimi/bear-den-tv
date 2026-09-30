// Tests for the TV's first-run setup and "Start with this PC" (setup.go):
// onboarding.complete and autostart.configure through the real shell
// handler, what they persist or write, and state.onboarding/state.autostart
// in the shell view only.

package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/autostart"
	"bear-den-tv/internal/shellipc"
)

// sendSetup sends one setup message from the shell and waits for its reply.
func (h *harness) sendSetup(m shellipc.Message, id string) shellipc.Result {
	h.t.Helper()
	if err := h.shell.Send(m); err != nil {
		h.t.Fatal(err)
	}
	return h.shellResult(id)
}

// noSetupForPhones checks that phones and anonymous viewers never get
// onboarding or autostart.
func (h *harness) noSetupForPhones() {
	h.t.Helper()
	for name, st := range map[string]contract.State{
		"controller": h.phones.Snapshot(context.Background(), &h.ctl),
		"owner":      h.phones.Snapshot(context.Background(), &h.owner),
		"anonymous":  h.c.buildState(viewAnonymous),
	} {
		if st.Onboarding != nil || st.Autostart != nil {
			h.t.Errorf("%s view carries onboarding %+v / autostart %+v", name, st.Onboarding, st.Autostart)
		}
	}
}

func TestOnboardingCompletePersists(t *testing.T) {
	h := newHarness(t)
	st := h.c.buildState(viewShell)
	if st.Onboarding == nil || st.Onboarding.Completed {
		t.Fatalf("a fresh box is not set up: %+v", st.Onboarding)
	}
	rev := h.c.opts.Config.Revision()
	if r := h.sendSetup(shellipc.OnboardingComplete{Type: shellipc.TypeOnboardingComplete, RequestID: "ob-1"}, "ob-1"); !r.OK || r.Error != "" {
		t.Fatalf("onboarding.complete refused: %+v", r)
	}
	if got := h.c.opts.Config.Revision(); got != rev+1 {
		t.Fatalf("revision %d → %d, want a bump", rev, got)
	}
	// What is on disk, read by a fresh store.
	fresh, err := config.Open(config.Options{Dir: filepath.Dir(h.c.opts.Config.Path()), Clock: clock.Real{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if !fresh.Current().Onboarding.Completed {
		t.Fatal("onboarding.completed not persisted")
	}
	st = h.c.buildState(viewShell)
	if st.Onboarding == nil || !st.Onboarding.Completed {
		t.Fatalf("shell view after onboarding.complete: %+v", st.Onboarding)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatalf("shell state is invalid: %v", err)
	}
	if r := h.sendSetup(shellipc.OnboardingComplete{Type: shellipc.TypeOnboardingComplete, RequestID: "ob-2"}, "ob-2"); !r.OK {
		t.Fatalf("a second onboarding.complete refused: %+v", r)
	}
	h.noSetupForPhones()
}

// A box whose owner already turned the phone remote on counts as set up.
func TestRemoteConfigureCompletesOnboarding(t *testing.T) {
	h := newHarness(t)
	if r := h.sendSetup(shellipc.RemoteConfigure{Type: shellipc.TypeRemoteConfigure, RequestID: "rc-1", Enabled: true, LANConsent: true, Transport: "trusted-lan-http", Interface: "eth0"}, "rc-1"); !r.OK {
		t.Fatalf("remote.configure refused: %+v", r)
	}
	if !h.c.opts.Config.Current().Onboarding.Completed {
		t.Fatal("remote.configure enabled did not mark onboarding completed")
	}
}

func TestAutostartConfigureWritesAndRemovesTheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	script := filepath.Join(home, "bear den", "scripts", "start-session.sh")
	h := newHarness(t, func(o *Options) {
		o.Autostart = &Autostart{Path: autostart.File(), Script: func() (string, error) { return script, nil }}
	})
	entry := filepath.Join(home, ".config", "autostart", "bear-den-tv.desktop")
	if a := h.c.buildState(viewShell).Autostart; a == nil || a.Enabled || !a.Available || a.Reason != "" {
		t.Fatalf("before: %+v", a)
	}
	if r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-1", Enabled: true}, "as-1"); !r.OK {
		t.Fatalf("autostart on refused: %+v", r)
	}
	raw, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("no entry written: %v", err)
	}
	if want := "Exec=" + autostart.ExecQuote(script) + " --watch\n"; !strings.Contains(string(raw), want) || string(raw) != autostart.DesktopEntry(script) {
		t.Fatalf("entry is not the CLI's (want %q):\n%s", want, raw)
	}
	st := h.c.buildState(viewShell)
	if a := st.Autostart; a == nil || !a.Enabled || !a.Available {
		t.Fatalf("after on: %+v", a)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatalf("shell state is invalid: %v", err)
	}
	h.noSetupForPhones()

	if r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-2"}, "as-2"); !r.OK {
		t.Fatalf("autostart off refused: %+v", r)
	}
	if _, err := os.Stat(entry); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("entry still there after off: %v", err)
	}
	if a := h.c.buildState(viewShell).Autostart; a.Enabled {
		t.Fatalf("after off: %+v", a)
	}
	if r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-3"}, "as-3"); !r.OK {
		t.Fatalf("off with nothing there refused: %+v", r)
	}
}

func TestAutostartWithoutStartScriptFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	h := newHarness(t, func(o *Options) {
		o.Autostart = &Autostart{Path: autostart.File(), Script: func() (string, error) {
			return "", errors.New("start script not found for " + home + "/bin/bear-den-tv")
		}}
	})
	r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-1", Enabled: true}, "as-1")
	if r.OK || r.Error != reasonAutostartScript {
		t.Fatalf("want ok false with %q, got %+v", reasonAutostartScript, r)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "autostart", "bear-den-tv.desktop")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an entry was written without a start script: %v", err)
	}
	st := h.c.buildState(viewShell)
	if a := st.Autostart; a == nil || a.Enabled || a.Available || a.Reason != reasonAutostartScript || strings.Contains(a.Reason, home) {
		t.Fatalf("state: %+v", a)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatalf("shell state is invalid: %v", err)
	}
}

func TestAutostartUnavailableWithoutSupport(t *testing.T) {
	h := newHarness(t)
	r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-1", Enabled: true}, "as-1")
	if r.OK || r.Error != reasonAutostartSession {
		t.Fatalf("want ok false with %q, got %+v", reasonAutostartSession, r)
	}
	if r := h.sendSetup(shellipc.AutostartConfigure{Type: shellipc.TypeAutostartConfigure, RequestID: "as-2"}, "as-2"); r.OK {
		t.Fatalf("off accepted without support: %+v", r)
	}
	if a := h.c.buildState(viewShell).Autostart; a == nil || a.Available || a.Enabled || a.Reason != reasonAutostartSession {
		t.Fatalf("state: %+v", a)
	}
}
