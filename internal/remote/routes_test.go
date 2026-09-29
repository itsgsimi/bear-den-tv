// Tests for the route table (routes.go): what phones can and cannot reach.

package remote

import (
	"strings"
	"testing"

	"bear-den-tv/internal/contract"
)

// Phones can redeem an invitation but never issue one, family or guest:
// invitations come only from the TV and the local CLI over IPC
// (contracts/http.md#guest-passes). The only pairing route is the claim.
func TestPhonesCannotIssueInvitations(t *testing.T) {
	for _, rt := range routes {
		p := strings.ToLower(rt.pattern)
		if strings.Contains(p, "pair") || strings.Contains(p, "invit") || strings.Contains(p, "guest") || strings.Contains(p, "grant") {
			if rt.pattern != "/api/v1/pair/claim" {
				t.Errorf("%s %s lets a phone reach pairing", rt.method, rt.pattern)
			}
		}
	}
}

// Every permission-gated route stays closed to a guest pass.
func TestGuestsFailEveryPermissionGatedRoute(t *testing.T) {
	guest := Viewer{DeviceID: "dev_g", Permissions: []contract.Permission{contract.PermGuest}}
	for _, rt := range routes {
		if rt.perm != "" && guest.Has(rt.perm) {
			t.Errorf("guest passes the %s gate of %s %s", rt.perm, rt.method, rt.pattern)
		}
	}
}
