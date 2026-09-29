// Now playing: what an app's own MPRIS player reports, kept in memory for
// phones with the controller permission or a guest pass (state.now_playing;
// contracts/http.md "Now playing", docs/security.md): the app in front, or,
// while the shell is in front, the app that was in front before Home as long
// as its player reports Playing or Paused (now_playing.foreground false; the
// media actions then name that app as their target, doMedia). Never logged,
// persisted or put in the shell view, diagnostics or exports.

package session

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

// NowPlayingPoll is how often a playing player's position is re-read while
// a phone is connected. The position is not signalled as it advances, and
// phones extrapolate between readings, so this only corrects drift.
var NowPlayingPoll = 5 * time.Second

// nowPlayingReadTimeout bounds one read of a player over D-Bus.
const nowPlayingReadTimeout = 2 * time.Second

// nowPlaying is the latest reading of the foreground app's player.
type nowPlaying struct {
	appID      string
	foreground bool // false: appID is behind Home
	player     platform.MediaPlayer
	info       platform.MediaInfo
	readAt     int64 // coordinator monotonic ms (the clock of generated_at_ms)
}

// npState is the now-playing bookkeeping inside Coordinator.
type npState struct {
	cur          *nowPlaying
	phoneStreams int // live phone state streams with the controller permission
	kick         chan struct{}
	iterations   atomic.Int64 // completed loop passes (tests wait on it)
}

// kickNowPlaying asks the watcher to re-evaluate and re-read now.
func (c *Coordinator) kickNowPlaying() {
	select {
	case c.np.kick <- struct{}{}:
	default:
	}
}

// clearNowPlayingLocked forgets the reading at once (retarget, lock); the
// watcher is kicked by the caller to drop its subscription.
func (c *Coordinator) clearNowPlayingLocked() bool {
	had := c.np.cur != nil
	c.np.cur = nil
	return had
}

// nowPlayingSubjectLocked is the player whose state may be shown: the
// foreground app's own player (found by mediaMatchFor, exactly as media
// actions use it), or with the shell in front the player of the app behind
// Home (foreground false); only while unlocked and while the owner allows it.
func (c *Coordinator) nowPlayingSubjectLocked(cfg config.Config) (platform.MediaPlayer, string, bool) {
	if c.locked || !cfg.Remote.ShowNowPlaying() {
		return nil, "", false
	}
	switch c.target.Kind {
	case "app":
		appID := strOr(c.target.AppID)
		if c.media == nil || c.media.appID != appID || c.media.player == nil {
			return nil, "", false
		}
		return c.media.player, appID, true
	case "shell":
		if b := c.behind; b != nil && b.player != nil {
			return b.player, b.appID, false
		}
	}
	return nil, "", false
}

// noteBehindLocked follows the app behind Home on a change of target from
// prev: an app giving way to the shell stays behind it with the player it
// had (found again by probeMedia when it had none yet); another app in
// front ends it. Lock and unknown windows leave it as it is.
func (c *Coordinator) noteBehindLocked(prev contract.Target) {
	switch {
	case c.target.Kind == "shell" && prev.Kind == "app":
		appID := strOr(prev.AppID)
		if m := c.media; m != nil && m.appID == appID {
			b := *m
			c.behind = &b
		} else {
			c.behind = &mediaProbe{appID: appID}
		}
	case c.target.Kind == "app":
		c.behind = nil
	}
}

// behindControlLocked is the app behind Home whose playback phones may
// control from the shell: its own player was found, allows control, reports
// Playing or Paused (the reading phones see), and it is not a web app (a web
// page's video is controlled only through the page, with the app in front).
// Otherwise nil, with the reason when there is a reading to explain.
func (c *Coordinator) behindControlLocked() (*mediaProbe, string) {
	b := c.behind
	if c.target.Kind != "shell" || b == nil || b.player == nil {
		return nil, ""
	}
	cur := c.np.cur
	if cur == nil || cur.foreground || cur.appID != b.appID || c.nowPlayingLocked() == nil {
		return nil, ""
	}
	label := b.appID
	if app, ok := c.opts.Config.Current().Application(b.appID); ok {
		label = app.Label
	}
	if _, isWeb := c.webSpecFor(b.appID); isWeb {
		return nil, "Open " + label + " to control it."
	}
	if !b.canCtl {
		return nil, label + " does not expose verified media controls."
	}
	return b, label
}

// watchNowPlaying keeps c.np.cur in step with the foreground app's player:
// it subscribes to that player's change signals, re-reads on every signal
// and kick, and re-reads a playing player every NowPlayingPoll while a phone
// is connected. It runs until ctx is done.
func (c *Coordinator) watchNowPlaying(ctx context.Context) {
	var (
		watched     platform.MediaPlayer
		stopWatch   = func() {}
		changes     <-chan struct{}
		poll        = c.clock.NewTimer(NowPlayingPoll)
		read        = true
		pollPending = false
	)
	defer func() { stopWatch(); poll.Stop() }()
	for {
		if read {
			cfg := c.opts.Config.Current()
			c.mu.Lock()
			player, appID, fg := c.nowPlayingSubjectLocked(cfg)
			c.mu.Unlock()
			if player != watched {
				stopWatch()
				watched, changes, stopWatch = player, nil, func() {}
				if player != nil {
					wctx, cancel := context.WithCancel(ctx)
					if ch, err := player.Watch(wctx); err == nil {
						changes, stopWatch = ch, cancel
					} else {
						cancel()
						// The error names the bus, never what is playing.
						c.log.Info("session: media changes not signalled; re-reading on actions and the poll only", "err", err)
					}
				}
			}
			c.refreshNowPlaying(ctx, player, appID, fg)
			poll.Reset(NowPlayingPoll)
			pollPending = true
		}
		c.np.iterations.Add(1)
		read = false
		var pollC <-chan time.Time
		if pollPending {
			pollC = poll.C()
		}
		select {
		case <-ctx.Done():
			return
		case <-c.np.kick:
			read = true
		case _, ok := <-changes:
			if !ok {
				changes = nil
				continue
			}
			read = true
		case <-pollC:
			pollPending = false
			read = c.nowPlayingPollWanted()
			if !read {
				poll.Reset(NowPlayingPoll)
				pollPending = true
			}
		}
	}
}

// nowPlayingPollWanted: the position is worth re-reading only while a phone
// is connected and something is playing.
func (c *Coordinator) nowPlayingPollWanted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.np.phoneStreams > 0 && c.np.cur != nil && c.np.cur.info.Status == "Playing"
}

// refreshNowPlaying reads player (nil clears) and publishes when what phones
// would see changed. A failed read fails closed: nothing is shown.
func (c *Coordinator) refreshNowPlaying(ctx context.Context, player platform.MediaPlayer, appID string, fg bool) {
	var next *nowPlaying
	if player != nil {
		rctx, cancel := context.WithTimeout(ctx, nowPlayingReadTimeout)
		info, err := player.Info(rctx)
		cancel()
		if err == nil {
			next = &nowPlaying{appID: appID, foreground: fg, player: player, info: info}
		}
	}
	cfg := c.opts.Config.Current()
	c.mu.Lock()
	// The foreground may have changed while the player was being read.
	if cur, curApp, curFg := c.nowPlayingSubjectLocked(cfg); next != nil && (cur != player || curApp != appID || curFg != fg) {
		next = nil
	}
	prev := c.np.cur
	changed := true
	switch {
	case next == nil:
		changed = prev != nil
		c.np.cur = nil
	case prev != nil && prev.player == next.player && prev.foreground == next.foreground && sameMedia(prev.info, next.info) && next.info.Status != "Playing":
		changed = false // a paused or stopped player re-read to the same state
	default:
		next.readAt = c.nowMs()
		c.np.cur = next
	}
	c.mu.Unlock()
	if changed {
		c.publish()
	}
}

func sameMedia(a, b platform.MediaInfo) bool {
	return a.Status == b.Status && a.Title == b.Title && a.Album == b.Album && slices.Equal(a.Artists, b.Artists) &&
		a.Length == b.Length && a.HasPosition == b.HasPosition && a.Position == b.Position && a.Rate == b.Rate
}

// nowPlayingLocked is state.now_playing for the current target, or nil when
// there is nothing honest to show (no title, an unknown status, a reading
// for another app, a stopped player behind Home).
func (c *Coordinator) nowPlayingLocked() *contract.NowPlaying {
	cur := c.np.cur
	if cur == nil || c.locked {
		return nil
	}
	switch {
	case cur.foreground && c.target.Kind == "app" && strOr(c.target.AppID) == cur.appID:
	case !cur.foreground && c.target.Kind == "shell" && c.behind != nil && c.behind.appID == cur.appID:
		if cur.info.Status != "Playing" && cur.info.Status != "Paused" {
			return nil
		}
	default:
		return nil
	}
	info := cur.info
	title := clip(info.Title)
	var status string
	switch info.Status {
	case "Playing":
		status = contract.NowPlayingPlaying
	case "Paused":
		status = contract.NowPlayingPaused
	case "Stopped":
		status = contract.NowPlayingStopped
	}
	if title == "" || status == "" {
		return nil
	}
	fg := cur.foreground
	np := &contract.NowPlaying{AppID: cur.appID, Foreground: &fg, Title: title, Status: status, PositionAt: cur.readAt, Rate: info.Rate}
	if np.Rate < 0 {
		np.Rate = 0
	}
	switch {
	case len(info.Artists) > 0:
		np.Subtitle = clip(strings.Join(info.Artists, ", "))
	case info.Album != "":
		np.Subtitle = clip(info.Album)
	}
	if ms := info.Length.Milliseconds(); ms > 0 {
		np.LengthMs = &ms
	}
	if info.HasPosition {
		ms := max(info.Position.Milliseconds(), 0)
		if np.LengthMs != nil && ms > *np.LengthMs {
			ms = *np.LengthMs
		}
		np.PositionMs = &ms
	}
	return np
}

// clip trims s and cuts it to contract.NowPlayingTextMax characters.
func clip(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= contract.NowPlayingTextMax {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:contract.NowPlayingTextMax-1])) + "…"
}

// phoneStreamOpened counts a controller phone's state stream (the poll runs
// only while one is open) until ctx ends.
func (c *Coordinator) phoneStreamOpened(ctx context.Context) {
	c.mu.Lock()
	c.np.phoneStreams++
	c.mu.Unlock()
	c.kickNowPlaying()
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		c.np.phoneStreams--
		c.mu.Unlock()
	}()
}

// setNowPlaying is remote.now_playing (IPC): store the owner's choice; the
// next snapshots gain or lose state.now_playing at once.
func (c *Coordinator) setNowPlaying(enabled bool) error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Remote.NowPlaying = &enabled
		return nil
	}); err != nil {
		return err
	}
	if !enabled {
		c.mu.Lock()
		c.clearNowPlayingLocked()
		c.mu.Unlock()
	}
	c.kickNowPlaying()
	c.publish()
	return nil
}
