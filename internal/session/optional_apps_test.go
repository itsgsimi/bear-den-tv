// Tests for optional apps in the state snapshot: an application with config
// hide_when_missing is `hidden` while its Flatpak is not installed, for the
// shell and for phones (contracts/state.schema.json applications[].hidden;
// internal/session/state.go appStatesLocked).

package session

import (
	"context"
	"testing"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/contract"
)

// someInstalled reports only the listed Flatpak ids as installed.
type someInstalled struct {
	*fakeLauncher
	installed map[string]bool
}

func (s someInstalled) Discover(_ context.Context, id string) (applications.Installation, error) {
	if s.installed[id] {
		return applications.Installation{Installed: true, Version: "1.0", Scope: "user"}, nil
	}
	return applications.Installation{Installed: false, Scope: "none"}, nil
}

func TestOptionalAppsHiddenWhenMissing(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		fl := o.Launcher.(*fakeLauncher)
		// Spotify is installed; Jellyfin and RetroArch are not; nor is Plex,
		// a core app, which must still show (as "Not installed").
		o.Launcher = someInstalled{fakeLauncher: fl, installed: map[string]bool{
			"rocks.shy.VacuumTube": true, "com.moonlight_stream.Moonlight": true, "com.spotify.Client": true,
		}}
	})
	// Chromium is not installed either: every web app is hidden (and the
	// streaming sites are also off by default).
	want := map[string]bool{"plex-htpc": false, "youtube": false, "moonlight": false, "spotify": false, "jellyfin": true, "retroarch": true,
		"netflix": true, "disney-plus": true, "hulu": true, "browser": true}
	check := func(who string, apps []contract.AppState) {
		t.Helper()
		if len(apps) != len(want) {
			t.Fatalf("%s: %d applications, want %d", who, len(apps), len(want))
		}
		for _, a := range apps {
			if a.Hidden != want[a.ID] {
				t.Errorf("%s: %s hidden=%v want %v (installed=%v)", who, a.ID, a.Hidden, want[a.ID], a.Installed)
			}
		}
	}
	h.eventually("discovery", func() bool {
		for _, a := range h.c.buildState(viewShell).Applications {
			if a.ID == "spotify" && a.Installed {
				return true
			}
		}
		return false
	})
	shell := h.c.buildState(viewShell)
	check("shell", shell.Applications)
	check("phone", h.phones.Snapshot(context.Background(), &h.ctl).Applications)
	if _, err := contract.MarshalAndValidateState(shell); err != nil {
		t.Fatal(err)
	}
}
