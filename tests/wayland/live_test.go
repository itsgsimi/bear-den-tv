//go:build wayland_live

// Live test of the Wayland adapter (internal/platform/wayland) against a real
// wlroots compositor. It runs inside the headless sway container started by
// scripts/wayland-container-test.sh, which opens the test clients whose
// app_ids it passes in BDTV_WL_APPS (comma-separated, the last one opened
// first in the list's order of focus: sway focuses the newest window).
// Not part of `make test`: it needs Docker. See
// docs/decisions/0007-wayland-profile.md.

package wayland_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/wayland"
)

func appIDs(t *testing.T) []string {
	v := os.Getenv("BDTV_WL_APPS")
	if v == "" || os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("run by scripts/wayland-container-test.sh (needs WAYLAND_DISPLAY and BDTV_WL_APPS)")
	}
	return strings.Split(v, ",")
}

// swayFocused asks sway itself which app_id has focus: evidence that does
// not go through the adapter under test.
func swayFocused(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("swaymsg", "-r", "-t", "get_tree").Output()
	if err != nil {
		t.Fatalf("swaymsg: %v", err)
	}
	var root swayNode
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("swaymsg tree: %v", err)
	}
	return root.focusedAppID()
}

type swayNode struct {
	Focused     bool    `json:"focused"`
	AppID       *string `json:"app_id"`
	Shell       string  `json:"shell"`
	WindowProps *struct {
		Class string `json:"class"`
	} `json:"window_properties"`
	Nodes         []swayNode `json:"nodes"`
	FloatingNodes []swayNode `json:"floating_nodes"`
}

// focusedAppID is the focused view's app_id (an XWayland view's class).
func (n swayNode) focusedAppID() string {
	if n.Focused {
		if n.AppID != nil {
			return *n.AppID
		}
		if n.WindowProps != nil {
			return n.WindowProps.Class
		}
		return ""
	}
	for _, c := range append(append([]swayNode{}, n.Nodes...), n.FloatingNodes...) {
		if id := c.focusedAppID(); id != "" {
			return id
		}
	}
	return ""
}

// TestLiveShellWindow runs while `bear-den-tv session` supervises the real TV
// shell in the container: the shell is an XWayland window (the toolchain's
// Qt has no wayland platform plugin), identified by app_id, and fullscreen.
func TestLiveShellWindow(t *testing.T) {
	if os.Getenv("BDTV_WL_SHELL") == "" {
		t.Skip("run by scripts/wayland-container-test.sh --shell")
	}
	ctx := context.Background()
	a := wayland.New(ctx, wayland.Options{})
	defer a.Close()
	ws, err := a.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		if w.AppID != "bear-den-tv-shell" {
			continue
		}
		t.Logf("shell toplevel id=%d app_id=%q mapped=%v fullscreen=%v", w.ID, w.AppID, w.Mapped, w.Fullscreen)
		if !w.Mapped || !w.Fullscreen {
			t.Fatalf("shell window not mapped fullscreen: %+v", w)
		}
		if fg, _ := a.ObserveForeground(ctx); !fg.Known || fg.Window.ID != w.ID {
			t.Fatalf("shell is not in front: %+v", fg)
		}
		if got := swayView(t, "bear-den-tv-shell"); got != "xwayland" {
			t.Fatalf("sway says the shell view is %q, want xwayland", got)
		}
		t.Log("shell is an XWayland view, fullscreen, in front")
		return
	}
	t.Fatalf("no shell toplevel among %+v", ws)
}

// swayView returns the "shell" (xdg_shell or xwayland) of the view whose
// app_id or X11 class is id.
func swayView(t *testing.T, id string) string {
	t.Helper()
	out, err := exec.Command("swaymsg", "-r", "-t", "get_tree").Output()
	if err != nil {
		t.Fatalf("swaymsg: %v", err)
	}
	var root swayNode
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	var find func(n swayNode) string
	find = func(n swayNode) string {
		if (n.AppID != nil && *n.AppID == id) || (n.WindowProps != nil && n.WindowProps.Class == id) {
			return n.Shell
		}
		for _, c := range append(append([]swayNode{}, n.Nodes...), n.FloatingNodes...) {
			if s := find(c); s != "" {
				return s
			}
		}
		return ""
	}
	return find(root)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLiveSway(t *testing.T) {
	want := appIDs(t)
	ctx := context.Background()
	a := wayland.New(ctx, wayland.Options{})
	defer a.Close()
	t.Logf("adapter %s, family %s", a.Name(), a.Family())
	if a.Name() != wayland.NameWlr || a.Family() != wayland.FamilyWlroots {
		t.Fatalf("adapter %s family %s; capabilities %+v", a.Name(), a.Family(), a.Capabilities())
	}
	caps := a.Capabilities()
	for k, c := range caps {
		t.Logf("capability %-18s available=%v backend=%s reason=%q", k, c.Available, c.Backend, c.Reason)
	}
	if !caps[platform.CapObserveForeground].Available || !caps[platform.CapActivate].Available {
		t.Fatal("observe_foreground and activate must be available on sway")
	}
	if caps[platform.CapInput].Available || caps[platform.CapPhysicalHome].Available {
		t.Fatal("input and physical_home must stay unavailable")
	}

	wins, err := a.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byApp := map[string]platform.WindowInfo{}
	for _, w := range wins {
		t.Logf("toplevel id=%d app_id=%q mapped=%v fullscreen=%v", w.ID, w.AppID, w.Mapped, w.Fullscreen)
		byApp[w.AppID] = w
	}
	for _, id := range want {
		if _, ok := byApp[id]; !ok {
			t.Fatalf("toplevel %q not listed", id)
		}
	}

	fg, err := a.ObserveForeground(ctx)
	if err != nil || !fg.Known {
		t.Fatalf("foreground: %v %+v", err, fg)
	}
	if got := swayFocused(t); fg.Window.AppID != got {
		t.Fatalf("adapter says %q is in front, sway says %q", fg.Window.AppID, got)
	}
	t.Logf("foreground app_id=%q (sway agrees)", fg.Window.AppID)

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, err := a.WatchForeground(wctx)
	if err != nil {
		t.Fatal(err)
	}
	<-ch

	// Activate every other window in turn; each must be observed by the
	// watch and confirmed by sway.
	for _, id := range want {
		if id == fg.Window.AppID {
			continue
		}
		if err := a.Activate(ctx, byApp[id].ID); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
		select {
		case got := <-ch:
			if !got.Known || got.Window.AppID != id {
				t.Fatalf("after activating %s the watch reported %+v", id, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("activating %s was not observed", id)
		}
		if s := swayFocused(t); s != id {
			t.Fatalf("activated %s but sway focuses %q", id, s)
		}
		t.Logf("activated %q: watch and sway agree", id)
	}

	last := byApp[want[len(want)-1]]
	if err := a.SetFullscreen(ctx, last.ID, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "fullscreen", func() bool {
		ws, _ := a.ListWindows(ctx)
		for _, w := range ws {
			if w.ID == last.ID {
				return w.Fullscreen
			}
		}
		return false
	})
	t.Logf("fullscreened %q", last.AppID)

	if err := a.RequestClose(ctx, last.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "closed toplevel to leave the list", func() bool {
		ws, _ := a.ListWindows(ctx)
		for _, w := range ws {
			if w.ID == last.ID {
				return false
			}
		}
		return true
	})
	t.Logf("closed %q", last.AppID)
	if err := a.DeliverKey(ctx, fg.Window.ID, platform.KeyUp); err == nil {
		t.Fatal("a key was delivered on Wayland")
	}
}
