// Tests for the per-app adapter registry (adapters.go).

package adapters

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/platform"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if got := r.Names(); !reflect.DeepEqual(got, []string{"browser", "disney-plus", "hulu", "jellyfin", "moonlight", "netflix", "plex-htpc", "retroarch", "spotify", "vacuumtube"}) {
		t.Fatalf("%v", got)
	}
	if _, ok := r.ForName("kodi"); ok {
		t.Fatal("unknown adapter resolved")
	}
	if a, ok := ForName("plex-htpc"); !ok || a.FlatpakID() != "tv.plex.PlexHTPC" {
		t.Fatal("plex-htpc")
	}
	if len(r.All()) != 10 {
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

// The optional apps are rows of data: id, arguments, window match, key map,
// MPRIS match and Home's pause, all checked here (docs in each constructor).
func TestOptionalApps(t *testing.T) {
	cases := []struct {
		name, id string
		args     []string
		class    []string
		keys     map[string]platform.Key
		unmapped []string
		media    string
		home     HomePause
	}{
		{
			name: "spotify", id: "com.spotify.Client", class: []string{"spotify", "Spotify"},
			keys:     map[string]platform.Key{"nav.up": platform.KeyUp, "nav.down": platform.KeyDown, "select": platform.KeySelect},
			unmapped: []string{"nav.left", "nav.right", "back", "media.play", "media.pause"},
			media:    "spotify",
			home:     HomePause{Kind: "none"},
		},
		{
			name: "jellyfin", id: "org.jellyfin.JellyfinDesktop", args: []string{"--fullscreen", "--tv"},
			class: []string{"org.jellyfin.JellyfinDesktop", "org.jellyfin.JellyfinDesktop"},
			keys: map[string]platform.Key{"nav.up": platform.KeyUp, "nav.down": platform.KeyDown, "nav.left": platform.KeyLeft,
				"nav.right": platform.KeyRight, "select": platform.KeySelect, "back": platform.KeyBack},
			unmapped: []string{"media.play", "media.pause"},
			media:    "org.jellyfin.JellyfinDesktop",
			home:     HomePause{Kind: "mpris"},
		},
		{
			name: "retroarch", id: "org.libretro.RetroArch", args: []string{"--fullscreen"}, class: []string{"retroarch", "RetroArch"},
			keys: map[string]platform.Key{"nav.up": platform.KeyUp, "nav.down": platform.KeyDown, "nav.left": platform.KeyLeft,
				"nav.right": platform.KeyRight, "select": platform.KeyLetterX, "back": platform.KeyLetterZ},
			unmapped: []string{"media.play", "media.pause"},
			media:    "org.libretro.RetroArch",
			home:     HomePause{Kind: "key", Key: platform.KeyLetterP, Toggle: true},
		},
	}
	for _, c := range cases {
		a, ok := ForName(c.name)
		if !ok {
			t.Fatalf("%s is not registered", c.name)
		}
		if a.FlatpakID() != c.id || strings.Join(a.ApprovedArgs(), " ") != strings.Join(c.args, " ") {
			t.Errorf("%s: id %s args %v", c.name, a.FlatpakID(), a.ApprovedArgs())
		}
		if !a.MatchWindow(platform.WindowInfo{Class: c.class}) || a.MatchWindow(platform.WindowInfo{Class: []string{"plexhtpc"}, Title: c.name}) {
			t.Errorf("%s: window matching", c.name)
		}
		if !a.MatchWindow(platform.WindowInfo{AppID: c.id}) {
			t.Errorf("%s: Wayland app_id %s not matched", c.name, c.id)
		}
		for action, want := range c.keys {
			if got, ok := a.KeyFor(action); !ok || got != want {
				t.Errorf("%s: KeyFor(%s)=%q,%v want %q", c.name, action, got, ok, want)
			}
		}
		for _, action := range c.unmapped {
			if k, ok := a.KeyFor(action); ok {
				t.Errorf("%s: %s must stay unmapped, got %q", c.name, action, k)
			}
		}
		// RetroArch quits on Escape (input_exit_emulator): never send it.
		for _, action := range []string{"back", "select", "nav.up"} {
			if k, _ := a.KeyFor(action); c.name == "retroarch" && k == platform.KeyBack {
				t.Errorf("retroarch: %s maps to Escape, which quits RetroArch", action)
			}
		}
		if a.MediaMatch() != c.media {
			t.Errorf("%s: MediaMatch %q want %q", c.name, a.MediaMatch(), c.media)
		}
		h := HomePauseOf(a)
		if h.Kind != c.home.Kind || h.Key != c.home.Key || h.Toggle != c.home.Toggle || h.Why == "" {
			t.Errorf("%s: HomePause %+v want %+v (with a reason)", c.name, h, c.home)
		}
		if a.PauseVerified() {
			t.Errorf("%s: pause must stay unverified", c.name)
		}
	}
}

// Web adapters: the streaming sites in Google Chrome and the Browser tile
// in Brave by default, no XTEST keys at all (input goes through the page),
// windows told apart by the per-adapter WM_CLASS Bear Den starts the browser
// with, and never by a plain browser window.
func TestWebAdapters(t *testing.T) {
	cases := []struct{ name, mode, hints, flatpak string }{
		{NetflixName, WebModeApp, "netflix", ChromeFlatpakID},
		{DisneyPlusName, WebModeApp, "disney-plus", ChromeFlatpakID},
		{HuluName, WebModeApp, "hulu", ChromeFlatpakID},
		{BrowserName, WebModeBrowser, "", BraveFlatpakID},
	}
	for _, c := range cases {
		a, ok := ForName(c.name)
		if !ok {
			t.Fatalf("%s missing", c.name)
		}
		spec, ok := WebOf(a)
		if !ok || spec.Mode != c.mode || spec.Hints != c.hints || spec.Class != "BearDenWeb-"+c.name {
			t.Errorf("%s: web spec %+v", c.name, spec)
		}
		if a.FlatpakID() != c.flatpak || len(a.ApprovedArgs()) != 0 {
			t.Errorf("%s: flatpak %s args %v", c.name, a.FlatpakID(), a.ApprovedArgs())
		}
		for _, action := range []string{"nav.up", "select", "back", "media.play"} {
			if k, ok := a.KeyFor(action); ok {
				t.Errorf("%s: %s maps to XTEST key %q", c.name, action, k)
			}
		}
		if !a.MatchWindow(platform.WindowInfo{Class: []string{"www.example.com", "BearDenWeb-" + c.name}}) {
			t.Errorf("%s: own window not matched", c.name)
		}
		for _, plain := range [][]string{{"google-chrome", "Google-chrome"}, {"brave-browser", "Brave-browser"}, {"chromium-browser", "Chromium"}} {
			if a.MatchWindow(platform.WindowInfo{Class: plain}) {
				t.Errorf("%s: a plain browser window %v matched", c.name, plain)
			}
		}
		if a.MediaMatch() != c.flatpak || HomePauseOf(a).Kind != "page" {
			t.Errorf("%s: media %q home %+v", c.name, a.MediaMatch(), HomePauseOf(a))
		}
	}
	// Each streaming site is its own app: one Chrome install, but no site
	// matches another's window.
	for _, a := range cases {
		ad, _ := ForName(a.name)
		for _, b := range cases {
			if a.name != b.name && ad.MatchWindow(platform.WindowInfo{Class: []string{"www.example.com__", "BearDenWeb-" + b.name}}) {
				t.Errorf("%s matched %s's window", a.name, b.name)
			}
		}
	}
	if _, ok := WebOf(PlexHTPC()); ok {
		t.Error("plex-htpc reported as a web adapter")
	}
}

// Every adapter row has notes, and every note fits state.applications[].notes:
// 1..160 characters, one plain sentence ending in a full stop, no URL; room
// is left for a derived note (the unverified browser). An app Home leaves
// running (HomePause none, or a key that is never sent) says so.
func TestNotes(t *testing.T) {
	for _, ad := range NewRegistry().All() {
		notes := NotesOf(ad)
		if len(notes) == 0 {
			t.Errorf("%s: no notes", ad.Name())
		}
		if len(notes) > 5 {
			t.Errorf("%s: %d notes leave no room for a derived one (6 at most)", ad.Name(), len(notes))
		}
		for _, n := range notes {
			if len([]rune(n)) < 1 || len([]rune(n)) > 160 {
				t.Errorf("%s: %d characters: %q", ad.Name(), len([]rune(n)), n)
			}
			if strings.Contains(n, "://") || strings.Contains(strings.ToLower(n), "www.") {
				t.Errorf("%s: a URL in %q", ad.Name(), n)
			}
			if !strings.HasSuffix(n, ".") || strings.Contains(n, "\n") {
				t.Errorf("%s: not one plain sentence: %q", ad.Name(), n)
			}
		}
		if k := HomePauseOf(ad).Kind; k == "none" || k == "key" {
			if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "Home") }) {
				t.Errorf("%s: Home leaves it running (HomePause %s) but no note says so: %q", ad.Name(), k, notes)
			}
		}
	}
	// A copy: callers cannot change the table.
	n := NotesOf(PlexHTPC())
	n[0] = "changed"
	if NotesOf(PlexHTPC())[0] == "changed" {
		t.Fatal("NotesOf returned the table itself")
	}
}

// The browser table: Google Chrome (the streaming sites' default) and Brave
// (the Browser tile's), and Chromium nowhere: not a row, not installable,
// not a Flatpak a web app may run from.
func TestBrowserTable(t *testing.T) {
	var names []string
	for _, b := range Browsers() {
		names = append(names, b.Name)
		if b.FlatpakID == "org.chromium.Chromium" || b.Name == "chromium" {
			t.Errorf("Chromium is still a browser: %+v", b)
		}
	}
	if !reflect.DeepEqual(names, []string{ChromeBrowser, BraveBrowser}) {
		t.Fatalf("browsers %v", names)
	}
	chrome, _ := BrowserNamed(ChromeBrowser)
	brave, _ := BrowserNamed(BraveBrowser)
	if chrome.Label != "Google Chrome" || chrome.FlatpakID != "com.google.Chrome" || !chrome.GrantProfileRoot || !chrome.BundledWidevine || !chrome.BlockProfileWidevine || chrome.StreamingUnverified {
		t.Errorf("chrome row %+v", chrome)
	}
	if brave.FlatpakID != "com.brave.Browser" || brave.BundledWidevine || brave.BlockProfileWidevine || !brave.StreamingUnverified {
		t.Errorf("brave row %+v", brave)
	}
	if chrome.ProfileRoot == brave.ProfileRoot || chrome.ProfileRoot == "web" || brave.ProfileRoot == "web" {
		t.Errorf("profile roots %q %q (web/ was Chromium's)", chrome.ProfileRoot, brave.ProfileRoot)
	}
	notes := strings.Join(chrome.Notes, "\n")
	for _, want := range []string{"shares usage data with Google", "720p", "community-packaged"} {
		if !strings.Contains(notes, want) {
			t.Errorf("chrome notes lack %q: %q", want, notes)
		}
	}
	if DefaultBrowserFor(WebSpec{Mode: WebModeApp}).Name != ChromeBrowser || DefaultBrowserFor(WebSpec{Mode: WebModeBrowser}).Name != BraveBrowser {
		t.Error("defaults are not Chrome for streaming and Brave for the tile")
	}
	nf, _ := ForName(NetflixName)
	if RunsIn(nf, "org.chromium.Chromium") || !RunsIn(nf, BraveFlatpakID) || !RunsIn(nf, ChromeFlatpakID) {
		t.Error("RunsIn disagrees with the table")
	}
	for _, id := range NewRegistry().InstallableFlatpakIDs() {
		if id == "org.chromium.Chromium" {
			t.Error("Chromium is installable")
		}
	}
}
