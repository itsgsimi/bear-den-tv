// Package session is the coordinator core: it observes the desktop (foreground,
// lock), owns the context epoch and the authoritative state snapshot, routes
// named actions to the shell or to verified external windows, and implements
// both the remote.Backend seam (phones) and the shellipc.Handler seam (shell).
package session

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"bear-den-tv/internal/achievements"
	"bear-den-tv/internal/actions"
	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/themes"
	"bear-den-tv/internal/weather"
)

// ShellLabel is the target label for the TV shell.
const ShellLabel = "Bear Den TV"

// ShellClassFragments identify the shell's top-level window by WM_CLASS, or
// by app_id on Wayland (Qt uses the binary name for both).
var ShellClassFragments = []string{"bear-den-tv-shell", "beardentvshell"}

// Timeouts for asynchronous results (contracts/actions.md).
var (
	ActivateObserveTimeout = 5 * time.Second
	LaunchObserveTimeout   = 30 * time.Second
	MediaObserveTimeout    = 2 * time.Second
)

// CloseFollowFor is how long a closed app stays watched for windows it opens
// in reply to the close; those are closed too. Moonlight answers a close of its
// stream window by showing its host list again. Relaunching the app ends the
// watch early.
var CloseFollowFor = 4 * time.Second

const closeFollowPoll = 250 * time.Millisecond

// Options wires the coordinator to its collaborators. Desktop, Config, and
// Pairing are required; the rest may be nil and are reported as unavailable.
type Options struct {
	Clock    clock.Clock
	Logger   *slog.Logger
	Desktop  platform.DesktopAdapter
	Lock     platform.LockObserver
	Audio    platform.AudioBackend
	Media    platform.MediaLocator
	Launcher applications.Launcher
	Adapters *adapters.Registry
	Config   *config.Store
	Pairing  *pairing.Service
	// Tuner runs the playback detection test after startup and applies the
	// best settings (config startup.tune_apps); nil skips it (dev, tests).
	Tuner Tuner
	// Themes resolves layout.ui.background into what phones show
	// (state.appearance); nil gives phones the bare colours.
	Themes *themes.Registry
	Feed   *providers.Feed
	// Weather keeps the local weather reading (state.weather, shell view)
	// and answers weather.search; nil reports weather as unavailable.
	Weather *weather.Poller
	// Display turns the display off and on (display.off, the sleep timer);
	// nil reports display.off unavailable (power.go).
	Display platform.DisplayPower
	// Suspend is what logind said about suspending (state.power.suspend);
	// nil omits it. Bear Den never suspends.
	Suspend *platform.Capability
	// Plex is the Plex sign-in flow and rows (state.plex, state.content,
	// plex.* IPC); nil when the session has no Plex connector.
	Plex PlexLink
	// Achievements counts Den badges (state.achievements, achievements.*
	// IPC; achievements.go); nil omits them.
	Achievements *achievements.Tracker
	// Supervisor restarts the shell for shell.restart; nil when --no-shell.
	Supervisor *shellipc.Supervisor
	DevMode    bool
	// AppsRefresh overrides DefaultAppsRefresh (tests).
	AppsRefresh time.Duration
	// Diagnostics produces the redacted doctor report for owners.
	Diagnostics func(ctx context.Context) map[string]any
}

type appRuntime struct {
	install     applications.Installation
	discovered  bool
	instance    *applications.Instance
	launchState string
	lastError   *string
}

type mediaProbe struct {
	appID    string
	player   platform.MediaPlayer
	canCtl   bool
	probedAt time.Time
}

// Coordinator is the single owner of session state.
type Coordinator struct {
	opts    Options
	log     *slog.Logger
	clock   clock.Clock
	start   time.Time
	dedup   *actions.Dedup
	holds   *actions.Holds
	limits  contract.Limits
	shellMu sync.Mutex
	shellIP *shellipc.Server

	mu             sync.Mutex
	epoch          int64
	locked         bool
	foreground     platform.Foreground
	target         contract.Target
	targetWindow   platform.WindowID
	shellWindow    platform.WindowID
	shellPID       int
	shellFocus     contract.ShellState
	shellTextField bool // the shell's last focus report had text_field true
	shellState     string
	apps           map[string]*appRuntime
	media          *mediaProbe
	np             npState    // now playing for phones (nowplaying.go); memory only
	pw             powerState // sleep timer and display (power.go)
	notifications  []contract.Notification
	previewing     bool
	remote         contract.RemoteState
	confirms       map[string]int64 // confirm_id → pending layout revision
	audioCap       platform.Capability
	fullscreened   map[platform.WindowID]bool // app windows already asked to go fullscreen
	closing        map[string]*closeWatch     // app id → watch for windows opened while closing
	playback       *contract.Playback         // latest playback detection summary (shell view)
	tune           tuneState
	lastShellJSON  []byte // last state sent to the shell (dedupe)
	// shellStartedLocked: the shell connected while the session was locked or
	// inactive, when logind withholds the GPU; it is restarted on unlock so it
	// renders with the GPU instead of software (measured: ~170% vs ~25% CPU).
	shellStartedLocked bool
	lockKnown          bool // the lock observer has reported at least once

	subs    map[chan struct{}]struct{}
	results map[chan contract.ActionResult]string // channel → device id ("" = shell)
}

// New builds a coordinator; call Run to start observing the desktop.
func New(opts Options) *Coordinator {
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Adapters == nil {
		opts.Adapters = adapters.NewRegistry()
	}
	if opts.AppsRefresh <= 0 {
		opts.AppsRefresh = DefaultAppsRefresh
	}
	c := &Coordinator{
		opts:         opts,
		log:          opts.Logger,
		clock:        opts.Clock,
		start:        opts.Clock.Now(),
		dedup:        actions.NewDedup(opts.Clock),
		limits:       contract.DefaultLimits,
		epoch:        1,
		target:       contract.Target{Kind: "none", Label: "Nothing"},
		shellFocus:   contract.ShellState{Screen: "unknown"},
		shellState:   shellipc.StateStopped,
		apps:         map[string]*appRuntime{},
		subs:         map[chan struct{}]struct{}{},
		results:      map[chan contract.ActionResult]string{},
		confirms:     map[string]int64{},
		audioCap:     platform.Capability{Reason: "checking PC audio"},
		fullscreened: map[platform.WindowID]bool{},
		closing:      map[string]*closeWatch{},
		remote:       contract.RemoteState{Transport: "local-only", Addresses: []string{}, Limits: contract.DefaultLimits},
	}
	c.holds = actions.NewHolds(opts.Clock, c.limits, c.holdTap, c.publish)
	c.np.kick = make(chan struct{}, 1)
	c.initAchievements()
	return c
}

// AttachShellServer gives the coordinator the IPC server it pushes state and
// input through. It must be called before shells connect.
func (c *Coordinator) AttachShellServer(s *shellipc.Server) {
	c.shellMu.Lock()
	c.shellIP = s
	c.shellMu.Unlock()
}

func (c *Coordinator) shellClient() *shellipc.Client {
	c.shellMu.Lock()
	s := c.shellIP
	c.shellMu.Unlock()
	if s == nil {
		return nil
	}
	return s.Shell()
}

// SetShellState records a supervisor state change (stopped, starting, ...).
func (c *Coordinator) SetShellState(st string) {
	c.mu.Lock()
	c.shellState = st
	c.mu.Unlock()
	c.publish()
}

// SetRemoteStatus records the listener status for state.remote.
func (c *Coordinator) SetRemoteStatus(listening bool, addresses []string) {
	c.mu.Lock()
	c.remote.Listening = listening
	if addresses == nil {
		addresses = []string{}
	}
	c.remote.Addresses = addresses
	c.mu.Unlock()
	c.publish()
}

// Notify queues a transient notification for the TV and phones.
func (c *Coordinator) Notify(kind, text string) {
	if len(text) > 200 {
		text = text[:200]
	}
	now := c.nowMs()
	n := contract.Notification{ID: randomID(), Kind: kind, Text: text, CreatedMs: now}
	c.mu.Lock()
	c.notifications = append(c.notifications, n)
	if len(c.notifications) > 5 {
		c.notifications = c.notifications[len(c.notifications)-5:]
	}
	c.mu.Unlock()
	if sh := c.shellClient(); sh != nil {
		_ = sh.Send(shellipc.Notify{Type: shellipc.TypeNotify, ID: n.ID, NotifyKind: kind, Text: text})
	}
	c.publish()
}

func (c *Coordinator) nowMs() int64 { return c.clock.Since(c.start).Milliseconds() }

// Run observes foreground and lock changes until ctx is done.
func (c *Coordinator) Run(ctx context.Context) error {
	go c.autoTune(ctx)
	c.discoverApps(ctx)
	if c.opts.Audio != nil {
		go c.watchAudio(ctx)
	}
	go c.watchApps(ctx)
	go c.watchNowPlaying(ctx)
	go c.watchRevocations(ctx)
	// Subscribe before the initial read so a change between the two is never lost.
	fgCh, err := c.opts.Desktop.WatchForeground(ctx)
	if err != nil {
		c.log.Warn("session: foreground watch unavailable", "err", err)
		fgCh = nil
	}
	if fg, err := c.opts.Desktop.ObserveForeground(ctx); err == nil {
		c.onForeground(fg)
	}
	var lockCh <-chan bool
	if c.opts.Lock != nil {
		if l, err := c.opts.Lock.Locked(ctx); err == nil {
			c.onLock(l)
		}
		if ch, err := c.opts.Lock.Watch(ctx); err == nil {
			lockCh = ch
		} else {
			c.log.Warn("session: lock watch unavailable", "err", err)
		}
	}
	tick := c.clock.NewTimer(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			c.holds.CancelAll("shutdown")
			c.stopPower() // the display on, its settings restored
			return nil
		case fg, ok := <-fgCh:
			if !ok {
				fgCh = nil
				continue
			}
			c.onForeground(fg)
		case l, ok := <-lockCh:
			if !ok {
				lockCh = nil
				continue
			}
			c.onLock(l)
		case <-tick.C():
			// Countdowns (pairing, pending layout) and the content feed have
			// no change signal of their own; publish() drops unchanged
			// snapshots, so this costs one state build per second.
			c.publish()
			tick.Reset(time.Second)
		}
	}
}

// watchRevocations cancels the holds and forgets the de-dup entries of every
// revoked device, whoever revoked it: the TV, an owner phone, the phone
// itself, or the end of a guest pass (internal/pairing/guest.go).
func (c *Coordinator) watchRevocations(ctx context.Context) {
	ch, err := c.opts.Pairing.Revocations(ctx)
	if err != nil {
		c.log.Warn("session: revocation watch unavailable", "err", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-ch:
			c.holds.CancelDevice(id, "revoked")
			c.dedup.Forget(id)
			c.publish()
		}
	}
}

// AudioRefresh is how often the PC audio capability is re-probed; the probe
// execs pactl, so it never runs under the state lock.
var AudioRefresh = 15 * time.Second

// DefaultAppsRefresh is how often running applications are reconciled against
// the window list and Flatpak instances, so an app closed from inside (or that
// crashed) stops showing as running.
const DefaultAppsRefresh = 3 * time.Second

func (c *Coordinator) watchApps(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.opts.AppsRefresh):
		}
		c.reconcileApps(ctx)
	}
}

// reconcileApps derives each app's liveness from what is really there: a
// mapped window matched by its adapter, or a live Flatpak instance.
func (c *Coordinator) reconcileApps(ctx context.Context) {
	cfg := c.opts.Config.Current()
	wins, werr := c.opts.Desktop.ListWindows(ctx)
	hasWindow := map[string]bool{}
	if werr == nil {
		for _, a := range cfg.Applications {
			ad, ok := c.opts.Adapters.ForName(a.Adapter)
			if !ok {
				continue
			}
			for _, w := range wins {
				if w.Mapped && ad.MatchWindow(w) {
					hasWindow[a.ID] = true
					break
				}
			}
		}
	}
	live := map[string]bool{}
	if c.opts.Launcher != nil {
		if insts, err := c.opts.Launcher.Instances(ctx); err == nil {
			for _, in := range insts {
				live[in.FlatpakID] = true
			}
		} else if werr != nil {
			return // no evidence either way; change nothing
		}
	}
	changed, exited := false, false
	var exitedIDs []string
	c.mu.Lock()
	for _, a := range cfg.Applications {
		rt := c.appLocked(a.ID)
		running := hasWindow[a.ID] || live[a.Launch.AppID]
		switch {
		case rt.launchState == "launching":
			// doLaunch owns the transition out of launching.
		case running && rt.launchState != "running":
			rt.launchState = "running"
			changed = true
		case !running && (rt.launchState == "running" || rt.instance != nil):
			rt.launchState = "exited"
			rt.instance = nil
			changed, exited = true, true
			exitedIDs = append(exitedIDs, a.Launch.AppID)
		}
	}
	// An app that exits leaves whatever window is underneath in front (often the
	// bare desktop); bring Bear Den back unless something recognized is in front.
	// (The target may still name the exited app if its foreground change has
	// not been processed yet, so judge by whether that app still has a window.)
	frontAlive := c.target.Kind == "app" && hasWindow[strOr(c.target.AppID)]
	returnHome := exited && !c.locked && c.target.Kind != "shell" && !frontAlive
	// Forget fullscreen bookkeeping for windows that no longer exist.
	if werr == nil {
		present := map[platform.WindowID]bool{}
		for _, w := range wins {
			present[w.ID] = true
		}
		for id := range c.fullscreened {
			if !present[id] {
				delete(c.fullscreened, id)
			}
		}
	}
	c.mu.Unlock()
	if changed {
		c.publish()
	}
	if returnHome {
		c.activateShell(ctx)
	}
	if len(exitedIDs) > 0 {
		// Settings that waited for these apps to close (they rewrite their
		// own settings on exit, so this runs after they are gone).
		go c.tuneAfterExit(exitedIDs)
	}
}

// activateShell brings the shell window to the front (no-op when not found).
func (c *Coordinator) activateShell(ctx context.Context) {
	if win, ok := c.findShellWindow(ctx); ok {
		if err := c.opts.Desktop.Activate(ctx, win); err != nil {
			c.log.Warn("session: could not bring the shell forward", "err", err)
		}
	}
}

// ensureFullscreen asks the desktop to make an app window fullscreen once per
// window: TV clients belong fullscreen, and some (Plex HTPC, Moonlight) start
// windowed.
func (c *Coordinator) ensureFullscreen(w platform.WindowInfo) {
	fs, ok := c.opts.Desktop.(platform.Fullscreener)
	if !ok || w.Fullscreen {
		return
	}
	c.mu.Lock()
	if c.fullscreened[w.ID] {
		c.mu.Unlock()
		return
	}
	c.fullscreened[w.ID] = true
	c.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := fs.SetFullscreen(ctx, w.ID, true); err != nil {
			c.log.Warn("session: fullscreen request failed", "window", uint64(w.ID), "err", err)
		}
	}()
}

func (c *Coordinator) watchAudio(ctx context.Context) {
	for {
		cp := c.opts.Audio.Capability()
		c.mu.Lock()
		changed := cp != c.audioCap
		c.audioCap = cp
		c.mu.Unlock()
		if changed {
			c.publish()
		}
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(AudioRefresh):
		}
	}
}

func (c *Coordinator) discoverApps(ctx context.Context) {
	if c.opts.Launcher == nil {
		return
	}
	for _, a := range c.opts.Config.Current().Applications {
		inst, err := c.opts.Launcher.Discover(ctx, a.Launch.AppID)
		c.mu.Lock()
		rt := c.appLocked(a.ID)
		if err == nil {
			rt.install = inst
			rt.discovered = true
		} else {
			rt.install = applications.Installation{Scope: "unknown"}
		}
		c.mu.Unlock()
	}
	c.setInstalledApps()
	c.publish()
}

func (c *Coordinator) appLocked(id string) *appRuntime {
	rt := c.apps[id]
	if rt == nil {
		rt = &appRuntime{launchState: "idle"}
		c.apps[id] = rt
	}
	return rt
}

func (c *Coordinator) onLock(locked bool) {
	c.mu.Lock()
	if !c.lockKnown {
		// A shell that connected before the first observation started under
		// whatever this first reading says.
		c.lockKnown = true
		if !locked {
			c.shellStartedLocked = false
		}
	}
	changed := c.locked != locked
	c.locked = locked
	if changed {
		c.retargetLocked()
		c.epoch++ // lock and unlock always invalidate what phones saw
		c.clearNowPlayingLocked()
	}
	c.mu.Unlock()
	if changed {
		if locked {
			c.holds.CancelAll("locked")
		}
		c.mu.Lock()
		restart := !locked && c.shellStartedLocked && c.opts.Supervisor != nil
		c.shellStartedLocked = false
		c.mu.Unlock()
		if restart {
			c.log.Info("session: restarting shell after unlock to regain GPU rendering")
			go func() { _ = c.opts.Supervisor.Restart() }()
		}
		if !locked {
			// The app in front may have changed behind the lock screen.
			go c.probeMedia(context.Background())
		}
		c.kickNowPlaying()
		c.publish()
	}
}

func (c *Coordinator) onForeground(fg platform.Foreground) {
	c.mu.Lock()
	c.foreground = fg
	prev := c.target
	c.retargetLocked()
	changed := prev.Kind != c.target.Kind || strOr(prev.AppID) != strOr(c.target.AppID)
	if changed {
		c.epoch++
		c.media = nil
		c.clearNowPlayingLocked()
	}
	c.mu.Unlock()
	if changed {
		c.holds.CancelAll("target_changed")
		c.kickNowPlaying()
		go c.probeMedia(context.Background())
	}
	if c.Target().Kind == "app" && fg.Known {
		c.ensureFullscreen(fg.Window)
	}
	if changed {
		c.noteHome() // Home back in front (counts only when it is on Home)
	}
	c.publish()
}

// retargetLocked derives the control target from lock + foreground.
func (c *Coordinator) retargetLocked() {
	c.targetWindow = 0
	switch {
	case c.locked:
		c.target = contract.Target{Kind: "locked", Label: "Locked", Observed: true}
		return
	case !c.foreground.Known:
		c.target = contract.Target{Kind: "unknown", Label: "Unknown window", Observed: false}
		return
	}
	w := c.foreground.Window
	if c.isShellWindow(w) {
		c.shellWindow = w.ID
		c.targetWindow = w.ID
		label := ShellLabel
		c.target = contract.Target{Kind: "shell", Label: ShellLabel, Observed: true, WindowTitle: &label}
		return
	}
	for _, a := range c.opts.Config.Current().Applications {
		ad, ok := c.opts.Adapters.ForName(a.Adapter)
		if !ok || !ad.MatchWindow(w) {
			continue
		}
		id, label := a.ID, a.Label
		c.targetWindow = w.ID
		// Window titles of external apps may name private media; redact to the label.
		c.target = contract.Target{Kind: "app", AppID: &id, Label: label, Observed: true, WindowTitle: &label}
		rt := c.appLocked(a.ID)
		if rt.launchState == "launching" || rt.launchState == "idle" || rt.launchState == "exited" {
			rt.launchState = "running"
		}
		return
	}
	c.target = contract.Target{Kind: "unknown", Label: "Another window", Observed: true}
}

func (c *Coordinator) isShellWindow(w platform.WindowInfo) bool {
	if c.shellPID > 0 && w.PID == c.shellPID {
		return true
	}
	return adapters.MatchClass(w.Class, ShellClassFragments) || adapters.MatchAppID(w.AppID, "", ShellClassFragments)
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Epoch returns the current context epoch.
func (c *Coordinator) Epoch() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

// Target returns the current control target.
func (c *Coordinator) Target() contract.Target {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.target
}

// publish wakes every subscriber; each rebuilds its own redacted view.
func (c *Coordinator) publish() {
	c.mu.Lock()
	subs := make([]chan struct{}, 0, len(c.subs))
	for ch := range c.subs {
		subs = append(subs, ch)
	}
	c.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	c.shellMu.Lock()
	s := c.shellIP
	c.shellMu.Unlock()
	if s == nil {
		return
	}
	// Build the shell view once and skip identical snapshots: every snapshot
	// makes the shell re-parse and re-bind the whole UI.
	st := c.buildState(viewShell)
	c.observePlex(st)
	key := stateKey(st)
	// Never call into the IPC server while holding c.mu: it calls the
	// handler (which takes c.mu) while holding its own lock.
	connected := s.Shell() != nil
	c.mu.Lock()
	same := connected && bytes.Equal(key, c.lastShellJSON)
	if !same {
		c.lastShellJSON = key
	}
	c.mu.Unlock()
	if same {
		return
	}
	s.Broadcast(func(*shellipc.Client) contract.State { return st })
}

// stateKey is the snapshot's identity for change detection: its JSON without
// the always-changing generated_at_ms.
func stateKey(st contract.State) []byte {
	st.GeneratedAtMs = 0
	b, _ := json.Marshal(st)
	return b
}

func (c *Coordinator) subscribe(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 1)
	c.mu.Lock()
	c.subs[ch] = struct{}{}
	c.mu.Unlock()
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		delete(c.subs, ch)
		c.mu.Unlock()
	}()
	return ch
}

// emitResult delivers a late action result to the sender's result streams.
func (c *Coordinator) emitResult(deviceID string, res contract.ActionResult) {
	c.mu.Lock()
	var out []chan contract.ActionResult
	for ch, dev := range c.results {
		if dev == deviceID {
			out = append(out, ch)
		}
	}
	c.mu.Unlock()
	for _, ch := range out {
		select {
		case ch <- res:
		default:
		}
	}
}
