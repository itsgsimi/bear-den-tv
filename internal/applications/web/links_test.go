package web

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

func TestCheckLinkAcceptsOnlyWebLinks(t *testing.T) {
	for _, ok := range []string{
		"https://accounts.spotify.com/en/login?continue=https%3A%2F%2Faccounts.spotify.com%2Foauth2",
		"http://example.org/",
	} {
		if err := CheckLink(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "file:///etc/passwd", "javascript:alert(1)", "spotify:track:1", "https://", "/relative",
		"https://example.org/\nSet-Cookie", "https://example.org/" + strings.Repeat("a", MaxLinkLength),
	} {
		if err := CheckLink(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://127.0.0.1:4381/login?code=x": true,
		"http://127.1.2.3/":                  true,
		"http://localhost:8080/cb":           true,
		"http://[::1]:5000/":                 true,
		"https://accounts.spotify.com/":      false,
		"http://192.168.1.50/":               false,
		"http://127.0.0.1.example.org/":      false,
		"ftp://127.0.0.1/":                   false,
	} {
		if got := IsLoopback(raw); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", raw, got, want)
		}
	}
}

// A sign-in in the Browser tile, as Spotify's goes: the sign-in page
// redirects to the app's callback on this machine (a loopback address,
// Spotify's http://127.0.0.1:4381/login), which redirects on to a final web
// page; the tab never rests on the loopback address. The hop is reported
// with its tab, the callback was really fetched once, and the tab closes.
func TestE2ESignInTabReportsTheLoopbackHandBack(t *testing.T) {
	srv := fixtureServer(t)
	var hits atomic.Int32
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" && r.URL.Query().Get("code") == "signed-in" {
			hits.Add(1)
		}
		http.Redirect(w, r, "https://signin.test/done", http.StatusFound)
	}))
	defer callback.Close()
	signin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authorize" {
			http.Redirect(w, r, callback.URL+"/login?code=signed-in", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("<!doctype html><title>Done</title>You are signed in."))
	}))
	signin.Config.ErrorLog = log.New(io.Discard, "", 0)
	signin.StartTLS()
	defer signin.Close()
	starter := testStarter(t, srv)
	for i, a := range starter.Prefix {
		if strings.HasPrefix(a, "--host-resolver-rules=") {
			starter.Prefix[i] = a + ", MAP signin.test " + strings.TrimPrefix(signin.URL, "https://")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	data, err := os.MkdirTemp("", "bdtv-web-links-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { time.Sleep(200 * time.Millisecond); _ = os.RemoveAll(data) })
	m := NewManager(Options{DataHome: data, Starter: starter})
	type handBack struct{ appID, targetID, url string }
	got := make(chan handBack, 4)
	m.OnLoopback(func(appID, targetID, url string) { got <- handBack{appID, targetID, url} })

	app := config.Application{ID: "browser", Label: "Browser", Adapter: "browser", Launch: config.Launch{Kind: "flatpak", AppID: "com.brave.Browser"}}
	ad, _ := adapters.ForName("browser")
	spec, _ := adapters.WebOf(ad)
	if _, err := m.LaunchAt(ctx, app, spec, "https://fixtures.test/grid.html"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(context.Background(), "browser", true) }()
	e := &e2e{t: t, m: m, appID: "browser", ctx: ctx}
	e.waitFor("the link LaunchAt was given", func() bool {
		return strings.HasSuffix(e.str("location.pathname"), "grid.html")
	})

	tab, err := m.OpenTab(ctx, "browser", "https://signin.test/authorize")
	if err != nil || tab == "" {
		t.Fatalf("OpenTab: %q %v", tab, err)
	}
	select {
	case hb := <-got:
		if hb.appID != "browser" || hb.targetID != tab || !strings.HasPrefix(hb.url, callback.URL+"/login") {
			t.Fatalf("hand-back %+v, want the tab %s at the callback", hb, tab)
		}
	case <-ctx.Done():
		t.Fatal("the loopback hand-back (a redirect hop) was never reported")
	}
	e.waitFor("the callback to be fetched", func() bool { return hits.Load() == 1 })
	if n := m.Tabs("browser"); n != 2 {
		t.Fatalf("%d tabs, want 2", n)
	}
	if err := m.CloseTab(ctx, "browser", tab); err != nil {
		t.Fatal(err)
	}
	e.waitFor("the sign-in tab to close", func() bool { return m.Tabs("browser") == 1 })
	if hits.Load() != 1 {
		t.Fatalf("the callback was fetched %d times", hits.Load())
	}
	if _, err := m.OpenTab(ctx, "browser", "file:///etc/passwd"); err == nil {
		t.Fatal("a file link was opened")
	}
}
