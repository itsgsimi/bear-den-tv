// Sign-in links (docs/operations.md "Sign-ins on the TV"): an app on the TV
// asks the desktop to open a web page (Spotify's Log in), Bear Den shows it
// in the Browser tile's browser, where the remote and the phone's keyboard
// work, and notices when the page hands its result back to the app on this
// machine: a request of a tab's main frame to a loopback address (Spotify
// waits on http://127.0.0.1:4381/login, reached as one hop of a redirect that
// ends on open.spotify.com, seen on the TV). Requests to loopback addresses
// pause (Fetch on the browser, those patterns only) and go on at once; a
// navigation that ends on one is seen too. The coordinator then closes that
// tab and returns to the app (internal/session/links.go).

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

// MaxLinkLength bounds a link Bear Den opens (sign-in links carry a few
// hundred characters of parameters).
const MaxLinkLength = 8192

// CheckLink accepts an absolute http or https link with a host, of at most
// MaxLinkLength bytes, without control characters.
func CheckLink(raw string) error {
	if raw == "" || len(raw) > MaxLinkLength {
		return fmt.Errorf("web: a link must be 1 to %d characters", MaxLinkLength)
	}
	if strings.ContainsAny(raw, "\x00\r\n\t") {
		return fmt.Errorf("web: a link may not contain control characters")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("web: not a link: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("web: only http and https links open on the TV")
	}
	return nil
}

// IsLoopback reports whether raw is an http(s) link to this machine:
// localhost, 127.0.0.0/8 or ::1.
func IsLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// OnLoopback registers fn, called on its own goroutine when the main frame
// of a tab in an app's browser commits a navigation to a loopback address
// (IsLoopback). appID is the app whose browser it is, targetID the tab.
func (m *Manager) OnLoopback(fn func(appID, targetID, url string)) {
	m.mu.Lock()
	m.onLoopback = fn
	m.mu.Unlock()
}

// LaunchAt is Launch opening link instead of the app's own start page (a
// sign-in link in the Browser tile). A running browser is returned as is:
// open the link in it with OpenTab.
func (m *Manager) LaunchAt(ctx context.Context, app config.Application, spec adapters.WebSpec, link string) (applications.Instance, error) {
	if err := CheckLink(link); err != nil {
		return applications.Instance{}, err
	}
	return m.launch(ctx, app, spec, link)
}

// OpenTab opens link in a new foreground tab of app's running browser and
// returns the tab's target id.
func (m *Manager) OpenTab(ctx context.Context, appID, link string) (string, error) {
	if err := CheckLink(link); err != nil {
		return "", err
	}
	b := m.get(appID)
	if b == nil {
		return "", ErrNotRunning
	}
	var out struct {
		TargetID string `json:"targetId"`
	}
	if err := b.conn.Call(ctx, "", "Target.createTarget", map[string]any{"url": link}, &out); err != nil {
		return "", fmt.Errorf("web: open a tab: %w", err)
	}
	return out.TargetID, nil
}

// CloseTab closes one tab of app's browser (its last tab closes the
// browser, as closing its window would).
func (m *Manager) CloseTab(ctx context.Context, appID, targetID string) error {
	b := m.get(appID)
	if b == nil {
		return ErrNotRunning
	}
	return b.conn.Call(ctx, "", "Target.closeTarget", map[string]any{"targetId": targetID}, nil)
}

// Tabs is how many tabs (page targets) app's browser has open.
func (m *Manager) Tabs(appID string) int {
	b := m.get(appID)
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pages)
}

// frameNavigated handles Page.frameNavigated for one page: a main-frame
// navigation to a loopback address is reported to OnLoopback.
func (b *Browser) frameNavigated(sessionID string, params json.RawMessage) {
	var p struct {
		Frame struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
			URL      string `json:"url"`
		} `json:"frame"`
	}
	if json.Unmarshal(params, &p) != nil || p.Frame.ParentID != "" {
		return
	}
	b.mu.Lock()
	pg := b.pages[sessionID]
	b.mu.Unlock()
	if pg == nil || pg.targetID != p.Frame.ID {
		return
	}
	b.reportLoopback(p.Frame.ID, p.Frame.URL)
}

// targetInfoChanged handles Target.targetInfoChanged (the browser announces
// a tab's new address once a navigation commits): a tab's first navigation
// may commit before its own Page events are on, so both signals count.
func (b *Browser) targetInfoChanged(params json.RawMessage) {
	var p struct {
		TargetInfo struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
		} `json:"targetInfo"`
	}
	if json.Unmarshal(params, &p) != nil || p.TargetInfo.Type != "page" {
		return
	}
	b.reportLoopback(p.TargetInfo.TargetID, p.TargetInfo.URL)
}

// loopbackPatterns are the requests Fetch pauses (setupPage): those to this
// machine only, so nothing else a page loads is held or seen.
var loopbackPatterns = []map[string]any{
	{"urlPattern": "http://127.0.0.1*", "requestStage": "Request"},
	{"urlPattern": "http://localhost*", "requestStage": "Request"},
	{"urlPattern": "http://[::1]*", "requestStage": "Request"},
}

// requestPaused handles Fetch.requestPaused (enabled on the browser, so for
// every tab): the request goes on at once, and a tab's main-frame request to
// a loopback address (a redirect hop included) is reported.
func (b *Browser) requestPaused(sessionID string, params json.RawMessage) {
	var p struct {
		RequestID string `json:"requestId"`
		FrameID   string `json:"frameId"`
		Type      string `json:"resourceType"`
		Request   struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if json.Unmarshal(params, &p) != nil || p.RequestID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.conn.Call(ctx, sessionID, "Fetch.continueRequest", map[string]any{"requestId": p.RequestID}, nil); err != nil {
			b.m.log.Warn("web: a paused request could not go on", "app", b.appID, "err", err)
		}
	}()
	if p.Type != "Document" {
		return
	}
	// A tab's main frame has the tab's target id.
	b.mu.Lock()
	main := false
	for _, pg := range b.pages {
		if pg.targetID == p.FrameID {
			main = true
			break
		}
	}
	b.mu.Unlock()
	if main {
		b.reportLoopback(p.FrameID, p.Request.URL)
	}
}

// reportLoopback tells OnLoopback, once per tab and address, that tab
// targetID is at url when url is a loopback address.
func (b *Browser) reportLoopback(targetID, url string) {
	if targetID == "" || !IsLoopback(url) {
		return
	}
	b.mu.Lock()
	if b.loopback == nil {
		b.loopback = map[string]string{}
	}
	seen := b.loopback[targetID] == url
	b.loopback[targetID] = url
	b.mu.Unlock()
	if seen {
		return
	}
	b.m.mu.Lock()
	fn := b.m.onLoopback
	b.m.mu.Unlock()
	if fn != nil {
		go fn(b.appID, targetID, url)
	}
}
