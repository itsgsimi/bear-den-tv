// Tests for web apps in the coordinator (web.go): Apps → Streaming sites
// (IPC app.enable) and the hidden/enabled state, launching through the web
// manager, nav/media/text routed to the page, the touchpad's pointer actions
// (web adapters only, verified foreground, never guests, rate limits), and
// Home pausing a playing page. The web manager is a fake here; the real one
// is driven against Chromium in web_e2e_test.go and
// internal/applications/web/e2e_test.go.

package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

// fakeWeb stands in for web.Manager: launching maps a window with the
// adapter's class on the fake desktop; everything sent is recorded.
type fakeWeb struct {
	desk *fake.Desktop

	mu       sync.Mutex
	running  map[string]bool
	status   map[string]web.Status
	applied  []string
	pointers []string
	pauses   int
	launched []string // "<app id> <browser flatpak id>"
	watch    func()
	gate     chan struct{} // when set, Launch waits for it to close
	fail     error         // when set, Launch returns it (after the gate)
}

func newFakeWeb(desk *fake.Desktop) *fakeWeb {
	return &fakeWeb{desk: desk, running: map[string]bool{}, status: map[string]web.Status{}}
}

func (f *fakeWeb) Launch(_ context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error) {
	f.mu.Lock()
	gate, fail := f.gate, f.fail
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if fail != nil {
		return applications.Instance{}, fail
	}
	f.mu.Lock()
	f.launched = append(f.launched, app.ID+" "+app.Launch.AppID)
	f.running[app.ID] = true
	f.status[app.ID] = web.Status{V: 1, Visible: true, Video: "none", Viewport: web.Viewport{W: 1280, H: 720}}
	f.mu.Unlock()
	w := f.desk.AddWindow(platform.WindowInfo{PID: 7000, Class: []string{"www.example.com", spec.Class}})
	go func() {
		time.Sleep(20 * time.Millisecond)
		f.desk.SetActive(w)
	}()
	return applications.Instance{FlatpakID: app.Launch.AppID, PID: 7000}, nil
}

// PID is a distinct pretend browser process per running app (7000 + the
// app's launch order).
func (f *fakeWeb) PID(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.running[id] {
		return 0
	}
	for i, l := range f.launched {
		if strings.HasPrefix(l, id+" ") {
			return 7000 + i
		}
	}
	return 0
}

func (f *fakeWeb) Running(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running[id]
}

func (f *fakeWeb) Status(id string) (web.Status, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.status[id]
	return st, ok
}

func (f *fakeWeb) setStatus(id string, edit func(*web.Status)) {
	f.mu.Lock()
	st := f.status[id]
	edit(&st)
	f.status[id] = st
	fn := f.watch
	f.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (f *fakeWeb) Apply(_ context.Context, id, action string, _ map[string]any) (web.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, action)
	return web.Outcome{Observed: action != "select", Detail: map[string]any{"page": "moved"}}, nil
}

func (f *fakeWeb) Pointer(_ context.Context, id, action string, _ map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pointers = append(f.pointers, action)
	return nil
}

func (f *fakeWeb) PauseIfPlaying(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status[id].Video != "playing" {
		return false, nil
	}
	f.pauses++
	return true, nil
}

func (f *fakeWeb) Close(context.Context, string, bool) error { return nil }

func (f *fakeWeb) Watch(fn func()) {
	f.mu.Lock()
	f.watch = fn
	f.mu.Unlock()
}

func (f *fakeWeb) counts() (applied, pointers int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.applied), len(f.pointers)
}

// lyingDesk reports another window as the foreground when asked directly,
// while its watch channel still says the web app is in front: the moment
// between the capability check and the input, when focus moved.
type lyingDesk struct {
	*fake.Desktop
	mu  sync.Mutex
	lie bool
}

func (d *lyingDesk) ObserveForeground(ctx context.Context) (platform.Foreground, error) {
	d.mu.Lock()
	lie := d.lie
	d.mu.Unlock()
	if lie {
		return platform.Foreground{Known: true, Window: platform.WindowInfo{ID: 999999, Class: []string{"other"}}}, nil
	}
	return d.Desktop.ObserveForeground(ctx)
}

func webHarness(t *testing.T, configure ...func(*Options)) (*harness, *fakeWeb, *lyingDesk) {
	t.Helper()
	var fw *fakeWeb
	var ld *lyingDesk
	h := newHarness(t, append([]func(*Options){func(o *Options) {
		fd := o.Desktop.(*fake.Desktop)
		fw = newFakeWeb(fd)
		ld = &lyingDesk{Desktop: fd}
		o.Web = fw
		o.Desktop = ld
	}}, configure...)...)
	return h, fw, ld
}

// enable turns a web app on through the TV's IPC, as Apps → Streaming
// sites does, and returns the reply.
func enable(h *harness, appID string, on bool) shellipc.Result {
	h.t.Helper()
	id := "enable-" + appID + randomID()
	if err := h.shell.Send(shellipc.AppEnable{Type: shellipc.TypeAppEnable, RequestID: id, AppID: appID, Enabled: on}); err != nil {
		h.t.Fatal(err)
	}
	return h.shellResult(id)
}

func appState(st contract.State, id string) contract.AppState {
	for _, a := range st.Applications {
		if a.ID == id {
			return a
		}
	}
	return contract.AppState{}
}

func TestStreamingSitesAreOffUntilEnabled(t *testing.T) {
	h, _, _ := webHarness(t)
	h.eventually("discovery", func() bool { return appState(h.c.buildState(viewShell), "netflix").Installed })
	st := h.c.buildState(viewShell)
	nf := appState(st, "netflix")
	if nf.Enabled == nil || *nf.Enabled || !nf.Hidden {
		t.Fatalf("netflix by default: enabled=%v hidden=%v", nf.Enabled, nf.Hidden)
	}
	br := appState(st, "browser")
	if br.Enabled == nil || !*br.Enabled || br.Hidden {
		t.Fatalf("browser by default: enabled=%v hidden=%v", br.Enabled, br.Hidden)
	}
	if appState(st, "plex-htpc").Enabled != nil {
		t.Fatal("a Flatpak app reports enabled")
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}
	// Turned off, it cannot be launched either.
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "netflix"}))
	if res.Outcome != contract.OutcomeAccepted {
		t.Fatalf("launch: %+v", res)
	}
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatalf("app.enable: %+v", r)
	}
	nf = appState(h.c.buildState(viewShell), "netflix")
	if !*nf.Enabled || nf.Hidden {
		t.Fatalf("after enable: %+v", nf)
	}
	if !h.c.opts.Config.Current().Applications[6].IsEnabled() {
		t.Fatal("not stored in config.json")
	}
	if r := enable(h, "plex-htpc", false); r.OK || !strings.Contains(r.Error, "Only streaming sites") {
		t.Fatalf("a Flatpak app through app.enable: %+v", r)
	}
	if r := enable(h, "ghost", true); r.OK {
		t.Fatal("an unknown app was turned on")
	}
	if r := enable(h, "netflix", false); !r.OK || !appState(h.c.buildState(viewShell), "netflix").Hidden {
		t.Fatal("netflix could not be turned off again")
	}
}

func TestDisabledWebAppIsNotLaunched(t *testing.T) {
	h, fw, _ := webHarness(t)
	res := h.c.doLaunch(context.Background(), phoneSender(h.ctl), h.req(contract.ActionAppLaunch, map[string]any{"app_id": "hulu"}))
	if res.Code != contract.CodeUnsupported || fw.Running("hulu") {
		t.Fatalf("disabled hulu: %+v running=%v", res, fw.Running("hulu"))
	}
}

func launchWeb(h *harness, id string) {
	h.t.Helper()
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": id}))
	if res.Outcome != contract.OutcomeAccepted {
		h.t.Fatalf("launch %s: %+v", id, res)
	}
	h.eventually(id+" in front", func() bool {
		t := h.c.Target()
		return t.Kind == "app" && strOr(t.AppID) == id
	})
}

func TestWebAppInputAndPointerGoThroughThePage(t *testing.T) {
	h, fw, _ := webHarness(t)
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatal(r.Error)
	}
	launchWeb(h, "netflix")
	caps := h.c.buildState(viewShell).Capabilities
	for _, a := range []string{contract.ActionNavLeft, contract.ActionSelect, contract.ActionBack, contract.ActionPointerMove, contract.ActionPointerClick, contract.ActionPointerScroll} {
		if !caps[a].Available || caps[a].Backend != backendWeb {
			t.Errorf("%s: %+v", a, caps[a])
		}
	}
	if caps[contract.ActionPointerMove].Holdable || !caps[contract.ActionNavLeft].Holdable {
		t.Error("holdable flags wrong")
	}
	if caps[contract.ActionTextSubmit].Available || caps[contract.ActionTextSubmit].Reason != "No text field is focused." {
		t.Errorf("text.submit without a field: %+v", caps[contract.ActionTextSubmit])
	}
	if caps[contract.ActionMediaPause].Available || caps[contract.ActionMediaPause].Reason != "No video on this page." {
		t.Errorf("media without a video: %+v", caps[contract.ActionMediaPause])
	}
	res := h.submit(h.ctl, h.req(contract.ActionNavLeft, nil))
	if res.Outcome != contract.OutcomeObserved || res.Detail["page"] != "moved" {
		t.Fatalf("nav.left: %+v", res)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionSelect, nil)); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("select: %+v", res)
	}
	if keys := h.desk.Keys(); len(keys) != 0 {
		t.Fatalf("XTEST keys reached a web app: %+v", keys)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionPointerMove, map[string]any{"dx": 5.0, "dy": -3.0})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("pointer.move: %+v", res)
	}
	// The page reports a text field and a playing video: text and media open up.
	fw.setStatus("netflix", func(s *web.Status) { s.TextField, s.Video = true, "playing" })
	h.eventually("text and media capabilities", func() bool {
		c := h.c.buildState(viewShell).Capabilities
		return c[contract.ActionTextSubmit].Available && c[contract.ActionMediaPause].Available
	})
	if res = h.submit(h.ctl, h.req(contract.ActionTextSubmit, map[string]any{"text": "DEMO"})); res.Outcome == contract.OutcomeFailed {
		t.Fatalf("text.submit: %+v", res)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionMediaPause, nil)); res.Outcome == contract.OutcomeFailed {
		t.Fatalf("media.pause: %+v", res)
	}
	applied, pointers := fw.counts()
	if applied != 4 || pointers != 1 {
		t.Fatalf("applied %d pointer %d", applied, pointers)
	}
	// Home pauses the playing page first, then brings the shell forward.
	res = h.submit(h.ctl, h.req(contract.ActionHome, nil))
	if res.Outcome != contract.OutcomeAccepted {
		t.Fatalf("home: %+v", res)
	}
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	fw.mu.Lock()
	pauses := fw.pauses
	fw.mu.Unlock()
	if pauses != 1 {
		t.Fatalf("home paused %d times", pauses)
	}
}

func TestPointerOnlyForWebAppsAndNeverForGuests(t *testing.T) {
	h, fw, _ := webHarness(t)
	caps := h.c.buildState(viewShell).Capabilities
	if caps[contract.ActionPointerMove].Available || caps[contract.ActionPointerMove].Reason != "The touchpad works only in web apps." {
		t.Fatalf("pointer with the shell in front: %+v", caps[contract.ActionPointerMove])
	}
	res := h.submit(h.ctl, h.req(contract.ActionPointerClick, map[string]any{"button": "left"}))
	if res.Outcome != contract.OutcomeFailed {
		t.Fatalf("pointer.click on the shell: %+v", res)
	}
	// A Flatpak app in front: still no touchpad.
	res = h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "plex-htpc"}))
	if res.Outcome != contract.OutcomeAccepted {
		t.Fatal(res)
	}
	h.eventually("plex in front", func() bool { t := h.c.Target(); return t.Kind == "app" && strOr(t.AppID) == "plex-htpc" })
	if cp := h.c.buildState(viewShell).Capabilities[contract.ActionPointerMove]; cp.Available {
		t.Fatalf("pointer with Plex in front: %+v", cp)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionPointerMove, map[string]any{"dx": 1.0, "dy": 1.0})); res.Code != contract.CodeUnsupported {
		t.Fatalf("pointer.move on Plex: %+v", res)
	}
	// The browser in front: guests are still refused.
	launchWeb(h, "browser")
	guest := guestViewer(time.Now().Add(time.Hour))
	for _, a := range []string{contract.ActionPointerMove, contract.ActionPointerClick, contract.ActionPointerScroll} {
		if res = h.submit(guest, h.req(a, validArgs(a))); res.Code != contract.CodeForbidden {
			t.Errorf("guest %s: %+v", a, res)
		}
	}
	if _, pointers := fw.counts(); pointers != 0 {
		t.Fatalf("%d pointer actions reached the page", pointers)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionPointerScroll, map[string]any{"dy": 120.0})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("controller scroll: %+v", res)
	}
}

// activations records every window the fake desktop activated.
type activations struct {
	mu  sync.Mutex
	ids []platform.WindowID
}

func (a *activations) record(o *Options) {
	o.Desktop.(*lyingDesk).OnActivate = func(w platform.WindowID) {
		a.mu.Lock()
		a.ids = append(a.ids, w)
		a.mu.Unlock()
	}
}

func (a *activations) of(w platform.WindowID) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, id := range a.ids {
		if id == w {
			n++
		}
	}
	return n
}

// reconciled waits until the apps reconcile pass running now (or the last
// one) has finished: passes run one after another and each starts by asking
// the launcher for its instances.
func (h *harness) reconciled() {
	h.t.Helper()
	fl := h.c.opts.Launcher.(*fakeLauncher)
	n := fl.instanceCalls()
	h.eventually("the next reconcile pass", func() bool { return fl.instanceCalls() > n })
}

// switchFromPlex has Plex in front, then opens the browser with its launch
// held at the gate: Plex has been closed and its exit reconciled while the
// browser is still starting.
func switchFromPlex(t *testing.T, fail error) (*harness, *fakeWeb, *activations, chan struct{}) {
	t.Helper()
	acts := &activations{}
	h, fw, _ := webHarness(t, acts.record)
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "plex-htpc"}))
	if res.Outcome != contract.OutcomeAccepted {
		t.Fatal(res)
	}
	h.eventually("plex in front", func() bool { t := h.c.Target(); return t.Kind == "app" && strOr(t.AppID) == "plex-htpc" })
	gate := make(chan struct{})
	fw.mu.Lock()
	fw.gate, fw.fail = gate, fail
	fw.mu.Unlock()
	if res = h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "browser"})); res.Outcome != contract.OutcomeAccepted {
		t.Fatalf("launch browser: %+v", res)
	}
	h.eventually("plex exited", func() bool {
		return h.windowsOf("plexhtpc") == 0 && appState(h.phones.Snapshot(context.Background(), &h.ctl), "plex-htpc").LaunchState == "exited"
	})
	h.reconciled()
	return h, fw, acts, gate
}

// TestOpeningAnotherAppNeverRaisesTheShellInBetween: opening the browser
// closes Plex; Plex's exit used to bring the shell forward while the browser
// was starting, and on a busy machine that raise landed after the browser
// came to the front ("Browser lost focus before the pointer moved", or the
// browser never seen in front). The return to Home now waits for the launch.
func TestOpeningAnotherAppNeverRaisesTheShellInBetween(t *testing.T) {
	h, fw, acts, gate := switchFromPlex(t, nil)
	if n := acts.of(h.shellW); n != 0 {
		t.Fatalf("the shell was raised %d times while the browser was starting", n)
	}
	close(gate)
	h.eventually("browser in front", func() bool { t := h.c.Target(); return t.Kind == "app" && strOr(t.AppID) == "browser" })
	h.reconciled()
	if n := acts.of(h.shellW); n != 0 {
		t.Fatalf("the shell was raised %d times over the browser", n)
	}
	if res := h.submit(h.ctl, h.req(contract.ActionPointerScroll, map[string]any{"dy": 120.0})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("controller scroll: %+v", res)
	}
	if _, pointers := fw.counts(); pointers != 1 {
		t.Fatalf("%d pointer actions reached the page, want 1", pointers)
	}
}

// TestFailedLaunchAfterAnExitReturnsHome: the return to Home that waited for
// a launch still happens when that launch fails.
func TestFailedLaunchAfterAnExitReturnsHome(t *testing.T) {
	h, _, _, gate := switchFromPlex(t, errors.New("chromium did not start"))
	close(gate)
	h.eventually("shell back in front", func() bool { return h.c.Target().Kind == "shell" })
}

func TestWebInputNeedsTheVerifiedForeground(t *testing.T) {
	h, fw, ld := webHarness(t)
	launchWeb(h, "browser")
	ld.mu.Lock()
	ld.lie = true
	ld.mu.Unlock()
	for _, r := range []contract.ActionRequest{
		h.req(contract.ActionNavDown, nil),
		h.req(contract.ActionPointerMove, map[string]any{"dx": 3.0, "dy": 3.0}),
		h.req(contract.ActionPointerClick, map[string]any{"button": "left"}),
	} {
		if res := h.submit(h.ctl, r); res.Code != contract.CodeTargetUnfocused {
			t.Errorf("%s with focus elsewhere: %+v", r.Action, res)
		}
	}
	if applied, pointers := fw.counts(); applied != 0 || pointers != 0 {
		t.Fatalf("input reached the page: %d %d", applied, pointers)
	}
}

func TestPointerRateLimits(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	c := New(Options{Clock: fc})
	allowed := func(dev, action string, n int) int {
		ok := 0
		for i := 0; i < n; i++ {
			if c.pointerAllow(dev, action) {
				ok++
			}
		}
		return ok
	}
	if got := allowed("a", contract.ActionPointerMove, 100); got != 60 {
		t.Fatalf("moves in one instant: %d, want 60", got)
	}
	if got := allowed("b", contract.ActionPointerMove, 10); got != 10 {
		t.Fatalf("another phone was limited by the first: %d", got)
	}
	if got := allowed("a", contract.ActionPointerClick, 10); got != 5 {
		t.Fatalf("clicks: %d, want 5", got)
	}
	if got := allowed("a", contract.ActionPointerScroll, 50); got != 30 {
		t.Fatalf("scrolls: %d, want 30", got)
	}
	fc.Advance(500 * time.Millisecond)
	if got := allowed("a", contract.ActionPointerMove, 100); got != 30 {
		t.Fatalf("moves after half a second: %d, want 30", got)
	}
	if c.pointerAllow("a", contract.ActionNavUp) {
		t.Fatal("a non-pointer action has a pointer allowance")
	}
}

func TestPointerRateLimitedResult(t *testing.T) {
	h, fw, _ := webHarness(t)
	launchWeb(h, "browser")
	for h.c.pointerAllow(h.ctl.DeviceID, contract.ActionPointerClick) {
	}
	res := h.submit(h.ctl, h.req(contract.ActionPointerClick, map[string]any{"button": "left"}))
	if res.Code != contract.CodeRateLimited {
		t.Fatalf("over the limit: %+v", res)
	}
	if _, p := fw.counts(); p != 0 {
		t.Fatal("a rate-limited click reached the page")
	}
}
