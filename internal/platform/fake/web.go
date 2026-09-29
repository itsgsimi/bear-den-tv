// A pretend web app manager for `bear-den-tv dev` (session.WebApps): opening
// a web app maps a window with the adapter's class on the fake desktop, and
// its page reports a DEMO status (no video, no text field), so the phone's
// touchpad and the web capabilities can be seen without Chromium. With
// `dev --dev-browser PATH` the real manager (internal/applications/web) runs
// a real Chromium instead.

package fake

import (
	"context"
	"sync"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/platform"
)

// Web is the pretend manager.
type Web struct {
	Desk *Desktop

	mu      sync.Mutex
	windows map[string]platform.WindowID
	watch   func()
}

// NewWeb builds a pretend manager on desk.
func NewWeb(desk *Desktop) *Web { return &Web{Desk: desk, windows: map[string]platform.WindowID{}} }

// Launch maps a window for the app and brings it to the front.
func (w *Web) Launch(_ context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error) {
	win := w.Desk.AddWindow(platform.WindowInfo{PID: 30000, Class: []string{"demo.page", spec.Class}, Title: "DEMO " + app.Label})
	w.mu.Lock()
	w.windows[app.ID] = win
	w.mu.Unlock()
	time.AfterFunc(300*time.Millisecond, func() { w.Desk.SetActive(win) })
	return applications.Instance{FlatpakID: app.Launch.AppID, PID: 30000}, nil
}

// PID is the pretend browser process while the app runs (no DEMO player
// descends from it, so web apps show no Now playing in dev).
func (w *Web) PID(appID string) int {
	if w.Running(appID) {
		return 30000
	}
	return 0
}

// Running reports whether the app's pretend window still exists.
func (w *Web) Running(appID string) bool {
	w.mu.Lock()
	win, ok := w.windows[appID]
	w.mu.Unlock()
	return ok && w.Desk.HasWindow(win)
}

// Status is a DEMO page: visible, no video, no text field.
func (w *Web) Status(appID string) (web.Status, bool) {
	if !w.Running(appID) {
		return web.Status{}, false
	}
	return web.Status{V: 1, Visible: true, Video: "none", Viewport: web.Viewport{W: 1920, H: 1080}}, true
}

// Apply pretends the page moved its focus (observed) or clicked (delivered).
func (w *Web) Apply(_ context.Context, appID, action string, _ map[string]any) (web.Outcome, error) {
	if !w.Running(appID) {
		return web.Outcome{}, web.ErrNotRunning
	}
	switch action {
	case "nav.up", "nav.down", "nav.left", "nav.right":
		return web.Outcome{Observed: true, Detail: map[string]any{"page": "moved"}}, nil
	case "back":
		return web.Outcome{Observed: true, Detail: map[string]any{"page": "at_root"}}, nil
	}
	return web.Outcome{Detail: map[string]any{"page": "click"}}, nil
}

// Pointer accepts every move, click and scroll.
func (w *Web) Pointer(_ context.Context, appID, _ string, _ map[string]any) error {
	if !w.Running(appID) {
		return web.ErrNotRunning
	}
	return nil
}

// PauseIfPlaying: nothing plays on a DEMO page.
func (w *Web) PauseIfPlaying(context.Context, string) (bool, error) { return false, nil }

// Close removes the app's window.
func (w *Web) Close(_ context.Context, appID string, _ bool) error {
	w.mu.Lock()
	win, ok := w.windows[appID]
	delete(w.windows, appID)
	w.mu.Unlock()
	if ok {
		w.Desk.RemoveWindow(win)
	}
	return nil
}

// Watch stores the change callback (the pretend page never changes).
func (w *Web) Watch(fn func()) {
	w.mu.Lock()
	w.watch = fn
	w.mu.Unlock()
}
