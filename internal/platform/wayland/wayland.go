// Package wayland is the desktop adapter for Wayland sessions. It does what a
// normal client can do safely and says precisely what it cannot, per
// compositor family (docs/decisions/0007-wayland-profile.md):
//
//   - wlroots compositors (sway, labwc, Hyprland, Wayfire, niri) offer
//     zwlr_foreign_toplevel_manager_v1: the adapter lists toplevels by app_id,
//     observes the activated one, activates, closes and fullscreens them
//     (client.go, over the hand-written wire format in wire.go).
//   - KDE Plasma, GNOME and compositors with only ext-foreign-toplevel-list
//     get observe_foreground and activate unavailable with a reason naming
//     what would be needed.
//   - Input is never injected: a consented RemoteDesktop portal session or
//     the wlroots virtual-keyboard protocol would be needed, and neither is
//     built (the ADR records the path). physical_home needs the
//     GlobalShortcuts portal.
//   - lock_observation comes from the logind/screensaver observer the caller
//     passes in; it does not depend on the display protocol.
//
// Nothing here is guessed: an ambiguous or missing activated toplevel is an
// unknown foreground, and the coordinator refuses input for it (spec §6.3).
package wayland

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/platform"
)

// Adapter names reported by Name.
const (
	// Name is the adapter's name when no window protocol is usable.
	Name = "wayland-limited"
	// NameWlr is the adapter's name when zwlr_foreign_toplevel_manager_v1 is bound.
	NameWlr = "wayland-wlr"
	// BackendWlr is the capability backend for wlr-foreign-toplevel features.
	BackendWlr = "wlr-foreign-toplevel"
)

// Compositor families the adapter tells apart (Family).
const (
	FamilyWlroots  = "wlroots"
	FamilyKDE      = "kde"
	FamilyGNOME    = "gnome"
	FamilyExtList  = "ext-list-only"
	FamilyUnknown  = "unknown"
	FamilyNoSocket = "unreachable"
)

// Reasons shared by tests and docs.
const (
	reasonInput        = "input on Wayland is not implemented: it needs a consented RemoteDesktop portal session (GNOME, KDE) or the wlroots virtual-keyboard protocol; see docs/decisions/0007-wayland-profile.md"
	reasonPhysicalHome = "physical Home on Wayland needs the GlobalShortcuts portal; not implemented; use the phone remote's Home"
	reasonKDE          = "KDE Plasma does not offer wlr-foreign-toplevel to clients, and the org_kde_plasma_window_management / KWin scripting bridge is not implemented"
	reasonGNOME        = "GNOME Shell offers no window-list protocol to clients; it would need a GNOME Shell extension, which Bear Den does not ship"
	reasonExtList      = "the compositor lists windows (ext-foreign-toplevel-list-v1) but does not tell clients which one is active or let them activate one"
	reasonUnknown      = "the compositor does not offer zwlr_foreign_toplevel_manager_v1, the only window protocol this adapter speaks"
)

// ConnectTimeout bounds connecting and the two initial roundtrips.
const ConnectTimeout = 3 * time.Second

// LockCapability is what the adapter needs from a lock observer to report
// the lock_observation capability; *lock.Observer satisfies it.
type LockCapability interface {
	Capability() platform.Capability
}

// Options configures New.
type Options struct {
	// Env is the imported session environment (WAYLAND_DISPLAY,
	// XDG_RUNTIME_DIR, XDG_CURRENT_DESKTOP); nil reads the process environment.
	Env map[string]string
	// Lock supplies lock_observation; nil reports it unavailable.
	Lock LockCapability
	// Dial overrides how the compositor socket is opened (tests).
	Dial func(ctx context.Context, path string) (net.Conn, error)
}

type watcher struct{ notify chan struct{} }

// Adapter implements platform.DesktopAdapter and platform.Fullscreener.
type Adapter struct {
	lock    LockCapability
	family  string
	reason  string // why window features are unavailable; "" on wlroots
	client  *client
	display string

	mu       sync.Mutex
	watchers map[*watcher]struct{}
}

// New connects to the compositor and learns its family. It never fails: an
// unreachable socket or a compositor without a usable protocol is an
// ordinary reduced profile whose Capabilities say why.
func New(ctx context.Context, opts Options) *Adapter {
	env := opts.Env
	if env == nil {
		env = map[string]string{}
		for _, k := range []string{"WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_CURRENT_DESKTOP"} {
			if v := os.Getenv(k); v != "" {
				env[k] = v
			}
		}
	}
	a := &Adapter{lock: opts.Lock, watchers: map[*watcher]struct{}{}}
	path, err := SocketPath(env)
	if err != nil {
		a.family, a.reason = FamilyNoSocket, err.Error()
		return a
	}
	a.display = env["WAYLAND_DISPLAY"]
	dial := opts.Dial
	if dial == nil {
		dial = func(ctx context.Context, path string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	conn, err := dial(ctx, path)
	if err != nil {
		a.family, a.reason = FamilyNoSocket, fmt.Sprintf("cannot connect to the Wayland display %q: %v", a.display, err)
		return a
	}
	c := newClient(newWire(conn), a.notifyWatchers)
	if err := c.start(ctx); err != nil {
		c.shutdown()
		a.family, a.reason = FamilyNoSocket, fmt.Sprintf("Wayland display %q: %v", a.display, err)
		return a
	}
	a.family = classify(c, env["XDG_CURRENT_DESKTOP"])
	a.reason = familyReason(a.family)
	if a.family != FamilyWlroots {
		// Nothing to observe; keep no connection open.
		c.shutdown()
		return a
	}
	a.client = c
	return a
}

// SocketPath resolves WAYLAND_DISPLAY the way libwayland does: an absolute
// path as is, else relative to XDG_RUNTIME_DIR.
func SocketPath(env map[string]string) (string, error) {
	d := env["WAYLAND_DISPLAY"]
	if d == "" {
		d = "wayland-0"
	}
	if filepath.IsAbs(d) {
		return d, nil
	}
	rt := env["XDG_RUNTIME_DIR"]
	if rt == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is not set, so the Wayland socket %q cannot be found", d)
	}
	return filepath.Join(rt, d), nil
}

// classify names the compositor family from its globals, then the desktop name.
func classify(c *client, desktop string) string {
	d := strings.ToLower(desktop)
	switch {
	case c.managerBound():
		return FamilyWlroots
	case c.has(ifaceKDEWindowMgt) || strings.Contains(d, "kde"):
		return FamilyKDE
	case c.has(ifaceGnomeShell) || strings.Contains(d, "gnome"):
		return FamilyGNOME
	case c.has(ifaceExtToplevel):
		return FamilyExtList
	}
	return FamilyUnknown
}

func (c *client) managerBound() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manager != 0
}

func familyReason(f string) string {
	switch f {
	case FamilyWlroots:
		return ""
	case FamilyKDE:
		return reasonKDE
	case FamilyGNOME:
		return reasonGNOME
	case FamilyExtList:
		return reasonExtList
	}
	return reasonUnknown
}

// Family is the detected compositor family (Family* constants).
func (a *Adapter) Family() string { return a.family }

// Name implements platform.DesktopAdapter.
func (a *Adapter) Name() string {
	if a.client != nil {
		return NameWlr
	}
	return Name
}

// DisplaySession implements platform.DesktopAdapter.
func (*Adapter) DisplaySession() string { return "wayland" }

// windowReason is why window features are unavailable now, or "".
func (a *Adapter) windowReason() string {
	if a.client == nil {
		return a.reason
	}
	if err := a.client.err(); err != nil {
		return err.Error()
	}
	return ""
}

// Capabilities implements platform.DesktopAdapter with a reason for every
// unavailable feature.
func (a *Adapter) Capabilities() map[string]platform.Capability {
	caps := map[string]platform.Capability{}
	if r := a.windowReason(); r != "" {
		caps[platform.CapObserveForeground] = platform.Capability{Backend: a.Name(), Reason: r}
		caps[platform.CapActivate] = platform.Capability{Backend: a.Name(), Reason: r}
	} else {
		caps[platform.CapObserveForeground] = platform.Capability{Backend: BackendWlr, Available: true}
		caps[platform.CapActivate] = platform.Capability{Backend: BackendWlr, Available: true}
	}
	caps[platform.CapInput] = platform.Capability{Backend: a.Name(), Reason: reasonInput}
	if a.lock != nil {
		caps[platform.CapLockObservation] = a.lock.Capability()
	} else {
		caps[platform.CapLockObservation] = platform.Capability{Backend: a.Name(), Reason: "no lock observer configured"}
	}
	caps[platform.CapPhysicalHome] = platform.Capability{Backend: a.Name(), Reason: reasonPhysicalHome}
	return caps
}

func (a *Adapter) unsupported(op string) error {
	r := a.windowReason()
	if r == "" {
		r = "not available"
	}
	return fmt.Errorf("%w: %s on Wayland: %s", platform.ErrUnsupported, op, r)
}

func (a *Adapter) ready(op string) error {
	if a.windowReason() != "" {
		return a.unsupported(op)
	}
	return nil
}

// ObserveForeground implements platform.DesktopAdapter: the one toplevel the
// compositor reports activated. None, or more than one, is Known=false.
func (a *Adapter) ObserveForeground(context.Context) (platform.Foreground, error) {
	if err := a.ready("observe foreground"); err != nil {
		return platform.Foreground{}, err
	}
	w, ok := a.client.active()
	if !ok {
		return platform.Foreground{}, nil
	}
	return platform.Foreground{Known: true, Window: w}, nil
}

// WatchForeground implements platform.DesktopAdapter: the current foreground
// first, then one Foreground per change of the activated toplevel. The
// channel closes when ctx ends or the connection dies.
func (a *Adapter) WatchForeground(ctx context.Context) (<-chan platform.Foreground, error) {
	if err := a.ready("watch foreground"); err != nil {
		return nil, err
	}
	w := &watcher{notify: make(chan struct{}, 1)}
	a.mu.Lock()
	a.watchers[w] = struct{}{}
	a.mu.Unlock()
	out := make(chan platform.Foreground, 1)
	go func() {
		defer close(out)
		defer func() {
			a.mu.Lock()
			delete(a.watchers, w)
			a.mu.Unlock()
		}()
		first := true
		var last platform.Foreground
		for {
			if a.client.err() != nil {
				return
			}
			fg, _ := a.ObserveForeground(ctx)
			if first || fg.Known != last.Known || fg.Window.ID != last.Window.ID || fg.Window.AppID != last.Window.AppID {
				first, last = false, fg
				select {
				case out <- fg:
				case <-ctx.Done():
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-w.notify:
			}
		}
	}()
	return out, nil
}

func (a *Adapter) notifyWatchers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for w := range a.watchers {
		select {
		case w.notify <- struct{}{}:
		default:
		}
	}
}

// ListWindows implements platform.DesktopAdapter. Minimized toplevels are
// listed with Mapped=false.
func (a *Adapter) ListWindows(context.Context) ([]platform.WindowInfo, error) {
	if err := a.ready("list windows"); err != nil {
		return nil, err
	}
	return a.client.windows(), nil
}

// Activate implements platform.DesktopAdapter with
// zwlr_foreign_toplevel_handle_v1.activate on the bound seat.
func (a *Adapter) Activate(_ context.Context, w platform.WindowID) error {
	if err := a.ready("activate"); err != nil {
		return err
	}
	return a.client.activate(w)
}

// RequestClose implements platform.DesktopAdapter with
// zwlr_foreign_toplevel_handle_v1.close (the app may still ask to save).
func (a *Adapter) RequestClose(_ context.Context, w platform.WindowID) error {
	if err := a.ready("request close"); err != nil {
		return err
	}
	return a.client.close(w)
}

// SetFullscreen implements platform.Fullscreener (handle version 2+).
func (a *Adapter) SetFullscreen(_ context.Context, w platform.WindowID, on bool) error {
	if err := a.ready("fullscreen"); err != nil {
		return err
	}
	return a.client.setFullscreen(w, on)
}

// DeliverKey implements platform.DesktopAdapter: input is never injected on
// Wayland (see reasonInput).
func (a *Adapter) DeliverKey(context.Context, platform.WindowID, platform.Key) error {
	return fmt.Errorf("%w: %s", platform.ErrUnsupported, reasonInput)
}

// Close implements platform.DesktopAdapter; safe to call more than once.
func (a *Adapter) Close() error {
	if a.client != nil {
		a.client.shutdown()
	}
	return nil
}
