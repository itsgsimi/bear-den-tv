// Now playing for Plex HTPC from the owner's Plex server: Plex HTPC
// publishes no MPRIS player (seen on a real TV, 2026-09-29), so while a
// controller phone is connected and Plex HTPC is in front or was in front
// before Home, the coordinator asks the signed-in Plex connector what this
// TV's Plex HTPC is playing (internal/plexlink NowPlaying: /status/sessions
// on the chosen server) every PlexPlayingPoll, backing off on errors
// (PlexPlayingBackoff). The reading becomes state.now_playing with source
// "plex_server"; it is read-only (Bear Den cannot verify a control sent
// through the server, so none is offered). Privacy is Now playing's
// (contracts/http.md, docs/security.md): memory only, never logged, never in
// the shell view; the owner's diagnostics carry only the status and reason.

package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/plexlink"
)

// PlexPlaying asks the Plex connector what this TV's Plex HTPC is playing
// (plexlink.Manager).
type PlexPlaying interface {
	NowPlaying(ctx context.Context) (plexlink.Playing, error)
}

// PlexPlayingPoll is how often the server is asked while wanted.
var PlexPlayingPoll = 5 * time.Second

// PlexPlayingBackoff is the pause after the 1st, 2nd, ... failed ask in a
// row; the last value repeats.
var PlexPlayingBackoff = []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}

// plexPlayingTimeout bounds one ask.
const plexPlayingTimeout = 5 * time.Second

// msgPlexAskFailed is the status reason while the server cannot be asked.
const msgPlexAskFailed = "Your Plex server could not be asked what is playing."

// plexReading is the latest answer for one app.
type plexReading struct {
	appID  string
	p      plexlink.Playing
	readAt int64 // coordinator monotonic ms
}

// plexNPState is the Plex bookkeeping inside Coordinator.
type plexNPState struct {
	cur        *plexReading
	status     string // last status for diagnostics: off | idle | found | unidentified | error
	reason     string
	kick       chan struct{}
	iterations atomic.Int64 // completed loop passes (tests wait on it)
	asks       atomic.Int64 // asks sent to the connector (tests)
	loop       plexLoopState
}

// plexLoopState is where the loop waits, for tests to know it is idle: parked
// in its select, with its timer armed for deadline or not armed.
type plexLoopState struct {
	mu       sync.Mutex
	parked   bool
	armed    bool
	deadline time.Time
}

func (l *plexLoopState) set(parked, armed bool, deadline time.Time) {
	l.mu.Lock()
	l.parked, l.armed, l.deadline = parked, armed, deadline
	l.mu.Unlock()
}

// idle reports whether the loop waits with nothing due at now.
func (l *plexLoopState) idle(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.parked && (!l.armed || l.deadline.After(now))
}

// plexAppLocked is the configured Plex HTPC app whose playback may be
// shown now: a controller phone is connected, the owner allows Now playing,
// the session is unlocked, the app is in front or behind Home, and it has no
// MPRIS player of its own (which would win).
func (c *Coordinator) plexAppLocked(cfg config.Config) (string, bool) {
	if c.opts.PlexPlaying == nil || c.locked || !cfg.Remote.ShowNowPlaying() || c.np.phoneStreams == 0 {
		return "", false
	}
	for _, a := range cfg.Applications {
		if a.Adapter != adapters.PlexHTPCName {
			continue
		}
		switch {
		case c.target.Kind == "app" && strOr(c.target.AppID) == a.ID:
			if c.media != nil && c.media.appID == a.ID && c.media.player != nil {
				return "", false
			}
		case c.target.Kind == "shell" && c.behind != nil && c.behind.appID == a.ID:
			if c.behind.player != nil {
				return "", false
			}
		default:
			return "", false
		}
		return a.ID, true
	}
	return "", false
}

func (c *Coordinator) kickPlexPlaying() {
	select {
	case c.plexNP.kick <- struct{}{}:
	default:
	}
}

// watchPlexPlaying asks the connector while plexAppLocked says so: at once
// when kicked (a phone connects, the foreground changes), then every
// PlexPlayingPoll, or after the backoff when the ask failed. It runs until
// ctx is done.
func (c *Coordinator) watchPlexPlaying(ctx context.Context) {
	timer := c.clock.NewTimer(PlexPlayingPoll)
	defer timer.Stop()
	armed, read, failures := true, true, 0
	var deadline time.Time
	for {
		if read {
			cfg := c.opts.Config.Current()
			c.mu.Lock()
			appID, want := c.plexAppLocked(cfg)
			c.mu.Unlock()
			if !want {
				c.setPlexReading(nil, "", "")
				timer.Stop()
				armed = false
			} else {
				rctx, cancel := context.WithTimeout(ctx, plexPlayingTimeout)
				c.plexNP.asks.Add(1)
				p, err := c.opts.PlexPlaying.NowPlaying(rctx)
				cancel()
				delay := PlexPlayingPoll
				if err != nil {
					failures++
					delay = PlexPlayingBackoff[min(failures, len(PlexPlayingBackoff))-1]
					c.setPlexReading(nil, "error", msgPlexAskFailed)
				} else {
					failures = 0
					c.setPlexReading(&plexReading{appID: appID, p: p}, p.Status, p.Reason)
				}
				timer.Reset(delay)
				armed, deadline = true, c.clock.Now().Add(delay)
			}
		}
		c.plexNP.iterations.Add(1)
		read = false
		var tick <-chan time.Time
		if armed {
			tick = timer.C()
		}
		c.plexNP.loop.set(true, armed, deadline)
		select {
		case <-ctx.Done():
			return
		case <-c.plexNP.kick:
			c.plexNP.loop.set(false, armed, deadline)
			// A kick asks at once, except during a backoff.
			read = failures == 0 || !armed
		case <-tick:
			c.plexNP.loop.set(false, armed, deadline)
			armed, read = false, true
		}
	}
}

// setPlexReading stores a reading (nil forgets it) and the status for
// diagnostics, logs a status change (never a title) and publishes when what
// phones see may have changed.
func (c *Coordinator) setPlexReading(r *plexReading, status, reason string) {
	c.mu.Lock()
	prev := c.plexNP.cur
	if r != nil {
		r.readAt = c.nowMs()
	}
	c.plexNP.cur = r
	statusChanged := status != c.plexNP.status || reason != c.plexNP.reason
	c.plexNP.status, c.plexNP.reason = status, reason
	c.mu.Unlock()
	if statusChanged && status != "" {
		c.log.Info("session: Plex server playback for Now playing", "status", status, "reason", reason)
	}
	if prev != nil || r != nil || statusChanged {
		c.publish()
	}
}

// plexNowPlayingLocked is state.now_playing from the Plex server's reading,
// for the Plex app in front (foreground true) or behind Home (false), or nil.
// buffering is sent as playing at rate 0, so phones hold the position.
func (c *Coordinator) plexNowPlayingLocked() *contract.NowPlaying {
	r := c.plexNP.cur
	if r == nil || c.locked || r.p.Status != plexlink.PlayingFound {
		return nil
	}
	var fg bool
	switch {
	case c.target.Kind == "app" && strOr(c.target.AppID) == r.appID:
		fg = true
	case c.target.Kind == "shell" && c.behind != nil && c.behind.appID == r.appID:
	default:
		return nil
	}
	title := clip(r.p.Title)
	status, rate := "", 1.0
	switch r.p.State {
	case "playing":
		status = contract.NowPlayingPlaying
	case "buffering":
		status, rate = contract.NowPlayingPlaying, 0
	case "paused":
		status = contract.NowPlayingPaused
	}
	if title == "" || status == "" {
		return nil
	}
	src := contract.NowPlayingSourcePlexServer
	np := &contract.NowPlaying{AppID: r.appID, Foreground: &fg, Source: &src, Title: title, Status: status, PositionAt: r.readAt, Rate: rate}
	if s := clip(r.p.Subtitle); s != "" {
		np.Subtitle = s
	}
	if ms := r.p.Duration.Milliseconds(); ms > 0 {
		np.LengthMs = &ms
	}
	pos := max(r.p.Offset.Milliseconds(), 0)
	if np.LengthMs != nil && pos > *np.LengthMs {
		pos = *np.LengthMs
	}
	np.PositionMs = &pos
	return np
}

// plexMediaReasonLocked is why media actions are unavailable for appID
// while its playback comes from the Plex server, or "" when it does not.
func (c *Coordinator) plexMediaReasonLocked(appID, label string) string {
	r := c.plexNP.cur
	if r == nil || r.appID != appID {
		return ""
	}
	switch r.p.Status {
	case plexlink.PlayingFound:
		return label + " is shown from your Plex server, read-only: control it on the TV."
	case plexlink.PlayingUnidentified:
		return r.p.Reason
	}
	return ""
}

// plexDiagnostics is the owners' diagnostics entry: status and reason only.
func (c *Coordinator) plexDiagnostics() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.plexNP.status
	if st == "" {
		st = "not_asked"
	}
	return map[string]any{"status": st, "reason": c.plexNP.reason, "source": "plex_server"}
}
