// Tests that the plex.* IPC messages (plex.go, contracts/ipc.md) decode to
// their typed values and round-trip through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestPlexMessagesDecode(t *testing.T) {
	cases := map[string]Message{
		`{"type":"plex.sign_in","request_id":"r1"}`:                                  PlexSignIn{Type: TypePlexSignIn, RequestID: "r1"},
		`{"type":"plex.cancel","request_id":"r2"}`:                                   PlexCancel{Type: TypePlexCancel, RequestID: "r2"},
		`{"type":"plex.choose_server","request_id":"r3","server_id":"abc"}`:          PlexChooseServer{Type: TypePlexChooseServer, RequestID: "r3", ServerID: "abc"},
		`{"type":"plex.choose_libraries","request_id":"r4","library_ids":["1","2"]}`: PlexChooseLibraries{Type: TypePlexChooseLibraries, RequestID: "r4", LibraryIDs: []string{"1", "2"}},
		`{"type":"plex.sign_out","request_id":"r5"}`:                                 PlexSignOut{Type: TypePlexSignOut, RequestID: "r5"},
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
	if _, err := Decode([]byte(`{"type":"plex.token"}`)); err == nil {
		t.Fatal("an unknown plex.* type decoded")
	}
}
