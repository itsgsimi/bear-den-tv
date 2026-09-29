// Session tests for the sleep timer and screen off (power.go), on the fake
// clock with a fake display and fake MPRIS players: set, replace and cancel;
// the warning a minute before and the inputs that cancel it; the firing
// order (pause only through a verified player, then Home, then the display
// off); waking on a phone action or a TV key with the waking press
// swallowed; the locked session; the display restored when the coordinator
// stops; capabilities and state.power per viewer.

package session

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

type pwHarness struct {
	*harness
	clk     *clock.Fake
	media   *fake.Media
	display *fake.Display
}

func newPowerHarness(t *testing.T, configure ...func(*Options)) *pwHarness {
	t.Helper()
	clk := clock.NewFake(time.Unix(1_800_000_000, 0))
	media, display := fake.NewMedia(), fake.NewDisplay()
	h := newHarness(t, append([]func(*Options){func(o *Options) {
		o.Clock, o.Media, o.Display = clk, media, display
	}}, configure...)...)
	return &pwHarness{harness: h, clk: clk, media: media, display: display}
}

func (h *pwHarness) power() *contract.Power {
	return h.phones.Snapshot(context.Background(), &h.ctl).Power
}

func (h *pwHarness) waitPower(what string, cond func(p *contract.Power) bool) *contract.Power {
	h.t.Helper()
	var p *contract.Power
	h.eventually(what, func() bool {
		p = h.power()
		return p != nil && cond(p)
	})
	return p
}

func (h *pwHarness) sleepIn(minutes int) contract.ActionResult {
	h.t.Helper()
	return h.submit(h.ctl, h.req(contract.ActionSleepTimer, map[string]any{"minutes": float64(minutes)}))
}

func (h *pwHarness) inputCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.inputs)
}

// plexInFront puts Plex in front with a verified, playing player.
func (h *pwHarness) plexInFront() *fake.Player {
	h.t.Helper()
	plex := demoPlex(h.clk)
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	w := h.desk.AddWindow(platform.WindowInfo{PID: 9100, Class: []string{"plexhtpc", "plexhtpc"}, Mapped: true})
	h.desk.SetActive(w)
	h.eventually("verified media controls", func() bool {
		return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause].Available
	})
	return plex
}

func TestSleepTimerSetReplaceCancel(t *testing.T) {
	h := newPowerHarness(t)
	if p := h.power(); p == nil || p.SleepAtMs != nil || p.Warning || p.Display != contract.DisplayOn {
		t.Fatalf("initial power %+v", p)
	}
	start := h.c.nowMs()
	res := h.sleepIn(30)
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	p := h.waitPower("timer set", func(p *contract.Power) bool { return p.SleepAtMs != nil })
	if *p.SleepAtMs != start+30*60_000 || p.SleepMinutes != 30 || res.Detail["sleep_at_ms"] != *p.SleepAtMs {
		t.Fatalf("30 min: %+v detail %v (start %d)", p, res.Detail, start)
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}

	// Replace: the new timer counts from now and the old one never fires.
	h.clk.Advance(10 * time.Minute)
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	p = h.waitPower("replaced", func(p *contract.Power) bool { return p.SleepMinutes == 15 })
	if *p.SleepAtMs != start+25*60_000 {
		t.Fatalf("replaced sleep_at_ms %d, want %d", *p.SleepAtMs, start+25*60_000)
	}

	// Cancel: nothing fires, even long after either deadline.
	res = h.sleepIn(0)
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if v, ok := res.Detail["sleep_at_ms"]; !ok || v != nil {
		t.Fatalf("cancel detail %v", res.Detail)
	}
	h.waitPower("cancelled", func(p *contract.Power) bool { return p.SleepAtMs == nil && p.SleepMinutes == 0 })
	h.clk.Advance(2 * time.Hour)
	time.Sleep(50 * time.Millisecond)
	if calls := h.display.Calls(); len(calls) != 0 || h.c.lastSleep() != nil {
		t.Fatalf("a cancelled timer fired: display %v, last %+v", calls, h.c.lastSleep())
	}

	// Stale epochs do not matter to the power actions; odd minutes from the
	// (unvalidated) shell are refused, never read as a cancel.
	req := h.req(contract.ActionSleepTimer, map[string]any{"minutes": float64(45)})
	req.ContextEpoch = 0
	expectOutcome(t, h.submit(h.ctl, req), contract.OutcomeObserved, contract.CodeOK)
	for _, bad := range []map[string]any{{"minutes": float64(20)}, {}, {"minutes": "45"}, {"minutes": 12.5}} {
		res := h.c.submit(context.Background(), sender{key: shellSender}, h.req(contract.ActionSleepTimer, bad))
		expectOutcome(t, res, contract.OutcomeFailed, contract.CodeInvalid)
	}
	if p := h.power(); p.SleepMinutes != 45 {
		t.Fatalf("a refused request changed the timer: %+v", p)
	}
}

func TestSleepWarningAndInputCancels(t *testing.T) {
	h := newPowerHarness(t)
	warnAt := func() {
		t.Helper()
		expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
		h.clk.Advance(14*time.Minute - time.Second)
		if p := h.waitPower("timer", func(p *contract.Power) bool { return p.SleepAtMs != nil }); p.Warning {
			t.Fatal("warning more than a minute early")
		}
		h.clk.Advance(time.Second)
		h.waitPower("warning", func(p *contract.Power) bool { return p.Warning })
	}
	cancelled := func(what string) {
		t.Helper()
		h.waitPower(what+" cancels the timer", func(p *contract.Power) bool { return p.SleepAtMs == nil && !p.Warning })
		h.clk.Advance(5 * time.Minute)
		time.Sleep(30 * time.Millisecond)
		if h.c.lastSleep() != nil || len(h.display.Calls()) != 0 {
			t.Fatalf("%s: the timer still fired", what)
		}
	}

	// A phone press cancels and still does its job.
	warnAt()
	n := h.inputCount()
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionNavDown, nil)), contract.OutcomeObserved, contract.CodeOK)
	if h.inputCount() != n+1 {
		t.Fatal("the press that cancelled the warning was not delivered")
	}
	cancelled("a phone press")

	// A TV key the shell swallowed (IPC power.activity).
	warnAt()
	if err := h.shell.Send(shellipc.PowerActivity{Type: shellipc.TypePowerActivity}); err != nil {
		t.Fatal(err)
	}
	cancelled("a TV key seen by the shell")

	// A TV key sent to an app in front: the X idle counter restarts.
	warnAt()
	h.display.SetIdle(0)
	h.clk.Advance(SleepIdlePoll)
	cancelled("TV input seen as idle time")
}

func TestSleepFiresPauseThenHomeThenDisplayOff(t *testing.T) {
	h := newPowerHarness(t)
	plex := h.plexInFront()
	var mu sync.Mutex
	var pausedAtOff bool
	var targetAtOff string
	h.display.OnOff = func() { // what had happened when the display first went dark
		mu.Lock()
		defer mu.Unlock()
		if targetAtOff == "" {
			pausedAtOff = slices.Contains(plex.Calls(), "Pause")
			targetAtOff = h.c.Target().Kind
		}
	}
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	h.clk.Advance(15 * time.Minute)
	h.eventually("the timer fired", func() bool { return h.c.lastSleep() != nil })

	out := h.c.lastSleep()
	if !slices.Equal(out.Steps, []string{"pause", "home", "display"}) || out.Locked {
		t.Fatalf("steps %v locked %v", out.Steps, out.Locked)
	}
	if out.Pause != stepObserved || out.Home != stepObserved || out.Display != stepObserved {
		t.Fatalf("outcomes %+v", out)
	}
	mu.Lock()
	if !pausedAtOff || targetAtOff != "shell" {
		t.Fatalf("when the display went off: paused %v, in front %q", pausedAtOff, targetAtOff)
	}
	mu.Unlock()
	if st, _ := plex.Status(context.Background()); st != "Paused" {
		t.Fatalf("player %s", st)
	}
	p := h.waitPower("display off", func(p *contract.Power) bool { return p.Display == contract.DisplayOff })
	if p.SleepAtMs != nil || p.Warning {
		t.Fatalf("after firing %+v", p)
	}
	if len(h.desk.Keys()) != 0 {
		t.Fatalf("keys were sent: %v", h.desk.Keys())
	}
	if calls := h.display.Calls(); !slices.Equal(calls, []string{"Off"}) {
		t.Fatalf("display calls %v, want one Off", calls)
	}
}

func TestSleepNeverGuessesAPause(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(h *pwHarness) *fake.Player
	}{
		{"player not controllable", func(h *pwHarness) *fake.Player {
			p := demoPlex(h.clk)
			p.SetCanControl(false)
			h.media.Add(adapters.PlexHTPCFlatpakID, p)
			h.front("plexhtpc")
			return p
		}},
		{"no player for the app in front", func(h *pwHarness) *fake.Player {
			// Another app's player must not be used.
			p := demoPlex(h.clk)
			h.media.Add(adapters.PlexHTPCFlatpakID, p)
			h.front("vacuumtube")
			return p
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPowerHarness(t)
			player := tc.setup(h)
			h.eventually("media checked", func() bool {
				cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]
				return !cp.Available && cp.Reason != "" && cp.Reason[:8] != "Checking"
			})
			expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
			h.clk.Advance(15 * time.Minute)
			h.eventually("the timer fired", func() bool { return h.c.lastSleep() != nil })
			out := h.c.lastSleep()
			if out.Pause != stepSkipped || out.Home != stepObserved || out.Display != stepObserved {
				t.Fatalf("outcomes %+v", out)
			}
			if slices.Contains(player.Calls(), "Pause") || len(h.desk.Keys()) != 0 {
				t.Fatalf("paused anyway: player %v keys %v", player.Calls(), h.desk.Keys())
			}
		})
	}
}

func (h *pwHarness) front(class string) {
	w := h.desk.AddWindow(platform.WindowInfo{PID: 9100, Class: []string{class, class}, Mapped: true})
	h.desk.SetActive(w)
	h.eventually("app in front", func() bool { return h.c.Target().Kind == "app" })
}

func TestWakeOnPhoneActionSwallowsThePress(t *testing.T) {
	h := newPowerHarness(t)
	res := h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if h.display.IsOn() || !h.display.Changed() {
		t.Fatal("the display is not off")
	}
	h.waitPower("display off", func(p *contract.Power) bool { return p.Display == contract.DisplayOff })

	// Screen off again while off: stays off, no wake.
	res = h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if res.Detail["already_off"] != true || h.display.IsOn() {
		t.Fatalf("second display.off %+v on=%v", res.Detail, h.display.IsOn())
	}

	// The press that wakes it is swallowed: nothing reaches the shell.
	n := h.inputCount()
	res = h.submit(h.ctl, h.req(contract.ActionSelect, nil))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeDisplayOff)
	if !h.display.IsOn() || h.display.Changed() {
		t.Fatal("the display was not turned on with its settings restored")
	}
	time.Sleep(20 * time.Millisecond)
	if h.inputCount() != n {
		t.Fatal("the waking press was delivered")
	}
	h.waitPower("display on", func(p *contract.Power) bool { return p.Display == contract.DisplayOn })
	// The next press works.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeObserved, contract.CodeOK)
	if h.inputCount() != n+1 {
		t.Fatal("the press after waking was not delivered")
	}

	// A sleep timer set while the screen is off wakes it and still applies.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	expectOutcome(t, h.sleepIn(30), contract.OutcomeObserved, contract.CodeOK)
	if p := h.waitPower("timer set", func(p *contract.Power) bool { return p.SleepAtMs != nil }); p.Display != contract.DisplayOn {
		t.Fatalf("after sleep_timer while off: %+v", p)
	}
}

func TestWakeOnTVInput(t *testing.T) {
	h := newPowerHarness(t)
	// A key on the TV while an app is in front: the display turns itself on
	// and the coordinator notices on its next read.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	h.display.Wake()
	if !h.display.Changed() {
		t.Fatal("fake: settings should still be changed until restored")
	}
	h.clk.Advance(DisplayWakePoll)
	h.waitPower("display on after a TV key", func(p *contract.Power) bool { return p.Display == contract.DisplayOn })
	if h.display.Changed() {
		t.Fatal("settings not restored after a TV key woke the display")
	}

	// A key the shell swallowed (power.activity) wakes it at once.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	if err := h.shell.Send(shellipc.PowerActivity{Type: shellipc.TypePowerActivity}); err != nil {
		t.Fatal(err)
	}
	h.waitPower("display on after power.activity", func(p *contract.Power) bool { return p.Display == contract.DisplayOn })
	if h.display.Changed() || !h.display.IsOn() {
		t.Fatal("power.activity did not restore the display")
	}
}

func TestLockedSleepOnlyTurnsTheDisplayOff(t *testing.T) {
	h := newPowerHarness(t)
	plex := h.plexInFront()
	expectOutcome(t, h.sleepIn(15), contract.OutcomeObserved, contract.CodeOK)
	h.lock.set(true)
	h.eventually("locked", func() bool { return h.c.Target().Kind == "locked" })

	// Phones are refused like for every other action; state.power still shows.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeFailed, contract.CodeLocked)
	expectOutcome(t, h.sleepIn(0), contract.OutcomeFailed, contract.CodeLocked)
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if st.Power == nil || st.Power.SleepMinutes != 15 || st.Capabilities[contract.ActionDisplayOff].Available || st.Capabilities[contract.ActionSleepTimer].Available {
		t.Fatalf("locked: power %+v caps %+v", st.Power, st.Capabilities)
	}

	h.clk.Advance(15 * time.Minute)
	h.eventually("the timer fired", func() bool { return h.c.lastSleep() != nil })
	out := h.c.lastSleep()
	if !out.Locked || out.Pause != stepSkipped || out.Home != stepSkipped || out.Display != stepObserved || !slices.Equal(out.Steps, []string{"display"}) {
		t.Fatalf("locked firing %+v", out)
	}
	if slices.Contains(plex.Calls(), "Pause") || h.c.Target().Kind != "locked" || len(h.desk.Keys()) != 0 {
		t.Fatalf("something reached the app while locked: player %v target %s keys %v", plex.Calls(), h.c.Target().Kind, h.desk.Keys())
	}
	// A phone press while locked still wakes the display (the TV then shows
	// only the lock screen) and is refused.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionSelect, nil)), contract.OutcomeFailed, contract.CodeLocked)
	if !h.display.IsOn() || h.display.Changed() {
		t.Fatal("a press while locked did not wake the display")
	}
}

func TestDisplayRestoredWhenTheCoordinatorStops(t *testing.T) {
	h := newPowerHarness(t)
	expectOutcome(t, h.sleepIn(60), contract.OutcomeObserved, contract.CodeOK)
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeObserved, contract.CodeOK)
	if !h.display.Changed() {
		t.Fatal("display not off")
	}
	h.stop()
	h.eventually("settings restored on stop", func() bool { return !h.display.Changed() && h.display.IsOn() })
	if calls := h.display.Calls(); calls[len(calls)-1] != "On" {
		t.Fatalf("calls %v", calls)
	}
	h.clk.Advance(2 * time.Hour)
	time.Sleep(30 * time.Millisecond)
	if h.c.lastSleep() != nil {
		t.Fatal("the timer fired after the coordinator stopped")
	}
}

func TestPowerCapabilitiesAndViews(t *testing.T) {
	// No display backend: display.off unavailable with a reason, the timer
	// still available (it pauses and goes Home).
	h := newHarness(t, func(o *Options) {
		o.Suspend = &platform.Capability{Backend: "logind", Reason: "The system asks for a password to suspend."}
	})
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if cp := st.Capabilities[contract.ActionDisplayOff]; cp.Available || cp.Reason == "" {
		t.Fatalf("display.off without a backend: %+v", cp)
	}
	if cp := st.Capabilities[contract.ActionSleepTimer]; !cp.Available || cp.Backend != backendCoordinator {
		t.Fatalf("sleep_timer: %+v", cp)
	}
	if st.Power == nil || st.Power.Suspend == nil || st.Power.Suspend.Available || st.Power.Suspend.Reason != "The system asks for a password to suspend." {
		t.Fatalf("suspend report %+v", st.Power)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionDisplayOff, nil)), contract.OutcomeFailed, contract.CodeUnsupported)
	if h.c.buildState(viewShell).Power == nil || h.c.buildState(viewAnonymous).Power != nil {
		t.Fatal("power must reach the shell and never anonymous viewers")
	}

	// A backend that cannot: its reason; one that can: its backend name.
	ph := newPowerHarness(t)
	if cp := ph.phones.Snapshot(context.Background(), &ph.ctl).Capabilities[contract.ActionDisplayOff]; !cp.Available || cp.Backend != fake.DisplayBackend {
		t.Fatalf("display.off with a backend: %+v", cp)
	}
	ph.display.SetCapability(platform.Capability{Backend: "x11-dpms", Reason: "the X server has no DPMS extension"})
	cp := ph.phones.Snapshot(context.Background(), &ph.ctl).Capabilities[contract.ActionDisplayOff]
	if cp.Available || cp.Reason != "The screen cannot be turned off here: the X server has no DPMS extension" {
		t.Fatalf("unavailable backend: %+v", cp)
	}
	ph.display.SetCapability(platform.Capability{Available: true, Backend: fake.DisplayBackend})
	ph.display.FailOff(errors.New("fake: BadMatch"))
	expectOutcome(t, ph.submit(ph.ctl, ph.req(contract.ActionDisplayOff, nil)), contract.OutcomeFailed, contract.CodeInternal)
	if p := ph.power(); p.Display != contract.DisplayOn {
		t.Fatalf("a failed Off reported the display off: %+v", p)
	}

	// Controllers only: a phone without permissions is forbidden.
	none := h.ctl
	none.Permissions = nil
	expectOutcome(t, h.submit(none, h.req(contract.ActionSleepTimer, map[string]any{"minutes": float64(15)})), contract.OutcomeFailed, contract.CodeForbidden)
}
