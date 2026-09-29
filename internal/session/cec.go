// TV control over HDMI-CEC (contracts/actions.md "TV control over HDMI-CEC",
// tv.power; state.cec; config cec; ADR 0008). Everything here is off unless
// Options.TV finds an adapter and the owner enabled config cec.enabled:
//
//   - the sleep timer and display.off send Standby after the display step;
//   - the press that wakes the display, any action after Bear Den put the TV
//     in standby, a TV key while the display was off, and home send Image
//     View On and Active Source, in the background (tvWake);
//   - with cec.volume_target tv, audio.volume_delta and audio.mute press the
//     TV's volume keys instead of changing the PC's volume;
//   - watchCEC probes the adapter and reads the TV's power once a minute
//     (CECPoll) and after every command, off the coordinator's lock.
//
// Every HDMI-CEC call is bounded by CECTimeout; none runs under c.mu.

package session

import (
	"context"
	"errors"
	"math"
	"time"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

// HDMI-CEC timings (variables for tests).
var (
	// CECTimeout bounds one HDMI-CEC command.
	CECTimeout = 3 * time.Second
	// CECPoll is how often the adapter is probed and, while enabled, the
	// TV's power read.
	CECPoll = time.Minute
)

const backendCEC = "hdmi-cec"

// cecVolumeStep is how much of audio.volume_delta one TV volume key press
// stands for; at most cecMaxPresses presses per action.
const (
	cecVolumeStep  = 5
	cecMaxPresses  = 5
	cecNoAdapter   = "TV control over HDMI-CEC is not available on this installation."
	cecOffReason   = "TV control over HDMI (CEC) is off. Turn it on in TV Settings."
	cecCheckReason = "Looking for an HDMI-CEC adapter…"
)

// cecState is the HDMI-CEC bookkeeping inside Coordinator (guarded by c.mu).
type cecState struct {
	probed      bool
	cap         platform.Capability
	tvPower     string // contract.TVPower*
	standbyByUs bool   // Bear Den put the TV in standby; the next input wakes it
	wasEnabled  bool   // for releasing the adapter when the owner turns it off
	kick        chan struct{}
}

func (c *Coordinator) cecSettings() config.CEC { return c.opts.Config.Current().CECSettings() }

// cecCapLocked is the capability of HDMI-CEC control as a whole: available
// only with an adapter and the owner's setting on.
func (c *Coordinator) cecCapLocked(s config.CEC) contract.Capability {
	switch {
	case c.opts.TV == nil:
		return unavailable(cecNoAdapter)
	case !c.cec.probed:
		return unavailable(cecCheckReason)
	case !c.cec.cap.Available:
		return unavailable(c.cec.cap.Reason)
	case !s.Enabled:
		return unavailable(cecOffReason)
	}
	return available(backendCEC)
}

// cecActive reports whether HDMI-CEC commands may be sent now.
func (c *Coordinator) cecActive() bool {
	s := c.cecSettings()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cecCapLocked(s).Available
}

// cecStateLocked is state.cec.
func (c *Coordinator) cecStateLocked(s config.CEC) *contract.CEC {
	st := &contract.CEC{Enabled: s.Enabled, VolumeTarget: s.VolumeTarget, TVPower: contract.TVPowerUnknown}
	if st.VolumeTarget == "" {
		st.VolumeTarget = contract.VolumeTargetPC
	}
	switch {
	case c.opts.TV == nil:
		st.Reason = cecNoAdapter
	case !c.cec.probed:
		st.Reason = cecCheckReason
	case !c.cec.cap.Available:
		st.Reason = c.cec.cap.Reason
	default:
		st.Available = true
		if s.Enabled && c.cec.tvPower != "" {
			st.TVPower = c.cec.tvPower
		}
	}
	return st
}

// cecCapsLocked adds tv.power and, while the volume buttons drive the TV,
// replaces the audio capabilities (unlocked only).
func (c *Coordinator) cecCapsLocked(caps map[string]contract.Capability) {
	s := c.cecSettings()
	cp := c.cecCapLocked(s)
	caps[contract.ActionTVPower] = cp
	if s.TVVolume() {
		if !cp.Available {
			cp = unavailable("TV volume over HDMI-CEC is not available: " + cp.Reason)
		}
		caps[contract.ActionAudioVolume] = cp
		caps[contract.ActionAudioMute] = cp
	}
}

// kickCEC asks watchCEC to probe and read the TV now.
func (c *Coordinator) kickCEC() {
	select {
	case c.cec.kick <- struct{}{}:
	default:
	}
}

// watchCEC probes the adapter and reads the TV's power at start, every
// CECPoll and on kickCEC, until ctx is done; it releases the adapter when
// the owner turns HDMI-CEC off.
func (c *Coordinator) watchCEC(ctx context.Context) {
	for {
		c.refreshCEC(ctx)
		select {
		case <-ctx.Done():
			return
		case <-c.cec.kick:
		case <-c.clock.After(CECPoll):
		}
	}
}

func (c *Coordinator) refreshCEC(ctx context.Context) {
	s := c.cecSettings()
	c.mu.Lock()
	release := c.cec.wasEnabled && !s.Enabled
	c.cec.wasEnabled = s.Enabled
	c.mu.Unlock()
	if release {
		// Give the logical address back; Probe below reopens without one.
		_ = c.opts.TV.Close()
	}
	pctx, cancel := context.WithTimeout(ctx, CECTimeout)
	cp := c.opts.TV.Probe(pctx)
	cancel()
	power := contract.TVPowerUnknown
	if cp.Available && s.Enabled {
		power = c.readTVPower(ctx)
	}
	c.mu.Lock()
	c.cec.probed, c.cec.cap, c.cec.tvPower = true, cp, power
	c.mu.Unlock()
	c.publish()
}

// readTVPower asks the TV for its power (bounded); unknown when it does not answer.
func (c *Coordinator) readTVPower(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, CECTimeout)
	defer cancel()
	p, err := c.opts.TV.PowerStatus(ctx)
	if err != nil {
		return contract.TVPowerUnknown
	}
	switch p {
	case platform.TVPowerOn:
		return contract.TVPowerOn
	case platform.TVPowerStandby:
		return contract.TVPowerStandby
	}
	return contract.TVPowerUnknown
}

// noteTVPower records a fresh reading and publishes it.
func (c *Coordinator) noteTVPower(p string) {
	c.mu.Lock()
	changed := c.cec.tvPower != p
	c.cec.tvPower = p
	c.mu.Unlock()
	if changed {
		c.publish()
	}
}

// tvOn sends Image View On, then Active Source. It returns the first error.
func (c *Coordinator) tvOn(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, CECTimeout)
	defer cancel()
	err := c.opts.TV.PowerOn(ctx)
	// Active Source even when the TV did not acknowledge Image View On: some
	// TVs only wake on it.
	if aerr := c.opts.TV.ActiveSource(ctx); err == nil {
		err = aerr
	}
	if err == nil {
		c.mu.Lock()
		c.cec.standbyByUs = false
		c.mu.Unlock()
	}
	return err
}

// tvStandby sends Standby to the TV.
func (c *Coordinator) tvStandby(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, CECTimeout)
	defer cancel()
	if err := c.opts.TV.Standby(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.cec.standbyByUs = true
	c.mu.Unlock()
	return nil
}

// tvWake turns the TV on and makes Bear Den its input, in the background;
// a no-op unless HDMI-CEC is active.
func (c *Coordinator) tvWake(why string) {
	if !c.cecActive() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*CECTimeout)
		defer cancel()
		if err := c.tvOn(ctx); err != nil {
			c.log.Info("session: the TV could not be woken over HDMI-CEC", "by", why, "err", err)
		}
		c.noteTVPower(c.readTVPower(ctx))
	}()
}

// wakeTVAfterStandby is powerGate's part: any action after Bear Den put the
// TV in standby wakes it (the display wake calls tvWake itself).
func (c *Coordinator) wakeTVAfterStandby(action string) {
	c.mu.Lock()
	was := c.cec.standbyByUs
	c.mu.Unlock()
	if was && action != contract.ActionTVPower && action != contract.ActionDisplayOff {
		c.tvWake("action " + action)
	}
}

// sleepTV is the sleep timer's last step: Standby after the display step.
func (c *Coordinator) sleepTV(ctx context.Context) (string, string) {
	if !c.cecActive() {
		return stepSkipped, "TV control over HDMI-CEC is off or unavailable"
	}
	if err := c.tvStandby(ctx); err != nil {
		c.log.Info("session: the TV did not take standby over HDMI-CEC", "err", err)
		return stepFailed, "the TV did not acknowledge standby"
	}
	c.noteTVPower(c.readTVPower(ctx))
	return stepDelivered, ""
}

// standbyAfterDisplayOff is display.off's HDMI-CEC step, in the background.
func (c *Coordinator) standbyAfterDisplayOff() {
	if !c.cecActive() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*CECTimeout)
		defer cancel()
		if err := c.tvStandby(ctx); err != nil {
			c.log.Info("session: the TV did not take standby over HDMI-CEC", "err", err)
		}
		c.noteTVPower(c.readTVPower(ctx))
	}()
}

// doTVPower is the tv.power action.
func (c *Coordinator) doTVPower(ctx context.Context, req contract.ActionRequest) contract.ActionResult {
	cp, _ := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, contract.CodeUnsupported, cp.Reason)
	}
	want, _ := req.Args["power"].(string)
	var err error
	switch want {
	case contract.TVPowerArgOn:
		err = c.tvOn(ctx)
	case contract.TVPowerArgStandby:
		err = c.tvStandby(ctx)
	default:
		return c.fail(req, contract.CodeInvalid, "Choose on or standby.")
	}
	if err != nil {
		c.log.Info("session: tv.power over HDMI-CEC failed", "power", want, "err", err)
		return c.cecFailed(req, err, "The TV did not acknowledge the HDMI-CEC message.")
	}
	got := c.readTVPower(ctx)
	c.noteTVPower(got)
	if got == want {
		return c.result(req, contract.OutcomeObserved, map[string]any{"tv_power": got})
	}
	res := c.result(req, contract.OutcomeDelivered, map[string]any{"tv_power": got})
	res.Message = "The TV acknowledged the message but did not report " + want + "."
	return res
}

// cecFailed maps an HDMI-CEC error to a result: timeout when the call ran
// out of time, otherwise internal with msg.
func (c *Coordinator) cecFailed(req contract.ActionRequest, err error, msg string) contract.ActionResult {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return c.fail(req, contract.CodeTimeout, "The HDMI-CEC adapter did not answer in time.")
	}
	return c.fail(req, contract.CodeInternal, msg)
}

// cecPresses is how many volume key presses audio.volume_delta stands for.
func cecPresses(delta int) int {
	n := int(math.Ceil(math.Abs(float64(delta)) / cecVolumeStep))
	return max(1, min(cecMaxPresses, n))
}

// doTVVolume is audio.volume_delta and audio.mute with cec.volume_target tv.
func (c *Coordinator) doTVVolume(ctx context.Context, req contract.ActionRequest) contract.ActionResult {
	cp, _ := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, contract.CodeUnsupported, cp.Reason)
	}
	ctx, cancel := context.WithTimeout(ctx, CECTimeout)
	defer cancel()
	var keys []platform.TVKey
	if req.Action == contract.ActionAudioMute {
		k := platform.TVUnmute
		if boolArg(req.Args, "muted") {
			k = platform.TVMute
		}
		keys = []platform.TVKey{k}
	} else {
		delta := intArg(req.Args, "delta")
		k := platform.TVVolumeUp
		if delta < 0 {
			k = platform.TVVolumeDown
		}
		for range cecPresses(delta) {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		if err := c.opts.TV.VolumeKey(ctx, k); err != nil {
			c.log.Info("session: TV volume over HDMI-CEC failed", "key", k, "err", err)
			return c.cecFailed(req, err, "TV volume could not be changed over HDMI-CEC.")
		}
	}
	// The TV does not report its volume: delivered, never observed.
	return c.result(req, contract.OutcomeDelivered, map[string]any{"volume_target": contract.VolumeTargetTV})
}

// configureCEC is IPC cec.configure: stores config cec and probes at once.
func (c *Coordinator) configureCEC(enabled bool, target string) error {
	if target != contract.VolumeTargetPC && target != contract.VolumeTargetTV {
		return errors.New("volume_target must be pc or tv")
	}
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.CEC = &config.CEC{Enabled: enabled, VolumeTarget: target}
		return nil
	}); err != nil {
		return err
	}
	if !enabled {
		c.mu.Lock()
		c.cec.standbyByUs = false
		c.mu.Unlock()
	}
	c.kickCEC()
	c.publish()
	return nil
}
