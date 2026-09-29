// Home's pause for a native app in front (contracts/actions.md `home`,
// adapters.HomePause): only a pause Bear Den can verify. The app's own MPRIS
// player must report Playing before and Paused (or Stopped) after; nothing
// else is ever sent. A pause key that toggles (RetroArch's p) is never sent,
// because without a readable paused state it could resume a paused game.
// Web apps pause through their page instead (web.go, pauseWebForHome).

package session

import (
	"context"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

// homePausePoll is how often Home re-reads a player's status after Pause.
const homePausePoll = 50 * time.Millisecond

// pauseAppForHome pauses a native app in front before Home when its
// home_policy is pause-if-supported and its HomePause is "mpris". It returns
// the result detail ({"paused": bool}) or nil when Home does not try (web
// apps, leave-running, no pause declared). paused is true only when the
// player was seen Playing and then Paused.
func (c *Coordinator) pauseAppForHome(ctx context.Context, target contract.Target) map[string]any {
	appID := strOr(target.AppID)
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok || app.HomePolicy != "pause-if-supported" {
		return nil
	}
	ad, ok := c.opts.Adapters.ForName(app.Adapter)
	if !ok {
		return nil
	}
	if _, isWeb := adapters.WebOf(ad); isWeb {
		return nil
	}
	hp := adapters.HomePauseOf(ad)
	switch hp.Kind {
	case "mpris":
	case "key":
		// No readable state for a key: a toggle could resume a paused app,
		// and even a plain pause key could not be confirmed. Left alone.
		c.log.Info("session: Home leaves the app running (its pause key cannot be verified)", "app", appID)
		return map[string]any{"paused": false}
	default:
		return nil
	}
	player := c.homePlayer(ctx, appID, ad.MediaMatch())
	if player == nil {
		return map[string]any{"paused": false}
	}
	ctx, cancel := context.WithTimeout(ctx, MediaObserveTimeout)
	defer cancel()
	if st, err := player.Status(ctx); err != nil || st != "Playing" {
		return map[string]any{"paused": false} // nothing playing: nothing to pause
	}
	if err := player.Pause(ctx); err != nil {
		c.log.Info("session: app refused to pause for Home", "app", appID, "err", err)
		return map[string]any{"paused": false}
	}
	defer c.kickNowPlaying()
	for {
		if st, err := player.Status(ctx); err == nil && (st == "Paused" || st == "Stopped") {
			return map[string]any{"paused": true}
		}
		select {
		case <-ctx.Done():
			c.log.Info("session: Home sent Pause but the player did not report paused", "app", appID)
			return map[string]any{"paused": false}
		case <-time.After(homePausePoll):
		}
	}
}

// homePlayer is the app's own controllable MPRIS player: the one the media
// actions use when it was already found, else a fresh exact match.
func (c *Coordinator) homePlayer(ctx context.Context, appID, match string) platform.MediaPlayer {
	c.mu.Lock()
	m := c.media
	c.mu.Unlock()
	if m != nil && m.appID == appID && m.player != nil {
		if !m.canCtl {
			return nil
		}
		return m.player
	}
	if c.opts.Media == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, MediaObserveTimeout)
	defer cancel()
	p, found, err := c.opts.Media.Find(ctx, match)
	if err != nil || !found {
		return nil
	}
	if can, err := p.CanControl(ctx); err != nil || !can {
		return nil
	}
	return p
}
