// Session tests for TV control over HDMI-CEC (cec.go) with a fake TV
// (platform/fake/tv.go) on the fake clock: absent adapter and the owner's
// setting both keep the bus silent; the sleep timer sends Standby only
// after the display went off; wake, Home and any action after standby send
// Image View On + Active Source; tv.power outcomes and deadlines; volume
// routing to the TV and back; state.cec per view; cec.configure.

package session

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

type cecHarness struct {
	*pwHarness
	tv    *fake.TV
	audio *fakeAudio
}

// fakeAudio is a PC sink that records what it was asked.
type fakeAudio struct {
	mu    sync.Mutex
	calls []string
}

func (a *fakeAudio) Capability() platform.Capability {
	return platform.Capability{Available: true, Backend: "fake-pulse"}
}

func (a *fakeAudio) VolumeDelta(_ context.Context, percent int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, "delta")
	return nil
}

func (a *fakeAudio) SetMute(_ context.Context, muted bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, "mute")
	return nil
}

func (a *fakeAudio) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

func newCECHarness(t *testing.T, tv *fake.TV) *cecHarness {
	t.Helper()
	audio := &fakeAudio{}
	h := newPowerHarness(t, func(o *Options) { o.TV, o.Audio = tv, audio })
	ch := &cecHarness{pwHarness: h, tv: tv, audio: audio}
	ch.cecState("the adapter probed", func(s *contract.CEC) bool { return !strings.Contains(s.Reason, "Looking") })
	return ch
}

func (h *cecHarness) cecState(what string, cond func(s *contract.CEC) bool) *contract.CEC {
	h.t.Helper()
	var s *contract.CEC
	h.eventually(what, func() bool {
		s = h.phones.Snapshot(context.Background(), &h.ctl).CEC
		return s != nil && cond(s)
	})
	return s
}

// configure sends IPC cec.configure as the shell does and waits for it.
func (h *cecHarness) configure(enabled bool, target string) {
	h.t.Helper()
	if err := h.shell.Send(shellipc.CECConfigure{Type: shellipc.TypeCECConfigure, RequestID: "cec-" + randomID(), Enabled: enabled, VolumeTarget: target}); err != nil {
		h.t.Fatal(err)
	}
	h.cecState("cec.configure applied", func(s *contract.CEC) bool { return s.Enabled == enabled && s.VolumeTarget == target })
}

// poll advances the fake clock by CECPoll until cond holds (the watcher may
// not be waiting on the clock yet when the first advance happens).
func (h *cecHarness) poll(what string, cond func(s *contract.CEC) bool) {
	h.t.Helper()
	h.eventually(what, func() bool {
		h.clk.Advance(CECPoll)
		s := h.phones.Snapshot(context.Background(), &h.ctl).CEC
		return s != nil && cond(s)
	})
}

func (h *cecHarness) caps() map[string]contract.Capability {
	return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities
}

func (h *cecHarness) waitCalls(what string, want ...string) {
	h.t.Helper()
	h.eventually(what, func() bool {
		calls := h.tv.Calls()
		i := 0
		for _, c := range calls {
			if i < len(want) && c == want[i] {
				i++
			}
		}
		return i == len(want)
	})
}

// commands are the TV calls other than power status reads.
func commands(calls []string) []string {
	out := []string{}
	for _, c := range calls {
		if c != "status" {
			out = append(out, c)
		}
	}
	return out
}

func TestCECWithoutAnAdapterIsUnavailableWithTheReason(t *testing.T) {
	tv := fake.NewTV()
	const reason = "No HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter"
	tv.SetCapability(platform.Capability{Backend: fake.TVBackend, Reason: reason})
	h := newCECHarness(t, tv)
	s := h.cecState("unavailable", func(s *contract.CEC) bool { return !s.Available })
	if s.Reason != reason || s.Enabled || s.TVPower != contract.TVPowerUnknown || s.VolumeTarget != contract.VolumeTargetPC {
		t.Fatalf("state.cec %+v", s)
	}
	// The owner may turn it on anyway (for a later adapter); still nothing.
	h.configure(true, contract.VolumeTargetTV)
	if cp := h.caps()[contract.ActionTVPower]; cp.Available || cp.Reason != reason {
		t.Fatalf("tv.power %+v", cp)
	}
	if cp := h.caps()[contract.ActionAudioVolume]; cp.Available || !strings.Contains(cp.Reason, reason) {
		t.Fatalf("TV volume without an adapter must be unavailable with the reason, not the PC's: %+v", cp)
	}
	res := h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "on"}))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeUnsupported)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionAudioVolume, map[string]any{"delta": float64(5)})), contract.OutcomeFailed, contract.CodeUnsupported)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeFailed, contract.CodeDisplayOff)
	time.Sleep(30 * time.Millisecond)
	if calls := tv.Calls(); len(calls) != 0 || len(h.audio.Calls()) != 0 {
		t.Fatalf("something was sent: tv %v, pc %v", calls, h.audio.Calls())
	}
	// No TV control wired at all (Options.TV nil).
	h2 := newPowerHarness(t)
	st := h2.phones.Snapshot(context.Background(), &h2.ctl)
	if st.CEC == nil || st.CEC.Available || st.CEC.Reason == "" || st.Capabilities[contract.ActionTVPower].Available {
		t.Fatalf("no TV control: %+v %+v", st.CEC, st.Capabilities[contract.ActionTVPower])
	}
}

// Off by default: an adapter alone sends nothing, not even a status read.
func TestCECIsOffUntilTheOwnerTurnsItOn(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	s := h.cecState("present, off", func(s *contract.CEC) bool { return s.Available })
	if s.Enabled || s.TVPower != contract.TVPowerUnknown {
		t.Fatalf("state.cec %+v", s)
	}
	if cp := h.caps()[contract.ActionTVPower]; cp.Available || cp.Reason != cecOffReason {
		t.Fatalf("tv.power while off: %+v", cp)
	}
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	h.clk.Advance(15 * time.Minute)
	h.eventually("the timer fired", func() bool { return h.c.lastSleep() != nil })
	if out := h.c.lastSleep(); slices.Contains(out.Steps, "tv") || out.TV != "" {
		t.Fatalf("a TV step ran while off: %+v", out)
	}
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeFailed, contract.CodeDisplayOff)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionHome, nil)), contract.OutcomeAccepted, contract.CodeOK)
	h.clk.Advance(CECPoll)
	time.Sleep(30 * time.Millisecond)
	if calls := h.tv.Calls(); len(calls) != 0 {
		t.Fatalf("the bus was used while off: %v", calls)
	}
}

// The sleep timer's firing: pause/Home/display off as before, then Standby,
// never before the display went off.
func TestSleepSendsStandbyAfterTheDisplayWentOff(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.configure(true, contract.VolumeTargetPC)
	h.cecState("tv on", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerOn })
	var mu sync.Mutex
	var atOff []string
	h.display.OnOff = func() {
		mu.Lock()
		defer mu.Unlock()
		if atOff == nil {
			atOff = commands(h.tv.Calls())
		}
	}
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	h.clk.Advance(15 * time.Minute)
	h.eventually("the timer fired", func() bool { return h.c.lastSleep() != nil })
	out := h.c.lastSleep()
	if !slices.Equal(out.Steps, []string{"pause", "home", "display", "tv"}) || out.TV != stepDelivered || out.Display != stepObserved {
		t.Fatalf("steps %v, outcome %+v", out.Steps, out)
	}
	mu.Lock()
	if len(atOff) != 0 {
		t.Fatalf("HDMI-CEC was used before the display went off: %v", atOff)
	}
	mu.Unlock()
	if got := commands(h.tv.Calls()); !slices.Equal(got, []string{"standby"}) {
		t.Fatalf("TV commands %v, want one standby", got)
	}
	h.cecState("tv standby", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerStandby })

	// A TV that does not take it is reported, not hidden.
	h.tv.Fail("standby", errors.New("not acknowledged"))
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	h.clk.Advance(15 * time.Minute)
	h.eventually("fired again", func() bool { o := h.c.lastSleep(); return o != out })
	if o := h.c.lastSleep(); o.TV != stepFailed || o.Reasons["tv"] == "" {
		t.Fatalf("failed standby: %+v", o)
	}
}

func TestDisplayOffSendsStandby(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.configure(true, contract.VolumeTargetPC)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	h.waitCalls("standby after display.off", "standby")
	if h.display.IsOn() {
		t.Fatal("the display is on")
	}
	// A display that cannot be turned off sends nothing.
	h2 := newCECHarness(t, fake.NewTV())
	h2.configure(true, contract.VolumeTargetPC)
	h2.display.SetCapability(platform.Capability{Backend: fake.DisplayBackend, Reason: "no DPMS"})
	expectOutcome(t, h2.submit(h2.ctl, h2.req(contract.ActionDisplayOff, nil)), contract.OutcomeFailed, contract.CodeUnsupported)
	time.Sleep(30 * time.Millisecond)
	if got := commands(h2.tv.Calls()); len(got) != 0 {
		t.Fatalf("standby without the display going off: %v", got)
	}
}

// Waking: the press that wakes the display (swallowed), a TV key while it
// was off, and Home each send Image View On then Active Source.
func TestWakeAndHomeTurnTheTVOnAndSwitchToBearDen(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.configure(true, contract.VolumeTargetPC)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	h.waitCalls("standby", "standby")
	h.cecState("tv standby", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerStandby })

	n := h.inputCount()
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeFailed, contract.CodeDisplayOff)
	h.waitCalls("power on then active source", "standby", "power_on", "active_source")
	h.cecState("tv on", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerOn })
	if h.inputCount() != n {
		t.Fatal("the waking press was delivered")
	}

	// A key the shell swallowed while the display was off.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	h.waitCalls("second standby", "standby", "power_on", "active_source", "standby")
	if err := h.shell.Send(shellipc.PowerActivity{Type: shellipc.TypePowerActivity}); err != nil {
		t.Fatal(err)
	}
	h.waitCalls("wake on a TV key", "standby", "power_on", "active_source", "standby", "power_on", "active_source")

	// Home with the display on: still on, and Bear Den's input.
	before := len(commands(h.tv.Calls()))
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionHome, nil)), contract.OutcomeAccepted, contract.CodeOK)
	h.eventually("home sends power on + active source", func() bool {
		got := commands(h.tv.Calls())
		return len(got) == before+2 && got[before] == "power_on" && got[before+1] == "active_source"
	})
}

// After Bear Den put the TV in standby (display left on), any action wakes
// it and still applies.
func TestAnyActionAfterStandbyWakesTheTV(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.configure(true, contract.VolumeTargetPC)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "standby"})), contract.OutcomeObserved, contract.CodeOK)
	n := h.inputCount()
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeObserved, contract.CodeOK)
	if h.inputCount() != n+1 {
		t.Fatal("the press was not delivered")
	}
	h.waitCalls("woken", "standby", "power_on", "active_source")
	// Once awake, further presses leave the bus alone.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeObserved, contract.CodeOK)
	time.Sleep(30 * time.Millisecond)
	if got := commands(h.tv.Calls()); len(got) != 3 {
		t.Fatalf("TV commands %v", got)
	}
}

func TestTVPowerAction(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.tv.SetPower(platform.TVPowerStandby)
	h.configure(true, contract.VolumeTargetPC)

	res := h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "on"}))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if res.Detail["tv_power"] != contract.TVPowerOn || !slices.Equal(commands(h.tv.Calls()), []string{"power_on", "active_source"}) {
		t.Fatalf("on: %+v, calls %v", res.Detail, h.tv.Calls())
	}
	res = h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "standby"}))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	h.cecState("standby", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerStandby })

	// Acknowledged but not confirmed: delivered, with a message.
	h.tv.SetAnswers(false)
	res = h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "on"}))
	expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
	if res.Message == "" || res.Detail["tv_power"] != contract.TVPowerUnknown {
		t.Fatalf("unconfirmed: %+v", res)
	}
	h.tv.SetAnswers(true)
	// Not acknowledged: failed, internal.
	h.tv.Fail("standby", errors.New("not acknowledged"))
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "standby"})), contract.OutcomeFailed, contract.CodeInternal)
	h.tv.Fail("standby", nil)

	// A stuck adapter never holds the action past CECTimeout.
	defer func(d time.Duration) { CECTimeout = d }(CECTimeout)
	CECTimeout = 100 * time.Millisecond
	h.tv.Hang(true)
	start := time.Now()
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "on"})), contract.OutcomeFailed, contract.CodeTimeout)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tv.power took %v", d)
	}
	// Home never waits on the bus, even when it hangs.
	start = time.Now()
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionHome, nil)), contract.OutcomeAccepted, contract.CodeOK)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("home waited %v on HDMI-CEC", d)
	}
	h.tv.Hang(false)

	// A guest pass never powers the TV (not on the guest allow-list).
	before := len(h.tv.Calls())
	expectOutcome(t, h.submit(guestViewer(time.Now().Add(time.Hour)), h.req(contract.ActionTVPower, map[string]any{"power": "standby"})), contract.OutcomeFailed, contract.CodeForbidden)
	if len(h.tv.Calls()) != before {
		t.Fatal("a guest reached the TV")
	}

	// Old epoch: still applies (a power action); locked: refused.
	r := h.req(contract.ActionTVPower, map[string]any{"power": "on"})
	r.ContextEpoch = 0
	expectOutcome(t, h.submit(h.ctl, r), contract.OutcomeObserved, contract.CodeOK)
	h.lock.set(true)
	h.eventually("locked", func() bool { _, _, l := h.c.current(); return l })
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionTVPower, map[string]any{"power": "standby"})), contract.OutcomeFailed, contract.CodeLocked)
	if s := h.phones.Snapshot(context.Background(), &h.ctl).CEC; s == nil || !s.Enabled {
		t.Fatalf("state.cec while locked: %+v", s)
	}
}

// Volume: the PC by default; the TV's keys with volume_target tv; never a
// silent fall-back to the PC.
func TestVolumeRouting(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.eventually("pc audio", func() bool { return h.caps()[contract.ActionAudioVolume].Available })
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionAudioVolume, map[string]any{"delta": float64(5)})), contract.OutcomeDelivered, contract.CodeOK)
	// volume_target tv while CEC is off: still the PC.
	h.configure(false, contract.VolumeTargetTV)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionAudioMute, map[string]any{"muted": true})), contract.OutcomeDelivered, contract.CodeOK)
	if got := h.audio.Calls(); !slices.Equal(got, []string{"delta", "mute"}) || len(h.tv.Calls()) != 0 {
		t.Fatalf("pc %v, tv %v", got, h.tv.Calls())
	}

	h.configure(true, contract.VolumeTargetTV)
	if cp := h.caps()[contract.ActionAudioVolume]; !cp.Available || cp.Backend != backendCEC {
		t.Fatalf("audio.volume_delta with the TV chosen: %+v", cp)
	}
	for _, tc := range []struct {
		action string
		args   map[string]any
		want   []string
	}{
		{contract.ActionAudioVolume, map[string]any{"delta": float64(5)}, []string{"volume:volume_up"}},
		{contract.ActionAudioVolume, map[string]any{"delta": float64(-12)}, []string{"volume:volume_down", "volume:volume_down", "volume:volume_down"}},
		{contract.ActionAudioVolume, map[string]any{"delta": float64(100)}, slices.Repeat([]string{"volume:volume_up"}, 5)},
		{contract.ActionAudioMute, map[string]any{"muted": true}, []string{"volume:mute"}},
		{contract.ActionAudioMute, map[string]any{"muted": false}, []string{"volume:unmute"}},
	} {
		before := len(commands(h.tv.Calls()))
		res := h.submit(h.ctl, h.req(tc.action, tc.args))
		expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
		if got := commands(h.tv.Calls())[before:]; !slices.Equal(got, tc.want) {
			t.Fatalf("%s %v: %v, want %v", tc.action, tc.args, got, tc.want)
		}
	}
	if got := h.audio.Calls(); len(got) != 2 {
		t.Fatalf("the PC was changed while the TV was chosen: %v", got)
	}

	// The adapter goes away: unavailable with the reason, not the PC.
	h.tv.SetCapability(platform.Capability{Backend: fake.TVBackend, Reason: "No HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter"})
	h.poll("gone", func(s *contract.CEC) bool { return !s.Available })
	cp := h.caps()[contract.ActionAudioMute]
	if cp.Available || !strings.HasPrefix(cp.Reason, "TV volume over HDMI-CEC is not available") {
		t.Fatalf("audio.mute without the adapter: %+v", cp)
	}
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionAudioVolume, map[string]any{"delta": float64(5)})), contract.OutcomeFailed, contract.CodeUnsupported)
	if got := h.audio.Calls(); len(got) != 2 {
		t.Fatalf("fell back to the PC: %v", got)
	}
	if cecPresses(1) != 1 || cecPresses(-5) != 1 || cecPresses(6) != 2 || cecPresses(-100) != 5 {
		t.Fatal("cecPresses")
	}
}

// state.cec for each view, the TV power read by the poll, cec.configure
// validation, and releasing the adapter when the owner turns it off.
func TestCECStateAndConfigure(t *testing.T) {
	h := newCECHarness(t, fake.NewTV())
	h.configure(true, contract.VolumeTargetPC)
	h.cecState("tv on", func(s *contract.CEC) bool { return s.Available && s.TVPower == contract.TVPowerOn })
	// Turned off with the TV's own remote: seen on the next poll.
	h.tv.SetPower(platform.TVPowerStandby)
	h.poll("standby by the poll", func(s *contract.CEC) bool { return s.TVPower == contract.TVPowerStandby })

	if st := h.c.buildState(viewShell); st.CEC == nil || !st.CEC.Enabled {
		t.Fatalf("shell view: %+v", st.CEC)
	}
	if st := h.c.buildState(viewAnonymous); st.CEC != nil {
		t.Fatalf("anonymous view: %+v", st.CEC)
	}
	if _, err := contract.MarshalAndValidateState(h.c.buildState(viewShell)); err != nil {
		t.Fatal(err)
	}
	if err := h.c.configureCEC(true, "soundbar"); err == nil {
		t.Fatal("volume_target soundbar accepted")
	}
	if got := h.opts().Config.Current().CECSettings(); !got.Enabled || got.VolumeTarget != contract.VolumeTargetPC {
		t.Fatalf("a refused change was stored: %+v", got)
	}
	closed := h.tv.Closed()
	h.configure(false, contract.VolumeTargetPC)
	h.eventually("adapter released", func() bool { return h.tv.Closed() > closed })
	if s := h.cecState("off", func(s *contract.CEC) bool { return !s.Enabled }); s.TVPower != contract.TVPowerUnknown {
		t.Fatalf("tv_power while off: %+v", s)
	}
}

func (h *cecHarness) opts() Options { return h.c.opts }
