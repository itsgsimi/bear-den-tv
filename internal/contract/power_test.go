// Tests for the power contract (contract.go): state.power marshals to what
// state.schema.json accepts (a missing timer is null, not omitted), the power
// actions are known, ignore stale epochs, and the result code display_off
// validates.

package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func powerState(p *Power) State {
	return State{
		Protocol: 1, ContextEpoch: 1, DeviceName: "Bear Den", ConfigRevision: 1,
		Session:       SessionState{DisplaySession: "x11", DesktopAdapter: "fake", ShellState: "running"},
		Target:        Target{Kind: "none", Label: "Nothing"},
		Capabilities:  map[string]Capability{},
		Shell:         ShellState{Screen: "home"},
		Applications:  []AppState{},
		Remote:        RemoteState{Transport: "local-only", Addresses: []string{}, Limits: DefaultLimits},
		Notifications: []Notification{},
		Power:         p,
	}
}

func TestPowerMarshalsToSchema(t *testing.T) {
	raw, err := MarshalAndValidateState(powerState(&Power{Display: DisplayOn}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"sleep_at_ms":null`) || strings.Contains(string(raw), `"sleep_minutes"`) || strings.Contains(string(raw), `"suspend"`) {
		t.Fatalf("no timer must be sleep_at_ms null, sleep_minutes and suspend omitted: %s", raw)
	}
	at := int64(2_685_000)
	raw, err = MarshalAndValidateState(powerState(&Power{SleepAtMs: &at, SleepMinutes: 45, Warning: true, Display: DisplayOff,
		Suspend: &SuspendReport{Reason: "The system asks for a password to suspend."}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"sleep_at_ms":2685000`) || !strings.Contains(string(raw), `"suspend":{"available":false`) {
		t.Fatalf("timer or suspend missing: %s", raw)
	}
	if _, err := MarshalAndValidateState(powerState(&Power{Display: "dim"})); err == nil {
		t.Fatal("display dim validated")
	}
	if _, err := MarshalAndValidateState(powerState(&Power{Display: DisplayOn, SleepMinutes: 20, SleepAtMs: &at})); err == nil {
		t.Fatal("sleep_minutes 20 validated")
	}
}

func TestPowerActionsAreKnownAndEscapeStaleEpochs(t *testing.T) {
	for _, a := range []string{ActionSleepTimer, ActionDisplayOff} {
		found := false
		for _, b := range AllActions {
			found = found || a == b
		}
		if !found || !IgnoresStaleEpoch(a) || IsNav(a) {
			t.Fatalf("%s: in AllActions %v, ignores stale epochs %v, nav %v", a, found, IgnoresStaleEpoch(a), IsNav(a))
		}
	}
	for _, m := range append([]int{0}, SleepChoices...) {
		req := map[string]any{"protocol": 1, "request_id": "5d1f0c62-8a7e-4c1b-9f3e-2b6a4d8e0c11", "context_epoch": 1, "target": "shell",
			"action": ActionSleepTimer, "args": map[string]any{"minutes": m}}
		raw, _ := json.Marshal(req)
		if _, err := ValidateActionRequest(raw); err != nil {
			t.Fatalf("minutes %d: %v", m, err)
		}
	}
	res := Failed(ActionRequest{RequestID: "5d1f0c62-8a7e-4c1b-9f3e-2b6a4d8e0c11"}, 3, TargetInfo{Kind: "shell", Label: "Bear Den TV"}, CodeDisplayOff, "The screen was off.")
	raw, _ := json.Marshal(res)
	if _, err := ValidateActionResult(raw); err != nil {
		t.Fatalf("display_off result: %v", err)
	}
}
