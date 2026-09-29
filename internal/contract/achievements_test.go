// Tests for the Den badges contract (contract.go Achievements): the Go types
// marshal to what state.schema.json accepts, celebrate is omitted when empty,
// and the schema refuses badges to guests, to a locked session and with a
// time in the earned day.

package contract

import (
	"strings"
	"testing"
)

func badgeState(a *Achievements) State {
	st := powerState(nil)
	st.Achievements = a
	return st
}

func sampleBadges() *Achievements {
	return &Achievements{
		Enabled:  true,
		Earned:   []EarnedBadge{{ID: "first-night-in", Day: "2026-09-28"}},
		Progress: []BadgeProgress{{ID: "first-night-in", Count: 1, Goal: 1}, {ID: "movie-night", Count: 3, Goal: 10}},
	}
}

func TestAchievementsMarshalToSchema(t *testing.T) {
	raw, err := MarshalAndValidateState(badgeState(sampleBadges()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"celebrate"`) {
		t.Fatalf("an empty celebrate must be omitted: %s", raw)
	}
	a := sampleBadges()
	a.Celebrate = []string{"first-night-in"}
	if _, err := MarshalAndValidateState(badgeState(a)); err != nil {
		t.Fatalf("shell view with celebrate: %v", err)
	}
	a = sampleBadges()
	a.Earned[0].Day = "2026-09-28T23:10:00Z"
	if _, err := MarshalAndValidateState(badgeState(a)); err == nil {
		t.Fatal("an earned day with a time validated")
	}
	a = sampleBadges()
	a.Progress[0].Goal = 0
	if _, err := MarshalAndValidateState(badgeState(a)); err == nil {
		t.Fatal("a zero goal validated")
	}
	locked := badgeState(sampleBadges())
	locked.Session.Locked = true
	if _, err := MarshalAndValidateState(locked); err == nil {
		t.Fatal("badges in a locked snapshot validated")
	}
	guest := badgeState(sampleBadges())
	exp := int64(1)
	guest.Me = &Me{DeviceID: "dev_g", DeviceName: "Guest", Permissions: []Permission{PermGuest}, ExpiresAtMs: &exp}
	if _, err := MarshalAndValidateState(guest); err == nil {
		t.Fatal("badges for a guest pass validated")
	}
	phone := badgeState(sampleBadges())
	phone.Me = &Me{DeviceID: "dev_c", DeviceName: "Phone", Permissions: []Permission{PermController}}
	if _, err := MarshalAndValidateState(phone); err != nil {
		t.Fatalf("controller phone view: %v", err)
	}
	phone.Achievements.Celebrate = []string{"first-night-in"}
	if _, err := MarshalAndValidateState(phone); err == nil {
		t.Fatal("celebrate on a phone validated")
	}
}
