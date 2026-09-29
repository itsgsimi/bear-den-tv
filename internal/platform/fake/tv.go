// Fake HDMI-CEC TV: an in-memory platform.TVControl for session tests and
// `bear-den-tv dev` (nothing touches a real bus). It keeps a TV power state
// that PowerOn and Standby change and PowerStatus reads, records every call
// in order (Calls), and can report no adapter (SetCapability), fail one
// operation (Fail), hang until the caller's deadline or until released
// (Hang), or stop
// answering power status (SetAnswers).

package fake

import (
	"context"
	"errors"
	"sync"

	"bear-den-tv/internal/platform"
)

// TVBackend is the fake's capability backend name.
const TVBackend = "fake-cec"

// TV is a scriptable platform.TVControl.
type TV struct {
	mu      sync.Mutex
	cap     platform.Capability
	power   platform.TVPower
	answers bool
	calls   []string
	fail    map[string]error
	hang    bool
	release chan struct{} // closed by Hang(false): hung commands return
	closed  int
}

var _ platform.TVControl = (*TV)(nil)

// NewTV returns an adapter that is present with a TV that is on and answers.
func NewTV() *TV {
	return &TV{cap: platform.Capability{Available: true, Backend: TVBackend}, power: platform.TVPowerOn, answers: true, fail: map[string]error{}}
}

// SetCapability replaces what Probe reports.
func (t *TV) SetCapability(cp platform.Capability) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cap = cp
}

// SetPower sets the TV's power (as if its own remote was used).
func (t *TV) SetPower(p platform.TVPower) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.power = p
}

// Power is the TV's power now.
func (t *TV) Power() platform.TVPower {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.power
}

// SetAnswers makes PowerStatus answer (true) or time out (false).
func (t *TV) SetAnswers(ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.answers = ok
}

// Fail makes the operation named op ("power_on", "standby",
// "active_source", "volume", "status") return err; nil restores it.
func (t *TV) Fail(op string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		delete(t.fail, op)
	} else {
		t.fail[op] = err
	}
}

// ErrReleased is what a hung command returns when Hang(false) releases it.
var ErrReleased = errors.New("fake TV: released from a hang")

// Hang makes every command wait for the caller's deadline; Hang(false)
// releases the commands still waiting (they return ErrReleased).
func (t *TV) Hang(on bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case on && !t.hang:
		t.release = make(chan struct{})
	case !on && t.hang:
		close(t.release)
	}
	t.hang = on
}

// Calls lists the commands in order ("power_on", "standby",
// "active_source", "volume:<key>", "status"); Probe and Close are not listed.
func (t *TV) Calls() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.calls...)
}

// Closed counts Close calls.
func (t *TV) Closed() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// Probe implements platform.TVControl.
func (t *TV) Probe(context.Context) platform.Capability {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cap
}

func (t *TV) do(ctx context.Context, op, call string, apply func()) error {
	t.mu.Lock()
	t.calls = append(t.calls, call)
	hang, release, err, avail, reason := t.hang, t.release, t.fail[op], t.cap.Available, t.cap.Reason
	t.mu.Unlock()
	if hang {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return ErrReleased
		}
	}
	if !avail {
		return &unavailableError{reason}
	}
	if err != nil {
		return err
	}
	t.mu.Lock()
	apply()
	t.mu.Unlock()
	return nil
}

type unavailableError struct{ reason string }

func (e *unavailableError) Error() string { return "fake cec: " + e.reason }

// PowerOn implements platform.TVControl.
func (t *TV) PowerOn(ctx context.Context) error {
	return t.do(ctx, "power_on", "power_on", func() { t.power = platform.TVPowerOn })
}

// Standby implements platform.TVControl.
func (t *TV) Standby(ctx context.Context) error {
	return t.do(ctx, "standby", "standby", func() { t.power = platform.TVPowerStandby })
}

// ActiveSource implements platform.TVControl.
func (t *TV) ActiveSource(ctx context.Context) error {
	return t.do(ctx, "active_source", "active_source", func() {})
}

// VolumeKey implements platform.TVControl.
func (t *TV) VolumeKey(ctx context.Context, k platform.TVKey) error {
	return t.do(ctx, "volume", "volume:"+string(k), func() {})
}

// PowerStatus implements platform.TVControl.
func (t *TV) PowerStatus(ctx context.Context) (platform.TVPower, error) {
	var p platform.TVPower
	var answered bool
	err := t.do(ctx, "status", "status", func() { p, answered = t.power, t.answers })
	if err != nil {
		return platform.TVPowerUnknown, err
	}
	if !answered {
		return platform.TVPowerUnknown, &unavailableError{"the TV did not answer"}
	}
	return p, nil
}

// Close implements platform.TVControl.
func (t *TV) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed++
	return nil
}
