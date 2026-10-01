// Tests that link.open (links.go, contracts/ipc.md) decodes to its typed
// value and round-trips through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestLinkOpenDecodes(t *testing.T) {
	frame := `{"type":"link.open","request_id":"r1","url":"https://accounts.spotify.com/en/login"}`
	want := LinkOpen{Type: TypeLinkOpen, RequestID: "r1", URL: "https://accounts.spotify.com/en/login"}
	got, err := Decode([]byte(frame))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
	enc, err := Encode(got)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := Decode(enc[:len(enc)-1]); err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("round trip gave %#v, %v", again, err)
	}
}
