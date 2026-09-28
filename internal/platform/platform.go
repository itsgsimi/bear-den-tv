// Package platform defines the desktop-session seam: foreground observation,
// window activation, bounded input delivery, and lock observation. Concrete
// backends live in subpackages (x11, fake, wayland). The coordinator never
// routes input to a window the backend cannot identify as the intended target.
package platform

import (
	"context"
	"errors"
)

// WindowID is a backend-specific window identity (X11 window id, or an opaque
// handle for other backends).
type WindowID uint64

// Key is a logical input the coordinator may deliver. Backends map these to
// keysyms/keycodes; nothing else can be injected.
type Key string

const (
	KeyUp        Key = "up"
	KeyDown      Key = "down"
	KeyLeft      Key = "left"
	KeyRight     Key = "right"
	KeySelect    Key = "select"     // Return
	KeyBack      Key = "back"       // Escape (Back for Leanback-style UIs)
	KeyPlayPause Key = "play_pause" // XF86AudioPlay
	KeyPause     Key = "pause"      // XF86AudioPause
	KeyPlay      Key = "play"       // XF86AudioPlay (only where idempotent play exists)
	KeyRewind    Key = "rewind"     // XF86AudioRewind
	KeyForward   Key = "forward"    // XF86AudioForward
)

// WindowInfo is what the backend can observe about a top-level window.
// Title is redacted by the coordinator before it reaches phones.
type WindowInfo struct {
	ID         WindowID
	PID        int      // 0 when unknown (Flatpak windows report sandbox pids)
	Class      []string // WM_CLASS instance, class
	Title      string
	Mapped     bool
	Fullscreen bool
}

// Foreground is the observed active window. Known=false means the backend
// could not determine the foreground (no active window, unsupported, error).
type Foreground struct {
	Known  bool
	Window WindowInfo
}

// Capability describes one backend feature.
type Capability struct {
	Available bool
	Backend   string
	Reason    string
}

// Capability keys reported by DesktopAdapter.Capabilities.
const (
	CapObserveForeground = "observe_foreground"
	CapActivate          = "activate"
	CapInput             = "input"
	CapLockObservation   = "lock_observation"
	CapPhysicalHome      = "physical_home"
)

// ErrUnsupported is returned by backends that cannot perform an operation.
var ErrUnsupported = errors.New("platform: unsupported on this backend")

// ErrNotForeground is returned by DeliverKey when the window is not verified as
// the active window immediately before delivery.
var ErrNotForeground = errors.New("platform: target window is not in the foreground")

// DesktopAdapter is the DesktopAdapter contract from the specification (§9.2).
type DesktopAdapter interface {
	// Name identifies the backend (x11-ewmh-xtest, wayland-limited, fake).
	Name() string
	// DisplaySession is "x11", "wayland", or "unknown".
	DisplaySession() string
	// Capabilities reports the backend's features with reasons for unavailability.
	Capabilities() map[string]Capability
	// ObserveForeground returns the current active window, or Known=false.
	ObserveForeground(ctx context.Context) (Foreground, error)
	// WatchForeground emits a Foreground on every active-window change until ctx
	// is done. The channel is closed on exit.
	WatchForeground(ctx context.Context) (<-chan Foreground, error)
	// ListWindows lists mapped top-level windows.
	ListWindows(ctx context.Context) ([]WindowInfo, error)
	// Activate raises and focuses the window (EWMH _NET_ACTIVE_WINDOW).
	Activate(ctx context.Context, w WindowID) error
	// RequestClose asks the window to close normally (WM_DELETE_WINDOW).
	RequestClose(ctx context.Context, w WindowID) error
	// DeliverKey taps one logical key into w after re-verifying that w is the
	// active window; returns ErrNotForeground otherwise. Never leaves a key down.
	DeliverKey(ctx context.Context, w WindowID, k Key) error
	// Close releases backend resources.
	Close() error
}

// Fullscreener is implemented by backends that can ask the window manager to
// make a window fullscreen (EWMH _NET_WM_STATE_FULLSCREEN on X11). TV clients
// are put fullscreen when they come to the front; the request is idempotent.
type Fullscreener interface {
	SetFullscreen(ctx context.Context, w WindowID, on bool) error
}

// LockObserver reports whether the desktop session is locked. Input and private
// state must stop while locked; the coordinator never unlocks anything.
type LockObserver interface {
	Locked(ctx context.Context) (bool, error)
	// Watch emits the lock state on every change until ctx is done.
	Watch(ctx context.Context) (<-chan bool, error)
}

// AudioBackend controls the PC's default sink (labeled "PC volume", never TV volume).
type AudioBackend interface {
	Capability() Capability
	VolumeDelta(ctx context.Context, percent int) error
	SetMute(ctx context.Context, muted bool) error
}

// MediaPlayer is an MPRIS-style player the coordinator found for a running app.
type MediaPlayer interface {
	// Status returns Playing, Paused, Stopped, or Unknown.
	Status(ctx context.Context) (string, error)
	Pause(ctx context.Context) error
	Play(ctx context.Context) error
	SeekRelative(ctx context.Context, seconds int) error
	CanControl(ctx context.Context) (bool, error)
}

// MediaLocator finds players belonging to a running application instance.
type MediaLocator interface {
	// Find returns the player whose DesktopEntry or bus name matches match, or
	// (nil,false) when none exists. match is the Flatpak app id or desktop entry.
	Find(ctx context.Context, match string) (MediaPlayer, bool, error)
}
