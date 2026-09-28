// Tests for action dedup and hold leases (dedup.go, holds.go; spec
// contracts/actions.md).

package actions

import (
	"errors"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

func req(id, action string, args map[string]any) contract.ActionRequest {
	return contract.ActionRequest{Protocol: 1, RequestID: id, ContextEpoch: 4, Target: "active", Action: action, Args: args}
}

func TestDedupReplayMismatchAndWindow(t *testing.T) {
	c := clock.NewFake(time.Unix(1000, 0))
	d := NewDedup(c)
	r := req("a", "nav.left", map[string]any{})
	if v, _ := d.Check("dev1", r); v != Fresh {
		t.Fatalf("first check = %v, want Fresh", v)
	}
	res := contract.ActionResult{RequestID: "a", Outcome: contract.OutcomeObserved}
	d.Remember("dev1", r, res)
	if v, got := d.Check("dev1", r); v != Replay || got.Outcome != contract.OutcomeObserved {
		t.Fatalf("replay = %v %v", v, got.Outcome)
	}
	if v, _ := d.Check("dev1", req("a", "nav.right", map[string]any{})); v != Mismatch {
		t.Fatalf("different payload = %v, want Mismatch", v)
	}
	if v, _ := d.Check("dev2", r); v != Fresh {
		t.Fatalf("other device = %v, want Fresh (per-session cache)", v)
	}
	c.Advance(DedupWindow)
	if v, _ := d.Check("dev1", r); v != Fresh {
		t.Fatalf("after window = %v, want Fresh", v)
	}
}

func TestDedupBoundedSize(t *testing.T) {
	d := NewDedup(clock.NewFake(time.Unix(0, 0)))
	for i := 0; i <= DedupSize; i++ {
		d.Remember("dev", req(string(rune('a'+i%26))+time.Duration(i).String(), "select", nil), contract.ActionResult{})
	}
	first := req("a"+time.Duration(0).String(), "select", nil)
	if v, _ := d.Check("dev", first); v != Fresh {
		t.Fatalf("oldest id should be evicted after %d entries", DedupSize)
	}
}

func TestDedupRememberUpdatesResult(t *testing.T) {
	d := NewDedup(clock.NewFake(time.Unix(0, 0)))
	r := req("x", "home", map[string]any{})
	d.Remember("dev", r, contract.ActionResult{Outcome: contract.OutcomeAccepted})
	d.Remember("dev", r, contract.ActionResult{Outcome: contract.OutcomeObserved})
	if _, got := d.Check("dev", r); got.Outcome != contract.OutcomeObserved {
		t.Fatalf("replay after completion = %s, want observed", got.Outcome)
	}
}

type tapRec struct {
	ch  chan string
	err error
}

func (r *tapRec) tap(dev, action string, epoch int64) (string, error) {
	r.ch <- action
	if r.err != nil {
		return "stale_epoch", r.err
	}
	return "", nil
}

func waitTimer(t *testing.T, c *clock.Fake) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for c.Pending() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("lease goroutine never armed its timer")
		}
		time.Sleep(time.Millisecond)
	}
}

func expectTap(t *testing.T, ch chan string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected a tap")
	}
}

func expectNoTap(t *testing.T, ch chan string) {
	t.Helper()
	select {
	case a := <-ch:
		t.Fatalf("unexpected tap %q", a)
	case <-time.After(50 * time.Millisecond):
	}
}

// A press released before the repeat delay must produce no lease taps at all:
// the client's own action is the only movement (regression: double steps).
func TestShortPressProducesNoRepeat(t *testing.T) {
	c := clock.NewFake(time.Unix(0, 0))
	rec := &tapRec{ch: make(chan string, 64)}
	h := NewHolds(c, contract.DefaultLimits, rec.tap, nil)
	h.Start("dev", "h1", "nav.left", 1)
	waitTimer(t, c)
	c.Advance(120 * time.Millisecond)
	h.Stop("dev", "h1", "stop")
	c.Advance(time.Second)
	expectNoTap(t, rec.ch)
}

func TestHoldRepeatsAndExpires(t *testing.T) {
	c := clock.NewFake(time.Unix(0, 0))
	rec := &tapRec{ch: make(chan string, 64)}
	h := NewHolds(c, contract.DefaultLimits, rec.tap, nil)
	if st := h.Start("dev", "h1", "nav.right", 4); st.State != HoldActive {
		t.Fatalf("start = %+v", st)
	}
	// No tap at start: the client sent the initial press itself.
	waitTimer(t, c)
	expectNoTap(t, rec.ch)
	c.Advance(350 * time.Millisecond) // repeat delay; renewed at t=0 so still inside 600 ms
	expectTap(t, rec.ch)
	if st := h.Renew("dev", "h1"); st.State != HoldActive {
		t.Fatalf("renew = %+v", st)
	}
	// Stop renewing: the lease must expire rather than repeat forever.
	for i := 0; i < 10 && h.Active().Active; i++ {
		waitTimer(t, c)
		c.Advance(time.Second / 6)
		select {
		case <-rec.ch:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if h.Active().Active {
		t.Fatal("lease still active without renewals")
	}
	if st := h.Renew("dev", "h1"); st.State != HoldExpired {
		t.Fatalf("renew after expiry = %+v, want expired", st)
	}
}

func TestHoldBusyReplaceAndCancel(t *testing.T) {
	c := clock.NewFake(time.Unix(0, 0))
	rec := &tapRec{ch: make(chan string, 64)}
	h := NewHolds(c, contract.DefaultLimits, rec.tap, nil)
	if st := h.Start("dev", "h1", "select", 1); st.State != HoldRejected {
		t.Fatalf("select must not be holdable: %+v", st)
	}
	h.Start("dev", "h1", "nav.up", 1)
	if st := h.Start("other", "h2", "nav.up", 1); st.State != HoldBusy {
		t.Fatalf("second device = %+v, want busy", st)
	}
	h.Start("dev", "h3", "nav.down", 1)
	if st := h.Renew("dev", "h1"); st.State != HoldCancelled || st.Reason != "replaced" {
		t.Fatalf("replaced lease = %+v", st)
	}
	h.CancelAll("epoch")
	if st := h.Renew("dev", "h3"); st.State != HoldCancelled || st.Reason != "epoch" {
		t.Fatalf("cancelled lease = %+v", st)
	}
	if h.Active().Active {
		t.Fatal("no lease should remain")
	}
}

func TestHoldTapFailureEndsLease(t *testing.T) {
	c := clock.NewFake(time.Unix(0, 0))
	rec := &tapRec{ch: make(chan string, 64), err: errors.New("stale")}
	h := NewHolds(c, contract.DefaultLimits, rec.tap, nil)
	h.Start("dev", "h1", "nav.left", 1)
	waitTimer(t, c)
	c.Advance(350 * time.Millisecond)
	expectTap(t, rec.ch)
	deadline := time.Now().Add(2 * time.Second)
	for h.Active().Active && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if st := h.Renew("dev", "h1"); st.State != HoldCancelled || st.Reason != "stale_epoch" {
		t.Fatalf("after failed tap = %+v", st)
	}
}
