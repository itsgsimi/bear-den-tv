//go:build e2e

// Package e2e drives a real, running Bear Den coordinator on the TV machine the
// way a phone does (HTTP + WebSocket against the loopback test listener) and
// checks the real X11 desktop underneath. It launches the installed clients,
// so it is opt-in (build tag `e2e`) and never runs in `go test ./...`.
//
// Run on the target, with the session unlocked:
//
//	scripts/start-session.sh --watch --dev-listen 127.0.0.1:18791
//	DISPLAY=:0 go test -tags e2e -count=1 -v ./tests/e2e/
//
// Environment (defaults in parentheses):
//
//	BDTV_E2E_URL     phone endpoint (http://127.0.0.1:18791)
//	BDTV_E2E_SOCKET  coordinator socket ($XDG_RUNTIME_DIR/bear-den-tv/shell.sock)
//	BDTV_E2E_BIN     bear-den-tv binary, used for `pair` (../../build/bin/bear-den-tv)
//	BDTV_E2E_APPS    application ids to exercise (plex-htpc,youtube,moonlight)
//	BDTV_E2E_SHOTS   screenshot directory (../../.tmp/e2e-shots)
//
// Only navigation and Back reach the clients (no Select), so the test never
// starts playback, buys anything, or changes client settings.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/x11"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type phone struct {
	t      *testing.T
	base   *url.URL
	client *http.Client
	csrf   string
	cookie string
}

func uuid4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (p *phone) do(method, path string, body any, out any) int {
	p.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, p.base.String()+path, rd)
	req.Host = p.base.Host
	req.Header.Set("Origin", p.base.Scheme+"://"+p.base.Host)
	req.Header.Set("Content-Type", "application/json")
	if p.csrf != "" {
		req.Header.Set("X-BDTV-CSRF", p.csrf)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			p.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	for _, c := range resp.Cookies() {
		if c.Name == "bdtv_session" {
			p.cookie = c.Value
		}
	}
	return resp.StatusCode
}

func pairPhone(t *testing.T) *phone {
	t.Helper()
	base, err := url.Parse(env("BDTV_E2E_URL", "http://127.0.0.1:18791"))
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	p := &phone{t: t, base: base, client: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
	sock := env("BDTV_E2E_SOCKET", filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "bear-den-tv", "shell.sock"))
	out, err := exec.Command(env("BDTV_E2E_BIN", "../../build/bin/bear-den-tv"), "pair", "--socket", sock).CombinedOutput()
	if err != nil {
		t.Fatalf("issuing an invitation: %v: %s", err, out)
	}
	code := regexp.MustCompile(`code: (\d{6})`).FindStringSubmatch(string(out))
	if code == nil {
		t.Fatalf("no pairing code in %q", out)
	}
	var claim struct {
		CSRF     string `json:"csrf_token"`
		DeviceID string `json:"device_id"`
	}
	if st := p.do("POST", "/api/v1/pair/claim", map[string]any{"invitation": nil, "code": code[1], "device_name": "e2e test phone"}, &claim); st != 200 {
		t.Fatalf("claim status %d", st)
	}
	p.csrf = claim.CSRF
	// Unpair this test phone when the run ends (a device may revoke itself),
	// so test runs do not pile up in the paired-phones list.
	t.Cleanup(func() { p.do("DELETE", "/api/v1/devices/"+claim.DeviceID, nil, nil) })
	return p
}

// with returns the same paired phone reporting failures to t (a subtest).
func (p *phone) with(t *testing.T) *phone {
	c := *p
	c.t = t
	return &c
}

func (p *phone) state() contract.State {
	p.t.Helper()
	var st contract.State
	if code := p.do("GET", "/api/v1/state", nil, &st); code != 200 {
		p.t.Fatalf("state status %d", code)
	}
	return st
}

func (p *phone) act(action string, args map[string]any) contract.ActionResult {
	p.t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	req := contract.ActionRequest{Protocol: 1, RequestID: uuid4(), ContextEpoch: p.state().ContextEpoch, Target: "active", Action: action, Args: args}
	var res contract.ActionResult
	if code := p.do("POST", "/api/v1/actions", req, &res); code != 200 {
		p.t.Fatalf("%s: status %d", action, code)
	}
	p.t.Logf("%-18s → %s/%s %s %v", action, res.Outcome, res.Code, res.Message, res.Detail)
	return res
}

func (p *phone) waitFor(what string, timeout time.Duration, pred func(contract.State) bool) contract.State {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		st := p.state()
		if pred(st) {
			return st
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out after %s waiting for %s (target %+v, shell %+v)", timeout, what, st.Target, st.Shell)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func appState(st contract.State, id string) contract.AppState {
	for _, a := range st.Applications {
		if a.ID == id {
			return a
		}
	}
	return contract.AppState{}
}

func isApp(id string) func(contract.State) bool {
	return func(st contract.State) bool {
		return st.Target.Kind == "app" && st.Target.AppID != nil && *st.Target.AppID == id
	}
}

func isShell(st contract.State) bool { return st.Target.Kind == "shell" }

// screenshot saves the X root window as PNG evidence.
func screenshot(t *testing.T, name string) {
	t.Helper()
	dir := env("BDTV_E2E_SHOTS", "../../.tmp/e2e-shots")
	_ = os.MkdirAll(dir, 0o755)
	conn, err := xgb.NewConnDisplay(os.Getenv("DISPLAY"))
	if err != nil {
		t.Logf("screenshot %s skipped: %v", name, err)
		return
	}
	defer conn.Close()
	scr := xproto.Setup(conn).DefaultScreen(conn)
	w, h := int(scr.WidthInPixels), int(scr.HeightInPixels)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fetch in bands to stay well under the maximum request/reply size.
	const band = 120
	for y := 0; y < h; y += band {
		bh := min(band, h-y)
		reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(scr.Root), 0, int16(y), uint16(w), uint16(bh), 0xffffffff).Reply()
		if err != nil {
			t.Logf("screenshot %s failed: %v", name, err)
			return
		}
		d := reply.Data
		for yy := 0; yy < bh; yy++ {
			for x := 0; x < w; x++ {
				i := (yy*w + x) * 4
				if i+2 >= len(d) {
					break
				}
				img.SetRGBA(x, y+yy, color.RGBA{R: d[i+2], G: d[i+1], B: d[i], A: 255})
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Logf("screenshot %s: %v", name, err)
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
	t.Logf("screenshot: %s", f.Name())
}

func desktop(t *testing.T) *x11.Adapter {
	t.Helper()
	a, err := x11.New(context.Background(), x11.Options{Display: os.Getenv("DISPLAY")})
	if err != nil {
		t.Fatalf("X11 (DISPLAY=%q): %v", os.Getenv("DISPLAY"), err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestTargetEndToEnd(t *testing.T) {
	p := pairPhone(t)
	x := desktop(t)
	st := p.state()
	if st.Session.Locked {
		t.Fatal("the TV session is locked: unlock it at the TV and rerun (a locked run is not a pass)")
	}
	if !st.Session.ShellConnected {
		t.Fatalf("shell not connected: %+v", st.Session)
	}
	// Start from Bear Den home.
	if r := p.act("home", nil); r.Outcome == contract.OutcomeFailed {
		t.Fatalf("home failed: %s", r.Message)
	}
	p.waitFor("Bear Den in front", 10*time.Second, isShell)
	screenshot(t, "00-home")

	t.Run("NavigationMovesExactlyOneStep", func(t *testing.T) { navigationOneStep(t, p.with(t)) })

	for _, id := range strings.Split(env("BDTV_E2E_APPS", "plex-htpc,youtube,moonlight"), ",") {
		id := strings.TrimSpace(id)
		t.Run("App/"+id, func(t *testing.T) { appLoop(t, p.with(t), x, id) })
	}
}

// navigationOneStep is the regression test for "Left/Right skips twice": a
// short press (tap + hold.start/stop within the repeat delay) moves one item.
func navigationOneStep(t *testing.T, p *phone) {
	// Walk to the first app tile of the favorites row.
	for i := 0; i < 4; i++ {
		p.act("nav.up", nil)
	}
	p.act("nav.down", nil)
	for i := 0; i < 4; i++ {
		p.act("nav.left", nil)
	}
	st := p.state()
	if st.Shell.Focus.SectionID == nil || *st.Shell.Focus.SectionID != "favorites" {
		t.Fatalf("expected focus in favorites, got %+v", st.Shell.Focus)
	}
	first := *st.Shell.Focus.ItemID

	// A real phone press: action tap plus a hold lease released after ~120 ms.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wsURL := "ws://" + p.base.Host + "/api/v1/events"
	hdr := http.Header{}
	hdr.Set("Origin", p.base.Scheme+"://"+p.base.Host)
	hdr.Set("Cookie", "bdtv_session="+p.cookie)
	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr, Host: p.base.Host})
	if err != nil {
		t.Fatalf("websocket: %v", err)
	}
	defer ws.CloseNow()
	go func() { // drain server events
		for {
			if _, _, err := ws.Read(ctx); err != nil {
				return
			}
		}
	}()
	holdID := uuid4()
	epoch := p.state().ContextEpoch
	start, _ := json.Marshal(map[string]any{"type": "hold.start", "hold_id": holdID, "action": "nav.right", "context_epoch": epoch})
	if err := ws.Write(ctx, websocket.MessageText, start); err != nil {
		t.Fatal(err)
	}
	p.act("nav.right", nil)
	time.Sleep(120 * time.Millisecond)
	stop, _ := json.Marshal(map[string]any{"type": "hold.stop", "hold_id": holdID})
	_ = ws.Write(ctx, websocket.MessageText, stop)
	time.Sleep(800 * time.Millisecond) // well past the repeat delay

	st = p.state()
	after := *st.Shell.Focus.ItemID
	// Expected: exactly the next tile. Derive it from the configured order.
	order := []string{}
	if st.Layout != nil {
		for _, s := range st.Layout.Sections {
			if s.ID == "favorites" {
				order = s.ApplicationIDs
			}
		}
	} else {
		// Controllers do not see the layout; the default favorites row lists
		// the applications in configuration order.
		for _, a := range st.Applications {
			order = append(order, a.ID)
		}
	}
	idx := -1
	for i, id := range order {
		if id == first {
			idx = i
		}
	}
	if idx < 0 || idx+1 >= len(order) {
		t.Logf("layout not visible to this device; checking only that focus moved (%s → %s)", first, after)
		if after == first {
			t.Fatal("focus did not move")
		}
		return
	}
	if after != order[idx+1] {
		t.Fatalf("short press moved %s → %s, want exactly one step to %s", first, after, order[idx+1])
	}
}

func appLoop(t *testing.T, p *phone, x *x11.Adapter, id string) {
	before := p.state()
	a := appState(before, id)
	if !a.Installed {
		t.Fatalf("%s is not installed (installation %q)", id, a.Installation)
	}
	focusBefore := before.Shell.Focus

	// Launch (or activate) and wait for the app's window to be verified in front.
	if r := p.act("app.launch", map[string]any{"app_id": id}); r.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch failed: %s", r.Message)
	}
	p.waitFor(id+" in front", 90*time.Second, isApp(id))

	// Fullscreen: the coordinator asks the window manager; verify on X11.
	var fg struct {
		class []string
		full  bool
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		f, err := x.ObserveForeground(context.Background())
		if err == nil && f.Known {
			fg.class, fg.full = f.Window.Class, f.Window.Fullscreen
			if fg.full {
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Logf("%s window class %v fullscreen=%v", id, fg.class, fg.full)
	if !fg.full {
		t.Errorf("%s window is not fullscreen", id)
	}
	time.Sleep(4 * time.Second) // let the client finish drawing
	screenshot(t, id+"-1-launched")

	// Keys reach the verified foreground window.
	for _, action := range []string{"nav.down", "nav.up", "nav.right", "nav.left"} {
		if r := p.act(action, nil); r.Outcome != contract.OutcomeDelivered {
			t.Errorf("%s in %s = %s/%s %s", action, id, r.Outcome, r.Code, r.Message)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if st := p.state(); !appState(st, id).Running {
		t.Errorf("%s not reported running while in front", id)
	}

	// Home returns to Bear Den with the same focus.
	if r := p.act("home", nil); r.Outcome == contract.OutcomeFailed {
		t.Fatalf("home failed: %s", r.Message)
	}
	st := p.waitFor("Bear Den back in front", 15*time.Second, isShell)
	if st.Shell.Screen != "home" {
		t.Errorf("after Home the shell shows %q, want home", st.Shell.Screen)
	}
	if deref(st.Shell.Focus.SectionID) != deref(focusBefore.SectionID) || deref(st.Shell.Focus.ItemID) != deref(focusBefore.ItemID) {
		t.Errorf("focus after Home %s/%s, want %s/%s", deref(st.Shell.Focus.SectionID), deref(st.Shell.Focus.ItemID), deref(focusBefore.SectionID), deref(focusBefore.ItemID))
	}
	if !appState(st, id).Running {
		t.Errorf("%s should still be running in the background after Home", id)
	}
	time.Sleep(1500 * time.Millisecond)
	screenshot(t, id+"-2-home")

	// Launch again: activates the existing window instead of a second instance.
	if r := p.act("app.launch", map[string]any{"app_id": id}); r.Outcome == contract.OutcomeFailed {
		t.Fatalf("re-activate failed: %s", r.Message)
	}
	p.waitFor(id+" re-activated", 20*time.Second, isApp(id))

	// Close: the app exits, stops showing as running, and Bear Den comes back.
	if r := p.act("app.close", map[string]any{"app_id": id, "force": false}); r.Outcome == contract.OutcomeFailed {
		t.Fatalf("close failed: %s", r.Message)
	}
	st = p.waitFor(id+" no longer running", 30*time.Second, func(s contract.State) bool { return !appState(s, id).Running })
	if got := appState(st, id).LaunchState; got != "exited" {
		t.Errorf("launch_state after close = %q, want exited", got)
	}
	p.waitFor("Bear Den in front after the app closed", 15*time.Second, isShell)
	time.Sleep(1500 * time.Millisecond)
	screenshot(t, id+"-3-closed")
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
