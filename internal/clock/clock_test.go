// Tests for the fake and real clocks (clock.go, fake.go).

package clock

import (
	"testing"
	"time"
)

func TestFakeFiresInOrderAndRunsCallbacks(t *testing.T) {
	f := NewFake(time.Unix(1000, 0))
	var order []string
	f.AfterFunc(2*time.Second, func() { order = append(order, "b") })
	f.AfterFunc(time.Second, func() { order = append(order, "a") })
	tm := f.NewTimer(3 * time.Second)
	f.Advance(2500 * time.Millisecond)
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v", order)
	}
	select {
	case <-tm.C():
		t.Fatal("timer fired early")
	default:
	}
	f.Advance(time.Second)
	select {
	case at := <-tm.C():
		if !at.Equal(time.Unix(1003, 0)) {
			t.Fatalf("fired at %v", at)
		}
	default:
		t.Fatal("timer did not fire")
	}
	if got := f.Now(); !got.Equal(time.Unix(1003, 500000000)) {
		t.Fatalf("now = %v", got)
	}
}

func TestFakeStopAndReset(t *testing.T) {
	f := NewFake(time.Unix(0, 0))
	fired := 0
	tm := f.AfterFunc(time.Second, func() { fired++ })
	if !tm.Stop() {
		t.Fatal("stop should report active")
	}
	f.Advance(2 * time.Second)
	if fired != 0 {
		t.Fatal("stopped timer fired")
	}
	tm.Reset(time.Second)
	f.Advance(time.Second)
	if fired != 1 {
		t.Fatalf("fired = %d", fired)
	}
	if f.Pending() != 0 {
		t.Fatalf("pending = %d", f.Pending())
	}
}

func TestFakeTimerArmedDuringAdvanceFires(t *testing.T) {
	f := NewFake(time.Unix(0, 0))
	n := 0
	f.AfterFunc(time.Second, func() {
		n++
		f.AfterFunc(time.Second, func() { n++ })
	})
	f.Advance(5 * time.Second)
	if n != 2 {
		t.Fatalf("n = %d", n)
	}
}

func TestRealClockMonotonic(t *testing.T) {
	var c Clock = Real{}
	a := c.Now()
	<-c.After(time.Millisecond)
	if c.Since(a) <= 0 {
		t.Fatal("time did not advance")
	}
}
