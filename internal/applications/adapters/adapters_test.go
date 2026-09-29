// Tests for the per-app adapter registry (adapters.go).

package adapters

import (
	"reflect"
	"testing"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/platform"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if got := r.Names(); !reflect.DeepEqual(got, []string{"moonlight", "plex-htpc", "vacuumtube"}) {
		t.Fatalf("%v", got)
	}
	if _, ok := r.ForName("kodi"); ok {
		t.Fatal("unknown adapter resolved")
	}
	if a, ok := ForName("plex-htpc"); !ok || a.FlatpakID() != "tv.plex.PlexHTPC" {
		t.Fatal("plex-htpc")
	}
	if len(r.All()) != 3 {
		t.Fatal("All")
	}
}

func TestPlexHTPC(t *testing.T) {
	a := PlexHTPC()
	if a.Name() != "plex-htpc" || a.FlatpakID() != "tv.plex.PlexHTPC" || a.MediaMatch() != "tv.plex.PlexHTPC" {
		t.Fatal(a.Name(), a.FlatpakID(), a.MediaMatch())
	}
	if args := a.ApprovedArgs(); len(args) != 0 {
		t.Fatalf("plex must launch without arguments: %v", args)
	}
	if a.PauseVerified() {
		t.Fatal("pause must stay unverified without recorded evidence")
	}
	for _, c := range [][]string{{"plex htpc", "Plex HTPC"}, {"plexhtpc", "plexhtpc"}, {"Plex"}} {
		if !a.MatchWindow(platform.WindowInfo{Class: c}) {
			t.Errorf("class %q not matched", c)
		}
	}
	for _, c := range [][]string{{"vacuumtube", "VacuumTube"}, {"xfce4-terminal", "Xfce4-terminal"}, nil} {
		if a.MatchWindow(platform.WindowInfo{Class: c, Title: "Plex HTPC"}) {
			t.Errorf("class %q matched (title must not count)", c)
		}
	}
	want := map[string]platform.Key{
		"nav.up": platform.KeyUp, "nav.down": platform.KeyDown, "nav.left": platform.KeyLeft, "nav.right": platform.KeyRight,
		"select": platform.KeySelect, "back": platform.KeyBack, "media.play": platform.KeyPlay, "media.pause": platform.KeyPause,
	}
	for action, k := range want {
		if got, ok := a.KeyFor(action); !ok || got != k {
			t.Errorf("KeyFor(%s)=%q,%v want %q", action, got, ok, k)
		}
	}
	for _, unmapped := range []string{"home", "text.submit", "media.seek_relative", "shell.restart", ""} {
		if _, ok := a.KeyFor(unmapped); ok {
			t.Errorf("%q must be unmapped", unmapped)
		}
	}
}

func TestVacuumTube(t *testing.T) {
	a := VacuumTube()
	if a.Name() != "vacuumtube" || a.FlatpakID() != "rocks.shy.VacuumTube" {
		t.Fatal(a.Name(), a.FlatpakID())
	}
	if args := a.ApprovedArgs(); !reflect.DeepEqual(args, []string{"--fullscreen", "--no-window-decorations"}) {
		t.Fatalf("%v", args)
	}
	args := a.ApprovedArgs()
	args[0] = "--mutated"
	if a.ApprovedArgs()[0] != "--fullscreen" {
		t.Fatal("ApprovedArgs must return a copy")
	}
	if a.PauseVerified() {
		t.Fatal("pause must stay unverified")
	}
	if !a.MatchWindow(platform.WindowInfo{Class: []string{"vacuumtube", "VacuumTube"}}) || a.MatchWindow(platform.WindowInfo{Class: []string{"plexhtpc"}}) {
		t.Fatal("window matching")
	}
	if _, ok := a.KeyFor("media.pause"); ok {
		t.Fatal("vacuumtube media keys must stay unmapped")
	}
	if k, ok := a.KeyFor("back"); !ok || k != platform.KeyBack {
		t.Fatal("back")
	}
}

func TestMatchClass(t *testing.T) {
	if MatchClass([]string{"Foo"}, []string{""}) {
		t.Fatal("empty fragment matched")
	}
	if !MatchClass([]string{"x", "PLEXhtpc"}, []string{"nope", "plex"}) {
		t.Fatal("case-insensitive substring")
	}
}

// Wayland windows carry an app_id instead of WM_CLASS
// (docs/decisions/0007-wayland-profile.md).
func TestMatchWaylandAppID(t *testing.T) {
	cases := []struct {
		adapter applications.Adapter
		appID   string
		want    bool
	}{
		{PlexHTPC(), "tv.plex.PlexHTPC", true},
		{PlexHTPC(), "TV.PLEX.PLEXHTPC", true},
		{VacuumTube(), "rocks.shy.VacuumTube", true},
		{VacuumTube(), "vacuumtube", true}, // XWayland: the X11 class
		{Moonlight(), "com.moonlight_stream.Moonlight", true},
		{Moonlight(), "tv.plex.PlexHTPC", false},
		{PlexHTPC(), "org.example.Terminal", false},
		{PlexHTPC(), "", false},
	}
	for _, tc := range cases {
		if got := tc.adapter.MatchWindow(platform.WindowInfo{AppID: tc.appID}); got != tc.want {
			t.Errorf("app_id %q: match %v, want %v", tc.appID, got, tc.want)
		}
	}
	// Exact Flatpak id, even with no fragment in common.
	if !MatchAppID("org.example.App", "org.example.App", []string{"zzz"}) || MatchAppID("", "", []string{""}) {
		t.Fatal("MatchAppID")
	}
}

func TestMoonlight(t *testing.T) {
	a := Moonlight()
	if a.Name() != "moonlight" || a.FlatpakID() != "com.moonlight_stream.Moonlight" || len(a.ApprovedArgs()) != 0 {
		t.Fatalf("moonlight adapter = %s %s %v", a.Name(), a.FlatpakID(), a.ApprovedArgs())
	}
	if !a.MatchWindow(platform.WindowInfo{Class: []string{"moonlight", "Moonlight"}}) || a.MatchWindow(platform.WindowInfo{Class: []string{"plexhtpc"}}) {
		t.Fatal("moonlight window matching")
	}
	if _, ok := a.KeyFor("media.pause"); ok {
		t.Fatal("moonlight media keys must stay unmapped")
	}
	if k, ok := a.KeyFor("back"); !ok || k != platform.KeyBack {
		t.Fatal("moonlight back must map to the back key")
	}
}
