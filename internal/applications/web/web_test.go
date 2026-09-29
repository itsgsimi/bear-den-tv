// Tests for the web app launch definition (web.go: profile directory,
// Chromium's argument list, the page address re-check), the embedded script
// and hints (script.go), and the closed set of effects Bear Den performs for
// a page (browser.go checkEffect, scriptCall).

package web

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

func browser(t *testing.T, name string) adapters.BrowserInfo {
	t.Helper()
	b, ok := adapters.BrowserNamed(name)
	if !ok {
		t.Fatal(name)
	}
	return b
}

func TestProfileDirIsPerAppAndBrowser(t *testing.T) {
	chromium, brave := browser(t, "chromium"), browser(t, "brave")
	got, err := ProfileDir("/home/u/.local/share", chromium, "netflix")
	if err != nil || got != "/home/u/.local/share/bear-den-tv/web/netflix" {
		t.Fatalf("got %q %v", got, err)
	}
	other, _ := ProfileDir("/home/u/.local/share", chromium, "hulu")
	if other == got {
		t.Fatal("two apps share a profile")
	}
	// Brave never opens a Chromium profile.
	if b, err := ProfileDir("/home/u/.local/share", brave, "netflix"); err != nil || b != "/home/u/.local/share/bear-den-tv/web-brave/netflix" {
		t.Fatalf("brave profile %q %v", b, err)
	}
	for _, bad := range []string{"", "../x", "Netflix", "a/b", "net flix", strings.Repeat("a", 65)} {
		if _, err := ProfileDir("/d", chromium, bad); err == nil {
			t.Errorf("app id %q accepted", bad)
		}
	}
	if _, err := ProfileDir("relative", chromium, "netflix"); err == nil {
		t.Error("relative data dir accepted")
	}
	if _, err := ProfileDir("/d", adapters.BrowserInfo{FlatpakID: "org.example.Browser", ProfileRoot: "../x"}, "netflix"); err == nil {
		t.Error("a browser outside the table got a profile root")
	}
}

func spec(t *testing.T, name string) adapters.WebSpec {
	t.Helper()
	ad, ok := adapters.ForName(name)
	if !ok {
		t.Fatal(name)
	}
	s, ok := adapters.WebOf(ad)
	if !ok {
		t.Fatal(name + " is not web")
	}
	return s
}

func TestLaunchArgvExact(t *testing.T) {
	app := ChromiumArgs(spec(t, "netflix"), "/d/bear-den-tv/web/netflix", "https://www.netflix.com/")
	want := []string{
		"--user-data-dir=/d/bear-den-tv/web/netflix",
		"--remote-debugging-pipe",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=BearDenWeb-netflix",
		"--start-fullscreen",
		"--app=https://www.netflix.com/",
	}
	if !reflect.DeepEqual(app, want) {
		t.Fatalf("app mode argv\n got %q\nwant %q", app, want)
	}
	full, err := FlatpakArgv("/d", browser(t, "chromium"), app)
	if err != nil || !reflect.DeepEqual(full[:3], []string{"flatpak", "run", "org.chromium.Chromium"}) || !reflect.DeepEqual(full[3:], want) {
		t.Fatalf("flatpak argv %q %v", full, err)
	}
	browser := ChromiumArgs(spec(t, "browser"), "/d/bear-den-tv/web/browser", BlankPage)
	wantB := []string{
		"--user-data-dir=/d/bear-den-tv/web/browser",
		"--remote-debugging-pipe",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=BearDenWeb-browser",
		"--start-maximized",
		"about:blank",
	}
	if !reflect.DeepEqual(browser, wantB) {
		t.Fatalf("browser argv\n got %q\nwant %q", browser, wantB)
	}
	// The control channel is the pipe only: nothing that listens, nothing
	// that loads code from disk.
	for _, a := range append(app, browser...) {
		for _, bad := range []string{"--remote-debugging-port", "--remote-debugging-address", "--load-extension", "--remote-allow-origins", "--disable-web-security"} {
			if strings.HasPrefix(a, bad) {
				t.Errorf("argv carries %s", a)
			}
		}
	}
}

func webApp(adapter, url string) config.Application {
	a := config.Application{ID: adapter, Label: adapter, Adapter: adapter, Launch: config.Launch{Kind: "flatpak", AppID: "org.chromium.Chromium"}}
	if url != "" {
		a.Web = &config.Web{URL: url}
	}
	return a
}

func TestStartURLRechecksRule11(t *testing.T) {
	if u, err := StartURL(webApp("browser", "")); err != nil || u != BlankPage {
		t.Fatalf("browser without a start page: %q %v", u, err)
	}
	if u, err := StartURL(webApp("netflix", "https://www.netflix.com/browse")); err != nil || u != "https://www.netflix.com/browse" {
		t.Fatalf("%q %v", u, err)
	}
	for _, bad := range []config.Application{
		webApp("netflix", ""),
		webApp("netflix", "http://www.netflix.com/"),
		webApp("netflix", "https://evil.example/"),
		webApp("hulu", "https://a:b@www.hulu.com/"),
		webApp("browser", "javascript:alert(1)"),
		webApp("browser", "file:///etc/passwd"),
		webApp("browser", "https://x.example/ --kiosk"),
		webApp("plex-htpc", "https://www.netflix.com/"),
	} {
		if u, err := StartURL(bad); err == nil {
			t.Errorf("%s %q accepted as %q", bad.Adapter, bad.WebURL(), u)
		}
	}
}

func TestEmbeddedScriptAndHints(t *testing.T) {
	nav, err := NavScript()
	if err != nil || !strings.Contains(string(nav), "__bdtvNav") {
		t.Fatalf("nav.js: %v", err)
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu"} {
		h, err := Hints(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !strings.Contains(string(h), `"verified":false`) {
			t.Errorf("%s hints are not marked unverified", id)
		}
		src, err := Source(id)
		i := strings.LastIndex(src, "\n;__bdtvNav.install(")
		if err != nil || i < 0 || !strings.HasSuffix(src, ");\n") {
			t.Fatalf("%s: source does not end with its install call: %v", id, err)
		}
		var got, want any
		if json.Unmarshal([]byte(src[i+len("\n;__bdtvNav.install("):len(src)-3]), &got) != nil || json.Unmarshal(h, &want) != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the install call does not carry the hints", id)
		}
	}
	if src, err := Source(""); err != nil || !strings.HasSuffix(src, ";__bdtvNav.install(null);\n") {
		t.Errorf("generic source: %v", err)
	}
	for _, bad := range []string{"../x", "Netflix", "missing"} {
		if _, err := Hints(bad); err == nil {
			t.Errorf("hints %q accepted", bad)
		}
	}
}

func TestScriptCallsAreFixedExpressions(t *testing.T) {
	if got := scriptCall("nav.left", nil); got != `__bdtv.apply("nav.left")` {
		t.Fatal(got)
	}
	n := -30
	if got := scriptCall("media.seek_relative", &n); got != `__bdtv.apply("media.seek_relative",-30)` {
		t.Fatal(got)
	}
	// Even a hostile name stays one quoted string literal.
	if got := scriptCall(`x");alert(1);("`, nil); !strings.HasPrefix(got, `__bdtv.apply("x\");alert(1);(\"")`) {
		t.Fatal(got)
	}
}

func TestCheckEffectIsAClosedSet(t *testing.T) {
	vp := Viewport{W: 1280, H: 720}
	ok := []struct {
		action string
		e      Effect
	}{
		{"select", Effect{Kind: "click", X: 10, Y: 10}},
		{"back", Effect{Kind: "click", X: 1279, Y: 719}},
		{"back", Effect{Kind: "keys", Keys: []string{"Escape"}}},
		{"media.pause", Effect{Kind: "keys", Keys: []string{" "}}},
		{"media.seek_relative", Effect{Kind: "keys", Keys: []string{"ArrowRight", "ArrowRight"}}},
		{"text.submit", Effect{Kind: "text"}},
	}
	for _, c := range ok {
		if err := checkEffect(c.action, &c.e, vp); err != nil {
			t.Errorf("%s %+v refused: %v", c.action, c.e, err)
		}
	}
	thirtyOne := make([]string, MaxKeys+1)
	for i := range thirtyOne {
		thirtyOne[i] = "ArrowRight"
	}
	bad := []struct {
		action string
		e      Effect
	}{
		{"select", Effect{Kind: "click", X: 1280, Y: 10}},
		{"select", Effect{Kind: "click", X: -1, Y: 10}},
		{"select", Effect{Kind: "click", X: 10, Y: 720}},
		{"nav.left", Effect{Kind: "click", X: 10, Y: 10}},
		{"select", Effect{Kind: "keys", Keys: []string{"Enter"}}},
		{"back", Effect{Kind: "keys", Keys: []string{"ArrowLeft"}}},
		{"media.pause", Effect{Kind: "keys", Keys: []string{"Delete"}}},
		{"media.pause", Effect{Kind: "keys", Keys: []string{"F12"}}},
		{"media.pause", Effect{Kind: "keys", Keys: nil}},
		{"media.seek_relative", Effect{Kind: "keys", Keys: thirtyOne}},
		{"media.pause", Effect{Kind: "text"}},
		{"select", Effect{Kind: "navigate"}},
	}
	for _, c := range bad {
		if err := checkEffect(c.action, &c.e, vp); err == nil {
			t.Errorf("%s %+v accepted", c.action, c.e)
		}
	}
}

// Brave: the same browser switches (Brave's help center lists each one;
// --remote-debugging-pipe reads fd 3 and writes fd 4 as in Chromium) on
// Brave's own profile, and `flatpak run` grants the Brave Flatpak its
// profile root and nothing else for this run: its finish-args give it
// neither home nor xdg-data.
func TestBraveArgvExact(t *testing.T) {
	brave := browser(t, "brave")
	profile, err := ProfileDir("/d", brave, "browser")
	if err != nil {
		t.Fatal(err)
	}
	got, err := FlatpakArgv("/d", brave, ChromiumArgs(spec(t, "browser"), profile, BlankPage))
	want := []string{
		"flatpak", "run", "--filesystem=/d/bear-den-tv/web-brave", "com.brave.Browser",
		"--user-data-dir=/d/bear-den-tv/web-brave/browser",
		"--remote-debugging-pipe",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=BearDenWeb-browser",
		"--start-maximized",
		"about:blank",
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("brave argv\n got %q\nwant %q (%v)", got, want, err)
	}
	stream, _ := ProfileDir("/d", brave, "netflix")
	got, _ = FlatpakArgv("/d", brave, ChromiumArgs(spec(t, "netflix"), stream, "https://www.netflix.com/"))
	want = []string{
		"flatpak", "run", "--filesystem=/d/bear-den-tv/web-brave", "com.brave.Browser",
		"--user-data-dir=/d/bear-den-tv/web-brave/netflix",
		"--remote-debugging-pipe",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=BearDenWeb-netflix",
		"--start-fullscreen",
		"--app=https://www.netflix.com/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("brave app argv\n got %q\nwant %q", got, want)
	}
	// Only table browsers run, with the table's own settings.
	if _, err := FlatpakArgv("/d", adapters.BrowserInfo{Name: "evil", FlatpakID: "org.example.Evil"}, nil); err == nil {
		t.Error("a browser outside the table got an argv")
	}
	if _, err := FlatpakArgv("/d", adapters.BrowserInfo{Name: "chromium", FlatpakID: "com.brave.Browser"}, nil); err == nil {
		t.Error("a mislabelled browser got an argv")
	}
	if _, err := FlatpakArgv("/d:rw", brave, nil); err == nil {
		t.Error("a data dir with ':' reached --filesystem")
	}
}
