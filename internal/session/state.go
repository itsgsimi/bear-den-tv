// Builds the state snapshot for the shell and phones (spec
// contracts/state.schema.json).

package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/remote"
)

type viewKind int

const (
	viewAnonymous viewKind = iota
	viewPhone
	viewShell
)

// Backend names reported in capabilities.
const (
	backendShell = "shell-ipc"
	// backendShellText is text.submit's backend (contracts/ipc.md, focus).
	backendShellText = "shell"
	backendAudio     = "pulseaudio"
	backendMPRIS     = "mpris"
	backendLaunch    = "flatpak"
)

func (c *Coordinator) buildState(view viewKind) contract.State {
	return c.buildStateFor(view, nil)
}

// buildStateFor assembles a snapshot redacted for the viewer (contracts/http.md
// and state.schema.json): pairing is shell-only, devices owner/shell only,
// layout editor/shell only, playback, weather and plex shell only, now_playing
// controller and guest phones only (never the shell); while locked no focus,
// devices, layout, content, playback, weather, plex or now_playing. A guest's
// me carries its pass end (contracts/http.md#guest-passes).
func (c *Coordinator) buildStateFor(view viewKind, v *remote.Viewer) contract.State {
	cfg := c.opts.Config.Current()
	pending := c.opts.Config.Pending()
	hold := c.holds.Active()

	c.mu.Lock()
	st := contract.State{
		Protocol:       1,
		ContextEpoch:   c.epoch,
		GeneratedAtMs:  c.nowMs(),
		DeviceName:     cfg.Device.DisplayName,
		DevMode:        c.opts.DevMode,
		ConfigRevision: cfg.Revision,
		Session: contract.SessionState{
			Locked:         c.locked,
			DisplaySession: displaySession(c.opts.Desktop.DisplaySession()),
			DesktopAdapter: c.opts.Desktop.Name(),
			ShellConnected: c.shellClient() != nil,
			ShellState:     c.shellState,
		},
		Target:        c.target,
		Capabilities:  c.capabilitiesLocked(),
		Shell:         c.shellFocus,
		Applications:  c.appStatesLocked(cfg.Applications),
		Remote:        c.remote,
		Notifications: append([]contract.Notification{}, c.notifications...),
		LayoutPending: pending,
	}
	locked := c.locked
	playback := c.playback // an immutable snapshot, replaced whole by the tuner
	nowPlaying := c.nowPlayingLocked()
	st.Power = c.powerLocked() // also while locked: the shell swallows the waking key
	c.mu.Unlock()
	showNowPlaying := cfg.Remote.ShowNowPlaying()
	st.Remote.NowPlaying = &showNowPlaying

	st.Remote.Enabled = cfg.Remote.Enabled && cfg.Onboarding.LANConsent
	if st.Remote.Enabled {
		st.Remote.Transport = cfg.Remote.Transport
	}
	st.Remote.HTTPS = cfg.Remote.Transport == "https"
	st.Remote.HTTPLayoutEditing = cfg.Remote.LayoutEditingOverHTTP()
	st.Remote.Hold = hold
	st.Remote.Addresses = append([]string{}, st.Remote.Addresses...)
	st.Remote.PairedDeviceCount = c.opts.Pairing.Count(context.Background())
	if st.Target.Kind == "app" {
		st.Target.WindowTitle = nil // phones never see external window titles
	}

	if locked {
		st.Shell.Focus = contract.Focus{}
		st.LayoutPending = nil
	}
	switch view {
	case viewShell:
		p := c.opts.Pairing.State()
		st.Pairing = &p
		if !locked {
			st.Devices = c.devices()
			l := cfg.Layout()
			st.Layout = &l
			st.Content = c.content()
			st.Playback = playback
			if c.opts.Weather != nil {
				w := c.opts.Weather.Snapshot()
				st.Weather = &w
			}
			st.Plex = c.plexState()
		}
	case viewPhone:
		if v != nil {
			st.Me = &contract.Me{DeviceID: v.DeviceID, DeviceName: v.DeviceName, Permissions: v.Permissions, TransportSecure: v.Secure}
			if v.Guest() {
				st.Me.ExpiresAtMs = v.ExpiresAtMs
			}
			ui := cfg.Layout().UI
			if c.opts.Themes != nil {
				a := c.opts.Themes.Appearance(ui)
				st.Appearance = &a
			} else {
				st.Appearance = &contract.Appearance{Background: ui.Background, Theme: ui.Theme, Accent: ui.Accent, ArtStyle: contract.ArtPixel}
				if ui.Classic() {
					st.Appearance.ArtStyle = contract.ArtClassic
				}
			}
			if !locked {
				if v.Has(contract.PermOwner) {
					st.Devices = c.devices()
				}
				if v.Has(contract.PermLayoutEditor) {
					l := cfg.Layout()
					st.Layout = &l
				}
				// What is playing names private media: controller phones
				// and guests (they watch the same screen) only, never while
				// locked, and only while the owner allows it
				// (contracts/http.md "Now playing").
				if v.Has(contract.PermGuest) && showNowPlaying {
					st.NowPlaying = nowPlaying
				}
				st.Content = c.content()
			}
		}
	case viewAnonymous:
		st.Power = nil
		st.Shell.Focus = contract.Focus{}
		st.Applications = []contract.AppState{}
		st.Notifications = []contract.Notification{}
		st.LayoutPending = nil
	}
	return st
}

func (c *Coordinator) devices() *[]contract.Device {
	list, err := c.opts.Pairing.List(context.Background())
	if err != nil || list == nil {
		list = []contract.Device{}
	}
	return &list
}

func (c *Coordinator) content() *contract.Content {
	if c.opts.Feed == nil {
		return c.plexContent()
	}
	ct := c.opts.Feed.Snapshot()
	return &ct
}

func displaySession(s string) string {
	switch s {
	case "x11", "wayland":
		return s
	}
	return "unknown"
}

func (c *Coordinator) appStatesLocked(apps []configApp) []contract.AppState {
	out := make([]contract.AppState, 0, len(apps))
	for _, a := range apps {
		rt := c.appLocked(a.ID)
		st := contract.AppState{
			ID: a.ID, Label: a.Label, Adapter: a.Adapter,
			Installed:    rt.install.Installed,
			Installation: installationScope(rt),
			LaunchState:  rt.launchState,
			LastError:    rt.lastError,
			Foreground:   c.target.Kind == "app" && strOr(c.target.AppID) == a.ID,
		}
		if rt.install.Version != "" {
			v := rt.install.Version
			st.Version = &v
		}
		st.Running = st.Foreground || rt.launchState == "running" || rt.instance != nil
		out = append(out, st)
	}
	return out
}

func (c *Coordinator) anyAppRunningLocked() bool {
	for _, rt := range c.apps {
		if rt.launchState == "running" || rt.instance != nil {
			return true
		}
	}
	return false
}

func installationScope(rt *appRuntime) string {
	if !rt.discovered {
		return "unknown"
	}
	switch rt.install.Scope {
	case "user", "system", "none":
		return rt.install.Scope
	}
	if !rt.install.Installed {
		return "none"
	}
	return "unknown"
}

func unavailable(reason string) contract.Capability {
	return contract.Capability{Available: false, Reason: reason}
}

func available(backend string) contract.Capability {
	return contract.Capability{Available: true, Backend: backend}
}

// capabilitiesLocked reports every action for the current target. It is the
// single source of truth: routing refuses what is reported unavailable.
func (c *Coordinator) capabilitiesLocked() map[string]contract.Capability {
	caps := make(map[string]contract.Capability, len(contract.AllActions))
	if c.locked {
		for _, a := range contract.AllActions {
			caps[a] = unavailable("The TV session is locked. Unlock it on the TV.")
		}
		return caps
	}
	desk := c.opts.Desktop.Capabilities()
	shellUp := c.shellClient() != nil

	// Directional input depends on the target.
	for _, a := range []string{contract.ActionNavUp, contract.ActionNavDown, contract.ActionNavLeft, contract.ActionNavRight, contract.ActionSelect, contract.ActionBack} {
		caps[a] = c.inputCapLocked(a, desk, shellUp)
	}
	// text.submit only into a focused shell text field (Settings → Weather
	// search); never into external apps.
	if c.target.Kind == "shell" && shellUp && c.shellTextField {
		caps[contract.ActionTextSubmit] = available(backendShellText)
	} else {
		caps[contract.ActionTextSubmit] = unavailable("No text field is focused.")
	}

	switch {
	case !shellUp:
		caps[contract.ActionHome] = unavailable("The TV shell is not running.")
	case !desk[platform.CapActivate].Available && c.target.Kind != "shell":
		caps[contract.ActionHome] = unavailable("This desktop cannot bring the shell forward: " + desk[platform.CapActivate].Reason)
	default:
		caps[contract.ActionHome] = available(backendShell)
	}

	if c.opts.Launcher == nil {
		caps[contract.ActionAppLaunch] = unavailable("Application launching is not available on this installation.")
		caps[contract.ActionAppClose] = unavailable("Application launching is not available on this installation.")
	} else {
		caps[contract.ActionAppLaunch] = available(backendLaunch)
		// Close works on the app in front or one left running behind Home.
		if c.target.Kind == "app" || c.anyAppRunningLocked() {
			caps[contract.ActionAppClose] = available(c.opts.Desktop.Name())
		} else {
			caps[contract.ActionAppClose] = unavailable("No application is running.")
		}
	}

	media := unavailable(ShellLabel + " is not a media player.")
	if c.target.Kind == "app" {
		switch {
		case c.media == nil || strOr(c.target.AppID) != c.media.appID:
			media = unavailable("Checking media controls for " + c.target.Label + "…")
		case c.media.player == nil || !c.media.canCtl:
			media = unavailable(c.target.Label + " does not expose verified media controls.")
		default:
			media = available(backendMPRIS)
		}
	} else if c.target.Kind == "unknown" {
		media = unavailable("The foreground window is not recognized.")
	}
	caps[contract.ActionMediaPlay] = media
	caps[contract.ActionMediaPause] = media
	caps[contract.ActionMediaSeek] = media

	if c.opts.Audio == nil {
		caps[contract.ActionAudioVolume] = unavailable("PC volume control is not available.")
	} else if ac := c.audioCap; !ac.Available {
		caps[contract.ActionAudioVolume] = unavailable("PC volume control is not available: " + ac.Reason)
	} else {
		caps[contract.ActionAudioVolume] = available(backendAudio)
	}
	caps[contract.ActionAudioMute] = caps[contract.ActionAudioVolume]

	if c.opts.Supervisor == nil {
		caps[contract.ActionShellRestart] = unavailable("The shell is not supervised by this coordinator.")
	} else {
		caps[contract.ActionShellRestart] = available("supervisor")
	}
	c.powerCapsLocked(caps)
	return caps
}

func (c *Coordinator) inputCapLocked(action string, desk map[string]platform.Capability, shellUp bool) contract.Capability {
	var cp contract.Capability
	switch c.target.Kind {
	case "shell":
		if !shellUp {
			cp = unavailable("The TV shell is not connected.")
		} else {
			cp = available(backendShell)
		}
	case "app":
		ad, ok := c.adapterFor(strOr(c.target.AppID))
		switch {
		case !desk[platform.CapInput].Available:
			cp = unavailable("Input delivery is unavailable: " + desk[platform.CapInput].Reason)
		case !ok:
			cp = unavailable("No adapter is registered for " + c.target.Label + ".")
		default:
			if _, mapped := ad.KeyFor(action); mapped {
				cp = available(c.opts.Desktop.Name())
			} else {
				cp = unavailable(fmt.Sprintf("%s has no verified mapping for %s.", c.target.Label, action))
			}
		}
	case "unknown":
		cp = unavailable("The foreground window is not recognized; press Home to return to " + ShellLabel + ".")
	default:
		cp = unavailable("Nothing is in the foreground.")
	}
	if cp.Available && contract.IsNav(action) {
		cp.Holdable = true
	}
	return cp
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
