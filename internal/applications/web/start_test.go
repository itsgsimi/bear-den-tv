// Tests for the Browser tile's start page (start.go): written into the
// browser's own profile root only, never through a symbolic link, private;
// nothing on it loads from the network; the cards are the coordinator's,
// escaped; only a card's own fragment on Bear Den's own start page is an
// open request. End to end (Playwright's Chromium, as in e2e_test.go): the
// Browser without a start page opens it, the D-pad reaches a card and OK
// asks the coordinator to open that site, and the address box takes the
// phone's text and goes there.

package web

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

func TestStartPageIsLocalPrivateAndEscaped(t *testing.T) {
	data := t.TempDir()
	brave := browser(t, "brave")
	u, err := WriteStartPage(data, brave, []StartCard{
		{AppID: "netflix", Label: "Netflix"},
		{AppID: "hulu", Label: `<b>Hulu & "friends"</b>`},
		{AppID: "../evil", Label: "Evil"},
		{AppID: "disney-plus", Label: "  "},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "bear-den-tv", "web-brave", "start", "index.html")
	if u != "file://"+path {
		t.Fatalf("url %q", u)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("page %v %v", fi, err)
	}
	if di, _ := os.Stat(filepath.Dir(path)); di.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", di.Mode())
	}
	raw, _ := os.ReadFile(path)
	page := string(raw)
	for _, want := range []string{`href="#open-netflix"`, `href="#open-hulu"`, "&lt;b&gt;Hulu &amp; &#34;friends&#34;&lt;/b&gt;",
		"Use the phone's touchpad for anything the arrows can't reach.", `id="addr"`, `default-src 'none'`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"#open-../evil", "Evil", "#open-disney-plus", "<b>Hulu"} {
		if strings.Contains(page, bad) {
			t.Errorf("page carries %q", bad)
		}
	}
	// Nothing loads from anywhere: no src, no stylesheet link, no url() or
	// @import, no preconnect; links only to the page's own fragments.
	if m := regexp.MustCompile(`(?i:\ssrc=|<link|@import|<iframe|<img|<object|<embed)|[^A-Za-z]url\(`).FindString(page); m != "" {
		t.Errorf("the page loads something: %q", m)
	}
	for _, href := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(page, -1) {
		if !strings.HasPrefix(href[1], "#open-") {
			t.Errorf("a link out of the page: %q", href[1])
		}
	}
	// No cards: says where to turn the sites on.
	if _, err := WriteStartPage(data, brave, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "Apps → Streaming sites") || strings.Contains(string(raw), "#open-") {
		t.Fatalf("empty page %s", raw)
	}
	// A symbolic link in its place is refused, with nothing written through it.
	data2 := t.TempDir()
	elsewhere := t.TempDir()
	root := filepath.Join(data2, "bear-den-tv", "web-chrome")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, "start")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteStartPage(data2, browser(t, "chrome"), nil); err == nil {
		t.Fatal("a symbolic link was written through")
	}
	if left, _ := os.ReadDir(elsewhere); len(left) != 0 {
		t.Fatalf("written through the link: %v", left)
	}
}

func TestOpenRequestIsOnlyACardOfTheStartPage(t *testing.T) {
	start := "file:///d/bear-den-tv/web-brave/start/index.html"
	for nav, want := range map[string]string{
		start + "#open-netflix":              "netflix",
		start + "#open-disney-plus":          "disney-plus",
		start + "#open-":                     "",
		start + "#open-Netflix":              "",
		start + "#open-../x":                 "",
		start + "#top":                       "",
		start:                                "",
		"https://evil.example/#open-netflix": "",
		"file:///d/bear-den-tv/web-brave/start/other.html#open-netflix": "",
	} {
		got, ok := OpenRequest(start, nav)
		if got != want || ok != (want != "") {
			t.Errorf("%s: %q %v, want %q", nav, got, ok, want)
		}
	}
	if _, ok := OpenRequest("", "#open-netflix"); ok {
		t.Error("no start page, yet a request")
	}
}

func TestE2EStartPageOpensSitesAndGoesToAddresses(t *testing.T) {
	srv := fixtureServer(t)
	starter := testStarter(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	data, err := os.MkdirTemp("", "bdtv-web-start-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { time.Sleep(200 * time.Millisecond); _ = os.RemoveAll(data) })
	m := NewManager(Options{DataHome: data, Starter: starter})
	m.SetStartCards(func() []StartCard { return []StartCard{{AppID: "netflix", Label: "Netflix"}} })
	opened := make(chan [2]string, 4)
	m.OnOpen(func(from, appID string) { opened <- [2]string{from, appID} })
	app := config.Application{ID: "browser", Label: "Browser", Adapter: "browser", Launch: config.Launch{Kind: "flatpak", AppID: "com.brave.Browser"}}
	ad, _ := adapters.ForName("browser")
	spec, _ := adapters.WebOf(ad)
	if _, err := m.Launch(ctx, app, spec); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(context.Background(), "browser", true) }()
	e := &e2e{t: t, m: m, appID: "browser", ctx: ctx}
	e.waitFor("the start page with the script", func() bool {
		st, ok := m.Status("browser")
		return ok && st.Viewport.W > 0 && strings.HasSuffix(e.str("location.pathname"), "/web-brave/start/index.html") && e.str("document.readyState") == "complete"
	})
	if got := e.str(`location.protocol`); got != "file:" {
		t.Fatalf("protocol %q", got)
	}
	// The D-pad reaches the card; OK asks for Netflix, as its own app.
	for i := 0; i < 4; i++ {
		e.apply("nav.up", nil)
	}
	if got := e.str(`(document.querySelector('[data-bdtv-focused]') || {}).dataset ? document.querySelector('[data-bdtv-focused]').dataset.app || "" : ""`); got != "netflix" {
		t.Fatalf("focus on %q, not the Netflix card", got)
	}
	e.apply("select", nil)
	select {
	case got := <-opened:
		if got != [2]string{"browser", "netflix"} {
			t.Fatalf("open %v", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the card asked for nothing")
	}
	e.waitFor("back on the start page", func() bool { return e.str(`location.hash`) == "" })
	// The address box: the phone's text, then Enter, goes there.
	for i := 0; i < 6; i++ {
		e.apply("nav.down", nil)
		if st, _ := m.Status("browser"); st.Focus != nil && st.Focus.TextField {
			break
		}
	}
	e.apply("select", nil)
	e.waitFor("the text field report", func() bool { st, _ := m.Status("browser"); return st.TextField })
	e.apply("text.submit", map[string]any{"text": "fixtures.test/grid.html"})
	e.waitFor("the address to open", func() bool {
		return e.str(`location.href`) == "https://fixtures.test/grid.html"
	})
	select {
	case got := <-opened:
		t.Fatalf("an address opened an app: %v", got)
	default:
	}
}
