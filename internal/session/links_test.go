// Tests for sign-ins on the TV (links.go, IPC link.open): a link an app
// opens goes to the Browser tile only while Bear Den or one of its apps is in
// front; the app that asked stays open while the Browser opens; the page
// handing back to a loopback address closes that tab and returns to the app.
// The web manager is a fake (its link side: linkFakeWeb); the real one is
// driven against Chromium in internal/applications/web/links_test.go.

package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/shellipc"
)

// linkFakeWeb adds the link side to fakeWeb.
type linkFakeWeb struct {
	*fakeWeb
	lmu        sync.Mutex
	launchedAt []string // links browsers were started on
	tabs       []string // links opened as tabs
	closed     []string // tabs closed
	onLoopback func(appID, targetID, url string)
}

func (l *linkFakeWeb) LaunchAt(ctx context.Context, app config.Application, spec adapters.WebSpec, link string) (applications.Instance, error) {
	l.lmu.Lock()
	l.launchedAt = append(l.launchedAt, link)
	l.lmu.Unlock()
	return l.fakeWeb.Launch(ctx, app, spec)
}

func (l *linkFakeWeb) OpenTab(_ context.Context, _ string, link string) (string, error) {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	l.tabs = append(l.tabs, link)
	return "tab-" + string(rune('0'+len(l.tabs))), nil
}

func (l *linkFakeWeb) CloseTab(_ context.Context, _ string, targetID string) error {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	l.closed = append(l.closed, targetID)
	return nil
}

func (l *linkFakeWeb) OnLoopback(fn func(appID, targetID, url string)) {
	l.lmu.Lock()
	l.onLoopback = fn
	l.lmu.Unlock()
}

// handBack plays the browser reporting a tab at a loopback address.
func (l *linkFakeWeb) handBack(appID, targetID string) {
	l.lmu.Lock()
	fn := l.onLoopback
	l.lmu.Unlock()
	fn(appID, targetID, "http://127.0.0.1:4381/login?code=x")
}

func (l *linkFakeWeb) seen() (launchedAt, tabs, closed []string) {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	return append([]string(nil), l.launchedAt...), append([]string(nil), l.tabs...), append([]string(nil), l.closed...)
}

// openURL is `bear-den-tv open-url`: link.open from a cli client.
func openURL(h *harness, link string) (handled bool, reason string) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := shellipc.Dial(ctx, h.sock, shellipc.ClientCLI, "test")
	if err != nil {
		h.t.Fatal(err)
	}
	defer conn.Close()
	id := "link-" + randomID()
	if err := conn.Send(shellipc.LinkOpen{Type: shellipc.TypeLinkOpen, RequestID: id, URL: link}); err != nil {
		h.t.Fatal(err)
	}
	res, err := conn.Result(id, 5*time.Second)
	if err != nil || !res.OK {
		h.t.Fatalf("link.open: %+v %v", res, err)
	}
	data, _ := res.Data.(map[string]any)
	reason, _ = data["reason"].(string)
	return data["handled"] == true, reason
}

func linkHarness(t *testing.T) (*harness, *linkFakeWeb) {
	t.Helper()
	old := signInSettle
	signInSettle = 10 * time.Millisecond
	t.Cleanup(func() { signInSettle = old })
	var lw *linkFakeWeb
	h, _, _ := webHarness(t, func(o *Options) {
		lw = &linkFakeWeb{fakeWeb: o.Web.(*fakeWeb)}
		o.Web = lw
	})
	h.eventually("discovery", func() bool { return appState(h.c.buildState(viewShell), "browser").Installed })
	return h, lw
}

func TestSignInLinkOpensInTheBrowserAndReturnsToTheApp(t *testing.T) {
	h, lw := linkHarness(t)
	const link = "https://accounts.example.org/en/login?continue=x"

	// Another window in front (the owner on the desktop): the desktop's own
	// browser takes it.
	other := h.desk.AddWindow(platform.WindowInfo{PID: 4242, Class: []string{"thunar", "Thunar"}})
	h.desk.SetActive(other)
	h.eventually("the desktop in front", func() bool { return h.c.Target().Kind == "unknown" })
	if handled, reason := openURL(h, link); handled || reason != "Bear Den is not in front" {
		t.Fatalf("with the desktop in front: handled=%v %q", handled, reason)
	}
	h.desk.SetActive(h.shellW)
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	if handled, reason := openURL(h, "file:///etc/passwd"); handled || reason != "not a web link" {
		t.Fatalf("a file link: handled=%v %q", handled, reason)
	}

	// An app in front asks: the Browser starts on the link, the app stays open.
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.eventually("plex in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	if handled, reason := openURL(h, link); !handled {
		t.Fatalf("not handled: %q", reason)
	}
	h.eventually("the Browser in front", func() bool { return strOr(h.c.Target().AppID) == "browser" })
	if launchedAt, _, _ := lw.seen(); len(launchedAt) != 1 || launchedAt[0] != link {
		t.Fatalf("the Browser started on %v", launchedAt)
	}
	time.Sleep(3 * closeFollowPoll)
	if n := h.windowsOf("plexhtpc"); n != 1 {
		t.Fatalf("the app waiting for its sign-in has %d windows, want 1", n)
	}

	// The page hands back: its tab closes and the app comes back.
	lw.handBack("browser", "page-1")
	h.eventually("plex back in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	if _, _, closed := lw.seen(); len(closed) != 1 || closed[0] != "page-1" {
		t.Fatalf("closed tabs %v", closed)
	}
	h.eventually("a toast", func() bool {
		for _, n := range h.c.buildState(viewShell).Notifications {
			if n.Text == "Signed in. Back to Plex." && n.Kind == "success" {
				return true
			}
		}
		return false
	})
	// A second hand-back with nothing pending changes nothing.
	lw.handBack("browser", "page-1")
	time.Sleep(50 * time.Millisecond)
	if _, _, closed := lw.seen(); len(closed) != 1 {
		t.Fatalf("closed tabs %v after a stray hand-back", closed)
	}
}

func TestSignInLinkOpensAsATabInARunningBrowser(t *testing.T) {
	h, lw := linkHarness(t)
	launchWeb(h, "browser")
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "youtube"}))
	h.eventually("youtube in front", func() bool { return strOr(h.c.Target().AppID) == "youtube" })
	if handled, reason := openURL(h, "https://accounts.example.org/login"); !handled {
		t.Fatalf("not handled: %q", reason)
	}
	h.eventually("the Browser in front", func() bool { return strOr(h.c.Target().AppID) == "browser" })
	launchedAt, tabs, _ := lw.seen()
	if len(launchedAt) != 0 || len(tabs) != 1 || !strings.HasPrefix(tabs[0], "https://accounts.example.org/") {
		t.Fatalf("started on %v, tabs %v", launchedAt, tabs)
	}
	if n := h.windowsOf("vacuumtube"); n != 1 {
		t.Fatalf("the app waiting for its sign-in has %d windows, want 1", n)
	}
	// Another tab reaching a loopback address is not the sign-in.
	lw.handBack("browser", "some-other-tab")
	time.Sleep(50 * time.Millisecond)
	if _, _, closed := lw.seen(); len(closed) != 0 {
		t.Fatalf("closed %v", closed)
	}
	lw.handBack("browser", "tab-1")
	h.eventually("youtube back in front", func() bool { return strOr(h.c.Target().AppID) == "youtube" })
	if _, _, closed := lw.seen(); len(closed) != 1 || closed[0] != "tab-1" {
		t.Fatalf("closed %v", closed)
	}
}
