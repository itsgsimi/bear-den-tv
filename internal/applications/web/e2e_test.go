// End-to-end test of the web app manager against a real Chromium: the
// toolchain's Playwright Chromium (never Flatpak, never a real streaming
// site), started through the same ExecStarter and DevTools pipe as on the
// TV, on the local fixture pages of apps/web-nav/tests/fixtures served over
// a loopback TLS server. It drives the page with the coordinator's named
// actions and checks what landed where. Skips, saying why, when no
// Playwright Chromium is installed (`npx playwright install chromium` in
// apps/web-nav, or BDTV_TEST_CHROMIUM=/path/to/chrome).

package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

// FindTestChromium finds a Chromium for tests: BDTV_TEST_CHROMIUM, else the
// newest Playwright download (internal/session/web_e2e_test.go has its twin).
func FindTestChromium() string {
	if p := os.Getenv("BDTV_TEST_CHROMIUM"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".cache", "ms-playwright", "chromium-*", "chrome-linux*", "chrome"))
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

// fixtureServer serves the navigation script's fixture pages over TLS.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir, err := filepath.Abs("../../../apps/web-nav/tests/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.FileServer(http.Dir(dir)))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // Chromium probes and drops TLS connections
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// testStarter is a headless Playwright Chromium that resolves fixtures.test
// to the fixture server (test-only flags; the TV never gets them).
func testStarter(t *testing.T, srv *httptest.Server) ExecStarter {
	t.Helper()
	bin := FindTestChromium()
	if bin == "" {
		t.Skip("no Playwright Chromium: run `npx playwright install chromium` in apps/web-nav or set BDTV_TEST_CHROMIUM (not a pass)")
	}
	host := strings.TrimPrefix(srv.URL, "https://")
	return ExecStarter{Env: os.Environ(), Prefix: []string{bin,
		"--headless=new", "--no-sandbox", "--ignore-certificate-errors", "--window-size=1280,720",
		"--host-resolver-rules=MAP fixtures.test " + host,
	}}
}

type e2e struct {
	t     *testing.T
	m     *Manager
	appID string
	ctx   context.Context
}

// mainWorld evaluates in the page's own world (what the site sees).
func (e *e2e) mainWorld(expr string) json.RawMessage {
	e.t.Helper()
	b := e.m.get(e.appID)
	pg, err := b.currentPage()
	if err != nil {
		e.t.Fatal(err)
	}
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := b.conn.Call(e.ctx, pg.session, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true}, &res); err != nil {
		e.t.Fatal(err)
	}
	return res.Result.Value
}

func (e *e2e) str(expr string) string {
	var s string
	_ = json.Unmarshal(e.mainWorld(expr), &s)
	return s
}

func (e *e2e) waitFor(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %s", what)
}

func (e *e2e) apply(action string, args map[string]any) Outcome {
	e.t.Helper()
	out, err := e.m.Apply(e.ctx, e.appID, action, args)
	if err != nil {
		e.t.Fatalf("%s: %v", action, err)
	}
	return out
}

func (e *e2e) open(page string) {
	e.t.Helper()
	b := e.m.get(e.appID)
	pg, _ := b.currentPage()
	if err := b.conn.Call(e.ctx, pg.session, "Page.navigate", map[string]any{"url": "https://fixtures.test/" + page}, nil); err != nil {
		e.t.Fatal(err)
	}
	e.waitFor(page+" with the script", func() bool {
		return strings.HasSuffix(e.str("location.pathname"), page) && e.str("document.readyState") == "complete" && func() bool {
			var st Status
			return b.evaluate(e.ctx, pg, "__bdtv.status()", &st) == nil
		}()
	})
}

func TestE2EChromiumDrivenByNamedActions(t *testing.T) {
	srv := fixtureServer(t)
	starter := testStarter(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Not t.TempDir: Chromium may still be flushing its profile when the
	// test ends; the directory is removed best effort.
	data, err := os.MkdirTemp("", "bdtv-web-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { time.Sleep(200 * time.Millisecond); _ = os.RemoveAll(data) })
	changes := make(chan struct{}, 64)
	m := NewManager(Options{DataHome: data, Starter: starter})
	m.Watch(func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	app := config.Application{ID: "browser", Label: "Browser", Adapter: "browser", Launch: config.Launch{Kind: "flatpak", AppID: "org.chromium.Chromium"}, Web: &config.Web{URL: "https://fixtures.test/grid.html"}}
	ad, _ := adapters.ForName("browser")
	spec, _ := adapters.WebOf(ad)
	inst, err := m.Launch(ctx, app, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close(context.Background(), "browser", true) }()
	if inst.PID <= 0 || !m.Running("browser") {
		t.Fatalf("instance %+v running %v", inst, m.Running("browser"))
	}
	if fi, err := os.Stat(filepath.Join(data, "bear-den-tv", "web", "browser")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("profile dir: %v %v", fi, err)
	}
	e := &e2e{t: t, m: m, appID: "browser", ctx: ctx}
	e.waitFor("the grid page with the script", func() bool {
		st, ok := m.Status("browser")
		return ok && st.Viewport.W > 0 && strings.HasSuffix(e.str("location.pathname"), "grid.html") && e.str("document.readyState") == "complete"
	})

	// The page's own scripts see neither the script nor its binding.
	if got := e.str(`typeof __bdtv + "/" + typeof __bdtvReport + "/" + typeof __bdtvNav`); got != "undefined/undefined/undefined" {
		t.Fatalf("the main world sees %s", got)
	}

	// D-pad: the focus the page reports is observed.
	out := e.apply("nav.right", nil)
	if !out.Observed || out.Detail["page"] != "moved" {
		t.Fatalf("nav.right: %+v", out)
	}
	e.apply("nav.down", nil)
	e.apply("nav.right", nil)
	if id := e.str(`document.querySelector('[data-bdtv-focused]').id`); id != "r1c2" {
		t.Fatalf("focus on %q", id)
	}
	// OK clicks with a trusted mouse event; delivered, not observed.
	out = e.apply("select", nil)
	if out.Observed || out.Detail["page"] != "click" {
		t.Fatalf("select: %+v", out)
	}
	e.waitFor("the overlay", func() bool { return e.str(`document.body.dataset.clicked || ""`) == "r1c2" })
	// Back closes the overlay through its Close button.
	if out = e.apply("back", nil); out.Detail["page"] != "closing_overlay" {
		t.Fatalf("back: %+v", out)
	}
	e.waitFor("the overlay to close", func() bool { return e.str(`document.body.dataset.clicked || ""`) == "close" })
	if out = e.apply("back", nil); out.Detail["page"] != "at_root" || !out.Observed {
		t.Fatalf("back at root: %+v", out)
	}
	// No video here: media is refused with the page's reason.
	if _, err := m.Apply(ctx, "browser", "media.pause", nil); err == nil || !strings.Contains(err.Error(), "No video") {
		t.Fatalf("media.pause on a grid: %v", err)
	}

	// Touchpad: move onto a card, click it, scroll the page.
	var r struct{ X, Y float64 }
	_ = json.Unmarshal(e.mainWorld(`(() => { const b = document.getElementById('r2c1').getBoundingClientRect(); return {X: b.left + b.width/2, Y: b.top + b.height/2}; })()`), &r)
	vw, vh := 1280.0, 720.0
	if st, ok := m.Status("browser"); ok {
		vw, vh = st.Viewport.W, st.Viewport.H
	}
	dx, dy := int(r.X-vw/2), int(r.Y-vh/2)
	for dx != 0 || dy != 0 {
		sx, sy := clampInt(dx, 400), clampInt(dy, 400)
		if err := m.Pointer(ctx, "browser", "pointer.move", map[string]any{"dx": float64(sx), "dy": float64(sy)}); err != nil {
			t.Fatal(err)
		}
		dx, dy = dx-sx, dy-sy
	}
	if err := m.Pointer(ctx, "browser", "pointer.click", map[string]any{"button": "left"}); err != nil {
		t.Fatal(err)
	}
	e.waitFor("the touchpad click", func() bool { return e.str(`document.body.dataset.clicked || ""`) == "r2c1" })
	e.apply("back", nil) // close the overlay again
	if err := m.Pointer(ctx, "browser", "pointer.scroll", map[string]any{"dy": float64(300)}); err != nil {
		t.Fatal(err)
	}
	e.waitFor("the page to scroll", func() bool { return e.str(`String(window.scrollY > 0)`) == "true" })
	// A move far off the page stays inside the viewport.
	for i := 0; i < 10; i++ {
		_ = m.Pointer(ctx, "browser", "pointer.move", map[string]any{"dx": float64(400), "dy": float64(400)})
	}
	b := m.get("browser")
	pg, _ := b.currentPage()
	lv, err := b.viewport(ctx, pg) // Chromium's own size (without the scrollbar)
	if err != nil {
		t.Fatal(err)
	}
	vw, vh = lv.W, lv.H
	b.mu.Lock()
	cx, cy := b.cx, b.cy
	b.mu.Unlock()
	if cx > vw || cy > vh || cx < vw-2 || cy < vh-2 {
		t.Fatalf("cursor left the viewport: %v,%v in %vx%v", cx, cy, vw, vh)
	}

	// Player: pause and seek through the site's own keys, never currentTime.
	e.open("player.html")
	e.waitFor("the video to play", func() bool { st, _ := m.Status("browser"); return st.Video == "playing" })
	out = e.apply("media.pause", nil)
	if !out.Observed || out.Detail["video"] != "paused" {
		t.Fatalf("media.pause: %+v", out)
	}
	if e.str(`document.body.dataset.lastKeyTrusted`) != "true" {
		t.Fatal("the pause key was not trusted input")
	}
	e.apply("media.seek_relative", map[string]any{"seconds": float64(30)})
	if got := e.str(`document.body.dataset.seeks`); got != "30" {
		t.Fatalf("seeks %q", got)
	}
	if e.str(`document.body.dataset.seekError || ""`) != "" {
		t.Fatal("currentTime was set directly")
	}
	e.apply("media.play", nil)
	paused, err := m.PauseIfPlaying(ctx, "browser")
	if err != nil || !paused {
		t.Fatalf("PauseIfPlaying: %v %v", paused, err)
	}
	if paused, _ = m.PauseIfPlaying(ctx, "browser"); paused {
		t.Fatal("pressed the pause key on a paused video")
	}

	// Search: text into the focused field, then Enter.
	e.open("search.html")
	e.apply("nav.down", nil)
	e.apply("select", nil)
	e.waitFor("the text field report", func() bool { st, _ := m.Status("browser"); return st.TextField })
	e.apply("text.submit", map[string]any{"text": "gam"})
	e.waitFor("the search to submit", func() bool { return e.str(`document.body.dataset.submitted || ""`) == "gam" })

	// Close ends Chromium and the connection.
	if err := m.Close(ctx, "browser", false); err != nil {
		t.Fatal(err)
	}
	e.waitFor("Chromium to exit", func() bool { return !m.Running("browser") })
	if _, err := m.Apply(ctx, "browser", "nav.up", nil); err != ErrNotRunning {
		t.Fatalf("after close: %v", err)
	}
}

func clampInt(v, lim int) int {
	if v > lim {
		return lim
	}
	if v < -lim {
		return -lim
	}
	return v
}
