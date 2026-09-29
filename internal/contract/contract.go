// Package contract holds the Go representation of protocol 1 (contracts/*.json)
// and validates raw messages against the embedded JSON Schemas. It is the only
// place where wire shapes are defined for Go code; every other package imports it.
package contract

import "log/slog"

// Protocol is the single supported protocol version.
const Protocol = 1

// Outcome is an action result outcome. Never upgrade without evidence.
type Outcome string

const (
	OutcomeAccepted  Outcome = "accepted"
	OutcomeDelivered Outcome = "delivered"
	OutcomeObserved  Outcome = "observed"
	OutcomeFailed    Outcome = "failed"
)

// Code is a machine-readable result code; "ok" for non-failed outcomes.
type Code string

const (
	CodeOK                  Code = "ok"
	CodeInvalid             Code = "invalid"
	CodeUnsupportedProtocol Code = "unsupported_protocol"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeRateLimited         Code = "rate_limited"
	CodeDuplicateMismatch   Code = "duplicate_mismatch"
	CodeStaleEpoch          Code = "stale_epoch"
	CodeNoTarget            Code = "no_target"
	CodeUnknownForeground   Code = "unknown_foreground"
	CodeLocked              Code = "locked"
	CodeUnsupported         Code = "unsupported"
	CodeTargetUnfocused     Code = "target_unfocused"
	CodeBusy                Code = "busy"
	CodeLaunchFailed        Code = "launch_failed"
	CodeTimeout             Code = "timeout"
	CodeInternal            Code = "internal"
	// CodeDisplayOff: the display was off and this press only turned it on
	// again; the action itself was not applied (contracts/actions.md).
	CodeDisplayOff Code = "display_off"
)

// Action names (contracts/actions.md).
const (
	ActionNavUp        = "nav.up"
	ActionNavDown      = "nav.down"
	ActionNavLeft      = "nav.left"
	ActionNavRight     = "nav.right"
	ActionSelect       = "select"
	ActionBack         = "back"
	ActionHome         = "home"
	ActionAppLaunch    = "app.launch"
	ActionAppClose     = "app.close"
	ActionMediaPlay    = "media.play"
	ActionMediaPause   = "media.pause"
	ActionMediaSeek    = "media.seek_relative"
	ActionAudioVolume  = "audio.volume_delta"
	ActionAudioMute    = "audio.mute"
	ActionTextSubmit   = "text.submit"
	ActionShellRestart = "shell.restart"
	ActionSleepTimer   = "power.sleep_timer"
	ActionDisplayOff   = "display.off"
	// ActionTVPower turns the TV on or to standby over HDMI-CEC (cec.go).
	ActionTVPower = "tv.power"
	// Pointer actions: the phone's touchpad for web apps (contracts/actions.md
	// "Pointer"). Only web adapters offer them, never to guests.
	ActionPointerMove   = "pointer.move"
	ActionPointerClick  = "pointer.click"
	ActionPointerScroll = "pointer.scroll"
	// App installs from Flathub, per user (contracts/actions.md "App
	// installs"): owner phones only (OwnerActions), never guests.
	ActionAppInstall       = "app.install"
	ActionAppInstallCancel = "app.install_cancel"
)

// AllActions lists every action name in protocol 1, in contract order.
var AllActions = []string{
	ActionNavUp, ActionNavDown, ActionNavLeft, ActionNavRight, ActionSelect, ActionBack, ActionHome,
	ActionAppLaunch, ActionAppClose, ActionMediaPlay, ActionMediaPause, ActionMediaSeek,
	ActionAudioVolume, ActionAudioMute, ActionTextSubmit, ActionShellRestart,
	ActionSleepTimer, ActionDisplayOff, ActionTVPower,
	ActionPointerMove, ActionPointerClick, ActionPointerScroll,
	ActionAppInstall, ActionAppInstallCancel,
}

// OwnerActions need the owner permission from a phone (contracts/actions.md
// "Who may send"); app.close with force is the one conditional case and is
// checked in the router.
var OwnerActions = map[string]bool{
	ActionShellRestart:     true,
	ActionAppInstall:       true,
	ActionAppInstallCancel: true,
}

// IsPointer reports whether the action is one of the touchpad's pointer actions.
func IsPointer(action string) bool {
	switch action {
	case ActionPointerMove, ActionPointerClick, ActionPointerScroll:
		return true
	}
	return false
}

// PointerRates is how many of each pointer action one device may send per
// second (contracts/actions.md "Pointer"); more is failed/rate_limited. A
// phone coalesces its finger's movement to one move per animation frame.
var PointerRates = map[string]int{ActionPointerMove: 60, ActionPointerScroll: 30, ActionPointerClick: 5}

// SleepChoices are the power.sleep_timer minutes a timer may be set to
// (0 cancels), in the order phones and the TV offer them.
var SleepChoices = []int{15, 30, 45, 60, 90, 120}

// IsNav reports whether the action is a directional navigation action (holdable).
func IsNav(action string) bool {
	switch action {
	case ActionNavUp, ActionNavDown, ActionNavLeft, ActionNavRight:
		return true
	}
	return false
}

// IgnoresStaleEpoch reports whether the action is an explicit escape that may
// ignore an obsolete context epoch (still authorized, still refused when
// locked). The power actions never touch the window in front, so what was
// in front when the button was drawn does not matter to them.
func IgnoresStaleEpoch(action string) bool {
	switch action {
	case ActionHome, ActionAppLaunch, ActionShellRestart, ActionSleepTimer, ActionDisplayOff, ActionTVPower,
		ActionAppInstall, ActionAppInstallCancel:
		return true
	}
	return false
}

// Permission is a device permission level. Higher levels include lower ones.
// guest is the lowest: a time-limited guest pass that never comes with
// another permission, so a guest never holds controller (docs/security.md).
type Permission string

const (
	PermGuest        Permission = "guest"
	PermController   Permission = "controller"
	PermLayoutEditor Permission = "layout_editor"
	PermOwner        Permission = "owner"
)

// Rank orders permissions for comparison; unknown permissions rank lowest.
func (p Permission) Rank() int {
	switch p {
	case PermGuest:
		return 1
	case PermController:
		return 2
	case PermLayoutEditor:
		return 3
	case PermOwner:
		return 4
	}
	return 0
}

// GuestActions is the guest-pass allow-list (contracts/actions.md "Who may
// send"): what a visitor watching the same screen needs, nothing that closes
// apps, restarts the shell, powers anything off or changes settings. Every
// action not listed here, including actions added later, is refused to
// guests: add a new action here only on purpose.
var GuestActions = map[string]bool{
	ActionNavUp: true, ActionNavDown: true, ActionNavLeft: true, ActionNavRight: true,
	ActionSelect: true, ActionBack: true, ActionHome: true,
	ActionAppLaunch:   true,
	ActionMediaPlay:   true,
	ActionMediaPause:  true,
	ActionMediaSeek:   true,
	ActionAudioVolume: true,
	ActionAudioMute:   true,
	ActionTextSubmit:  true,
}

// GuestMayUse reports whether a guest pass may send action.
func GuestMayUse(action string) bool { return GuestActions[action] }

// Guest pass durations (IPC pair.issue "pass", `bear-den-tv pair --guest`).
const (
	PassTonight = "tonight" // until 04:00 local time the next morning
	Pass24h     = "24h"
	Pass7d      = "7d"
)

// PassDurations lists the accepted guest pass durations, in picker order.
var PassDurations = []string{PassTonight, Pass24h, Pass7d}

// ActionRequest is contracts/action.schema.json#/$defs/request.
type ActionRequest struct {
	Protocol     int            `json:"protocol"`
	RequestID    string         `json:"request_id"`
	ContextEpoch int64          `json:"context_epoch"`
	Target       string         `json:"target"`
	Action       string         `json:"action"`
	Args         map[string]any `json:"args"`
}

// TargetInfo describes the control target an action was resolved against.
type TargetInfo struct {
	Kind  string  `json:"kind"` // shell | app | unknown | locked | none
	AppID *string `json:"app_id"`
	Label string  `json:"label"`
}

// ActionResult is contracts/action.schema.json#/$defs/result.
type ActionResult struct {
	Protocol     int            `json:"protocol"`
	RequestID    string         `json:"request_id"`
	Outcome      Outcome        `json:"outcome"`
	Code         Code           `json:"code"`
	Message      string         `json:"message"`
	ContextEpoch int64          `json:"context_epoch"`
	Target       TargetInfo     `json:"target"`
	Detail       map[string]any `json:"detail"`
}

// Failed builds a failed result for a request.
func Failed(req ActionRequest, epoch int64, target TargetInfo, code Code, msg string) ActionResult {
	return ActionResult{Protocol: Protocol, RequestID: req.RequestID, Outcome: OutcomeFailed, Code: code, Message: msg, ContextEpoch: epoch, Target: target, Detail: map[string]any{}}
}

// Result builds a non-failed result for a request.
func Result(req ActionRequest, outcome Outcome, epoch int64, target TargetInfo, detail map[string]any) ActionResult {
	if detail == nil {
		detail = map[string]any{}
	}
	return ActionResult{Protocol: Protocol, RequestID: req.RequestID, Outcome: outcome, Code: CodeOK, Message: "", ContextEpoch: epoch, Target: target, Detail: detail}
}

// HoldMessage is a WebSocket hold lease message (contracts/actions.md#holds).
type HoldMessage struct {
	Type         string `json:"type"` // hold.start | hold.renew | hold.stop
	HoldID       string `json:"hold_id"`
	Action       string `json:"action,omitempty"`
	ContextEpoch int64  `json:"context_epoch,omitempty"`
}

// Capability reports whether an action is available for the current target.
type Capability struct {
	Available bool   `json:"available"`
	Backend   string `json:"backend,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Holdable  bool   `json:"holdable,omitempty"`
}

// UI is layout.schema.json#/properties/ui.
type UI struct {
	Theme             string  `json:"theme"`
	Accent            string  `json:"accent"`
	Background        string  `json:"background"`
	TextScale         float64 `json:"text_scale"`
	TileDensity       string  `json:"tile_density"`
	SafeMarginPercent float64 `json:"safe_margin_percent"`
	ReducedMotion     bool    `json:"reduced_motion"`
	HighContrastFocus bool    `json:"high_contrast_focus"`
	HeroEnabled       bool    `json:"hero_enabled"`
	ClockEnabled      bool    `json:"clock_enabled"`
	ArtStyle          string  `json:"art_style,omitempty"` // pixel (default when empty) | classic
	AppIcons          string  `json:"app_icons,omitempty"` // app (default when empty) | bear_den
}

// App icon choices (UI.AppIcons): the app's own icon (its installed
// Flatpak's export) or Bear Den's drawings; the owner's brand folder wins
// either way (docs/THEMES.md → App icons).
const (
	AppIconsApp     = "app"
	AppIconsBearDen = "bear_den"
)

// IconsOf returns the app icon choice, empty meaning app.
func (u UI) IconsOf() string {
	if u.AppIcons == AppIconsBearDen {
		return AppIconsBearDen
	}
	return AppIconsApp
}

// Art styles (UI.ArtStyle): pixel art on one grid, or smooth classic art.
const (
	ArtPixel   = "pixel"
	ArtClassic = "classic"
)

// Classic reports whether the Classic art style is chosen (empty means pixel).
func (u UI) Classic() bool { return u.ArtStyle == ArtClassic }

// Section is layout.schema.json#/properties/sections/items.
type Section struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	ApplicationIDs []string `json:"application_ids,omitempty"`
	HideWhenEmpty  bool     `json:"hide_when_empty"`
}

// Layout is the portable, editable subset of the configuration.
type Layout struct {
	UI       UI        `json:"ui"`
	Sections []Section `json:"sections"`
}

// SessionState is state.schema.json#/properties/session.
type SessionState struct {
	Locked         bool   `json:"locked"`
	DisplaySession string `json:"display_session"`
	DesktopAdapter string `json:"desktop_adapter"`
	ShellConnected bool   `json:"shell_connected"`
	ShellState     string `json:"shell_state"`
}

// Target is state.schema.json#/properties/target.
type Target struct {
	Kind        string  `json:"kind"`
	AppID       *string `json:"app_id"`
	Label       string  `json:"label"`
	Observed    bool    `json:"observed"`
	WindowTitle *string `json:"window_title,omitempty"`
}

// Info converts a Target to the TargetInfo carried in action results.
func (t Target) Info() TargetInfo { return TargetInfo{Kind: t.Kind, AppID: t.AppID, Label: t.Label} }

// Focus is the shell's remembered focus by stable ids.
type Focus struct {
	SectionID *string `json:"section_id"`
	ItemID    *string `json:"item_id"`
}

// ShellState is state.schema.json#/properties/shell.
type ShellState struct {
	Screen string `json:"screen"`
	Focus  Focus  `json:"focus"`
}

// AppState is one entry of state.schema.json#/properties/applications.
type AppState struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Adapter      string  `json:"adapter"`
	Installed    bool    `json:"installed"`
	Version      *string `json:"version"`
	Installation string  `json:"installation"`
	Running      bool    `json:"running"`
	Foreground   bool    `json:"foreground"`
	LaunchState  string  `json:"launch_state"`
	LastError    *string `json:"last_error"`
	// Hidden: an optional app (config hide_when_missing) that is not
	// installed; the shell and phones draw no tile for it.
	Hidden bool `json:"hidden,omitempty"`
	// Enabled is present only for apps the owner can turn on and off on the
	// TV (web adapters, Settings → Streaming sites); false means turned off.
	Enabled *bool `json:"enabled,omitempty"`
	// Install is the app's Flatpak install from Flathub; shell and owner
	// phones only (state.schema.json#/$defs/install).
	Install *Install `json:"install,omitempty"`
}

// Install states (state.schema.json#/$defs/install).
const (
	InstallNone        = "none"
	InstallAvailable   = "available"
	InstallPreparing   = "preparing"
	InstallDownloading = "downloading"
	InstallInstalling  = "installing"
	InstallFailed      = "failed"
	InstallDone        = "done"
)

// Install phases.
const (
	PhaseIdle      = ""
	PhaseChecking  = "checking"
	PhaseRuntime   = "runtime"
	PhaseApp       = "app"
	PhaseFinishing = "finishing"
)

// DRM states of a streaming web app's profile (Widevine).
const (
	DRMReady     = "ready"
	DRMPreparing = "preparing"
	DRMPending   = "pending"
)

// Install is state.schema.json#/$defs/install.
type Install struct {
	State     string `json:"state"`
	Progress  int    `json:"progress"`
	Phase     string `json:"phase"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	DiskBytes *int64 `json:"disk_bytes,omitempty"`
	Message   string `json:"message,omitempty"`
	DRM       string `json:"drm,omitempty"`
}

// AppsState is state.schema.json#/properties/apps (shell and owner phones).
type AppsState struct {
	AutoUpdate bool `json:"auto_update"`
	// Browser and StreamingBrowser are config apps.browser and
	// apps.streaming_browser; Browsers is the adapter table's browsers.
	Browser          string          `json:"browser,omitempty"`
	StreamingBrowser string          `json:"streaming_browser,omitempty"`
	Browsers         []BrowserOption `json:"browsers,omitempty"`
}

// BrowserOption is state.schema.json#/properties/apps/properties/browsers/items.
type BrowserOption struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	FlatpakID           string `json:"flatpak_id"`
	StreamingUnverified bool   `json:"streaming_unverified"`
}

// HoldState is state.schema.json#/properties/remote/properties/hold.
type HoldState struct {
	Active   bool    `json:"active"`
	DeviceID *string `json:"device_id"`
	Action   *string `json:"action"`
}

// Limits is state.schema.json#/properties/remote/properties/limits.
type Limits struct {
	RepeatDelayMs   int `json:"repeat_delay_ms"`
	RepeatHz        int `json:"repeat_hz"`
	HoldRenewMs     int `json:"hold_renew_ms"`
	HoldExpiryMs    int `json:"hold_expiry_ms"`
	MaxMessageBytes int `json:"max_message_bytes"`
}

// DefaultLimits are the contract's starting values (tune after remote testing).
var DefaultLimits = Limits{RepeatDelayMs: 350, RepeatHz: 6, HoldRenewMs: 200, HoldExpiryMs: 600, MaxMessageBytes: 16384}

// RemoteState is state.schema.json#/properties/remote.
type RemoteState struct {
	Enabled           bool     `json:"enabled"`
	Transport         string   `json:"transport"`
	Listening         bool     `json:"listening"`
	Addresses         []string `json:"addresses"`
	HTTPS             bool     `json:"https"`
	HTTPLayoutEditing bool     `json:"http_layout_editing"`
	PairedDeviceCount int      `json:"paired_device_count"`
	// NowPlaying mirrors config remote.now_playing (default true): whether
	// phones may see state.now_playing. The coordinator always sets it (nil
	// only in documents from older producers); not sensitive.
	NowPlaying *bool     `json:"now_playing,omitempty"`
	Hold       HoldState `json:"hold"`
	Limits     Limits    `json:"limits"`
}

// Me is the viewing phone's own session.
type Me struct {
	DeviceID        string       `json:"device_id"`
	DeviceName      string       `json:"device_name"`
	Permissions     []Permission `json:"permissions"`
	TransportSecure bool         `json:"transport_secure"`
	// ExpiresAtMs is set for guest passes only: when the pass ends, Unix
	// epoch milliseconds on the coordinator's wall clock.
	ExpiresAtMs *int64 `json:"expires_at_ms,omitempty"`
}

// Pairing is the invitation currently displayed on the TV (shell only).
type Pairing struct {
	Active       bool     `json:"active"`
	Code         *string  `json:"code"`
	URL          *string  `json:"url"`
	QRModules    []string `json:"qr_modules"`
	ExpiresInS   int      `json:"expires_in_s"`
	AttemptsLeft int      `json:"attempts_left"`
	// Guest is true when the live invitation is a guest pass; PassExpiresAtMs
	// is then when that pass will end (Unix epoch ms, wall clock).
	Guest           bool   `json:"guest,omitempty"`
	PassExpiresAtMs *int64 `json:"pass_expires_at_ms,omitempty"`
}

// Device is a paired device record as shown to owners and the shell.
type Device struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	Connected   bool         `json:"connected"`
	LastSeenMs  int64        `json:"last_seen_ms"`
	CreatedAt   string       `json:"created_at"`
	// Guest marks a guest pass; ExpiresAtMs is when it ends and the device
	// is revoked (Unix epoch ms, wall clock), nil for family phones.
	Guest       bool   `json:"guest"`
	ExpiresAtMs *int64 `json:"expires_at_ms"`
}

// LayoutPending describes a timed layout change awaiting confirmation.
type LayoutPending struct {
	Revision         int64  `json:"revision"`
	PreviousRevision int64  `json:"previous_revision"`
	ExpiresInS       int    `json:"expires_in_s"`
	Source           string `json:"source"`
}

// Notification is a transient message shown on the TV and phones.
type Notification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedMs int64  `json:"created_ms"`
}

// ContentItem is one optional provider item (Plex connector).
type ContentItem struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Subtitle   string   `json:"subtitle"`
	Artwork    *string  `json:"artwork"`
	Progress   *float64 `json:"progress"`
	OpenAction string   `json:"open_action"`
	Demo       bool     `json:"demo"`
}

// ContentSection is provider items for one configured section.
type ContentSection struct {
	SectionID string        `json:"section_id"`
	Items     []ContentItem `json:"items"`
}

// Content is state.schema.json#/properties/content.
type Content struct {
	Provider string           `json:"provider"`
	Status   string           `json:"status"`
	Message  string           `json:"message"`
	Sections []ContentSection `json:"sections"`
}

// Appearance is the look a paired phone mirrors from the TV: the themed
// world (layout ui.background), the style (ui.theme) and the accent.
type Appearance struct {
	Background string            `json:"background"` // canonical theme id
	Theme      string            `json:"theme"`      // style: den-dark | plain-dark | performance
	Accent     string            `json:"accent"`
	Name       string            `json:"name,omitempty"`
	Pixel      bool              `json:"pixel"`               // the backdrop in use is pixel art: draw without smoothing
	ArtStyle   string            `json:"art_style,omitempty"` // pixel | classic (layout ui.art_style)
	AppIcons   string            `json:"app_icons,omitempty"` // app | bear_den (layout ui.app_icons)
	Palette    map[string]string `json:"palette,omitempty"`
	Focus      *AppearanceFocus  `json:"focus,omitempty"`
	Phone      *AppearancePhone  `json:"phone,omitempty"`
	Themes     []ThemeSummary    `json:"themes,omitempty"`
}

// AppearanceFocus is the phone's tile decoration: the theme's focus style and
// its tip ornament (a same-origin URL under /themes/).
type AppearanceFocus struct {
	Style      string `json:"style"`
	Tip        string `json:"tip,omitempty"`
	TipUpright bool   `json:"tip_upright,omitempty"`
}

// AppearancePhone is the phone's backdrop (same-origin URL), veil and particles.
type AppearancePhone struct {
	Backdrop  string `json:"backdrop,omitempty"`
	Veil      string `json:"veil,omitempty"`
	Particles string `json:"particles,omitempty"`
}

// Playback is the latest playback detection test for the TV's Settings →
// Playback screen (internal/applications/tuning): the box, the display, and
// per app what it decodes in hardware, what to expect, caveats and whether
// the best settings are applied.
type Playback struct {
	AtMs    int64          `json:"at_ms"`
	Summary string         `json:"summary"`
	Display string         `json:"display,omitempty"`
	Notes   []PlaybackNote `json:"notes"`
	Apps    []PlaybackApp  `json:"apps"`
}

// PlaybackNote is a caveat; Level is "info" or "warn".
type PlaybackNote struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// PlaybackApp is one app's line in the report.
type PlaybackApp struct {
	Label    string         `json:"label"`
	Adapter  string         `json:"adapter"`
	Hardware []string       `json:"hardware"`
	Expect   []string       `json:"expect"`
	Notes    []PlaybackNote `json:"notes"`
	Status   string         `json:"status"` // tuned | applied | pending | off | error | suggested
	Changes  []string       `json:"changes"`
	// Settings are the ones the owner may adjust by hand (Settings →
	// Advanced playback), with only the options this box can handle.
	Settings []PlaybackSetting `json:"settings,omitempty"`
}

// PlaybackSetting is one adjustable setting: Auto is detection's choice,
// Value the one in effect (the owner's override while it is offered, else
// Auto), Overridden whether the owner chose it by hand.
type PlaybackSetting struct {
	ID         string           `json:"id"`
	Label      string           `json:"label"`
	Auto       string           `json:"auto"`
	Value      string           `json:"value"`
	Overridden bool             `json:"overridden"`
	Options    []PlaybackOption `json:"options"`
}

// PlaybackOption is one offered value; Note says what it costs here.
type PlaybackOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
}

// ThemeSummary names an installed theme for pickers.
type ThemeSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Accent string `json:"accent,omitempty"`
}

// State is contracts/state.schema.json. Optional pointer fields are omitted
// when the viewer may not see them; Devices is a pointer so an owner/shell
// view can carry an explicit empty list while other viewers omit the key.
type State struct {
	Protocol       int                   `json:"protocol"`
	ContextEpoch   int64                 `json:"context_epoch"`
	GeneratedAtMs  int64                 `json:"generated_at_ms"`
	DeviceName     string                `json:"device_name"`
	DevMode        bool                  `json:"dev_mode"`
	ConfigRevision int64                 `json:"config_revision"`
	Session        SessionState          `json:"session"`
	Target         Target                `json:"target"`
	Capabilities   map[string]Capability `json:"capabilities"`
	Shell          ShellState            `json:"shell"`
	Applications   []AppState            `json:"applications"`
	Remote         RemoteState           `json:"remote"`
	Me             *Me                   `json:"me,omitempty"`
	Appearance     *Appearance           `json:"appearance,omitempty"`
	Pairing        *Pairing              `json:"pairing,omitempty"`
	Devices        *[]Device             `json:"devices,omitempty"`
	Layout         *Layout               `json:"layout,omitempty"`
	LayoutPending  *LayoutPending        `json:"layout_pending,omitempty"`
	Notifications  []Notification        `json:"notifications"`
	Content        *Content              `json:"content,omitempty"`
	Playback       *Playback             `json:"playback,omitempty"`
	Weather        *Weather              `json:"weather,omitempty"`
	NowPlaying     *NowPlaying           `json:"now_playing,omitempty"`
	Power          *Power                `json:"power,omitempty"`
	CEC            *CEC                  `json:"cec,omitempty"`
	Plex           *Plex                 `json:"plex,omitempty"`
	Achievements   *Achievements         `json:"achievements,omitempty"`
	Apps           *AppsState            `json:"apps,omitempty"`
}

// Achievements is state.schema.json#/properties/achievements: Den badges
// (internal/achievements). Only ids, counts and local days; Celebrate is for
// the shell view only.
type Achievements struct {
	Enabled   bool            `json:"enabled"`
	Earned    []EarnedBadge   `json:"earned"`
	Progress  []BadgeProgress `json:"progress"`
	Celebrate []string        `json:"celebrate,omitempty"`
}

// EarnedBadge is one earned badge and the local day (YYYY-MM-DD) it was earned.
type EarnedBadge struct {
	ID  string `json:"id"`
	Day string `json:"day"`
}

// BadgeProgress is how far one badge is: Count of Goal (Count <= Goal).
type BadgeProgress struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	Goal  int    `json:"goal"`
}

// Display power states (state.schema.json#/properties/power/display).
const (
	DisplayOn  = "on"
	DisplayOff = "off"
)

// Power is state.schema.json#/properties/power: the sleep timer and the
// display. SleepAtMs is in the coordinator monotonic milliseconds of
// GeneratedAtMs; nil when no timer is set.
type Power struct {
	SleepAtMs    *int64         `json:"sleep_at_ms"`
	SleepMinutes int            `json:"sleep_minutes,omitempty"`
	Warning      bool           `json:"warning"`
	Display      string         `json:"display"` // on | off
	Suspend      *SuspendReport `json:"suspend,omitempty"`
}

// SuspendReport says whether the box could be suspended, and why not.
type SuspendReport struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Now-playing statuses (state.schema.json#/properties/now_playing/status).
const (
	NowPlayingPlaying = "playing"
	NowPlayingPaused  = "paused"
	NowPlayingStopped = "stopped"
)

// NowPlayingTextMax is the longest title or subtitle sent, in characters.
const NowPlayingTextMax = 200

// NowPlaying is state.schema.json#/properties/now_playing: what an app's own
// MPRIS player reports while the app is in front or plays behind Home
// (Foreground false), for phones with the controller permission or a guest
// pass only. Title and Subtitle are private media names: they go to
// those phones and nowhere else, so LogValue and String redact them
// (docs/security.md).
type NowPlaying struct {
	AppID string `json:"app_id"`
	// Foreground is false while AppID plays (or is paused) behind Home with
	// the shell in front; the coordinator always sets it.
	Foreground *bool   `json:"foreground,omitempty"`
	Title      string  `json:"title"`
	Subtitle   string  `json:"subtitle,omitempty"`
	Status     string  `json:"status"` // playing | paused | stopped
	LengthMs   *int64  `json:"length_ms,omitempty"`
	PositionMs *int64  `json:"position_ms,omitempty"`
	PositionAt int64   `json:"position_at"` // coordinator monotonic ms, the clock of generated_at_ms
	Rate       float64 `json:"rate"`
}

// String describes n without its title or subtitle, so formatting it into a
// log line or error never leaks what is playing.
func (n NowPlaying) String() string {
	return "now_playing{app_id=" + n.AppID + " status=" + n.Status + " title=[title]}"
}

// LogValue implements slog.LogValuer with the titles redacted.
func (n NowPlaying) LogValue() slog.Value {
	return slog.GroupValue(slog.String("app_id", n.AppID), slog.String("status", n.Status), slog.String("title", "[title]"))
}

// Weather is state.schema.json#/properties/weather (shell view only): the
// local weather for the header chip and the Home scene (internal/weather).
type Weather struct {
	Status  string          `json:"status"` // disabled | connecting | ready | stale | error
	Message string          `json:"message"`
	Place   string          `json:"place"`
	Units   string          `json:"units"` // celsius | fahrenheit
	Scene   bool            `json:"scene"`
	Current *WeatherCurrent `json:"current"` // null when there is no usable reading
}

// WeatherCurrent is one reading, already converted to Weather.Units.
type WeatherCurrent struct {
	Temperature int    `json:"temperature"`
	Condition   string `json:"condition"` // clear | partly-cloudy | cloudy | fog | drizzle | rain | snow | thunder
	Intensity   string `json:"intensity"` // light | moderate | heavy
	IsDay       bool   `json:"is_day"`
	ObservedAt  string `json:"observed_at"` // RFC 3339
}

// WeatherPlace is a place as stored in config.weather.place and returned by
// the IPC weather.search reply; coordinates carry at most 2 decimals.
type WeatherPlace struct {
	Name      string  `json:"name"`
	Region    string  `json:"region"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
