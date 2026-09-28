// Package detect classifies the display session from the process environment
// and builds the matching desktop adapter: x11 when an X display answers,
// wayland-limited under Wayland, and an "unavailable" adapter whose
// capabilities carry the reason otherwise. Nothing here panics without a
// display; a missing or unreachable display is an ordinary reduced profile.
package detect

import (
	"context"
	"fmt"
	"os"
	"strings"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/wayland"
	"bear-den-tv/internal/platform/x11"
)

// Display session values as reported by platform.DesktopAdapter.DisplaySession.
const (
	SessionX11     = "x11"
	SessionWayland = "wayland"
	SessionUnknown = "unknown"
)

// SessionVars is the closed list of environment variables imported from the
// process: what detection and the session-bound adapters need, nothing else.
var SessionVars = []string{
	"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY",
	"XDG_SESSION_TYPE", "XDG_SESSION_ID", "XDG_RUNTIME_DIR", "XDG_SEAT", "XDG_VTNR",
	"DBUS_SESSION_BUS_ADDRESS", "XDG_CURRENT_DESKTOP", "DESKTOP_SESSION",
}

// SessionEnv imports the SessionVars that are set in the current process.
func SessionEnv() map[string]string {
	return SessionEnvFrom(os.LookupEnv)
}

// SessionEnvFrom imports SessionVars through lookup (os.LookupEnv shape).
func SessionEnvFrom(lookup func(string) (string, bool)) map[string]string {
	env := map[string]string{}
	for _, k := range SessionVars {
		if v, ok := lookup(k); ok && v != "" {
			env[k] = v
		}
	}
	return env
}

// Classify derives the display session from an imported environment:
// Wayland when XDG_SESSION_TYPE says so or WAYLAND_DISPLAY is set, else X11
// when XDG_SESSION_TYPE says so or DISPLAY is set, else unknown.
func Classify(env map[string]string) string {
	t := strings.ToLower(env["XDG_SESSION_TYPE"])
	switch {
	case t == SessionWayland || env["WAYLAND_DISPLAY"] != "":
		return SessionWayland
	case t == SessionX11 || env["DISPLAY"] != "":
		return SessionX11
	}
	return SessionUnknown
}

// Detect classifies the current process environment and returns the imported
// session variables alongside the verdict.
func Detect() (displaySession string, env map[string]string) {
	env = SessionEnv()
	return Classify(env), env
}

// Options configures NewDesktopAdapter.
type Options struct {
	// Env is the imported session environment; nil uses SessionEnv().
	Env map[string]string
	// Lock supplies the lock_observation capability to the X11 adapter.
	Lock x11.LockCapability
}

// NewDesktopAdapter builds the adapter for the detected session. It never
// fails: when the X display cannot be reached the result is an Unavailable
// adapter whose Capabilities explain why.
func NewDesktopAdapter(ctx context.Context, opts Options) platform.DesktopAdapter {
	env := opts.Env
	if env == nil {
		env = SessionEnv()
	}
	switch Classify(env) {
	case SessionWayland:
		return wayland.New()
	case SessionX11:
		a, err := x11.New(ctx, x11.Options{Display: env["DISPLAY"], Lock: opts.Lock})
		if err != nil {
			return NewUnavailable(SessionX11, err.Error())
		}
		return a
	}
	return NewUnavailable(SessionUnknown, "no display session detected (DISPLAY, WAYLAND_DISPLAY, and XDG_SESSION_TYPE unset)")
}

// UnavailableName is the backend name of the Unavailable adapter.
const UnavailableName = "unavailable"

// Unavailable is the adapter used when no backend could be constructed. Every
// capability is unavailable with the construction reason and every operation
// returns platform.ErrUnsupported.
type Unavailable struct {
	session string
	reason  string
}

// NewUnavailable returns an adapter reporting session with reason.
func NewUnavailable(session, reason string) *Unavailable {
	return &Unavailable{session: session, reason: reason}
}

// Name implements platform.DesktopAdapter.
func (*Unavailable) Name() string { return UnavailableName }

// DisplaySession implements platform.DesktopAdapter.
func (u *Unavailable) DisplaySession() string { return u.session }

// Reason is why no backend was constructed.
func (u *Unavailable) Reason() string { return u.reason }

// Capabilities implements platform.DesktopAdapter.
func (u *Unavailable) Capabilities() map[string]platform.Capability {
	caps := map[string]platform.Capability{}
	for _, k := range []string{platform.CapObserveForeground, platform.CapActivate, platform.CapInput, platform.CapLockObservation, platform.CapPhysicalHome} {
		caps[k] = platform.Capability{Backend: UnavailableName, Reason: u.reason}
	}
	return caps
}

func (u *Unavailable) err(op string) error {
	return fmt.Errorf("%w: %s: %s", platform.ErrUnsupported, op, u.reason)
}

// ObserveForeground implements platform.DesktopAdapter.
func (u *Unavailable) ObserveForeground(context.Context) (platform.Foreground, error) {
	return platform.Foreground{}, u.err("observe foreground")
}

// WatchForeground implements platform.DesktopAdapter.
func (u *Unavailable) WatchForeground(context.Context) (<-chan platform.Foreground, error) {
	return nil, u.err("watch foreground")
}

// ListWindows implements platform.DesktopAdapter.
func (u *Unavailable) ListWindows(context.Context) ([]platform.WindowInfo, error) {
	return nil, u.err("list windows")
}

// Activate implements platform.DesktopAdapter.
func (u *Unavailable) Activate(context.Context, platform.WindowID) error {
	return u.err("activate")
}

// RequestClose implements platform.DesktopAdapter.
func (u *Unavailable) RequestClose(context.Context, platform.WindowID) error {
	return u.err("request close")
}

// DeliverKey implements platform.DesktopAdapter.
func (u *Unavailable) DeliverKey(context.Context, platform.WindowID, platform.Key) error {
	return u.err("deliver key")
}

// Close implements platform.DesktopAdapter.
func (*Unavailable) Close() error { return nil }
