// Tests that the achievements.* IPC messages (achievements.go,
// contracts/ipc.md) decode to their typed values and round-trip through Encode.

package shellipc

import (
	"reflect"
	"testing"
)

func TestAchievementMessagesDecode(t *testing.T) {
	cases := map[string]Message{
		`{"type":"achievements.configure","request_id":"r1","enabled":false}`: AchievementsConfigure{Type: TypeAchievementsConfigure, RequestID: "r1"},
		`{"type":"achievements.reset","request_id":"r2"}`:                     AchievementsReset{Type: TypeAchievementsReset, RequestID: "r2"},
		`{"type":"achievements.celebrated","ids":["good-host"]}`:              AchievementsCelebrated{Type: TypeAchievementsCelebrated, IDs: []string{"good-host"}},
		`{"type":"achievements.event","event":"parade"}`:                      AchievementsEvent{Type: TypeAchievementsEvent, Event: AchievementEventParade},
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
	if _, err := Decode([]byte(`{"type":"achievements.counters"}`)); err == nil {
		t.Fatal("an unknown achievements.* type decoded")
	}
}
