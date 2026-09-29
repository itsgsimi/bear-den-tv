// The web app manager: starts Chromium for a web app with the DevTools pipe,
// attaches to every page it opens, injects the navigation script into the
// "bearden" isolated world, tracks what each page reports (video, text
// field, visibility), and turns the coordinator's named actions into script
// calls plus trusted input. Spec: package doc (web.go),
// docs/decisions/0010-web-apps-over-cdp-pipe.md.

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/config"
)

// Errors the coordinator maps to failure codes.
var (
	// ErrNotRunning: Bear Den has no DevTools connection to this app (it is
	// not running, or it was started before the coordinator restarted).
	ErrNotRunning = errors.New("web: not connected to this app")
	// ErrNoPage: Chromium has no page with the navigation script yet.
	ErrNoPage = errors.New("web: no page is ready")
)

// RefusedError is a page's (or Bear Den's own) refusal, with a reason fit
// for the phone.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return e.Reason }

func refused(reason string) error {
	if len(reason) > 200 {
		reason = reason[:200]
	}
	return &RefusedError{Reason: reason}
}

// Focus is where the script's focus ring is, without any label.
type Focus struct {
	Role      string `json:"role"`
	TextField bool   `json:"text_field"`
	Index     int    `json:"index"`
}

// Viewport is the page's visible size in CSS pixels.
type Viewport struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Status is what the script reports through the binding (apps/web-nav
// src/types.ts Status).
type Status struct {
	V          int      `json:"v"`
	Visible    bool     `json:"visible"`
	Video      string   `json:"video"` // none | playing | paused
	TextField  bool     `json:"text_field"`
	Fullscreen bool     `json:"fullscreen"`
	Viewport   Viewport `json:"viewport"`
	Focus      *Focus   `json:"focus,omitempty"`
}

// Effect is trusted input the script asks for (src/types.ts Effect).
type Effect struct {
	Kind string   `json:"kind"` // click | keys | text
	X    float64  `json:"x"`
	Y    float64  `json:"y"`
	Keys []string `json:"keys"`
}

// Reply is the script's answer to one apply call (src/types.ts Reply).
type Reply struct {
	OK      bool    `json:"ok"`
	Outcome string  `json:"outcome"`
	Reason  string  `json:"reason"`
	Effect  *Effect `json:"effect"`
	Focus   *Focus  `json:"focus"`
}

// Outcome is an applied action: Observed only when the page itself
// confirmed the result (focus moved, video paused); Detail goes into the
// action result. Detail never carries page text.
type Outcome struct {
	Observed bool
	Detail   map[string]any
}

// Process is a started Chromium.
type Process interface {
	PID() int
	Done() <-chan struct{}
}

// Starter starts Chromium with its argument list and the two pipe ends as
// fds 3 and 4.
type Starter interface {
	Start(ctx context.Context, chromiumArgs []string, extra []*os.File) (Process, error)
}

// ExecStarter runs Prefix + Chromium's arguments through the Flatpak
// launcher's runner (fixed argv, filtered environment, own session). The
// production prefix is FlatpakArgv(nil); `dev --dev-browser PATH` and the
// end-to-end test use a Chromium binary directly.
type ExecStarter struct {
	Runner flatpak.Runner
	Env    []string
	Prefix []string
}

// Start implements Starter.
func (s ExecStarter) Start(ctx context.Context, args []string, extra []*os.File) (Process, error) {
	if len(s.Prefix) == 0 {
		return nil, errors.New("web: no browser command")
	}
	argv := append(append([]string{}, s.Prefix...), args...)
	run := s.Runner
	if run == nil {
		run = flatpak.ExecRunner{}
	}
	return run.Start(ctx, flatpak.Command{Argv: argv, Env: s.Env, ExtraFiles: extra}, flatpak.NewRing(flatpak.RingSize), flatpak.NewRing(flatpak.RingSize))
}

// FlatpakStarter starts Flathub Chromium through `flatpak run`.
func FlatpakStarter() ExecStarter {
	return ExecStarter{Env: flatpak.PassthroughEnv(os.Environ()), Prefix: FlatpakArgv(nil)}
}

// Options configures NewManager.
type Options struct {
	// DataHome is $XDG_DATA_HOME (absolute); profiles live under
	// DataHome/bear-den-tv/web/<app-id>.
	DataHome string
	Starter  Starter
	Logger   *slog.Logger
	// SetupTimeout bounds the DevTools handshake after start (default 15 s).
	SetupTimeout time.Duration
}

// Manager owns the running web apps, one Chromium per app.
type Manager struct {
	opts     Options
	log      *slog.Logger
	mu       sync.Mutex
	running  map[string]*Browser
	onChange func()
}

// NewManager builds a manager.
func NewManager(o Options) *Manager {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.SetupTimeout <= 0 {
		o.SetupTimeout = 15 * time.Second
	}
	return &Manager{opts: o, log: o.Logger, running: map[string]*Browser{}}
}

// Watch registers fn to be called (from any goroutine) when an app starts,
// stops or its page reports new status.
func (m *Manager) Watch(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Manager) changed() {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Browser is one running web app.
type Browser struct {
	m      *Manager
	appID  string
	proc   Process
	conn   *Conn
	source string

	mu      sync.Mutex
	pages   map[string]*page // by session id
	order   []string         // attach order
	current string           // session id of the page last seen visible
	cx, cy  float64          // touchpad cursor, CSS pixels
	cursor  bool
}

type page struct {
	session  string
	targetID string
	ctx      int64 // execution context id of the bearden world in the main frame
	status   Status
	ready    chan struct{}
	once     sync.Once
}

// Running reports whether Bear Den holds a live DevTools connection to app.
func (m *Manager) Running(appID string) bool { return m.get(appID) != nil }

func (m *Manager) get(appID string) *Browser {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.running[appID]
	if b == nil {
		return nil
	}
	select {
	case <-b.conn.Done():
		return nil
	default:
		return b
	}
}

// Launch starts Chromium for app, or returns the running one.
func (m *Manager) Launch(ctx context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error) {
	if b := m.get(app.ID); b != nil {
		return applications.Instance{FlatpakID: adapters.ChromiumFlatpakID, PID: b.proc.PID()}, nil
	}
	if m.opts.Starter == nil {
		return applications.Instance{}, errors.New("web: no browser starter")
	}
	url, err := StartURL(app)
	if err != nil {
		return applications.Instance{}, err
	}
	profile, err := ProfileDir(m.opts.DataHome, app.ID)
	if err != nil {
		return applications.Instance{}, err
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return applications.Instance{}, fmt.Errorf("web: profile: %w", err)
	}
	source, err := Source(spec.Hints)
	if err != nil {
		return applications.Instance{}, err
	}
	// Chromium reads commands on fd 3 and writes replies on fd 4.
	toR, toW, err := os.Pipe()
	if err != nil {
		return applications.Instance{}, err
	}
	fromR, fromW, err := os.Pipe()
	if err != nil {
		toR.Close()
		toW.Close()
		return applications.Instance{}, err
	}
	proc, err := m.opts.Starter.Start(ctx, ChromiumArgs(spec, profile, url), []*os.File{toR, fromW})
	// The child has its own copies now; the coordinator keeps only its ends,
	// so the pipe has exactly two holders.
	toR.Close()
	fromW.Close()
	if err != nil {
		toW.Close()
		fromR.Close()
		return applications.Instance{}, err
	}
	b := &Browser{m: m, appID: app.ID, proc: proc, source: source, pages: map[string]*page{}}
	b.conn = NewConn(fromR, toW, b.onEvent)
	m.mu.Lock()
	m.running[app.ID] = b
	m.mu.Unlock()
	go b.watch(toW, fromR)

	sctx, cancel := context.WithTimeout(ctx, m.opts.SetupTimeout)
	defer cancel()
	if err := b.conn.Call(sctx, "", "Target.setDiscoverTargets", map[string]any{"discover": true}, nil); err != nil {
		b.kill()
		return applications.Instance{}, fmt.Errorf("web: devtools handshake: %w", err)
	}
	if err := b.conn.Call(sctx, "", "Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}, nil); err != nil {
		b.kill()
		return applications.Instance{}, fmt.Errorf("web: devtools auto-attach: %w", err)
	}
	m.changed()
	return applications.Instance{FlatpakID: adapters.ChromiumFlatpakID, PID: proc.PID()}, nil
}

func (b *Browser) watch(w, r *os.File) {
	select {
	case <-b.proc.Done():
	case <-b.conn.Done():
	}
	w.Close()
	r.Close()
	<-b.conn.Done()
	b.m.mu.Lock()
	if b.m.running[b.appID] == b {
		delete(b.m.running, b.appID)
	}
	b.m.mu.Unlock()
	b.m.changed()
}

func (b *Browser) kill() {
	if pid := b.proc.PID(); pid > 0 {
		// The starter gave Chromium its own session: signal the whole group.
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
}

// onEvent runs on the reader goroutine; anything that calls back into
// DevTools goes to its own goroutine.
func (b *Browser) onEvent(ev Event) {
	switch ev.Method {
	case "Target.attachedToTarget":
		var p struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			return
		}
		if p.TargetInfo.Type != "page" {
			go func() {
				_ = b.conn.Call(context.Background(), p.SessionID, "Runtime.runIfWaitingForDebugger", nil, nil)
			}()
			return
		}
		pg := &page{session: p.SessionID, targetID: p.TargetInfo.TargetID, ready: make(chan struct{})}
		b.mu.Lock()
		b.pages[p.SessionID] = pg
		b.order = append(b.order, p.SessionID)
		b.mu.Unlock()
		go b.setupPage(pg)
	case "Target.detachedFromTarget":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			return
		}
		b.mu.Lock()
		delete(b.pages, p.SessionID)
		for i, s := range b.order {
			if s == p.SessionID {
				b.order = append(b.order[:i], b.order[i+1:]...)
				break
			}
		}
		if b.current == p.SessionID {
			b.current = ""
		}
		b.mu.Unlock()
		b.m.changed()
	case "Runtime.executionContextCreated":
		var p struct {
			Context struct {
				ID      int64  `json:"id"`
				Name    string `json:"name"`
				AuxData struct {
					FrameID string `json:"frameId"`
				} `json:"auxData"`
			} `json:"context"`
		}
		if json.Unmarshal(ev.Params, &p) != nil || p.Context.Name != WorldName {
			return
		}
		b.mu.Lock()
		if pg := b.pages[ev.SessionID]; pg != nil && p.Context.AuxData.FrameID == pg.targetID {
			pg.ctx = p.Context.ID
			pg.once.Do(func() { close(pg.ready) })
		}
		b.mu.Unlock()
	case "Runtime.executionContextDestroyed":
		var p struct {
			ID int64 `json:"executionContextId"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			return
		}
		b.mu.Lock()
		if pg := b.pages[ev.SessionID]; pg != nil && pg.ctx == p.ID {
			pg.ctx = 0
		}
		b.mu.Unlock()
	case "Runtime.executionContextsCleared":
		b.mu.Lock()
		if pg := b.pages[ev.SessionID]; pg != nil {
			pg.ctx = 0
		}
		b.mu.Unlock()
	case "Runtime.bindingCalled":
		var p struct {
			Name    string `json:"name"`
			Payload string `json:"payload"`
			Context int64  `json:"executionContextId"`
		}
		if json.Unmarshal(ev.Params, &p) != nil || p.Name != ReportBinding || len(p.Payload) > 4096 {
			return
		}
		var st Status
		if json.Unmarshal([]byte(p.Payload), &st) != nil || st.V != 1 {
			return
		}
		b.mu.Lock()
		pg := b.pages[ev.SessionID]
		if pg == nil || pg.ctx == 0 || pg.ctx != p.Context {
			b.mu.Unlock()
			return
		}
		pg.status = sanitize(st)
		if st.Visible {
			b.current = ev.SessionID
		}
		b.mu.Unlock()
		b.m.changed()
	}
}

func sanitize(st Status) Status {
	switch st.Video {
	case "none", "playing", "paused":
	default:
		st.Video = "none"
	}
	st.Viewport.W = clamp(st.Viewport.W, 0, 16384)
	st.Viewport.H = clamp(st.Viewport.H, 0, 16384)
	if st.Focus != nil && len(st.Focus.Role) > 24 {
		st.Focus.Role = st.Focus.Role[:24]
	}
	return st
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

// setupPage prepares one page: the report binding and the script in the
// bearden world for every document it loads, then lets it run.
func (b *Browser) setupPage(pg *page) {
	ctx, cancel := context.WithTimeout(context.Background(), b.m.opts.SetupTimeout)
	defer cancel()
	call := func(method string, params any) error {
		return b.conn.Call(ctx, pg.session, method, params, nil)
	}
	for _, step := range []struct {
		method string
		params any
	}{
		{"Runtime.enable", nil},
		{"Page.enable", nil},
		{"Runtime.addBinding", map[string]any{"name": ReportBinding, "executionContextName": WorldName}},
		{"Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": b.source, "worldName": WorldName, "runImmediately": true}},
		{"Runtime.runIfWaitingForDebugger", nil},
	} {
		if err := call(step.method, step.params); err != nil {
			b.m.log.Warn("web: page setup failed", "app", b.appID, "step", step.method, "err", err)
			return
		}
	}
	// A document that was already loaded before we attached gets the world
	// from runImmediately; if Chromium did not create it, make it here.
	select {
	case <-pg.ready:
		return
	case <-time.After(500 * time.Millisecond):
	}
	var world struct {
		ID int64 `json:"executionContextId"`
	}
	if err := b.conn.Call(ctx, pg.session, "Page.createIsolatedWorld", map[string]any{"frameId": pg.targetID, "worldName": WorldName}, &world); err != nil {
		return
	}
	b.mu.Lock()
	if pg.ctx == 0 {
		pg.ctx = world.ID
	}
	id := pg.ctx
	b.mu.Unlock()
	pg.once.Do(func() { close(pg.ready) })
	_ = b.conn.Call(ctx, pg.session, "Runtime.evaluate", map[string]any{"expression": b.source, "contextId": id}, nil)
}

// currentPage is the page last reported visible, else the newest one.
func (b *Browser) currentPage() (*page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pg := b.pages[b.current]; pg != nil {
		return pg, nil
	}
	for i := len(b.order) - 1; i >= 0; i-- {
		if pg := b.pages[b.order[i]]; pg != nil {
			return pg, nil
		}
	}
	return nil, ErrNoPage
}

// evaluate runs a fixed expression in the page's bearden world.
func (b *Browser) evaluate(ctx context.Context, pg *page, expr string, out any) error {
	select {
	case <-pg.ready:
	case <-ctx.Done():
		return ErrNoPage
	case <-time.After(3 * time.Second):
		return ErrNoPage
	}
	b.mu.Lock()
	id := pg.ctx
	b.mu.Unlock()
	if id == 0 {
		// Between documents: the next one gets the script on load.
		time.Sleep(150 * time.Millisecond)
		b.mu.Lock()
		id = pg.ctx
		b.mu.Unlock()
		if id == 0 {
			return ErrNoPage
		}
	}
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := b.conn.Call(ctx, pg.session, "Runtime.evaluate", map[string]any{"expression": expr, "contextId": id, "returnByValue": true}, &res); err != nil {
		return err
	}
	if res.Exception != nil {
		return fmt.Errorf("web: script error: %s", res.Exception.Text)
	}
	if out != nil {
		if len(res.Result.Value) == 0 {
			return errors.New("web: script returned nothing")
		}
		return json.Unmarshal(res.Result.Value, out)
	}
	return nil
}

// scriptCall is the only way an expression is built: a fixed action name
// from the switch in Apply, and an optional integer.
func scriptCall(action string, arg *int) string {
	if arg != nil {
		return "__bdtv.apply(" + strconv.Quote(action) + "," + strconv.Itoa(*arg) + ")"
	}
	return "__bdtv.apply(" + strconv.Quote(action) + ")"
}

// Status returns what the app's current page last reported.
func (m *Manager) Status(appID string) (Status, bool) {
	b := m.get(appID)
	if b == nil {
		return Status{}, false
	}
	pg, err := b.currentPage()
	if err != nil {
		return Status{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return pg.status, pg.status.V == 1
}

func (b *Browser) freshStatus(ctx context.Context, pg *page) (Status, error) {
	var st Status
	if err := b.evaluate(ctx, pg, "__bdtv.status()", &st); err != nil {
		return st, err
	}
	st = sanitize(st)
	b.mu.Lock()
	pg.status = st
	b.mu.Unlock()
	return st, nil
}

// viewport asks Chromium (not the page) for the visible size.
func (b *Browser) viewport(ctx context.Context, pg *page) (Viewport, error) {
	var lm struct {
		CSS struct {
			W float64 `json:"clientWidth"`
			H float64 `json:"clientHeight"`
		} `json:"cssLayoutViewport"`
	}
	if err := b.conn.Call(ctx, pg.session, "Page.getLayoutMetrics", nil, &lm); err != nil {
		return Viewport{}, err
	}
	if lm.CSS.W <= 0 || lm.CSS.H <= 0 {
		return Viewport{}, errors.New("web: no viewport")
	}
	return Viewport{W: lm.CSS.W, H: lm.CSS.H}, nil
}

// checkEffect accepts only what the action may cause: a click inside the
// viewport for select and back, Escape for back, keys from AllowedKeys for
// media, text for text.submit. Everything else is refused.
func checkEffect(action string, e *Effect, vp Viewport) error {
	bad := refused("The page asked for something Bear Den does not do.")
	switch e.Kind {
	case "click":
		if action != "select" && action != "back" {
			return bad
		}
		if math.IsNaN(e.X) || math.IsNaN(e.Y) || e.X < 0 || e.Y < 0 || e.X >= vp.W || e.Y >= vp.H {
			return refused("That is outside the page.")
		}
	case "keys":
		if len(e.Keys) == 0 || len(e.Keys) > MaxKeys {
			return bad
		}
		for _, k := range e.Keys {
			if _, ok := AllowedKeys[k]; !ok {
				return bad
			}
			if action == "back" && k != "Escape" {
				return bad
			}
		}
		switch action {
		case "back", "media.play", "media.pause", "media.seek_relative":
		default:
			return bad
		}
	case "text":
		if action != "text.submit" {
			return bad
		}
	default:
		return bad
	}
	return nil
}

func (b *Browser) pressKey(ctx context.Context, pg *page, name string) error {
	k := AllowedKeys[name]
	down := map[string]any{"type": "rawKeyDown", "key": k.Key, "code": k.Code, "windowsVirtualKeyCode": k.VK, "nativeVirtualKeyCode": k.VK}
	if k.Text != "" {
		down["type"] = "keyDown"
		down["text"] = k.Text
		down["unmodifiedText"] = k.Text
	}
	if err := b.conn.Call(ctx, pg.session, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	return b.conn.Call(ctx, pg.session, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k.Key, "code": k.Code, "windowsVirtualKeyCode": k.VK, "nativeVirtualKeyCode": k.VK}, nil)
}

func (b *Browser) click(ctx context.Context, pg *page, x, y float64, button string) error {
	for _, typ := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		ev := map[string]any{"type": typ, "x": x, "y": y}
		if typ != "mouseMoved" {
			ev["button"] = button
			ev["clickCount"] = 1
		}
		if err := b.conn.Call(ctx, pg.session, "Input.dispatchMouseEvent", ev, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *Browser) perform(ctx context.Context, pg *page, e *Effect, text string) error {
	switch e.Kind {
	case "click":
		return b.click(ctx, pg, e.X, e.Y, "left")
	case "keys":
		for _, k := range e.Keys {
			if err := b.pressKey(ctx, pg, k); err != nil {
				return err
			}
		}
		return nil
	case "text":
		if err := b.conn.Call(ctx, pg.session, "Input.insertText", map[string]any{"text": text}, nil); err != nil {
			return err
		}
		return b.pressKey(ctx, pg, "Enter")
	}
	return refused("The page asked for something Bear Den does not do.")
}

func focusDetail(d map[string]any, f *Focus) {
	if f == nil {
		return
	}
	d["focus_role"] = f.Role
	d["focus_index"] = f.Index
	d["text_field"] = f.TextField
}

// Apply sends one named action to the app's page. The caller has verified
// that the app's window is the foreground and the capability is available.
func (m *Manager) Apply(ctx context.Context, appID, action string, args map[string]any) (Outcome, error) {
	b := m.get(appID)
	if b == nil {
		return Outcome{}, ErrNotRunning
	}
	pg, err := b.currentPage()
	if err != nil {
		return Outcome{}, err
	}
	var expr, text string
	switch action {
	case "nav.up", "nav.down", "nav.left", "nav.right", "select", "back", "media.play", "media.pause":
		expr = scriptCall(action, nil)
	case "media.seek_relative":
		s, ok := intArg(args, "seconds")
		if !ok || s == 0 || s < -600 || s > 600 {
			return Outcome{}, refused("Seek needs a number of seconds.")
		}
		expr = scriptCall(action, &s)
	case "text.submit":
		t, _ := args["text"].(string)
		if t == "" || len(t) > 1024 {
			return Outcome{}, refused("Nothing to type.")
		}
		text = t
		expr = scriptCall("text.prepare", nil)
	default:
		return Outcome{}, refused(action + " is not something a web page does.")
	}
	var r Reply
	if err := b.evaluate(ctx, pg, expr, &r); err != nil {
		return Outcome{}, err
	}
	if !r.OK {
		reason := r.Reason
		if reason == "" {
			reason = "The page could not do that."
		}
		return Outcome{}, refused(reason)
	}
	detail := map[string]any{"page": r.Outcome}
	focusDetail(detail, r.Focus)
	if r.Effect == nil {
		// The script did it itself and reported the result: focus moved,
		// field left, already paused, at the root.
		switch r.Outcome {
		case "history_back", "exited_fullscreen":
			return Outcome{Observed: false, Detail: detail}, nil
		}
		return Outcome{Observed: true, Detail: detail}, nil
	}
	vp, err := b.viewport(ctx, pg)
	if err != nil {
		return Outcome{}, err
	}
	if err := checkEffect(action, r.Effect, vp); err != nil {
		return Outcome{}, err
	}
	if err := b.perform(ctx, pg, r.Effect, text); err != nil {
		return Outcome{}, err
	}
	want := map[string]string{"media.pause": "paused", "media.play": "playing"}[action]
	if want == "" {
		return Outcome{Observed: false, Detail: detail}, nil
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := b.freshStatus(ctx, pg); err == nil && st.Video == want {
			detail["video"] = want
			return Outcome{Observed: true, Detail: detail}, nil
		}
		select {
		case <-ctx.Done():
			return Outcome{Detail: detail}, nil
		case <-time.After(100 * time.Millisecond):
		}
	}
	return Outcome{Detail: detail}, nil
}

// PauseIfPlaying presses the site's pause shortcut only when the page
// reports a playing video, and says whether the page then reported it
// paused (Home policy for web adapters).
func (m *Manager) PauseIfPlaying(ctx context.Context, appID string) (bool, error) {
	b := m.get(appID)
	if b == nil {
		return false, ErrNotRunning
	}
	pg, err := b.currentPage()
	if err != nil {
		return false, err
	}
	st, err := b.freshStatus(ctx, pg)
	if err != nil {
		return false, err
	}
	if st.Video != "playing" {
		return false, nil
	}
	out, err := m.Apply(ctx, appID, "media.pause", nil)
	if err != nil {
		return false, err
	}
	return out.Observed, nil
}

// Pointer moves, clicks or scrolls the touchpad cursor on the app's page:
// trusted mouse events at a position Bear Den keeps inside the viewport,
// with a visible cursor drawn by the script.
func (m *Manager) Pointer(ctx context.Context, appID, action string, args map[string]any) error {
	b := m.get(appID)
	if b == nil {
		return ErrNotRunning
	}
	pg, err := b.currentPage()
	if err != nil {
		return err
	}
	vp, err := b.viewport(ctx, pg)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if !b.cursor {
		b.cx, b.cy, b.cursor = vp.W/2, vp.H/2, true
	}
	b.cx = clamp(b.cx, 0, vp.W-1)
	b.cy = clamp(b.cy, 0, vp.H-1)
	b.mu.Unlock()
	switch action {
	case "pointer.move":
		dx, okx := intArg(args, "dx")
		dy, oky := intArg(args, "dy")
		if !okx || !oky || dx < -400 || dx > 400 || dy < -400 || dy > 400 {
			return refused("A move is at most 400 pixels.")
		}
		b.mu.Lock()
		b.cx = clamp(b.cx+float64(dx), 0, vp.W-1)
		b.cy = clamp(b.cy+float64(dy), 0, vp.H-1)
		x, y := b.cx, b.cy
		b.mu.Unlock()
		if err := b.conn.Call(ctx, pg.session, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": y}, nil); err != nil {
			return err
		}
		_ = b.evaluate(ctx, pg, "__bdtv.cursor("+strconv.Itoa(int(x))+","+strconv.Itoa(int(y))+")", nil)
		return nil
	case "pointer.click":
		button, _ := args["button"].(string)
		if button != "left" && button != "right" {
			return refused("Click left or right.")
		}
		b.mu.Lock()
		x, y := b.cx, b.cy
		b.mu.Unlock()
		return b.click(ctx, pg, x, y, button)
	case "pointer.scroll":
		dy, ok := intArg(args, "dy")
		if !ok || dy == 0 || dy < -2000 || dy > 2000 {
			return refused("A scroll is at most 2000 pixels.")
		}
		b.mu.Lock()
		x, y := b.cx, b.cy
		b.mu.Unlock()
		return b.conn.Call(ctx, pg.session, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": dy}, nil)
	}
	return refused(action + " is not a pointer action.")
}

// Close asks Chromium to close (Browser.close over the pipe), and ends the
// process group if it has not gone within 5 s, or at once with force.
func (m *Manager) Close(ctx context.Context, appID string, force bool) error {
	b := m.get(appID)
	if b == nil {
		return ErrNotRunning
	}
	if !force {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = b.conn.Call(cctx, "", "Browser.close", nil, nil)
		cancel()
		select {
		case <-b.proc.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	b.kill()
	return nil
}

func intArg(args map[string]any, k string) (int, bool) {
	switch v := args[k].(type) {
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	}
	return 0, false
}
