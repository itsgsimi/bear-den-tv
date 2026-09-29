// Package config owns config.json: built-in defaults, structural and semantic
// validation (contracts/config.md), atomic persistence with a last-known-good
// copy and revision history, and the layout-change workflow (revision check,
// timed confirmation for risky changes, undo, reset, export, import preview).
// The coordinator is the only writer; nothing here accepts input from phones
// without validation.
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/contract"
)

// SchemaVersion is the only config.json schema version this build accepts.
const SchemaVersion = 1

// Config is contracts/config.schema.json. Field names follow the JSON keys.
type Config struct {
	SchemaVersion int                `json:"schema_version"`
	Revision      int64              `json:"revision"`
	Device        Device             `json:"device"`
	UI            contract.UI        `json:"ui"`
	Applications  []Application      `json:"applications"`
	Sections      []contract.Section `json:"sections"`
	PlexContent   PlexContent        `json:"plex_content"`
	Remote        Remote             `json:"remote"`
	Cache         Cache              `json:"cache"`
	Startup       Startup            `json:"startup"`
	Privacy       Privacy            `json:"privacy"`
	Onboarding    Onboarding         `json:"onboarding"`
	// Playback holds the owner's manual playback settings; nil = none.
	Playback *Playback `json:"playback,omitempty"`
	// Weather is the local weather block; nil = off (contracts/config.md).
	Weather *Weather `json:"weather,omitempty"`
	// CEC is TV control over HDMI-CEC; nil = off (cec.go).
	CEC *CEC `json:"cec,omitempty"`
}

// Weather is config.weather: local weather for the Home header and scene.
// Place coordinates carry at most 2 decimals (rule 10).
type Weather struct {
	Enabled bool                   `json:"enabled"`
	Place   *contract.WeatherPlace `json:"place"`
	Units   string                 `json:"units"`
	Scene   bool                   `json:"scene"`
}

// Weather units.
const (
	UnitsCelsius    = "celsius"
	UnitsFahrenheit = "fahrenheit"
)

// WeatherSettings returns a copy of the weather block; an absent block is
// off, in Celsius, with the scene on.
func (c Config) WeatherSettings() Weather {
	if c.Weather == nil {
		return Weather{Units: UnitsCelsius, Scene: true}
	}
	return c.Weather.clone()
}

func (w Weather) clone() Weather {
	out := w
	if w.Place != nil {
		p := *w.Place
		out.Place = &p
	}
	return out
}

// Playback is config.playback: settings chosen by hand in TV Settings →
// Advanced playback, which automatic tuning uses instead of its own choice
// (docs/APP_PERFORMANCE.md). Overrides: adapter → setting id → option value.
type Playback struct {
	Overrides map[string]map[string]string `json:"overrides,omitempty"`
}

// PlaybackOverrides returns a copy of one app's manual playback settings.
func (c Config) PlaybackOverrides(adapter string) map[string]string {
	if c.Playback == nil || len(c.Playback.Overrides[adapter]) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range c.Playback.Overrides[adapter] {
		out[k] = v
	}
	return out
}

// SetPlaybackOverride stores value for one app's setting; "" removes it
// (back to automatic). Empty maps are pruned so config.json stays minimal.
func (c *Config) SetPlaybackOverride(adapter, setting, value string) {
	if value == "" {
		if c.Playback == nil {
			return
		}
		delete(c.Playback.Overrides[adapter], setting)
		if len(c.Playback.Overrides[adapter]) == 0 {
			delete(c.Playback.Overrides, adapter)
		}
		if len(c.Playback.Overrides) == 0 {
			c.Playback = nil
		}
		return
	}
	if c.Playback == nil {
		c.Playback = &Playback{}
	}
	if c.Playback.Overrides == nil {
		c.Playback.Overrides = map[string]map[string]string{}
	}
	if c.Playback.Overrides[adapter] == nil {
		c.Playback.Overrides[adapter] = map[string]string{}
	}
	c.Playback.Overrides[adapter][setting] = value
}

// Device is config.device.
type Device struct {
	DisplayName      string `json:"display_name"`
	PreferredDisplay string `json:"preferred_display"`
}

// Launch is config.applications[].launch: a closed Flatpak launch definition.
type Launch struct {
	Kind  string   `json:"kind"`
	AppID string   `json:"app_id"`
	Args  []string `json:"args"`
}

// Application is one registered external application.
type Application struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Adapter       string `json:"adapter"`
	Launch        Launch `json:"launch"`
	HomePolicy    string `json:"home_policy"`
	RemoteEnabled bool   `json:"remote_enabled"`
	// HideWhenMissing marks an optional app (Spotify, Jellyfin, RetroArch by
	// default): no tile while its Flatpak is not installed
	// (state.applications[].hidden) instead of "Not installed".
	HideWhenMissing bool `json:"hide_when_missing,omitempty"`
}

// PlexContent is config.plex_content; the token lives in the secret store.
type PlexContent struct {
	Enabled       bool     `json:"enabled"`
	ConnectionRef *string  `json:"connection_ref"`
	ServerURL     *string  `json:"server_url,omitempty"`
	LibraryIDs    []string `json:"library_ids,omitempty"`
}

// HTTPSFiles is config.remote.https.
type HTTPSFiles struct {
	CertificateFile *string `json:"certificate_file"`
	PrivateKeyFile  *string `json:"private_key_file"`
}

// Remote is config.remote: the LAN listener definition.
type Remote struct {
	Enabled           bool     `json:"enabled"`
	Transport         string   `json:"transport"`
	Port              int      `json:"port"`
	Interfaces        []string `json:"interfaces"`
	AllowedHosts      []string `json:"allowed_hosts"`
	MDNS              bool     `json:"mdns"`
	HTTPLayoutEditing bool     `json:"http_layout_editing"`
	// NowPlaying: phones with the controller permission see what the app in
	// front is playing (state.now_playing); nil = true.
	NowPlaying           *bool      `json:"now_playing,omitempty"`
	PairingExpirySeconds int        `json:"pairing_expiry_seconds"`
	PairingMaxAttempts   int        `json:"pairing_max_attempts"`
	HTTPS                HTTPSFiles `json:"https"`
}

// LayoutEditingOverHTTP reports whether HTTP layout editing is honored: only in
// trusted-lan-http (rule 6); in https device permission alone governs it.
func (r Remote) LayoutEditingOverHTTP() bool {
	return r.Transport == "trusted-lan-http" && r.HTTPLayoutEditing
}

// ShowNowPlaying reports whether phones may see state.now_playing
// (remote.now_playing, default true).
func (r Remote) ShowNowPlaying() bool { return r.NowPlaying == nil || *r.NowPlaying }

// Cache is config.cache.
type Cache struct {
	ArtworkMaxMiB int `json:"artwork_max_mib"`
}

// Startup is config.startup.
type Startup struct {
	AutostartEnabled bool `json:"autostart_enabled"`
	// TuneApps: apply the best playback settings automatically; nil = true.
	TuneApps *bool `json:"tune_apps,omitempty"`
}

// AutoTune reports whether the coordinator applies playback settings itself.
func (s Startup) AutoTune() bool { return s.TuneApps == nil || *s.TuneApps }

// Privacy is config.privacy; telemetry is always false in schema_version 1.
type Privacy struct {
	TelemetryEnabled bool `json:"telemetry_enabled"`
}

// Onboarding is config.onboarding.
type Onboarding struct {
	Completed  bool `json:"completed"`
	LANConsent bool `json:"lan_consent"`
}

// Layout returns the portable subset (ui + sections) as a deep copy.
func (c Config) Layout() contract.Layout {
	l := contract.Layout{UI: c.UI, Sections: make([]contract.Section, len(c.Sections))}
	for i, s := range c.Sections {
		l.Sections[i] = cloneSection(s)
	}
	return l
}

// SetLayout replaces the portable subset with a deep copy of l.
func (c *Config) SetLayout(l contract.Layout) {
	c.UI = l.UI
	c.Sections = make([]contract.Section, len(l.Sections))
	for i, s := range l.Sections {
		c.Sections[i] = cloneSection(s)
	}
}

// Application returns the registered application with the given id.
func (c Config) Application(id string) (Application, bool) {
	for _, a := range c.Applications {
		if a.ID == id {
			return a, true
		}
	}
	return Application{}, false
}

// Clone returns a deep copy so callers can mutate without aliasing the store.
func (c Config) Clone() Config {
	out := c
	out.Applications = make([]Application, len(c.Applications))
	for i, a := range c.Applications {
		out.Applications[i] = a
		out.Applications[i].Launch.Args = append([]string(nil), a.Launch.Args...)
	}
	out.SetLayout(c.Layout())
	out.PlexContent.LibraryIDs = append([]string(nil), c.PlexContent.LibraryIDs...)
	if c.PlexContent.ConnectionRef != nil {
		v := *c.PlexContent.ConnectionRef
		out.PlexContent.ConnectionRef = &v
	}
	if c.PlexContent.ServerURL != nil {
		v := *c.PlexContent.ServerURL
		out.PlexContent.ServerURL = &v
	}
	out.Remote.Interfaces = append([]string(nil), c.Remote.Interfaces...)
	out.Remote.AllowedHosts = append([]string(nil), c.Remote.AllowedHosts...)
	if c.Remote.HTTPS.CertificateFile != nil {
		v := *c.Remote.HTTPS.CertificateFile
		out.Remote.HTTPS.CertificateFile = &v
	}
	if c.Remote.HTTPS.PrivateKeyFile != nil {
		v := *c.Remote.HTTPS.PrivateKeyFile
		out.Remote.HTTPS.PrivateKeyFile = &v
	}
	if c.Startup.TuneApps != nil {
		v := *c.Startup.TuneApps
		out.Startup.TuneApps = &v
	}
	if c.Remote.NowPlaying != nil {
		v := *c.Remote.NowPlaying
		out.Remote.NowPlaying = &v
	}
	if c.Weather != nil {
		w := c.Weather.clone()
		out.Weather = &w
	}
	if c.CEC != nil {
		v := *c.CEC
		out.CEC = &v
	}
	if c.Playback != nil {
		out.Playback = &Playback{}
		if c.Playback.Overrides != nil {
			out.Playback.Overrides = make(map[string]map[string]string, len(c.Playback.Overrides))
			for adapter, settings := range c.Playback.Overrides {
				m := make(map[string]string, len(settings))
				for k, v := range settings {
					m[k] = v
				}
				out.Playback.Overrides[adapter] = m
			}
		}
	}
	return out
}

func cloneSection(s contract.Section) contract.Section {
	out := s
	if s.ApplicationIDs != nil {
		out.ApplicationIDs = append([]string{}, s.ApplicationIDs...)
	}
	return out
}

// Defaults returns the built-in configuration. It is the embedded fixture
// contracts/fixtures/config.default.valid.json, so the contract tests and the
// runtime agree on one document.
func Defaults() Config {
	raw, err := fs.ReadFile(bdtv.Contracts, "contracts/fixtures/config.default.valid.json")
	if err != nil {
		panic(fmt.Sprintf("config: embedded defaults missing: %v", err))
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		panic(fmt.Sprintf("config: embedded defaults unreadable: %v", err))
	}
	return c
}

// Marshal serializes c with stable indentation for on-disk files. Nil slices
// are written as empty arrays because the schema requires arrays.
func Marshal(c Config) ([]byte, error) {
	c = c.Clone()
	for i := range c.Applications {
		if c.Applications[i].Launch.Args == nil {
			c.Applications[i].Launch.Args = []string{}
		}
	}
	if c.Remote.Interfaces == nil {
		c.Remote.Interfaces = []string{}
	}
	if c.Remote.AllowedHosts == nil {
		c.Remote.AllowedHosts = []string{}
	}
	if c.Applications == nil {
		c.Applications = []Application{}
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
