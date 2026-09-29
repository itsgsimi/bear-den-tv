// Tests for the guest permission's place in the ranking and its allow-list
// (contract.go; spec contracts/actions.md, contracts/http.md#guest-passes).

package contract

import "testing"

func TestGuestRanksBelowController(t *testing.T) {
	if !(PermGuest.Rank() > 0 && PermGuest.Rank() < PermController.Rank()) {
		t.Fatalf("guest rank %d must be known and below controller %d", PermGuest.Rank(), PermController.Rank())
	}
	if !(PermController.Rank() < PermLayoutEditor.Rank() && PermLayoutEditor.Rank() < PermOwner.Rank()) {
		t.Fatal("controller < layout_editor < owner no longer holds")
	}
}

func TestGuestActionsAreKnownAndExcludeOwnerActions(t *testing.T) {
	known := map[string]bool{}
	for _, a := range AllActions {
		known[a] = true
	}
	for a := range GuestActions {
		if !known[a] {
			t.Errorf("guest allow-list names unknown action %q", a)
		}
	}
	for _, a := range []string{ActionAppClose, ActionShellRestart} {
		if GuestMayUse(a) {
			t.Errorf("guests may use %s", a)
		}
	}
	if GuestMayUse("power.off") || GuestMayUse("display.sleep") {
		t.Error("an action outside the allow-list is allowed to guests")
	}
}
