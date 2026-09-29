// Coordinator behaviour on Wayland desktops (docs/decisions/0007-wayland-profile.md):
// windows identified by app_id instead of WM_CLASS, and app launches on a
// desktop that cannot observe windows reported as delivered, never observed
// and never failed for want of a window.

package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

func TestWaylandAppIDIdentifiesShellAndApps(t *testing.T) {
	h := newHarness(t)
	plex := h.desk.AddWindow(platform.WindowInfo{AppID: "tv.plex.PlexHTPC", Mapped: true})
	shell := h.desk.AddWindow(platform.WindowInfo{AppID: "bear-den-tv-shell", Mapped: true})
	h.desk.SetActive(plex)
	h.eventually("plex targeted by app_id", func() bool {
		tg := h.c.Target()
		return tg.Kind == "app" && strOr(tg.AppID) == "plex-htpc"
	})
	h.desk.SetActive(shell)
	h.eventually("shell targeted by app_id", func() bool { return h.c.Target().Kind == "shell" })
	other := h.desk.AddWindow(platform.WindowInfo{AppID: "org.example.Terminal", Mapped: true})
	h.desk.SetActive(other)
	h.eventually("an unknown app_id is another window", func() bool {
		tg := h.c.Target()
		return tg.Kind == "unknown" && tg.Observed
	})
}

// blindDesktop is a desktop that, once blind, cannot observe or list windows
// (GNOME or KDE on Wayland).
type blindDesktop struct {
	platform.DesktopAdapter
	blind atomic.Bool
}

func (b *blindDesktop) Capabilities() map[string]platform.Capability {
	caps := b.DesktopAdapter.Capabilities()
	if b.blind.Load() {
		off := platform.Capability{Backend: "blind", Reason: "no window protocol"}
		caps[platform.CapObserveForeground], caps[platform.CapActivate] = off, off
	}
	return caps
}

func (b *blindDesktop) ListWindows(ctx context.Context) ([]platform.WindowInfo, error) {
	if b.blind.Load() {
		return nil, platform.ErrUnsupported
	}
	return b.DesktopAdapter.ListWindows(ctx)
}

// windowlessLauncher starts nothing visible; its instances are what it launched.
type windowlessLauncher struct {
	mu    sync.Mutex
	insts []applications.Instance
}

func (l *windowlessLauncher) Discover(context.Context, string) (applications.Installation, error) {
	return applications.Installation{Installed: true, Version: "1.0", Scope: "user"}, nil
}

func (l *windowlessLauncher) Launch(_ context.Context, id string, _ []string) (applications.Instance, error) {
	in := applications.Instance{FlatpakID: id, InstanceID: "1", PID: 9100}
	l.mu.Lock()
	l.insts = append(l.insts, in)
	l.mu.Unlock()
	return in, nil
}

func (l *windowlessLauncher) Instances(context.Context) ([]applications.Instance, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]applications.Instance(nil), l.insts...), nil
}

func (l *windowlessLauncher) Kill(context.Context, applications.Instance) error { return nil }

func TestLaunchOnBlindDesktopIsDeliveredNotObserved(t *testing.T) {
	var desk *blindDesktop
	h := newHarness(t, func(o *Options) {
		desk = &blindDesktop{DesktopAdapter: o.Desktop}
		o.Desktop = desk
		o.Launcher = &windowlessLauncher{}
	})
	desk.blind.Store(true)
	results, _ := h.phones.Results(context.Background(), h.ctl)
	expectOutcome(t, h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeAccepted, contract.CodeOK)
	var res contract.ActionResult
	select {
	case res = <-results:
	case <-time.After(3 * time.Second):
		t.Fatal("no launch result")
	}
	if res.Outcome != contract.OutcomeDelivered || res.Code != contract.CodeOK || res.Detail["observed"] != false {
		t.Fatalf("launch result = %+v, want a final delivered with observed=false", res)
	}
	// That was the final result: nothing else follows, and the app is
	// running (its process started), not launching or failed.
	select {
	case more := <-results:
		t.Fatalf("unexpected second result %+v", more)
	case <-time.After(200 * time.Millisecond):
	}
	h.eventually("launch_state running without an error", func() bool {
		for _, a := range h.phones.Snapshot(context.Background(), &h.ctl).Applications {
			if a.ID == "plex-htpc" {
				return a.LaunchState == "running" && a.LastError == nil && a.Running && !a.Foreground
			}
		}
		return false
	})
}
