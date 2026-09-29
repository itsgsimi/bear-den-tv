// Sleep timer and screen off (contracts/actions.md power.sleep_timer,
// display.off and "Sleep, screen off and wake"; state.power): the timer runs
// on the injected clock; SleepWarning before it fires state.power.warning
// turns true and any input cancels it; when it fires Bear Den pauses the
// foreground app only through its verified MPRIS player, goes Home through
// doHome, then turns the display off (while locked: the display only).
// While the display is off, any action or TV key turns it on again and the
// press that woke it is swallowed. The display's own settings are restored
// when it comes back on and when the coordinator stops. With HDMI-CEC on
// (cec.go) the TV also goes to standby after the display step and comes on
// again with the display.

package session

import (
	"context"
	"math"
	"slices"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// Timings of the sleep timer and the display watch (variables for tests).
var (
	// SleepWarning is how long before the timer fires the TV warns.
	SleepWarning = 60 * time.Second
	// SleepIdlePoll is how often, during the warning, the time since the
	// last TV input is read (a key sent to an app in front is not seen by
	// the shell).
	SleepIdlePoll = time.Second
	// DisplayWakePoll is how often, while the display is off, its power
	// level is read to notice that a TV key turned it on.
	DisplayWakePoll = 2 * time.Second
)

const backendCoordinator = "coordinator"

// Step outcomes recorded when the timer fires (sleepOutcome).
const (
	stepObserved    = "observed"
	stepDelivered   = "delivered"
	stepSkipped     = "skipped"
	stepFailed      = "failed"
	stepUnavailable = "unavailable"
)

// sleepOutcome is what the last firing did, step by step, with honest
// outcomes; logged, kept for tests and diagnostics, never guessed.
type sleepOutcome struct {
	AtMs    int64
	Locked  bool
	Steps   []string // "pause", "home", "display" in the order they ran
	Pause   string
	Home    string
	Display string
	TV      string // HDMI-CEC standby; empty while HDMI-CEC is off
	Reasons map[string]string
}

// powerState is the sleep timer and display bookkeeping inside Coordinator
// (guarded by c.mu).
type powerState struct {
	gen        uint64 // bumps on every set/cancel/fire; stale callbacks compare it
	sleepAt    time.Time
	minutes    int
	fire, warn clock.Timer
	idle       clock.Timer // warning-time idle poll
	warning    bool
	warnAt     time.Time
	displayOff bool
	wakeGen    uint64
	wake       clock.Timer // display-off poll
	last       *sleepOutcome
}

// powerLocked is state.power for every viewer but anonymous ones.
func (c *Coordinator) powerLocked() *contract.Power {
	p := &contract.Power{Display: contract.DisplayOn, Warning: c.pw.warning}
	if c.pw.displayOff {
		p.Display = contract.DisplayOff
	}
	if !c.pw.sleepAt.IsZero() {
		ms := c.pw.sleepAt.Sub(c.start).Milliseconds()
		p.SleepAtMs, p.SleepMinutes = &ms, c.pw.minutes
	}
	if s := c.opts.Suspend; s != nil {
		// Bear Den never suspends; the report only says why not.
		p.Suspend = &contract.SuspendReport{Available: false, Reason: s.Reason}
	}
	return p
}

// powerCapsLocked adds the power actions to caps (unlocked only; a locked
// session reports every action unavailable).
func (c *Coordinator) powerCapsLocked(caps map[string]contract.Capability) {
	caps[contract.ActionSleepTimer] = available(backendCoordinator)
	switch d := c.opts.Display; {
	case d == nil:
		caps[contract.ActionDisplayOff] = unavailable("Turning the screen off is not available on this desktop.")
	default:
		if cp := d.Capability(); cp.Available {
			caps[contract.ActionDisplayOff] = available(cp.Backend)
		} else {
			caps[contract.ActionDisplayOff] = unavailable("The screen cannot be turned off here: " + cp.Reason)
		}
	}
}

// stopSleepLocked cancels the timer and its warning.
func (c *Coordinator) stopSleepLocked() {
	c.pw.gen++
	for _, t := range []clock.Timer{c.pw.fire, c.pw.warn, c.pw.idle} {
		if t != nil {
			t.Stop()
		}
	}
	c.pw.fire, c.pw.warn, c.pw.idle = nil, nil, nil
	c.pw.sleepAt, c.pw.minutes, c.pw.warning, c.pw.warnAt = time.Time{}, 0, false, time.Time{}
}

// setSleep sets (minutes > 0), replaces or cancels (0) the timer and
// returns sleep_at_ms, or nil after a cancel.
func (c *Coordinator) setSleep(minutes int) *int64 {
	c.mu.Lock()
	c.stopSleepLocked()
	var at *int64
	if minutes > 0 {
		gen, d := c.pw.gen, time.Duration(minutes)*time.Minute
		c.pw.sleepAt, c.pw.minutes = c.clock.Now().Add(d), minutes
		ms := c.pw.sleepAt.Sub(c.start).Milliseconds()
		at = &ms
		c.pw.warn = c.clock.AfterFunc(max(0, d-SleepWarning), func() { c.startSleepWarning(gen) })
		// The firing talks to the shell and the player: never on the
		// clock's own goroutine.
		c.pw.fire = c.clock.AfterFunc(d, func() { go c.fireSleep(gen) })
	}
	c.mu.Unlock()
	c.publish()
	return at
}

// cancelSleepWarning cancels the timer when its warning is showing: any
// input means someone is awake. It reports whether it did.
func (c *Coordinator) cancelSleepWarning(why string) bool {
	c.mu.Lock()
	warning := c.pw.warning
	if warning {
		c.stopSleepLocked()
	}
	c.mu.Unlock()
	if warning {
		c.log.Info("session: sleep timer cancelled during its warning", "by", why)
		c.publish()
	}
	return warning
}

func (c *Coordinator) startSleepWarning(gen uint64) {
	c.mu.Lock()
	if gen != c.pw.gen || c.pw.sleepAt.IsZero() {
		c.mu.Unlock()
		return
	}
	c.pw.warning, c.pw.warnAt = true, c.clock.Now()
	c.armIdlePollLocked(gen)
	c.mu.Unlock()
	c.publish()
}

func (c *Coordinator) armIdlePollLocked(gen uint64) {
	if c.opts.Display == nil {
		return
	}
	c.pw.idle = c.clock.AfterFunc(SleepIdlePoll, func() { c.sleepIdlePoll(gen) })
}

// sleepIdlePoll cancels the timer when the TV saw input since the warning
// began (a key sent to an app in front never reaches the shell).
func (c *Coordinator) sleepIdlePoll(gen uint64) {
	c.mu.Lock()
	ok, since := gen == c.pw.gen && c.pw.warning, c.clock.Since(c.pw.warnAt)
	c.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	st, err := c.opts.Display.Status(ctx)
	cancel()
	if err == nil && st.IdleKnown && st.Idle < since {
		c.cancelSleepWarning("tv input")
		return
	}
	c.mu.Lock()
	if gen == c.pw.gen && c.pw.warning {
		c.armIdlePollLocked(gen)
	}
	c.mu.Unlock()
}

// fireSleep is the timer firing: pause (verified only), Home, display off;
// while locked the display only.
func (c *Coordinator) fireSleep(gen uint64) {
	c.mu.Lock()
	if gen != c.pw.gen || c.pw.sleepAt.IsZero() {
		c.mu.Unlock()
		return
	}
	c.stopSleepLocked()
	locked := c.locked
	c.mu.Unlock()
	c.publish()

	ctx, cancel := context.WithTimeout(context.Background(), LaunchObserveTimeout)
	defer cancel()
	out := &sleepOutcome{AtMs: c.nowMs(), Locked: locked, Reasons: map[string]string{}}
	if locked {
		// Nothing reaches an app or the shell behind the lock screen.
		out.Pause, out.Home = stepSkipped, stepSkipped
		out.Reasons["pause"], out.Reasons["home"] = "the session is locked", "the session is locked"
	} else {
		out.Steps = append(out.Steps, "pause")
		out.Pause, out.Reasons["pause"] = c.sleepPause(ctx)
		out.Steps = append(out.Steps, "home")
		out.Home, out.Reasons["home"] = c.sleepHome(ctx)
	}
	out.Steps = append(out.Steps, "display")
	var already bool
	out.Display, out.Reasons["display"], already = c.turnDisplayOff(ctx)
	if already {
		out.Reasons["display"] = "it was already off"
	}
	// HDMI-CEC (cec.go): the TV to standby after the display step.
	if c.cecActive() {
		out.Steps = append(out.Steps, "tv")
		out.TV, out.Reasons["tv"] = c.sleepTV(ctx)
	}
	c.mu.Lock()
	c.pw.last = out
	c.mu.Unlock()
	c.log.Info("session: sleep timer fired", "locked", locked, "pause", out.Pause, "home", out.Home, "display", out.Display, "tv", out.TV,
		"pause_reason", out.Reasons["pause"], "home_reason", out.Reasons["home"], "display_reason", out.Reasons["display"])
}

// sleepPause pauses the app in front only through its own verified MPRIS
// player (the one media actions use); never a guessed key.
func (c *Coordinator) sleepPause(ctx context.Context) (string, string) {
	c.mu.Lock()
	t, m := c.target, c.media
	c.mu.Unlock()
	switch {
	case t.Kind != "app":
		return stepSkipped, "no app is in front"
	case m == nil || m.appID != strOr(t.AppID) || m.player == nil || !m.canCtl:
		return stepSkipped, t.Label + " has no verified media controls"
	}
	if err := m.player.Pause(ctx); err != nil {
		return stepFailed, t.Label + " refused to pause"
	}
	defer c.kickNowPlaying()
	for i := 0; ; i++ {
		if st, err := m.player.Status(ctx); err == nil && (st == "Paused" || st == "Stopped") {
			return stepObserved, ""
		}
		if time.Duration(i)*100*time.Millisecond >= MediaObserveTimeout {
			return stepDelivered, "the player did not report paused"
		}
		select {
		case <-ctx.Done():
			return stepDelivered, "the player did not report paused"
		case <-c.clock.After(100 * time.Millisecond):
		}
	}
}

// sleepHome goes Home through the same path as the home action.
func (c *Coordinator) sleepHome(ctx context.Context) (string, string) {
	req := contract.ActionRequest{Protocol: 1, RequestID: "sleep-" + randomID(), Target: "shell", Action: contract.ActionHome, Args: map[string]any{}}
	res := c.doHome(ctx, sender{key: shellSender}, req)
	if res.Outcome == contract.OutcomeFailed {
		return stepFailed, res.Message
	}
	return string(res.Outcome), res.Message
}

// turnDisplayOff turns the display off and starts watching for it coming on
// again. It returns the outcome, a reason, and whether it was already off.
func (c *Coordinator) turnDisplayOff(ctx context.Context) (string, string, bool) {
	d := c.opts.Display
	if d == nil {
		return stepUnavailable, "turning the screen off is not available on this desktop", false
	}
	if cp := d.Capability(); !cp.Available {
		return stepUnavailable, cp.Reason, false
	}
	c.mu.Lock()
	already := c.pw.displayOff
	c.mu.Unlock()
	if err := d.Off(ctx); err != nil {
		c.log.Warn("session: the display could not be turned off", "err", err)
		return stepFailed, "the display could not be turned off", already
	}
	c.mu.Lock()
	c.pw.displayOff = true
	c.pw.wakeGen++
	c.armWakePollLocked(c.pw.wakeGen)
	c.mu.Unlock()
	c.publish()
	if st, err := d.Status(ctx); err == nil && !st.On {
		return stepObserved, "", already
	}
	return stepDelivered, "the display did not report off", already
}

func (c *Coordinator) armWakePollLocked(gen uint64) {
	if c.pw.wake != nil {
		c.pw.wake.Stop()
	}
	c.pw.wake = c.clock.AfterFunc(DisplayWakePoll, func() { c.displayWakePoll(gen) })
}

// displayWakePoll notices that a TV key turned the display on by itself
// (with an app in front the shell never sees that key) and restores the
// display settings.
func (c *Coordinator) displayWakePoll(gen uint64) {
	c.mu.Lock()
	ok := gen == c.pw.wakeGen && c.pw.displayOff
	c.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	st, err := c.opts.Display.Status(ctx)
	cancel()
	if err == nil && st.On {
		c.wakeDisplay("tv input")
		return
	}
	c.mu.Lock()
	if gen == c.pw.wakeGen && c.pw.displayOff {
		c.armWakePollLocked(gen)
	}
	c.mu.Unlock()
}

// wakeDisplay turns the display on and restores its settings; it reports
// whether the display was off.
func (c *Coordinator) wakeDisplay(why string) bool {
	c.mu.Lock()
	if !c.pw.displayOff {
		c.mu.Unlock()
		return false
	}
	c.pw.displayOff = false
	c.pw.wakeGen++
	if c.pw.wake != nil {
		c.pw.wake.Stop()
		c.pw.wake = nil
	}
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := c.opts.Display.On(ctx); err != nil {
		c.log.Warn("session: the display settings could not be restored; retrying at exit", "err", err)
	}
	cancel()
	c.log.Info("session: display on", "by", why)
	c.publish()
	if why != "shutdown" {
		c.tvWake(why) // HDMI-CEC: the TV on and on Bear Den's input (cec.go)
	}
	return true
}

// onTVActivity is IPC power.activity: the shell swallowed a TV key while
// the display was off or the warning showed.
func (c *Coordinator) onTVActivity() {
	c.wakeDisplay("tv key")
	c.cancelSleepWarning("tv key")
}

// powerGate runs first for every authorized action (route): it wakes the
// display and cancels a warning. A press that woke the display is swallowed
// (true) unless it is a power action, which still applies.
func (c *Coordinator) powerGate(action string) (swallow bool) {
	if action == contract.ActionDisplayOff {
		c.cancelSleepWarning("display.off")
		return false // an off display stays off
	}
	woke := c.opts.Display != nil && c.wakeDisplay("action "+action)
	if !woke {
		c.wakeTVAfterStandby(action)
	}
	c.cancelSleepWarning("action " + action)
	// While locked the press is refused as locked anyway, which says more.
	_, _, locked := c.current()
	return woke && !locked && action != contract.ActionSleepTimer && action != contract.ActionTVPower
}

// doSleepTimer is the power.sleep_timer action.
func (c *Coordinator) doSleepTimer(req contract.ActionRequest) contract.ActionResult {
	// Phone requests were schema-checked; shell requests were not, so a
	// missing or odd value is refused here rather than read as a cancel.
	m := -1
	switch v := req.Args["minutes"].(type) {
	case float64:
		if v == math.Trunc(v) {
			m = int(v)
		}
	case int:
		m = v
	}
	if m != 0 && !slices.Contains(contract.SleepChoices, m) {
		return c.fail(req, contract.CodeInvalid, "Choose 15, 30, 45, 60, 90 or 120 minutes, or 0 to cancel.")
	}
	at := c.setSleep(m)
	if m > 0 && c.opts.Achievements != nil {
		c.opts.Achievements.SleepTimerSet() // Sleepy Bear
	}
	detail := map[string]any{"sleep_at_ms": nil, "minutes": m}
	if at != nil {
		detail["sleep_at_ms"] = *at
	}
	return c.result(req, contract.OutcomeObserved, detail)
}

// doDisplayOff is the display.off action.
func (c *Coordinator) doDisplayOff(ctx context.Context, req contract.ActionRequest) contract.ActionResult {
	cp, _ := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, contract.CodeUnsupported, cp.Reason)
	}
	outcome, reason, already := c.turnDisplayOff(ctx)
	detail := map[string]any{}
	if already {
		detail["already_off"] = true
	}
	if outcome == stepObserved || outcome == stepDelivered {
		c.standbyAfterDisplayOff() // HDMI-CEC, in the background (cec.go)
	}
	switch outcome {
	case stepObserved:
		return c.result(req, contract.OutcomeObserved, detail)
	case stepDelivered:
		res := c.result(req, contract.OutcomeDelivered, detail)
		res.Message = "The screen was asked to turn off, but did not report off."
		return res
	case stepUnavailable:
		return c.fail(req, contract.CodeUnsupported, "The screen cannot be turned off here: "+reason)
	}
	return c.fail(req, contract.CodeInternal, "The screen could not be turned off.")
}

// stopPower is the coordinator stopping: the timer is dropped and the
// display turned on with its settings restored.
func (c *Coordinator) stopPower() {
	c.mu.Lock()
	c.stopSleepLocked()
	c.mu.Unlock()
	c.wakeDisplay("shutdown")
}

// lastSleep is the outcome of the last firing (tests, diagnostics).
func (c *Coordinator) lastSleep() *sleepOutcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pw.last
}
