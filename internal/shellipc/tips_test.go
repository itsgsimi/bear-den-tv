// Tests that the bear tip IPC messages (tips.go, contracts/ipc.md tips.*)
// decode to their typed values and round-trip through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestTipsMessagesDecode(t *testing.T) {
	cases := map[string]Message{
		`{"type":"tips.configure","request_id":"r1","enabled":false}`: TipsConfigure{Type: TypeTipsConfigure, RequestID: "r1"},
		`{"type":"tips.configure","request_id":"r2","enabled":true}`:  TipsConfigure{Type: TypeTipsConfigure, RequestID: "r2", Enabled: true},
		`{"type":"tips.reset","request_id":"r3"}`:                     TipsReset{Type: TypeTipsReset, RequestID: "r3"},
		`{"type":"tips.event","tip":"themes","event":"not_now"}`:      TipsEvent{Type: TypeTipsEvent, Tip: "themes", Event: TipEventNotNow},
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
	if _, err := Decode([]byte(`{"type":"tips.show"}`)); err == nil {
		t.Fatal("an unknown tips.* type decoded")
	}
}
