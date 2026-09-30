// Tests that the setup IPC messages (setup.go, contracts/ipc.md
// onboarding.complete and autostart.configure) decode to their typed values
// and round-trip through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestSetupMessagesDecode(t *testing.T) {
	cases := map[string]Message{
		`{"type":"onboarding.complete","request_id":"r1"}`:                 OnboardingComplete{Type: TypeOnboardingComplete, RequestID: "r1"},
		`{"type":"autostart.configure","request_id":"r2","enabled":true}`:  AutostartConfigure{Type: TypeAutostartConfigure, RequestID: "r2", Enabled: true},
		`{"type":"autostart.configure","request_id":"r3","enabled":false}`: AutostartConfigure{Type: TypeAutostartConfigure, RequestID: "r3"},
	}
	for frame, want := range cases {
		got, err := Decode([]byte(frame))
		if err != nil {
			t.Fatalf("%s: %v", frame, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %#v, want %#v", frame, got, want)
		}
		enc, err := Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(enc[:len(enc)-1])
		if err != nil || !reflect.DeepEqual(again, want) {
			t.Fatalf("%s: round trip gave %#v, %v", frame, again, err)
		}
	}
}
