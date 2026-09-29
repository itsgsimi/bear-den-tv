// Package suspend asks logind on the system bus whether this user may
// suspend the box (org.freedesktop.login1.Manager.CanSuspend) and turns the
// answer into a capability with a reason (state.power.suspend,
// contracts/http.md "Sleep timer and screen off"). It only asks: Bear Den
// never suspends today, and never handles a password. "challenge" (the
// system would ask for one, as on the reference TV) is reported as
// unavailable with that reason.
package suspend

import (
	"context"
	"fmt"
	"time"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

const (
	logindDest = "org.freedesktop.login1"
	logindPath = "/org/freedesktop/login1"
	canSuspend = "org.freedesktop.login1.Manager.CanSuspend"

	// Backend names the source of the answer.
	Backend = "logind"
)

// Reasons, one per logind answer (and for no answer).
const (
	ReasonChallenge = "The system asks for a password to suspend."
	ReasonNo        = "This user is not allowed to suspend the system."
	ReasonNA        = "This computer cannot suspend."
	ReasonYes       = "Suspend is allowed, but Bear Den does not offer it yet."
	ReasonNoAnswer  = "logind did not say whether the system can suspend."
)

// Check asks logind once (bounded to 2 s) and describes the answer. It never
// returns Available: true, because Bear Den has no suspend action yet.
func Check(ctx context.Context, bus dbusx.Bus) (platform.Capability, string) {
	cp := platform.Capability{Backend: Backend}
	if bus == nil {
		cp.Reason = ReasonNoAnswer
		return cp, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	body, err := bus.Call(ctx, logindDest, logindPath, canSuspend)
	if err != nil || len(body) != 1 {
		cp.Reason = ReasonNoAnswer
		return cp, ""
	}
	answer, _ := dbusx.Unwrap(body[0]).(string)
	cp.Reason = Describe(answer)
	return cp, answer
}

// Describe maps a CanSuspend answer ("yes", "no", "challenge", "na") to a
// user-facing reason.
func Describe(answer string) string {
	switch answer {
	case "challenge":
		return ReasonChallenge
	case "no":
		return ReasonNo
	case "na":
		return ReasonNA
	case "yes":
		return ReasonYes
	}
	return fmt.Sprintf("logind answered %q about suspending.", answer)
}
