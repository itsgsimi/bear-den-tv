// Coordinator tests with fake lock, launcher and desktop (coordinator.go and
// friends).

package session

import (
	"bear-den-tv/internal/applications/tuning"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/storage"
)

type fakeLock struct {
	mu     sync.Mutex
	locked bool
	ch     chan bool
}

func (l *fakeLock) Locked(context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.locked, nil
}

func (l *fakeLock) Watch(context.Context) (<-chan bool, error) { return l.ch, nil }

func (l *fakeLock) set(v bool) {
	l.mu.Lock()
	l.locked = v
	l.mu.Unlock()
	l.ch <- v
}

// fakeLauncher maps a window for the launched app and activates it, like a
// well-behaved client would.
type fakeLauncher struct {
	desk      *fake.Desktop
	class     map[string]string
	launches  []string
	instances int // Instances calls: one per apps reconcile pass
	mu        sync.Mutex
}

func (f *fakeLauncher) instanceCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.instances
}

func (f *fakeLauncher) Discover(_ context.Context, id string) (applications.Installation, error) {
	return applications.Installation{Installed: true, Version: "1.0", Scope: "user"}, nil
}

func (f *fakeLauncher) Launch(_ context.Context, id string, _ []string) (applications.Instance, error) {
	f.mu.Lock()
	f.launches = append(f.launches, id)
	f.mu.Unlock()
	w := f.desk.AddWindow(platform.WindowInfo{PID: 9000, Class: []string{f.class[id], f.class[id]}})
	go func() {
		time.Sleep(20 * time.Millisecond)
		f.desk.SetActive(w)
	}()
	return applications.Instance{FlatpakID: id, InstanceID: "1", PID: 9000}, nil
}

// Instances reports none (liveness comes from windows) and counts the call.
func (f *fakeLauncher) Instances(context.Context) ([]applications.Instance, error) {
	f.mu.Lock()
	f.instances++
	f.mu.Unlock()
	return nil, nil
}
func (f *fakeLauncher) Kill(context.Context, applications.Instance) error { return nil }

type harness struct {
	t      *testing.T
	c      *Coordinator
	desk   *fake.Desktop
	lock   *fakeLock
	shellW platform.WindowID
	shell  *shellipc.Conn
	phones *PhoneBackend
	ctl    remote.Viewer
	owner  remote.Viewer

	mu      sync.Mutex
	inputs  []shellipc.Input
	results map[string]shellipc.Result // replies to the shell's requests, by request_id
	places  map[string]shellipc.WeatherPlaces

	stop context.CancelFunc // ends Run, as the coordinator stopping
}

// shellResult waits for the coordinator's reply to one of the shell's requests.
func (h *harness) shellResult(requestID string) shellipc.Result {
	h.t.Helper()
	var r shellipc.Result
	h.eventually("reply to "+requestID, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		var ok bool
		r, ok = h.results[requestID]
		return ok
	})
	return r
}

func newHarness(t *testing.T, configure ...func(*Options)) *harness {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(config.Options{Dir: filepath.Join(dir, "config"), Clock: clock.Real{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	pair, err := pairing.New(pairing.Options{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	desk := fake.New()
	lock := &fakeLock{ch: make(chan bool, 4)}
	h := &harness{t: t, desk: desk, lock: lock}
	opts := Options{
		Desktop: desk, Lock: lock, Config: store, Pairing: pair, AppsRefresh: 30 * time.Millisecond,
		Launcher: &fakeLauncher{desk: desk, class: map[string]string{"tv.plex.PlexHTPC": "plexhtpc", "rocks.shy.VacuumTube": "vacuumtube"}},
	}
	for _, f := range configure {
		f(&opts)
	}
	h.c = New(opts)
	sock := filepath.Join(dir, "run", "shell.sock")
	srv, err := shellipc.Listen(shellipc.Options{SocketPath: sock, Handler: h.c.Shell(), Clock: clock.Real{}, CoordinatorVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	h.c.AttachShellServer(srv)
	ctx, cancel := context.WithCancel(context.Background())
	h.stop = cancel
	t.Cleanup(func() { cancel(); srv.Close() })
	h.shellW = desk.AddWindow(platform.WindowInfo{PID: 1, Class: []string{"bear-den-tv-shell", "bear-den-tv-shell"}})
	desk.SetActive(h.shellW)
	go h.c.Run(ctx)

	conn, err := shellipc.Dial(ctx, sock, shellipc.ClientShell, "test")
	if err != nil {
		t.Fatal(err)
	}
	h.shell = conn
	go h.fakeShell()
	h.phones = h.c.Phones()
	h.ctl = remote.Viewer{DeviceID: "dev-ctl", DeviceName: "Phone", Permissions: []contract.Permission{contract.PermController}}
	h.owner = remote.Viewer{DeviceID: "dev-own", DeviceName: "Owner", Permissions: []contract.Permission{contract.PermOwner}}
	h.eventually("shell connected and targeted", func() bool {
		st := h.phones.Snapshot(ctx, &h.ctl)
		return st.Session.ShellConnected && st.Target.Kind == "shell"
	})
	return h
}

// fakeShell answers input/home like the real shell: observed with focus detail.
func (h *harness) fakeShell() {
	for {
		m, err := h.shell.Recv()
		if err != nil {
			return
		}
		switch msg := m.(type) {
		case shellipc.Input:
			h.mu.Lock()
			h.inputs = append(h.inputs, msg)
			h.mu.Unlock()
			_ = h.shell.Send(shellipc.InputResult{Type: shellipc.TypeInputResult, RequestID: msg.RequestID, Outcome: "observed", Code: "ok", Detail: map[string]any{"section_id": "favorites", "item_id": "youtube"}})
		case shellipc.Home:
			_ = h.shell.Send(shellipc.InputResult{Type: shellipc.TypeInputResult, RequestID: msg.RequestID, Outcome: "observed", Code: "ok", Detail: map[string]any{}})
		case shellipc.Ping:
			_ = h.shell.Send(shellipc.Pong{Type: shellipc.TypePong})
		case shellipc.Result:
			h.mu.Lock()
			if h.results == nil {
				h.results = map[string]shellipc.Result{}
			}
			h.results[msg.RequestID] = msg
			h.mu.Unlock()
		case shellipc.WeatherPlaces:
			h.mu.Lock()
			if h.places == nil {
				h.places = map[string]shellipc.WeatherPlaces{}
			}
			h.places[msg.RequestID] = msg
			h.mu.Unlock()
		}
	}
}

// windowsOf counts the desktop's windows whose WM_CLASS instance is class.
func (h *harness) windowsOf(class string) int {
	n := 0
	wins, _ := h.desk.ListWindows(context.Background())
	for _, w := range wins {
		if len(w.Class) > 0 && w.Class[0] == class {
			n++
		}
	}
	return n
}

func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	h.eventuallyWithin(3*time.Second, what, cond)
}

// eventuallyWithin is eventually with another bound: only for waits on a
// real outside process (Chromium in web_e2e_test.go), never for the fakes.
func (h *harness) eventuallyWithin(bound time.Duration, what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(bound)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var reqSeq int

func (h *harness) req(action string, args map[string]any) contract.ActionRequest {
	reqSeq++
	if args == nil {
		args = map[string]any{}
	}
	return contract.ActionRequest{Protocol: 1, RequestID: time.Now().Format("150405.000000000") + string(rune('a'+reqSeq%26)), ContextEpoch: h.c.Epoch(), Target: "active", Action: action, Args: args}
}

func (h *harness) submit(v remote.Viewer, r contract.ActionRequest) contract.ActionResult {
	return h.phones.Submit(context.Background(), v, r)
}

func expectOutcome(t *testing.T, res contract.ActionResult, outcome contract.Outcome, code contract.Code) {
	t.Helper()
	if res.Outcome != outcome || res.Code != code {
		t.Fatalf("result = %s/%s (%q), want %s/%s", res.Outcome, res.Code, res.Message, outcome, code)
	}
}

func TestShellNavigationObservedAndEpochRules(t *testing.T) {
	h := newHarness(t)
	r := h.req("nav.left", nil)
	res := h.submit(h.ctl, r)
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if res.Detail["item_id"] != "youtube" || res.Target.Kind != "shell" {
		t.Fatalf("observed result lacks shell focus detail: %+v", res)
	}
	// Same id + payload replays; same id + different payload is refused.
	if again := h.submit(h.ctl, r); again.Outcome != contract.OutcomeObserved {
		t.Fatalf("replay = %+v", again)
	}
	h.mu.Lock()
	n := len(h.inputs)
	h.mu.Unlock()
	if n != 1 {
		t.Fatalf("replay reached the shell: %d inputs", n)
	}
	mis := r
	mis.Action = "nav.right"
	expectOutcome(t, h.submit(h.ctl, mis), contract.OutcomeFailed, contract.CodeDuplicateMismatch)

	stale := h.req("select", nil)
	stale.ContextEpoch--
	expectOutcome(t, h.submit(h.ctl, stale), contract.OutcomeFailed, contract.CodeStaleEpoch)
}

func TestPermissionsAndRedaction(t *testing.T) {
	h := newHarness(t)
	expectOutcome(t, h.submit(h.ctl, h.req("shell.restart", nil)), contract.OutcomeFailed, contract.CodeForbidden)
	expectOutcome(t, h.submit(remote.Viewer{DeviceID: "x"}, h.req("nav.up", nil)), contract.OutcomeFailed, contract.CodeForbidden)

	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if st.Pairing != nil || st.Devices != nil || st.Layout != nil {
		t.Fatalf("controller view leaks owner/shell data: pairing=%v devices=%v layout=%v", st.Pairing, st.Devices, st.Layout)
	}
	if st.Me == nil || st.Me.DeviceID != "dev-ctl" {
		t.Fatalf("me = %+v", st.Me)
	}
	own := h.phones.Snapshot(context.Background(), &h.owner)
	if own.Devices == nil || own.Layout == nil || own.Pairing != nil {
		t.Fatal("owner view must carry devices and layout but never pairing")
	}
	anon := h.phones.Snapshot(context.Background(), nil)
	if anon.Me != nil || len(anon.Applications) != 0 || anon.Shell.Focus.ItemID != nil {
		t.Fatal("anonymous view leaks private state")
	}
	if !st.Capabilities["nav.left"].Holdable || st.Capabilities["select"].Holdable {
		t.Fatal("only nav.* may be holdable")
	}
}

func TestUnknownForegroundFailsClosedAndHomeEscapes(t *testing.T) {
	h := newHarness(t)
	other := h.desk.AddWindow(platform.WindowInfo{PID: 77, Class: []string{"xterm", "XTerm"}})
	before := h.c.Epoch()
	h.desk.SetActive(other)
	h.eventually("unknown target", func() bool { return h.c.Target().Kind == "unknown" })
	if h.c.Epoch() == before {
		t.Fatal("target change must bump the epoch")
	}
	expectOutcome(t, h.submit(h.ctl, h.req("nav.down", nil)), contract.OutcomeFailed, contract.CodeUnknownForeground)
	if len(h.desk.Keys()) != 0 {
		t.Fatal("input reached an unknown window")
	}

	results, _ := h.phones.Results(context.Background(), h.ctl)
	home := h.req("home", nil)
	home.ContextEpoch = before // home ignores stale epochs
	expectOutcome(t, h.submit(h.ctl, home), contract.OutcomeAccepted, contract.CodeOK)
	select {
	case res := <-results:
		expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	case <-time.After(3 * time.Second):
		t.Fatal("no terminal home result")
	}
	if h.c.Target().Kind != "shell" {
		t.Fatal("home did not bring the shell forward")
	}
}

func TestLaunchThenKeysGoToVerifiedApp(t *testing.T) {
	h := newHarness(t)
	results, _ := h.phones.Results(context.Background(), h.ctl)
	expectOutcome(t, h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeAccepted, contract.CodeOK)
	var got []contract.Outcome
	deadline := time.After(3 * time.Second)
	for len(got) < 2 {
		select {
		case res := <-results:
			got = append(got, res.Outcome)
		case <-deadline:
			t.Fatalf("launch results so far: %v", got)
		}
	}
	if got[0] != contract.OutcomeDelivered || got[1] != contract.OutcomeObserved {
		t.Fatalf("launch results = %v, want delivered then observed", got)
	}
	tgt := h.c.Target()
	if tgt.Kind != "app" || strOr(tgt.AppID) != "plex-htpc" {
		t.Fatalf("target = %+v", tgt)
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if st.Target.WindowTitle != nil {
		t.Fatal("external window titles must not reach phones")
	}
	expectOutcome(t, h.submit(h.ctl, h.req("nav.right", nil)), contract.OutcomeDelivered, contract.CodeOK)
	keys := h.desk.Keys()
	if len(keys) != 1 || keys[0].Key != platform.KeyRight {
		t.Fatalf("keys = %+v", keys)
	}
	expectOutcome(t, h.submit(h.ctl, h.req("text.submit", map[string]any{"text": "hi"})), contract.OutcomeFailed, contract.CodeUnsupported)
}

// An app closed (or crashed) after launch must stop showing as running, and a
// launched app window is asked to go fullscreen (owner-reported bugs).
func TestClosedAppStopsRunningAndLaunchGoesFullscreen(t *testing.T) {
	h := newHarness(t)
	expectOutcome(t, h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeAccepted, contract.CodeOK)
	h.eventually("plex in front", func() bool { return h.c.Target().Kind == "app" })
	h.eventually("plex fullscreen", func() bool {
		fg, _ := h.desk.ObserveForeground(context.Background())
		return fg.Known && fg.Window.Fullscreen
	})
	running := func() bool {
		for _, a := range h.phones.Snapshot(context.Background(), &h.ctl).Applications {
			if a.ID == "plex-htpc" {
				return a.Running
			}
		}
		return false
	}
	if !running() {
		t.Fatal("launched app should be running")
	}
	// The app closes itself (not through Bear Den): its window disappears.
	fg, _ := h.desk.ObserveForeground(context.Background())
	h.desk.RemoveWindow(fg.Window.ID)
	h.eventually("plex no longer running", func() bool { return !running() })
	h.eventually("shell back in front after the app exited", func() bool { return h.c.Target().Kind == "shell" })
	for _, a := range h.phones.Snapshot(context.Background(), &h.ctl).Applications {
		if a.ID == "plex-htpc" && a.LaunchState != "exited" {
			t.Fatalf("launch_state = %q, want exited", a.LaunchState)
		}
	}
}

// Opening a different app closes the one already running (one app at a time),
// and a double press while an app starts launches it only once.
func TestOpeningAnotherAppClosesThePreviousOneAndNoDoubleLaunch(t *testing.T) {
	h := newHarness(t)
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"})) // impatient second press
	h.eventually("plex in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	if n := h.windowsOf("plexhtpc"); n != 1 {
		t.Fatalf("double press started %d Plex windows, want 1", n)
	}
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "youtube"}))
	h.eventually("youtube in front", func() bool { return strOr(h.c.Target().AppID) == "youtube" })
	h.eventually("plex closed", func() bool { return h.windowsOf("plexhtpc") == 0 })
}

// The phone's Close button: app.close is offered while an app is in front or
// left running behind Home, and closes the whole app even when it answers the
// close by opening another window (Moonlight ends the stream, then shows its
// host list). A relaunch right after a close keeps its new window.
func TestCloseAppFromPhone(t *testing.T) {
	h := newHarness(t)
	closable := func() contract.Capability {
		return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionAppClose]
	}
	if cp := closable(); cp.Available || cp.Reason != "No application is running." {
		t.Fatalf("app.close with nothing running = %+v", cp)
	}
	expectOutcome(t, h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeFailed, contract.CodeNoTarget)

	// Close from the front, with the app reopening a window once.
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.eventually("plex in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	if !closable().Available {
		t.Fatal("app.close should be available with an app in front")
	}
	var reopened atomic.Bool
	h.desk.SetOnClose(func(platform.WindowID) {
		if reopened.CompareAndSwap(false, true) {
			h.desk.AddWindow(platform.WindowInfo{PID: 9000, Class: []string{"plexhtpc", "plexhtpc"}})
		}
	})
	expectOutcome(t, h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeDelivered, contract.CodeOK)
	h.eventually("reopened window closed too", func() bool { return reopened.Load() && h.windowsOf("plexhtpc") == 0 })
	h.eventually("shell back in front", func() bool { return h.c.Target().Kind == "shell" })
	h.desk.SetOnClose(nil)

	// Relaunching while that close is still being followed keeps the new window.
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.eventually("plex in front again", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	time.Sleep(3 * closeFollowPoll)
	if n := h.windowsOf("plexhtpc"); n != 1 {
		t.Fatalf("relaunched Plex has %d windows, want 1", n)
	}

	// Close from Home, with the app still running behind the shell.
	h.submit(h.ctl, h.req("home", nil))
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	if !closable().Available {
		t.Fatal("app.close should be available with an app running behind Home")
	}
	expectOutcome(t, h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeDelivered, contract.CodeOK)
	h.eventually("plex closed", func() bool { return h.windowsOf("plexhtpc") == 0 })
	h.eventually("close no longer offered", func() bool { return !closable().Available })
}

func TestLockedRefusesEverythingAndRedacts(t *testing.T) {
	h := newHarness(t)
	h.lock.set(true)
	h.eventually("locked target", func() bool { return h.c.Target().Kind == "locked" })
	for _, a := range []string{"nav.up", "home", "app.launch"} {
		r := h.req(a, map[string]any{})
		if a == "app.launch" {
			r.Args = map[string]any{"app_id": "plex-htpc"}
		}
		expectOutcome(t, h.submit(h.ctl, r), contract.OutcomeFailed, contract.CodeLocked)
	}
	st := h.phones.Snapshot(context.Background(), &h.owner)
	if st.Devices != nil || st.Layout != nil || st.Content != nil || st.Shell.Focus.SectionID != nil {
		t.Fatal("locked snapshot must omit devices, layout, content, and focus")
	}
	if cp := st.Capabilities["home"]; cp.Available {
		t.Fatal("home must be unavailable while locked")
	}
}

func TestHoldRepeatsIntoShell(t *testing.T) {
	h := newHarness(t)
	st := h.phones.HoldStart(context.Background(), h.ctl, contract.HoldMessage{Type: "hold.start", HoldID: "h1", Action: "nav.right", ContextEpoch: h.c.Epoch()})
	if st.State != "active" {
		t.Fatalf("hold start = %+v", st)
	}
	busy := h.phones.HoldStart(context.Background(), h.owner, contract.HoldMessage{Type: "hold.start", HoldID: "h2", Action: "nav.left", ContextEpoch: h.c.Epoch()})
	if busy.State != "busy" {
		t.Fatalf("second device hold = %+v", busy)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(150 * time.Millisecond)
		h.phones.HoldRenew(context.Background(), h.ctl, "h1")
	}
	h.phones.HoldStop(context.Background(), h.ctl, "h1", "stop")
	h.mu.Lock()
	n := len(h.inputs)
	h.mu.Unlock()
	if n < 2 {
		t.Fatalf("held nav produced %d inputs, want repeats", n)
	}
	if h.c.holds.Active().Active {
		t.Fatal("hold still active after stop")
	}
}

// fakeTuner stands in for the playback detection test: Plex decodes H.264 in
// hardware and needs one setting; it is "running", so the setting waits.
type fakeTuner struct {
	mu      sync.Mutex
	applied []string
	retuned []string // "adapter apply=true|false"
	youtube bool     // also report YouTube, closed
}

func (f *fakeTuner) Detect(context.Context) tuning.Report {
	r := tuning.Report{At: time.Unix(1, 0), Host: tuning.Host{Cores: 2, MemGiB: 8},
		Notes: []tuning.Note{{Level: "warn", Text: "The TV output runs at 120 Hz"}},
		Apps: []tuning.AppReport{{Label: "Plex", Adapter: "plex-htpc", FlatpakID: "tv.plex.PlexHTPC",
			Caps:   tuning.Caps{Probed: true, Hardware: map[tuning.Codec]bool{tuning.H264: true}},
			Expect: []string{"H264 files play directly on the GPU."}, Running: true,
			Plan: &tuning.Plan{Changes: []tuning.Change{{Key: "hwdec", To: "auto-safe", Why: "GPU decoding"}}}}}}
	if f.youtube {
		r.Apps = append(r.Apps, tuning.AppReport{Label: "YouTube", Adapter: "vacuumtube", FlatpakID: "rocks.shy.VacuumTube",
			Caps: tuning.Caps{Probed: true, Hardware: map[tuning.Codec]bool{}}, Status: "tuned", Plan: &tuning.Plan{}})
	}
	return r
}

func (f *fakeTuner) Retune(_ context.Context, r *tuning.Report, adapter string, apply bool) (*tuning.AppReport, bool) {
	f.mu.Lock()
	f.retuned = append(f.retuned, fmt.Sprintf("%s apply=%v", adapter, apply))
	f.mu.Unlock()
	for i := range r.Apps {
		a := &r.Apps[i]
		if a.Adapter != adapter {
			continue
		}
		switch {
		case !apply:
			a.Status = "off"
		case a.Running:
			a.Status = "pending"
		default:
			a.Status = "applied"
		}
		return a, true
	}
	return nil, false
}

func (f *fakeTuner) ApplyReady(r *tuning.Report) {
	for i := range r.Apps {
		if r.Apps[i].Running {
			r.Apps[i].Status = "pending"
		}
	}
}

func (f *fakeTuner) ApplyApp(r *tuning.Report, id string) (*tuning.AppReport, bool) {
	for i := range r.Apps {
		if r.Apps[i].FlatpakID == id && r.Apps[i].Status == "pending" {
			r.Apps[i].Status = "applied"
			f.mu.Lock()
			f.applied = append(f.applied, id)
			f.mu.Unlock()
			return &r.Apps[i], true
		}
	}
	return nil, false
}

// The playback test runs after startup: the TV's state carries the report,
// and a setting that waited for an app is applied the moment it closes.
func TestPlaybackTuningAfterStartupAndOnExit(t *testing.T) {
	old := TuneDelay
	TuneDelay = 20 * time.Millisecond
	t.Cleanup(func() { TuneDelay = old })
	ft := &fakeTuner{}
	h := newHarness(t, func(o *Options) { o.Tuner = ft })
	playback := func() *contract.Playback { return h.c.buildState(viewShell).Playback }
	h.eventually("playback report in the shell state", func() bool { return playback() != nil })
	p := playback()
	if len(p.Apps) != 1 || p.Apps[0].Status != "pending" || p.Apps[0].Hardware[0] != "H264" || len(p.Notes) != 1 {
		t.Fatalf("playback = %+v", p)
	}
	if h.phones.Snapshot(context.Background(), &h.ctl).Playback != nil {
		t.Fatal("phones do not get the playback report")
	}
	// Plex starts and then closes: its pending setting is applied.
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.eventually("plex in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	fg, _ := h.desk.ObserveForeground(context.Background())
	h.desk.RemoveWindow(fg.Window.ID)
	h.eventually("setting applied after plex closed", func() bool { return playback().Apps[0].Status == "applied" })
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if len(ft.applied) != 1 || ft.applied[0] != "tv.plex.PlexHTPC" {
		t.Fatalf("applied = %v", ft.applied)
	}
}

// Settings → Advanced playback: playback.set accepts only an option this box
// offers, stores it in config.json, re-plans the app (applied now while it is
// closed), and "" returns it to automatic. With startup.tune_apps false the
// choice is stored but nothing is applied.
func TestPlaybackSetStoresTheOverrideAndRetunes(t *testing.T) {
	old := TuneDelay
	TuneDelay = 20 * time.Millisecond
	t.Cleanup(func() { TuneDelay = old })
	ft := &fakeTuner{youtube: true}
	h := newHarness(t, func(o *Options) { o.Tuner = ft })
	youtube := func() contract.PlaybackApp {
		if p := h.c.buildState(viewShell).Playback; p != nil && len(p.Apps) == 2 {
			return p.Apps[1]
		}
		return contract.PlaybackApp{}
	}
	setting := func(id string) contract.PlaybackSetting {
		for _, s := range youtube().Settings {
			if s.ID == id {
				return s
			}
		}
		return contract.PlaybackSetting{}
	}
	h.eventually("playback report with settings", func() bool { return setting("pause_on_blur").ID != "" })
	if s := setting("codecs"); s.Auto != "h264" || s.Value != "h264" || s.Overridden || len(s.Options) != 1 {
		t.Fatalf("entry box without VP9/AV1 hardware: only H.264 is offered: %+v", s)
	}
	seq := 0
	set := func(adapter, id, value string) shellipc.Result {
		seq++
		rid := fmt.Sprintf("pb-%d", seq)
		if err := h.shell.Send(shellipc.PlaybackSet{Type: shellipc.TypePlaybackSet, RequestID: rid, Adapter: adapter, Setting: id, Value: value}); err != nil {
			t.Fatal(err)
		}
		return h.shellResult(rid)
	}
	overrides := func() map[string]string { return h.c.opts.Config.Current().PlaybackOverrides("vacuumtube") }

	if r := set("vacuumtube", "pause_on_blur", "off"); !r.OK {
		t.Fatalf("offered value refused: %+v", r)
	}
	if overrides()["pause_on_blur"] != "off" {
		t.Fatalf("override not stored: %v", overrides())
	}
	if s := setting("pause_on_blur"); s.Value != "off" || s.Auto != "on" || !s.Overridden || youtube().Status != "applied" {
		t.Fatalf("state after the change: %+v status %s", s, youtube().Status)
	}
	for _, bad := range [][3]string{
		{"vacuumtube", "codecs", "av1"}, // not decoded in hardware on an entry box
		{"vacuumtube", "codecs", "rm -rf"},
		{"vacuumtube", "volume", "on"}, // not an adjustable setting
		{"moonlight", "fps", "60"},     // not installed here
	} {
		if r := set(bad[0], bad[1], bad[2]); r.OK || r.Error == "" {
			t.Fatalf("%v must fail closed with a reason: %+v", bad, r)
		}
	}
	if _, stored := overrides()["codecs"]; stored {
		t.Fatalf("a refused value is not stored: %v", overrides())
	}
	if r := set("vacuumtube", "pause_on_blur", ""); !r.OK || overrides() != nil || setting("pause_on_blur").Overridden {
		t.Fatalf("back to automatic: %+v %v", r, overrides())
	}

	// Automatic tuning off: the choice is kept, nothing is applied.
	if _, err := h.c.opts.Config.Update(func(c *config.Config) error { off := false; c.Startup.TuneApps = &off; return nil }); err != nil {
		t.Fatal(err)
	}
	if r := set("vacuumtube", "low_memory", "on"); !r.OK || overrides()["low_memory"] != "on" || youtube().Status != "off" {
		t.Fatalf("tune_apps=false: %+v %v %s", r, overrides(), youtube().Status)
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if got := strings.Join(ft.retuned, ";"); got != "vacuumtube apply=true;vacuumtube apply=true;vacuumtube apply=false" {
		t.Fatalf("retuned = %s", got)
	}
}
