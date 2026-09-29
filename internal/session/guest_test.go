// Guest pass tests: the allow-list gate in route() and HoldStart, the pass
// end checked on actions, the guest's redacted snapshot, and pair.issue with
// a pass over IPC (route.go phoneMay, state.go, ipc.go; spec
// contracts/http.md#guest-passes).

package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
)

func guestViewer(ends time.Time) remote.Viewer {
	ms := ends.UnixMilli()
	return remote.Viewer{DeviceID: "dev-guest", DeviceName: "Visitor", Permissions: []contract.Permission{contract.PermGuest}, ExpiresAtMs: &ms}
}

// validArgs gives each action arguments its schema accepts, so a refusal can
// only come from the permission gate.
func validArgs(action string) map[string]any {
	switch action {
	case contract.ActionAppLaunch:
		return map[string]any{"app_id": "plex-htpc"}
	case contract.ActionAppClose:
		return map[string]any{"app_id": "plex-htpc", "force": false}
	case contract.ActionMediaSeek:
		return map[string]any{"seconds": 10}
	case contract.ActionAudioVolume:
		return map[string]any{"delta": 5}
	case contract.ActionAudioMute:
		return map[string]any{"muted": true}
	case contract.ActionTextSubmit:
		return map[string]any{"text": "hello"}
	case contract.ActionPointerMove:
		return map[string]any{"dx": 10.0, "dy": -5.0}
	case contract.ActionPointerClick:
		return map[string]any{"button": "left"}
	case contract.ActionPointerScroll:
		return map[string]any{"dy": 120.0}
	case contract.ActionAppInstall, contract.ActionAppInstallCancel:
		return map[string]any{"app_id": "moonlight"}
	}
	return map[string]any{}
}

// Every action in the contract is either on the guest allow-list or refused
// to guests with forbidden; a new action added to AllActions is covered here
// without editing this test, and is refused until it joins GuestActions.
func TestGuestMaySendOnlyTheAllowList(t *testing.T) {
	h := newHarness(t)
	guest := guestViewer(time.Now().Add(time.Hour))
	for _, action := range contract.AllActions {
		res := h.submit(guest, h.req(action, validArgs(action)))
		if contract.GuestMayUse(action) {
			if res.Code == contract.CodeForbidden {
				t.Errorf("%s: guest refused although on the allow-list: %s", action, res.Message)
			}
			continue
		}
		if res.Outcome != contract.OutcomeFailed || res.Code != contract.CodeForbidden {
			t.Errorf("%s: guest got %s/%s, want failed/forbidden", action, res.Outcome, res.Code)
		}
	}
	// Names the parallel sleep-timer work adds (power.*, display.*) and any
	// other future action: refused to guests before they exist.
	for _, action := range []string{"power.off", "power.sleep_timer", "display.off", "anything.new", "pointer.move", "pointer.click", "pointer.scroll"} {
		if ok, _ := h.c.phoneMay(guest, contract.ActionRequest{Action: action}); ok {
			t.Errorf("guest allowed %q, which is not on the allow-list", action)
		}
	}
	// The same actions stay open to a controller (the gate is guest-specific).
	if res := h.submit(h.ctl, h.req(contract.ActionAppClose, validArgs(contract.ActionAppClose))); res.Code == contract.CodeForbidden {
		t.Errorf("controller refused app.close: %s", res.Message)
	}
	// A guest listing a higher permission as well is still a guest (fail closed).
	mixed := guest
	mixed.Permissions = []contract.Permission{contract.PermGuest, contract.PermOwner}
	if res := h.submit(mixed, h.req(contract.ActionShellRestart, nil)); res.Code != contract.CodeForbidden {
		t.Errorf("guest+owner restarted the shell: %s/%s", res.Outcome, res.Code)
	}
	if mixed.Has(contract.PermController) || mixed.Has(contract.PermOwner) {
		t.Error("a viewer holding guest satisfies controller or owner")
	}
}

// Once the pass has ended, actions and holds on an already open socket are
// refused even before pairing's timer revokes the device.
func TestEndedGuestPassRefusesActionsAndHolds(t *testing.T) {
	h := newHarness(t)
	ended := guestViewer(time.Now().Add(-time.Second))
	res := h.submit(ended, h.req(contract.ActionNavUp, nil))
	if res.Code != contract.CodeForbidden || !strings.Contains(res.Message, "ended") {
		t.Fatalf("ended pass: %s/%s %q", res.Outcome, res.Code, res.Message)
	}
	st := h.phones.HoldStart(context.Background(), ended, contract.HoldMessage{Type: "hold.start", HoldID: "h1", Action: contract.ActionNavUp, ContextEpoch: h.c.Epoch()})
	if st.State != "rejected" || st.Reason != "forbidden" {
		t.Fatalf("hold for an ended pass: %+v", st)
	}
	live := guestViewer(time.Now().Add(time.Hour))
	if st := h.phones.HoldStart(context.Background(), live, contract.HoldMessage{Type: "hold.start", HoldID: "h2", Action: contract.ActionNavUp, ContextEpoch: h.c.Epoch()}); st.State == "rejected" {
		t.Fatalf("guest hold on nav.up rejected: %+v", st)
	}
	h.phones.HoldStop(context.Background(), live, "h2", "test")
}

// A guest sees its own pass end and what is playing, never devices, layout,
// pairing or the shell-only fields.
func TestGuestSnapshotRedaction(t *testing.T) {
	h := newNPHarness(t)
	h.media.Add(adapters.PlexHTPCFlatpakID, demoPlex(h.clk))
	h.front("plexhtpc")
	h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })

	guest := guestViewer(h.clk.Now().Add(time.Hour))
	st := h.phones.Snapshot(context.Background(), &guest)
	if st.Me == nil || st.Me.ExpiresAtMs == nil || *st.Me.ExpiresAtMs != *guest.ExpiresAtMs {
		t.Fatalf("guest me: %+v", st.Me)
	}
	if st.Devices != nil || st.Layout != nil || st.Pairing != nil || st.Playback != nil || st.Weather != nil {
		t.Fatalf("guest sees owner/shell fields: devices=%v layout=%v pairing=%v", st.Devices != nil, st.Layout != nil, st.Pairing != nil)
	}
	if st.NowPlaying == nil {
		t.Fatal("guest does not see now_playing")
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}
	if ctl := h.phones.Snapshot(context.Background(), &h.ctl); ctl.Me.ExpiresAtMs != nil {
		t.Fatal("a family phone's me carries an expiry")
	}
}

// pair.issue with a pass issues a guest pass from the trusted socket; an
// unknown pass fails closed. Phones have no route to this at all
// (internal/remote routes_test.go).
func TestPairIssueGuestPassOverIPC(t *testing.T) {
	h := newHarness(t)
	if err := h.shell.Send(shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: "p1", Pass: contract.Pass7d}); err != nil {
		t.Fatal(err)
	}
	r := h.shellResult("p1")
	data, _ := r.Data.(map[string]any)
	if !r.OK || data["pass_expires_at_ms"] == nil {
		t.Fatalf("guest pair.issue: %+v", r)
	}
	st := h.c.buildState(viewShell)
	if st.Pairing == nil || !st.Pairing.Guest || st.Pairing.PassExpiresAtMs == nil {
		t.Fatalf("shell pairing state: %+v", st.Pairing)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}
	if err := h.shell.Send(shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: "p2", Pass: "forever"}); err != nil {
		t.Fatal(err)
	}
	if r := h.shellResult("p2"); r.OK {
		t.Fatal("unknown pass accepted")
	}
	if err := h.shell.Send(shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: "p3"}); err != nil {
		t.Fatal(err)
	}
	if r := h.shellResult("p3"); !r.OK {
		t.Fatalf("family pair.issue: %+v", r)
	}
	if st := h.c.buildState(viewShell); st.Pairing.Guest || st.Pairing.PassExpiresAtMs != nil {
		t.Fatalf("family invitation shows as a guest pass: %+v", st.Pairing)
	}
}
