// Den badges in the coordinator: the event points that move
// internal/achievements counters (a launch that worked, Home shown, a phone
// paired, a guest pass issued, a sleep timer set, the parade, a look chosen in
// Settings), the achievements.* IPC messages, and state.achievements per view
// (buildStateFor). Spec: contracts/http.md#den-badges-stateachievements,
// contracts/ipc.md; privacy: docs/security.md#den-badges.

package session

import (
	"context"
	"errors"

	"bear-den-tv/internal/achievements"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/themes"
	"bear-den-tv/internal/weather"
)

// errNoBadges answers achievements.* when the session has no tracker.
var errNoBadges = errors.New("badges are not available in this session")

// initAchievements wires the tracker to publish and gives it the built-in
// themes (Theme Tourist).
func (c *Coordinator) initAchievements() {
	t := c.opts.Achievements
	if t == nil {
		return
	}
	t.SetOnChange(c.publish)
	t.SetUniverse(achievements.PrefixTheme, themes.BuiltIn())
}

// setInstalledApps gives the tracker the installed apps (Couch Explorer).
func (c *Coordinator) setInstalledApps() {
	t := c.opts.Achievements
	if t == nil || c.opts.Launcher == nil {
		return
	}
	var ids []string
	c.mu.Lock()
	for _, a := range c.opts.Config.Current().Applications {
		if rt := c.apps[a.ID]; rt != nil && rt.install.Installed {
			ids = append(ids, a.ID)
		}
	}
	c.mu.Unlock()
	t.SetUniverse(achievements.PrefixApp, ids)
}

// noteLaunched counts a launch that worked (doLaunch). Never under c.mu.
func (c *Coordinator) noteLaunched(app configApp) {
	if t := c.opts.Achievements; t != nil {
		t.Launched(app.ID, app.Adapter)
	}
}

// look is the art style and canonical theme id the TV shows now.
func (c *Coordinator) look() (style, theme string) {
	ui := c.opts.Config.Current().Layout().UI
	style = contract.ArtPixel
	if ui.Classic() {
		style = contract.ArtClassic
	}
	theme = ui.Background
	if c.opts.Themes != nil {
		theme = c.opts.Themes.Canonical(ui.Background)
	}
	return style, theme
}

// noteHome counts Home being shown to someone: the shell is in front, the
// session unlocked and the shell's last focus report was on Home. Called
// after focus reports and when the shell comes back to the front; never
// under c.mu.
func (c *Coordinator) noteHome() {
	t := c.opts.Achievements
	if t == nil {
		return
	}
	c.mu.Lock()
	onHome := !c.locked && c.target.Kind == "shell" && c.shellFocus.Screen == "home"
	c.mu.Unlock()
	if !onHome {
		return
	}
	h := achievements.Home{}
	h.Style, h.Theme = c.look()
	if c.opts.Weather != nil {
		// Only a fresh reading, and only while weather is on (off is
		// status disabled): no weather, no weather badges.
		if w := c.opts.Weather.Snapshot(); w.Status == weather.StatusReady && w.Current != nil {
			h.Weather = w.Current.Condition
		}
	}
	t.HomeShown(h)
}

// NotePaired counts a phone that just paired (pairing.Options.OnPaired):
// Family Den wants two family phones paired at once, so only phones that are
// not guest passes count.
func (c *Coordinator) NotePaired() {
	t := c.opts.Achievements
	if t == nil {
		return
	}
	list, err := c.opts.Pairing.List(context.Background())
	if err != nil {
		return
	}
	family := 0
	for _, d := range list {
		if !d.Guest {
			family++
		}
	}
	t.Paired(family)
}

// noteLookChosen counts the art style and theme after a Settings change.
func (c *Coordinator) noteLookChosen() {
	if t := c.opts.Achievements; t != nil {
		t.LookSeen(c.look())
	}
}

// configureAchievements is achievements.configure: store config
// achievements.enabled; off stops counting at once.
func (c *Coordinator) configureAchievements(enabled bool) error {
	if c.opts.Achievements == nil {
		return errNoBadges
	}
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Achievements = &config.Achievements{Enabled: enabled}
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	return nil
}

// receiveAchievements handles the achievements.* IPC messages; it reports
// false for any other message. celebrated and event come from the shell
// only and have no reply.
func (h *ShellHandler) receiveAchievements(cl *shellipc.Client, m shellipc.Message, isShell bool) bool {
	c := h.c
	t := c.opts.Achievements
	switch msg := m.(type) {
	case shellipc.AchievementsConfigure:
		h.reply(cl, msg.RequestID, c.configureAchievements(msg.Enabled), nil)
	case shellipc.AchievementsReset:
		if t == nil {
			h.reply(cl, msg.RequestID, errNoBadges, nil)
			return true
		}
		err := t.Reset()
		if err == nil {
			c.log.Info("achievements: badges reset from the TV")
		}
		h.reply(cl, msg.RequestID, err, nil)
	case shellipc.AchievementsCelebrated:
		if isShell && t != nil {
			t.Celebrated(msg.IDs)
		}
	case shellipc.AchievementsEvent:
		if !isShell || t == nil {
			return true
		}
		switch msg.Event {
		case shellipc.AchievementEventParade:
			t.Parade()
		default:
			c.log.Warn("achievements: ignoring an unknown shell event", "event", msg.Event)
		}
	default:
		return false
	}
	return true
}
