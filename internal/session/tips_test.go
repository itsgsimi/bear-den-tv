// Tests for the TV's bear tips (tips.go): tips.event, tips.configure and
// tips.reset through the real shell handler on a fake clock, what they
// persist in config tips, and state.tips in the shell view only.

package session

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
)

func newTipsHarness(t *testing.T) (*harness, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, time.September, 29, 21, 30, 0, 0, time.Local))
	return newHarness(t, func(o *Options) { o.Clock = clk }), clk
}

// tips returns state.tips of the shell view, checking the whole state
// against the schema and that no phone view carries tips.
func (h *harness) tips() contract.Tips {
	h.t.Helper()
	st := h.c.buildState(viewShell)
	if st.Tips == nil {
		h.t.Fatal("the shell view has no tips")
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		h.t.Fatalf("shell state is invalid: %v", err)
	}
	for name, v := range map[string]contract.State{
		"controller": h.phones.Snapshot(context.Background(), &h.ctl),
		"owner":      h.phones.Snapshot(context.Background(), &h.owner),
		"anonymous":  h.c.buildState(viewAnonymous),
	} {
		if v.Tips != nil {
			h.t.Fatalf("the %s view carries tips %+v", name, v.Tips)
		}
	}
	return *st.Tips
}

// tipEvent sends tips.event (no reply) and waits for the config revision it
// causes.
func (h *harness) tipEvent(tip, event string) {
	h.t.Helper()
	rev := h.c.opts.Config.Revision()
	if err := h.shell.Send(shellipc.TipsEvent{Type: shellipc.TypeTipsEvent, Tip: tip, Event: event}); err != nil {
		h.t.Fatal(err)
	}
	h.eventually("tips."+event+" stored", func() bool { return h.c.opts.Config.Revision() > rev })
}

func TestTipsFreshBoxOffersEveryTip(t *testing.T) {
	h, _ := newTipsHarness(t)
	if got := h.tips(); !got.Enabled || got.Stopped || len(got.Done) != 0 || got.LastDay != "" {
		t.Fatalf("a fresh box: %+v", got)
	}
}

// Seen marks the tip done and uses the day on the coordinator's clock;
// the next day's tip moves the day.
func TestTipSeenIsDoneAndUsesTheDay(t *testing.T) {
	h, clk := newTipsHarness(t)
	h.tipEvent("themes", shellipc.TipEventSeen)
	got := h.tips()
	if !reflect.DeepEqual(got.Done, []string{"themes"}) || got.LastDay != "2026-09-29" {
		t.Fatalf("after seen: %+v", got)
	}
	// Answering the same tip keeps it once.
	h.tipEvent("themes", shellipc.TipEventOK)
	if got := h.tips(); !reflect.DeepEqual(got.Done, []string{"themes"}) {
		t.Fatalf("themes listed twice: %+v", got.Done)
	}
	clk.Advance(3 * time.Hour) // past midnight, local time
	h.tipEvent("add-apps", shellipc.TipEventSeen)
	if got := h.tips(); got.LastDay != "2026-09-30" || !reflect.DeepEqual(got.Done, []string{"themes", "add-apps"}) {
		t.Fatalf("the next day: %+v", got)
	}
	// What is on disk, read by a fresh store.
	fresh, err := config.Open(config.Options{Dir: filepath.Dir(h.c.opts.Config.Path()), Clock: clock.Real{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if tp := fresh.Current().Tips; tp == nil || tp.LastDay != "2026-09-30" || len(tp.Done) != 2 {
		t.Fatalf("not persisted: %+v", tp)
	}
}

// Three "Not now" in a row stop the tips; an OK in between starts the count
// again.
func TestThreeNotNowInARowStopTheTips(t *testing.T) {
	h, _ := newTipsHarness(t)
	h.tipEvent("themes", shellipc.TipEventNotNow)
	h.tipEvent("add-apps", shellipc.TipEventNotNow)
	h.tipEvent("phone-remote", shellipc.TipEventOK)
	h.tipEvent("now-playing", shellipc.TipEventNotNow)
	h.tipEvent("sleep-timer", shellipc.TipEventNotNow)
	if got := h.tips(); got.Stopped {
		t.Fatalf("stopped after an OK and two Not now: %+v", got)
	}
	h.tipEvent("badges", shellipc.TipEventNotNow)
	if got := h.tips(); !got.Stopped || len(got.Done) != 6 {
		t.Fatalf("three Not now in a row: %+v", got)
	}
	h.tipEvent("guest-pass", shellipc.TipEventNotNow)
	if s := h.c.opts.Config.Current().Tips.NotNowStreak; s != config.TipsStopAfter {
		t.Fatalf("streak %d, want it held at %d", s, config.TipsStopAfter)
	}
}

func TestTipsConfigureAndReset(t *testing.T) {
	h, _ := newTipsHarness(t)
	h.tipEvent("themes", shellipc.TipEventSeen)
	for i := 0; i < 3; i++ {
		h.tipEvent("themes", shellipc.TipEventNotNow)
	}
	if r := h.shellSend(shellipc.TipsConfigure{Type: shellipc.TypeTipsConfigure, RequestID: "tc-1"}, "tc-1"); !r.OK {
		t.Fatalf("tips.configure refused: %+v", r)
	}
	if got := h.tips(); got.Enabled || !got.Stopped || len(got.Done) != 1 {
		t.Fatalf("after off: %+v", got)
	}
	if r := h.shellSend(shellipc.TipsReset{Type: shellipc.TypeTipsReset, RequestID: "tr-1"}, "tr-1"); !r.OK {
		t.Fatalf("tips.reset refused: %+v", r)
	}
	if got := h.tips(); !got.Enabled || got.Stopped || len(got.Done) != 0 || got.LastDay != "" {
		t.Fatalf("after Show tips again: %+v", got)
	}
}

// An unknown tip or event changes nothing (and never reaches the config,
// whose schema would refuse it).
func TestTipEventUnknownIsIgnored(t *testing.T) {
	h, _ := newTipsHarness(t)
	rev := h.c.opts.Config.Revision()
	_ = h.shell.Send(shellipc.TipsEvent{Type: shellipc.TypeTipsEvent, Tip: "clippy", Event: shellipc.TipEventSeen})
	_ = h.shell.Send(shellipc.TipsEvent{Type: shellipc.TypeTipsEvent, Tip: "themes", Event: "maybe"})
	h.tipEvent("add-apps", shellipc.TipEventSeen) // processed after the two above
	if got := h.c.opts.Config.Revision(); got != rev+1 {
		t.Fatalf("revision %d → %d, want one bump", rev, got)
	}
	if got := h.tips(); !reflect.DeepEqual(got.Done, []string{"add-apps"}) {
		t.Fatalf("done %+v", got.Done)
	}
}
