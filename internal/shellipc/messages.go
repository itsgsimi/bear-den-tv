// Package shellipc implements contracts/ipc.md: the private Unix-socket
// protocol between the coordinator and the TV shell (plus trusted local cli
// clients), and the shell process supervisor. Every message is a typed Go
// struct; unknown types are rejected before they reach the coordinator.
package shellipc

import (
	"encoding/json"
	"errors"
	"fmt"

	"bear-den-tv/internal/contract"
)

// Message is any protocol message; Kind returns its "type" value.
type Message interface {
	Kind() string
}

// Message type names.
const (
	TypeHello            = "hello"
	TypeWelcome          = "welcome"
	TypeReject           = "reject"
	TypeState            = "state"
	TypeInput            = "input"
	TypeHome             = "home"
	TypeLayoutPreview    = "layout_preview"
	TypeLayoutPreviewEnd = "layout_preview_end"
	TypeConfirmRequest   = "confirm_request"
	TypeNotify           = "notify"
	TypeShutdown         = "shutdown"
	TypePing             = "ping"
	TypePong             = "pong"
	TypeFocus            = "focus"
	TypeInputResult      = "input_result"
	TypeConfirmResult    = "confirm_result"
	TypeRequest          = "request"
	TypeActionResult     = "action_result"
	TypeSettingsUpdate   = "settings.update"
	TypeSettingsResult   = "settings_result"
	TypePairIssue        = "pair.issue"
	TypePairCancel       = "pair.cancel"
	TypeDevicesRevoke    = "devices.revoke"
	TypeDevicesGrant     = "devices.grant"
	TypeRemoteConfigure  = "remote.configure"
	TypeRemoteNowPlaying = "remote.now_playing"
	TypeInstallRequest   = "applications.install_request"
	TypePlaybackSet      = "playback.set"
	TypeWeatherSearch    = "weather.search"
	TypeWeatherPlaces    = "weather_places"
	TypeWeatherConfigure = "weather.configure"
	TypeShellExit        = "shell.exit"
	TypePowerActivity    = "power.activity"
	TypeResult           = "result"
)

// Client kinds accepted in hello.
const (
	ClientShell = "shell"
	ClientCLI   = "cli"
)

// Reject reasons.
const (
	RejectUnsupportedProtocol   = "unsupported_protocol"
	RejectInvalidHello          = "invalid_hello"
	RejectInvalidClient         = "invalid_client"
	RejectShellAlreadyConnected = "shell_already_connected"
)

// Hello is the first message from a client.
type Hello struct {
	Type     string `json:"type"`
	Protocol int    `json:"protocol"`
	Client   string `json:"client"`
	Version  string `json:"version"`
	PID      int    `json:"pid"`
}

// Kind implements Message.
func (Hello) Kind() string { return TypeHello }

// Welcome accepts a client.
type Welcome struct {
	Type               string `json:"type"`
	Protocol           int    `json:"protocol"`
	SessionID          string `json:"session_id"`
	CoordinatorVersion string `json:"coordinator_version"`
}

// Kind implements Message.
func (Welcome) Kind() string { return TypeWelcome }

// Reject refuses a client and precedes the close.
type Reject struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Supported []int  `json:"supported,omitempty"`
}

// Kind implements Message.
func (Reject) Kind() string { return TypeReject }

// State carries a full snapshot.
type State struct {
	Type  string         `json:"type"`
	State contract.State `json:"state"`
}

// Kind implements Message.
func (State) Kind() string { return TypeState }

// Input asks the shell to apply a navigation/select/back/text action.
type Input struct {
	Type         string         `json:"type"`
	RequestID    string         `json:"request_id"`
	Action       string         `json:"action"`
	Args         map[string]any `json:"args"`
	ContextEpoch int64          `json:"context_epoch"`
}

// Kind implements Message.
func (Input) Kind() string { return TypeInput }

// Home asks the shell to return to the home screen.
type Home struct {
	Type         string `json:"type"`
	RequestID    string `json:"request_id"`
	RestoreFocus bool   `json:"restore_focus"`
}

// Kind implements Message.
func (Home) Kind() string { return TypeHome }

// LayoutPreview renders a draft without persisting it.
type LayoutPreview struct {
	Type       string          `json:"type"`
	Layout     contract.Layout `json:"layout"`
	ExpiresInS int             `json:"expires_in_s"`
}

// Kind implements Message.
func (LayoutPreview) Kind() string { return TypeLayoutPreview }

// LayoutPreviewEnd returns the shell to the persisted layout.
type LayoutPreviewEnd struct {
	Type string `json:"type"`
}

// Kind implements Message.
func (LayoutPreviewEnd) Kind() string { return TypeLayoutPreviewEnd }

// ConfirmRequest shows a timed Keep/Revert dialog.
type ConfirmRequest struct {
	Type        string `json:"type"`
	ConfirmID   string `json:"confirm_id"`
	ConfirmKind string `json:"kind"`
	Summary     string `json:"summary"`
	ExpiresInS  int    `json:"expires_in_s"`
}

// Kind implements Message.
func (ConfirmRequest) Kind() string { return TypeConfirmRequest }

// Notify shows a transient notification.
type Notify struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	NotifyKind string `json:"kind"`
	Text       string `json:"text"`
}

// Kind implements Message.
func (Notify) Kind() string { return TypeNotify }

// Shutdown asks the shell to exit cleanly with code 0.
type Shutdown struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Kind implements Message.
func (Shutdown) Kind() string { return TypeShutdown }

// Ping is a keepalive probe.
type Ping struct {
	Type string `json:"type"`
}

// Kind implements Message.
func (Ping) Kind() string { return TypePing }

// Pong answers Ping.
type Pong struct {
	Type string `json:"type"`
}

// Kind implements Message.
func (Pong) Kind() string { return TypePong }

// Focus is the shell's focus report.
type Focus struct {
	Type      string  `json:"type"`
	Screen    string  `json:"screen"`
	SectionID *string `json:"section_id"`
	ItemID    *string `json:"item_id"`
	ScrollX   float64 `json:"scroll_x"`
	// TextField is true while a shell text field has keyboard focus; it
	// makes text.submit available (contracts/ipc.md). Absent = false.
	TextField bool `json:"text_field,omitempty"`
}

// Kind implements Message.
func (Focus) Kind() string { return TypeFocus }

// InputResult is the terminal reply to Input/Home.
type InputResult struct {
	Type      string         `json:"type"`
	RequestID string         `json:"request_id"`
	Outcome   string         `json:"outcome"`
	Code      string         `json:"code"`
	Detail    map[string]any `json:"detail"`
}

// Kind implements Message.
func (InputResult) Kind() string { return TypeInputResult }

// ConfirmResult answers ConfirmRequest.
type ConfirmResult struct {
	Type      string `json:"type"`
	ConfirmID string `json:"confirm_id"`
	Accepted  bool   `json:"accepted"`
}

// Kind implements Message.
func (ConfirmResult) Kind() string { return TypeConfirmResult }

// Request is a shell-originated action; answered with ActionResult.
type Request struct {
	Type      string         `json:"type"`
	RequestID string         `json:"request_id"`
	Action    string         `json:"action"`
	Args      map[string]any `json:"args"`
}

// Kind implements Message.
func (Request) Kind() string { return TypeRequest }

// ActionResult answers Request.
type ActionResult struct {
	Type   string                `json:"type"`
	Result contract.ActionResult `json:"result"`
}

// Kind implements Message.
func (ActionResult) Kind() string { return TypeActionResult }

// SettingsUpdate is a TV settings layout change.
type SettingsUpdate struct {
	Type         string          `json:"type"`
	RequestID    string          `json:"request_id"`
	BaseRevision int64           `json:"base_revision"`
	Layout       contract.Layout `json:"layout"`
}

// Kind implements Message.
func (SettingsUpdate) Kind() string { return TypeSettingsUpdate }

// SettingsResult answers SettingsUpdate.
type SettingsResult struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Revision  int64  `json:"revision"`
	Error     string `json:"error"`
}

// Kind implements Message.
func (SettingsResult) Kind() string { return TypeSettingsResult }

// PairIssue asks for a fresh invitation.
type PairIssue struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (PairIssue) Kind() string { return TypePairIssue }

// PairCancel withdraws the displayed invitation.
type PairCancel struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (PairCancel) Kind() string { return TypePairCancel }

// DevicesRevoke revokes one device or "*".
type DevicesRevoke struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	DeviceID  string `json:"device_id"`
}

// Kind implements Message.
func (DevicesRevoke) Kind() string { return TypeDevicesRevoke }

// DevicesGrant changes a device's permissions (trusted local path only).
type DevicesGrant struct {
	Type        string                `json:"type"`
	RequestID   string                `json:"request_id"`
	DeviceID    string                `json:"device_id"`
	Permissions []contract.Permission `json:"permissions"`
}

// Kind implements Message.
func (DevicesGrant) Kind() string { return TypeDevicesGrant }

// RemoteConfigure is the local onboarding step that enables LAN exposure.
type RemoteConfigure struct {
	Type              string `json:"type"`
	RequestID         string `json:"request_id"`
	Enabled           bool   `json:"enabled"`
	Transport         string `json:"transport"`
	Interface         string `json:"interface"`
	Port              int    `json:"port"`
	HTTPLayoutEditing bool   `json:"http_layout_editing"`
	LANConsent        bool   `json:"lan_consent"`
}

// Kind implements Message.
func (RemoteConfigure) Kind() string { return TypeRemoteConfigure }

// RemoteNowPlaying turns phones' Now playing card on or off (TV Settings →
// Now playing on phones); stored as config remote.now_playing. Answered with
// Result.
type RemoteNowPlaying struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Enabled   bool   `json:"enabled"`
}

// Kind implements Message.
func (RemoteNowPlaying) Kind() string { return TypeRemoteNowPlaying }

// InstallRequest asks for a guided Flatpak install.
type InstallRequest struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	AppID     string `json:"app_id"`
}

// Kind implements Message.
func (InstallRequest) Kind() string { return TypeInstallRequest }

// PlaybackSet chooses one app's playback setting by hand (TV Settings →
// Advanced playback); Value "" returns it to automatic. Answered with Result.
type PlaybackSet struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Adapter   string `json:"adapter"`
	Setting   string `json:"setting"`
	Value     string `json:"value"`
}

// Kind implements Message.
func (PlaybackSet) Kind() string { return TypePlaybackSet }

// WeatherSearch asks the coordinator to geocode a place name (Settings →
// Weather); answered with WeatherPlaces.
type WeatherSearch struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Query     string `json:"query"`
}

// Kind implements Message.
func (WeatherSearch) Kind() string { return TypeWeatherSearch }

// WeatherPlaces answers WeatherSearch: up to 8 places with coordinates
// rounded to 2 decimals; on failure OK is false, Places empty, Error
// user-facing.
type WeatherPlaces struct {
	Type      string                  `json:"type"`
	RequestID string                  `json:"request_id"`
	OK        bool                    `json:"ok"`
	Places    []contract.WeatherPlace `json:"places"`
	Error     string                  `json:"error"`
}

// Kind implements Message.
func (WeatherPlaces) Kind() string { return TypeWeatherPlaces }

// WeatherConfigure turns weather on or off and chooses the place, units and
// scene; stored in config.json (config.weather). Answered with Result.
type WeatherConfigure struct {
	Type      string                 `json:"type"`
	RequestID string                 `json:"request_id"`
	Enabled   bool                   `json:"enabled"`
	Place     *contract.WeatherPlace `json:"place"`
	Units     string                 `json:"units"`
	Scene     bool                   `json:"scene"`
}

// Kind implements Message.
func (WeatherConfigure) Kind() string { return TypeWeatherConfigure }

// PowerActivity tells the coordinator that a key was pressed on the TV while
// the display was off or the sleep warning was showing; the shell swallowed
// that key. The coordinator turns the display on and cancels the timer
// (contracts/ipc.md). Shell only; no reply.
type PowerActivity struct {
	Type string `json:"type"`
}

// Kind implements Message.
func (PowerActivity) Kind() string { return TypePowerActivity }

// ShellExit announces an intentional exit; the supervisor must not restart.
type ShellExit struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Kind implements Message.
func (ShellExit) Kind() string { return TypeShellExit }

// Result is the generic terminal reply to request_id-bearing administrative
// messages (pair.*, devices.*, remote.configure, remote.now_playing,
// applications.install_request, playback.set, weather.configure).
// Data carries an operation-specific payload, for example the issued
// invitation for pair.issue. contracts/ipc.md does not list this message yet.
type Result struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// Kind implements Message.
func (Result) Kind() string { return TypeResult }

// ErrUnknownType is returned by Decode for a type outside the protocol.
var ErrUnknownType = errors.New("shellipc: unknown message type")

// Decode parses one frame into its typed message. Unknown fields are
// ignored; an unknown or missing type is an error.
func Decode(frame []byte) (Message, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(frame, &head); err != nil {
		return nil, fmt.Errorf("shellipc: invalid frame: %w", err)
	}
	var m Message
	switch head.Type {
	case TypeHello:
		m = &Hello{}
	case TypeWelcome:
		m = &Welcome{}
	case TypeReject:
		m = &Reject{}
	case TypeState:
		m = &State{}
	case TypeInput:
		m = &Input{}
	case TypeHome:
		m = &Home{}
	case TypeLayoutPreview:
		m = &LayoutPreview{}
	case TypeLayoutPreviewEnd:
		m = &LayoutPreviewEnd{}
	case TypeConfirmRequest:
		m = &ConfirmRequest{}
	case TypeNotify:
		m = &Notify{}
	case TypeShutdown:
		m = &Shutdown{}
	case TypePing:
		m = &Ping{}
	case TypePong:
		m = &Pong{}
	case TypeFocus:
		m = &Focus{}
	case TypeInputResult:
		m = &InputResult{}
	case TypeConfirmResult:
		m = &ConfirmResult{}
	case TypeRequest:
		m = &Request{}
	case TypeActionResult:
		m = &ActionResult{}
	case TypeSettingsUpdate:
		m = &SettingsUpdate{}
	case TypeSettingsResult:
		m = &SettingsResult{}
	case TypePairIssue:
		m = &PairIssue{}
	case TypePairCancel:
		m = &PairCancel{}
	case TypeDevicesRevoke:
		m = &DevicesRevoke{}
	case TypeDevicesGrant:
		m = &DevicesGrant{}
	case TypeRemoteConfigure:
		m = &RemoteConfigure{}
	case TypeRemoteNowPlaying:
		m = &RemoteNowPlaying{}
	case TypeInstallRequest:
		m = &InstallRequest{}
	case TypePlaybackSet:
		m = &PlaybackSet{}
	case TypeWeatherSearch:
		m = &WeatherSearch{}
	case TypeWeatherPlaces:
		m = &WeatherPlaces{}
	case TypeWeatherConfigure:
		m = &WeatherConfigure{}
	case TypeShellExit:
		m = &ShellExit{}
	case TypePowerActivity:
		m = &PowerActivity{}
	case TypeResult:
		m = &Result{}
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, head.Type)
	}
	if err := json.Unmarshal(frame, m); err != nil {
		return nil, fmt.Errorf("shellipc: invalid %s: %w", head.Type, err)
	}
	return deref(m), nil
}

// deref returns the value form so type switches on concrete structs work.
func deref(m Message) Message {
	switch t := m.(type) {
	case *Hello:
		t.Type = TypeHello
		return *t
	case *Welcome:
		return *t
	case *Reject:
		return *t
	case *State:
		return *t
	case *Input:
		return *t
	case *Home:
		return *t
	case *LayoutPreview:
		return *t
	case *LayoutPreviewEnd:
		return *t
	case *ConfirmRequest:
		return *t
	case *Notify:
		return *t
	case *Shutdown:
		return *t
	case *Ping:
		return *t
	case *Pong:
		return *t
	case *Focus:
		return *t
	case *InputResult:
		return *t
	case *ConfirmResult:
		return *t
	case *Request:
		return *t
	case *ActionResult:
		return *t
	case *SettingsUpdate:
		return *t
	case *SettingsResult:
		return *t
	case *PairIssue:
		return *t
	case *PairCancel:
		return *t
	case *DevicesRevoke:
		return *t
	case *DevicesGrant:
		return *t
	case *RemoteConfigure:
		return *t
	case *InstallRequest:
		return *t
	case *PlaybackSet:
		return *t
	case *WeatherSearch:
		return *t
	case *WeatherPlaces:
		return *t
	case *WeatherConfigure:
		return *t
	case *RemoteNowPlaying:
		return *t
	case *ShellExit:
		return *t
	case *PowerActivity:
		return *t
	case *Result:
		return *t
	}
	return m
}

// Encode serializes m as one newline-terminated frame, filling in "type".
func Encode(m Message) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	// Ensure the type field is present even when the caller left it empty.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err == nil {
		if t, ok := probe["type"]; !ok || string(t) == `""` {
			probe["type"] = json.RawMessage(fmt.Sprintf("%q", m.Kind()))
			raw, err = json.Marshal(probe)
			if err != nil {
				return nil, err
			}
		}
	}
	return append(raw, '\n'), nil
}
