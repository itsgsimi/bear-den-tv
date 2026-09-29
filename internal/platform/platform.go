// Package platform defines the desktop-session seam: foreground observation,
// window activation, bounded input delivery, and lock observation. Concrete
// backends live in subpackages (x11, fake, wayland). The coordinator never
// routes input to a window the backend cannot identify as the intended target.
package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
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
	// Letter keys some apps document as controls (RetroArch's default
	// retroarch.cfg: x = confirm, z = back, p = pause toggle). Never text:
	// text goes through text.submit to the shell only.
	KeyLetterX Key = "letter_x" // XK_x
	KeyLetterZ Key = "letter_z" // XK_z
	KeyLetterP Key = "letter_p" // XK_p
)

// WindowInfo is what the backend can observe about a top-level window.
// Title is redacted by the coordinator before it reaches phones.
type WindowInfo struct {
	ID    WindowID
	PID   int      // 0 when unknown (Flatpak windows report sandbox pids)
	Class []string // WM_CLASS instance, class (X11; nil on Wayland)
	// AppID is the Wayland toplevel app_id (xdg_toplevel.set_app_id, or the
	// X11 class for an XWayland window); empty on X11. Adapters match it the
	// way they match WM_CLASS (docs/decisions/0007-wayland-profile.md).
	AppID      string
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
	// Info reads what the player says it is playing: status, metadata,
	// position and rate. Fields the player does not report stay zero.
	Info(ctx context.Context) (MediaInfo, error)
	// Watch emits whenever the player signals a change of its metadata,
	// status or rate, or a seek, until ctx is done; the channel is then
	// closed. Signals are coalesced; the position itself is not signalled
	// while it advances, so readers re-read it (Info) on their own schedule.
	Watch(ctx context.Context) (<-chan struct{}, error)
}

// MediaInfo is one reading of a player (MPRIS PlaybackStatus, Metadata,
// Position, Rate). Title, Artists and Album name private media: the
// coordinator shows them only to authorized phones and never logs them.
type MediaInfo struct {
	// Status is Playing, Paused, Stopped or Unknown.
	Status string
	// Title is xesam:title; empty when the player reports none.
	Title string
	// Artists is xesam:artist; Album is xesam:album.
	Artists []string
	Album   string
	// Length is mpris:length; zero when unknown.
	Length time.Duration
	// Position is valid only when HasPosition is true.
	Position    time.Duration
	HasPosition bool
	// Rate is the playback rate; 1 when the player does not report it.
	Rate float64
}

// String describes the reading without its titles, so a log line or an
// error that formats it never names what is playing.
func (m MediaInfo) String() string {
	return fmt.Sprintf("media{status=%s title=[title] position=%v/%v rate=%v}", m.Status, m.Position, m.Length, m.Rate)
}

// LogValue implements slog.LogValuer with the titles redacted.
func (m MediaInfo) LogValue() slog.Value { return slog.StringValue(m.String()) }

// DisplayPower turns the TV's display off and on again (the sleep timer and
// display.off; X11 DPMS in platform/x11). Turning it off changes the
// desktop's own power settings, so an implementation captures them before
// its first Off and restores them exactly in On and in Close: the desktop
// must never be left blanking on its own afterwards (a film would go dark
// after the old idle timeout).
type DisplayPower interface {
	// Capability reports whether the display can be turned off, with a reason.
	Capability() Capability
	// Off captures the current power settings (once; a second Off keeps the
	// first capture) and turns the display off now.
	Off(ctx context.Context) error
	// On turns the display on and restores the captured settings exactly;
	// a no-op when nothing was captured.
	On(ctx context.Context) error
	// Status reads the display's power level and, where the backend can,
	// the time since the last user input on the TV.
	Status(ctx context.Context) (DisplayStatus, error)
	// Close restores the captured settings, if any, and releases the backend.
	Close() error
}

// DisplayStatus is one reading of the display.
type DisplayStatus struct {
	// On is true while the display is powered (any input wakes it).
	On bool
	// Idle is the time since the last keyboard or pointer input; valid only
	// when IdleKnown.
	Idle      time.Duration
	IdleKnown bool
}

// MediaMatch says which players belong to one app. Ownership is decided by
// the process that owns the player's bus name, never by the names a player
// reports about itself (docs/security.md "Now playing").
type MediaMatch struct {
	// FlatpakID: a player whose owning process runs in this Flatpak belongs
	// to the app. A player owned by another Flatpak never does, whatever it
	// calls itself.
	FlatpakID string
	// Names are the secondary, exact rules (the MPRIS DesktopEntry, or the
	// bus name org.mpris.MediaPlayer2.<name>[.instanceN]), used only for a
	// player whose owner runs outside any Flatpak, or when the locator
	// cannot see processes at all.
	Names []string
	// ProcessRoot, when positive, replaces both: the owning process must be
	// this process or one of its descendants (a web app's own browser, which
	// shares its Flatpak with other web apps).
	ProcessRoot int
}

// MediaLocator finds players belonging to a running application instance.
type MediaLocator interface {
	// Find returns the first player (in bus-name order) that belongs to the
	// app m describes, or (nil,false) when none does.
	Find(ctx context.Context, m MediaMatch) (MediaPlayer, bool, error)
}

// TVControl drives the TV over HDMI-CEC (platform/cec, ADR 0008): the wire
// in the HDMI cable, never the network. Every call is bounded by ctx and
// calls are serialized; a call that finds no usable adapter returns an error
// and the next Probe says why.
type TVControl interface {
	// Probe looks for the adapter and opens it, without sending anything on
	// the bus, and reports whether it is usable (Backend "hdmi-cec") or why not.
	Probe(ctx context.Context) Capability
	// PowerOn sends Image View On to the TV.
	PowerOn(ctx context.Context) error
	// Standby sends Standby to the TV.
	Standby(ctx context.Context) error
	// ActiveSource broadcasts Active Source with Bear Den's physical address,
	// so the TV switches to its input.
	ActiveSource(ctx context.Context) error
	// VolumeKey presses and releases one volume key (User Control Pressed,
	// then Released) on the audio system, or the TV when none answers.
	VolumeKey(ctx context.Context, k TVKey) error
	// PowerStatus asks the TV for its power status (Give Device Power Status)
	// and waits a bounded time for the answer.
	PowerStatus(ctx context.Context) (TVPower, error)
	// Close gives up the logical address this process claimed, if any, and
	// closes the adapter; a later Probe opens it again.
	Close() error
}

// TVKey is a volume key sent over HDMI-CEC (CEC User Control codes).
type TVKey string

const (
	TVVolumeUp   TVKey = "volume_up"   // 0x41
	TVVolumeDown TVKey = "volume_down" // 0x42
	TVMute       TVKey = "mute"        // 0x65 Mute Function (explicit, not the 0x43 toggle)
	TVUnmute     TVKey = "unmute"      // 0x66 Restore Volume Function
)

// TVPower is what the TV reports about its power.
type TVPower string

const (
	TVPowerOn      TVPower = "on"      // on, or in transition standby → on
	TVPowerStandby TVPower = "standby" // standby, or in transition on → standby
	TVPowerUnknown TVPower = "unknown"
)
