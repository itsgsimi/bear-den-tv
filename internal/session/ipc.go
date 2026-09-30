// ShellHandler: the coordinator's handling of shell IPC messages (spec
// contracts/ipc.md).

package session

import (
	"context"
	"errors"
	"fmt"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/shellipc"
)

// ShellHandler adapts the coordinator to shellipc.Handler.
type ShellHandler struct{ c *Coordinator }

// Shell returns the shellipc.Handler view of the coordinator.
func (c *Coordinator) Shell() *ShellHandler { return &ShellHandler{c: c} }

var _ shellipc.Handler = (*ShellHandler)(nil)

// InitialState implements shellipc.Handler. Shell and cli are trusted local
// peers and receive the shell view.
func (h *ShellHandler) InitialState(*shellipc.Client) contract.State {
	return h.c.buildState(viewShell)
}

// Connected implements shellipc.Handler.
func (h *ShellHandler) Connected(cl *shellipc.Client) {
	if cl.Kind() != shellipc.ClientShell {
		return
	}
	c := h.c
	c.mu.Lock()
	c.shellPID = cl.PID()
	c.retargetLocked() // the shell window may now be recognizable by pid
	c.lastShellJSON = nil
	c.shellStartedLocked = c.locked || !c.lockKnown
	c.mu.Unlock()
	if c.opts.Supervisor != nil {
		c.opts.Supervisor.MarkRunning()
	}
	c.log.Info("session: shell connected", "pid", cl.PID())
	c.publish()
}

// Disconnected implements shellipc.Handler.
func (h *ShellHandler) Disconnected(cl *shellipc.Client, err error) {
	if cl.Kind() != shellipc.ClientShell {
		return
	}
	c := h.c
	c.mu.Lock()
	c.shellPID = 0
	c.shellFocus = contract.ShellState{Screen: "unknown"}
	c.shellTextField = false
	c.previewing = false
	c.mu.Unlock()
	if c.opts.Supervisor != nil {
		c.opts.Supervisor.SocketLost()
	}
	c.log.Info("session: shell disconnected", "err", err)
	c.publish()
}

// Receive implements shellipc.Handler.
func (h *ShellHandler) Receive(cl *shellipc.Client, m shellipc.Message) {
	c := h.c
	ctx := context.Background()
	isShell := cl.Kind() == shellipc.ClientShell
	switch msg := m.(type) {
	case shellipc.Focus:
		if isShell {
			c.onShellFocus(msg.Screen, msg.SectionID, msg.ItemID, msg.TextField)
		}
	case shellipc.Request:
		if !isShell {
			h.reply(cl, msg.RequestID, errors.New("cli clients may not submit actions"), nil)
			return
		}
		req := contract.ActionRequest{Protocol: 1, RequestID: msg.RequestID, Target: "active", Action: msg.Action, Args: msg.Args}
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		epoch, _, _ := c.current()
		req.ContextEpoch = epoch
		res := c.submit(ctx, sender{key: shellSender}, req)
		_ = cl.Send(shellipc.ActionResult{Type: shellipc.TypeActionResult, Result: res})
	case shellipc.SettingsUpdate:
		res, err := c.opts.Config.ApplyLayout(msg.BaseRevision, msg.Layout, "tv")
		out := shellipc.SettingsResult{Type: shellipc.TypeSettingsResult, RequestID: msg.RequestID, OK: err == nil, Revision: res.Revision}
		if err != nil {
			out.Error = err.Error()
			out.Revision = c.opts.Config.Revision()
		}
		_ = cl.Send(out)
		if err == nil {
			c.noteLookChosen() // Style Switcher, Theme Tourist
		}
		if err == nil && res.Pending {
			c.askConfirm(res.Revision)
		}
	case shellipc.ConfirmResult:
		c.mu.Lock()
		rev, ok := c.confirms[msg.ConfirmID]
		delete(c.confirms, msg.ConfirmID)
		c.mu.Unlock()
		if !ok {
			return
		}
		if msg.Accepted {
			_ = c.opts.Config.Confirm(rev)
		} else {
			_ = c.opts.Config.Cancel(rev)
		}
	case shellipc.PairIssue:
		// Only trusted local peers (the shell, the cli) reach this socket:
		// phones never issue invitations, family or guest.
		var iss pairing.Issued
		var err error
		if msg.Pass == "" {
			iss, err = c.opts.Pairing.Issue(ctx, nil)
		} else {
			iss, err = c.opts.Pairing.IssuePass(ctx, msg.Pass)
		}
		var data any
		if err == nil {
			d := map[string]any{"code": iss.Code, "url": iss.URL, "expires_at_ms": iss.ExpiresAt.UnixMilli()}
			if iss.PassExpiresAt != nil {
				d["pass_expires_at_ms"] = iss.PassExpiresAt.UnixMilli()
			}
			data = d
		}
		h.reply(cl, msg.RequestID, err, data)
		if err == nil && msg.Pass != "" && c.opts.Achievements != nil {
			c.opts.Achievements.PassIssued() // Good Host
		}
	case shellipc.PairCancel:
		h.reply(cl, msg.RequestID, c.opts.Pairing.Cancel(ctx), nil)
	case shellipc.DevicesRevoke:
		err := c.opts.Pairing.Revoke(ctx, msg.DeviceID)
		if err == nil {
			if msg.DeviceID == "*" {
				c.holds.CancelAll("revoked")
			} else {
				c.holds.CancelDevice(msg.DeviceID, "revoked")
				c.dedup.Forget(msg.DeviceID)
			}
		}
		h.reply(cl, msg.RequestID, err, nil)
		c.publish()
	case shellipc.DevicesGrant:
		h.reply(cl, msg.RequestID, c.opts.Pairing.Grant(ctx, msg.DeviceID, msg.Permissions), nil)
		c.publish()
	case shellipc.RemoteConfigure:
		h.reply(cl, msg.RequestID, c.configureRemote(&msg), nil)
	case shellipc.RemoteNowPlaying:
		h.reply(cl, msg.RequestID, c.setNowPlaying(msg.Enabled), nil)
	case shellipc.AppEnable:
		h.reply(cl, msg.RequestID, c.setAppEnabled(ctx, msg.AppID, msg.Enabled), nil)
	case shellipc.CECConfigure:
		h.reply(cl, msg.RequestID, c.configureCEC(msg.Enabled, msg.VolumeTarget), nil)
	case shellipc.PlaybackSet:
		h.reply(cl, msg.RequestID, c.setPlayback(ctx, msg.Adapter, msg.Setting, msg.Value), nil)
	case shellipc.WeatherSearch:
		// Geocoding may take up to 10 s: answer from a goroutine so the
		// client's read loop keeps serving pings and other messages.
		go func() {
			out := shellipc.WeatherPlaces{Type: shellipc.TypeWeatherPlaces, RequestID: msg.RequestID, Places: []contract.WeatherPlace{}}
			if c.opts.Weather == nil {
				out.Error = "Weather is not available in this session."
			} else if places, err := c.opts.Weather.Search(ctx, msg.Query); err != nil {
				out.Error = err.Error()
			} else {
				out.OK, out.Places = true, places
			}
			_ = cl.Send(out)
		}()
	case shellipc.WeatherConfigure:
		h.reply(cl, msg.RequestID, c.configureWeather(&msg), nil)
	case shellipc.PlexSignIn, shellipc.PlexCancel, shellipc.PlexChooseServer, shellipc.PlexChooseLibraries, shellipc.PlexSignOut:
		h.handlePlex(cl, m)
	case shellipc.AchievementsConfigure, shellipc.AchievementsReset, shellipc.AchievementsCelebrated, shellipc.AchievementsEvent:
		h.receiveAchievements(cl, m, isShell)
	case shellipc.AppInstall:
		data, err := c.startInstall(msg.AppID)
		h.reply(cl, msg.RequestID, err, data)
	case shellipc.InstallRequest: // the older name of app.install
		data, err := c.startInstall(msg.AppID)
		h.reply(cl, msg.RequestID, err, data)
	case shellipc.AppInstallCancel:
		h.reply(cl, msg.RequestID, c.cancelInstall(msg.AppID), nil)
	case shellipc.AppInstallInfo:
		// Flathub may take seconds: answer from a goroutine so the read
		// loop keeps serving pings.
		go func() {
			data, err := c.installInfo(ctx, msg.AppID)
			h.reply(cl, msg.RequestID, err, data)
		}()
	case shellipc.AppsBrowser:
		h.reply(cl, msg.RequestID, c.setBrowsers(msg.Browser, msg.StreamingBrowser), nil)
	case shellipc.AppsConfigure:
		h.reply(cl, msg.RequestID, c.configureApps(msg.AutoUpdate), nil)
	case shellipc.OnboardingComplete:
		h.reply(cl, msg.RequestID, c.completeOnboarding(), nil)
	case shellipc.AutostartConfigure:
		h.reply(cl, msg.RequestID, c.configureAutostart(msg.Enabled), nil)
	case shellipc.PowerActivity:
		if isShell {
			c.onTVActivity()
		}
	case shellipc.ShellExit:
		if isShell && c.opts.Supervisor != nil {
			c.opts.Supervisor.MarkIntentionalExit()
		}
	default:
		c.log.Debug("session: ignoring shell message", "type", fmt.Sprintf("%T", m))
	}
}

func (h *ShellHandler) reply(cl *shellipc.Client, requestID string, err error, data any) {
	r := shellipc.Result{Type: shellipc.TypeResult, RequestID: requestID, OK: err == nil, Data: data}
	if err != nil {
		r.Error = err.Error()
	}
	_ = cl.Send(r)
}

func (c *Coordinator) onShellFocus(screen string, sectionID, itemID *string, textField bool) {
	switch screen {
	case "home", "settings", "pairing", "devices", "diagnostics", "dialog", "setup", "error":
	default:
		screen = "unknown"
	}
	c.mu.Lock()
	c.shellFocus = contract.ShellState{Screen: screen, Focus: contract.Focus{SectionID: sectionID, ItemID: itemID}}
	c.shellTextField = textField
	c.mu.Unlock()
	if screen == "home" {
		c.noteHome()
	}
	c.publish()
}

// configureWeather is weather.configure: validated like config.json (rule
// 10), coordinates rounded to 2 decimals before they are stored, then an
// immediate refresh; a new state follows.
func (c *Coordinator) configureWeather(m *shellipc.WeatherConfigure) error {
	if c.opts.Weather == nil {
		return errors.New("weather is not available in this session")
	}
	if m.Units != config.UnitsCelsius && m.Units != config.UnitsFahrenheit {
		return errors.New("units must be celsius or fahrenheit")
	}
	w := config.Weather{Enabled: m.Enabled, Units: m.Units, Scene: m.Scene}
	// place null keeps the stored place: the shell and the CLI only see its
	// name (state.weather.place), so toggles and unit changes send null.
	if m.Place == nil {
		w.Place = c.opts.Config.Current().WeatherSettings().Place
	}
	if m.Enabled && m.Place == nil && w.Place == nil {
		return errors.New("choose a place before turning weather on")
	}
	if m.Place != nil {
		p := *m.Place
		if n := len([]rune(p.Name)); n == 0 || n > 80 || len([]rune(p.Region)) > 80 || len([]rune(p.Country)) > 80 {
			return errors.New("the place name must be 1 to 80 characters (region and country up to 80)")
		}
		p.Latitude, p.Longitude = config.RoundCoordinate(p.Latitude), config.RoundCoordinate(p.Longitude)
		w.Place = &p
	}
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Weather = &w
		return nil
	}); err != nil {
		return err
	}
	c.opts.Weather.Configure(c.opts.Config.Current().WeatherSettings())
	c.opts.Weather.Kick()
	c.publish()
	return nil
}

// configureRemote is the trusted local onboarding step and the only path that
// may enable LAN exposure; enabling requires explicit consent. Turning the
// remote on also marks onboarding completed: a box whose owner already turned
// the phone remote on is treated as set up (contracts/config.md).
func (c *Coordinator) configureRemote(m *shellipc.RemoteConfigure) error {
	if m.Enabled && !m.LANConsent {
		return errors.New("enabling the phone remote requires consent to LAN exposure")
	}
	_, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Remote.Enabled = m.Enabled
		if m.Transport != "" {
			cfg.Remote.Transport = m.Transport
		}
		if m.Interface != "" {
			cfg.Remote.Interfaces = []string{m.Interface}
		}
		if m.Port != 0 {
			cfg.Remote.Port = m.Port
		}
		cfg.Remote.HTTPLayoutEditing = m.HTTPLayoutEditing
		cfg.Onboarding.LANConsent = m.LANConsent
		if m.Enabled {
			cfg.Onboarding.Completed = true
		}
		return nil
	})
	return err
}
