// Tests for the HDMI-CEC contract (cec.go): state.cec marshals to what
// state.schema.json accepts, tv.power is a known power action that ignores
// stale epochs, and its args are checked.

package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCECMarshalsToSchema(t *testing.T) {
	st := powerState(&Power{Display: DisplayOn})
	st.CEC = &CEC{Available: true, Enabled: true, VolumeTarget: VolumeTargetTV, TVPower: TVPowerStandby}
	raw, err := MarshalAndValidateState(st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"cec":{"available":true,"enabled":true,"volume_target":"tv","tv_power":"standby"}`) {
		t.Fatalf("cec: %s", raw)
	}
	st.CEC = &CEC{Reason: "No HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter", VolumeTarget: VolumeTargetPC, TVPower: TVPowerUnknown}
	if _, err := MarshalAndValidateState(st); err != nil {
		t.Fatalf("unavailable with a reason: %v", err)
	}
	for _, bad := range []CEC{
		{VolumeTarget: VolumeTargetPC, TVPower: "off"},
		{VolumeTarget: "soundbar", TVPower: TVPowerOn},
		{VolumeTarget: VolumeTargetPC},
	} {
		st.CEC = &bad
		if _, err := MarshalAndValidateState(st); err == nil {
			t.Fatalf("%+v validated", bad)
		}
	}
}

func TestTVPowerIsAKnownPowerAction(t *testing.T) {
	found := false
	for _, a := range AllActions {
		found = found || a == ActionTVPower
	}
	if !found || !IgnoresStaleEpoch(ActionTVPower) || IsNav(ActionTVPower) {
		t.Fatalf("tv.power: in AllActions %v, ignores stale epochs %v", found, IgnoresStaleEpoch(ActionTVPower))
	}
	req := func(args map[string]any) []byte {
		raw, _ := json.Marshal(map[string]any{"protocol": 1, "request_id": "5d1f0c62-8a7e-4c1b-9f3e-2b6a4d8e0c11", "context_epoch": 1, "target": "shell", "action": ActionTVPower, "args": args})
		return raw
	}
	for _, p := range []string{TVPowerArgOn, TVPowerArgStandby} {
		if _, err := ValidateActionRequest(req(map[string]any{"power": p})); err != nil {
			t.Fatalf("power %s: %v", p, err)
		}
	}
	for _, args := range []map[string]any{{}, {"power": "off"}, {"power": "on", "input": 2}} {
		if _, err := ValidateActionRequest(req(args)); err == nil {
			t.Fatalf("args %v validated", args)
		}
	}
}
