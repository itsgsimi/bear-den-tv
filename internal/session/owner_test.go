// The owner-only gate in route() (phoneMay, contract.OwnerActions): app
// installs and shell.restart need the owner permission from a phone; every
// lower permission, a guest pass included, is refused with forbidden before
// anything else is looked at (spec contracts/actions.md "Who may send").

package session

import (
	"testing"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
)

func TestOwnerActionsNeedTheOwnerPermission(t *testing.T) {
	h := newHarness(t)
	ms := time.Now().Add(time.Hour).UnixMilli()
	viewers := []struct {
		name    string
		v       remote.Viewer
		allowed bool
	}{
		{"guest", remote.Viewer{DeviceID: "dev-g", Permissions: []contract.Permission{contract.PermGuest}, ExpiresAtMs: &ms}, false},
		{"controller", remote.Viewer{DeviceID: "dev-c", Permissions: []contract.Permission{contract.PermController}}, false},
		{"layout editor", remote.Viewer{DeviceID: "dev-l", Permissions: []contract.Permission{contract.PermController, contract.PermLayoutEditor}}, false},
		{"owner", remote.Viewer{DeviceID: "dev-o", Permissions: []contract.Permission{contract.PermOwner}}, true},
	}
	for _, action := range []string{contract.ActionAppInstall, contract.ActionAppInstallCancel} {
		if !contract.OwnerActions[action] {
			t.Fatalf("%s is not in contract.OwnerActions", action)
		}
		for _, tc := range viewers {
			res := h.submit(tc.v, h.req(action, validArgs(action)))
			forbidden := res.Outcome == contract.OutcomeFailed && res.Code == contract.CodeForbidden
			if tc.allowed && forbidden {
				t.Errorf("%s from %s: refused as forbidden: %s", action, tc.name, res.Message)
			}
			if !tc.allowed && !forbidden {
				t.Errorf("%s from %s: got %s/%s, want failed/forbidden", action, tc.name, res.Outcome, res.Code)
			}
		}
	}
}
