// Tests for suspend.Check on a fake system bus: every logind answer maps to
// an unavailable capability with its own reason, and the method called is
// only CanSuspend (never Suspend).

package suspend

import (
	"context"
	"errors"
	"testing"

	"bear-den-tv/internal/platform/dbusx"
)

func TestCheckDescribesEveryAnswer(t *testing.T) {
	for answer, reason := range map[string]string{
		"challenge": ReasonChallenge, "no": ReasonNo, "na": ReasonNA, "yes": ReasonYes,
	} {
		bus := &dbusx.Fake{CallFn: func(_ context.Context, dest, path, method string, _ ...any) ([]any, error) {
			if dest != logindDest || path != logindPath || method != canSuspend {
				return nil, errors.New("unexpected call " + method)
			}
			return []any{answer}, nil
		}}
		cp, got := Check(context.Background(), bus)
		if cp.Available || cp.Reason != reason || got != answer || cp.Backend != Backend {
			t.Fatalf("%s: %+v %q", answer, cp, got)
		}
		if len(bus.Calls) != 1 || bus.Calls[0] != logindDest+" "+logindPath+" "+canSuspend {
			t.Fatalf("%s: calls %v", answer, bus.Calls)
		}
	}
}

func TestCheckWithoutAnAnswer(t *testing.T) {
	cp, _ := Check(context.Background(), &dbusx.Fake{})
	if cp.Available || cp.Reason != ReasonNoAnswer {
		t.Fatalf("no CallFn: %+v", cp)
	}
	if cp, _ := Check(context.Background(), nil); cp.Available || cp.Reason != ReasonNoAnswer {
		t.Fatalf("nil bus: %+v", cp)
	}
	odd := &dbusx.Fake{CallFn: func(context.Context, string, string, string, ...any) ([]any, error) { return []any{"maybe"}, nil }}
	if cp, _ := Check(context.Background(), odd); cp.Available || cp.Reason == "" {
		t.Fatalf("odd answer: %+v", cp)
	}
}
