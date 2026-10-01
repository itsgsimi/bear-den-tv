// Tests for apps that are slow to show their window or keep running without
// one (route.go: awaitSlowStart, quitIfHidden, quitThroughPlayer). On the
// reference TV Spotify's first start after installing took longer than
// LaunchObserveTimeout, and its window hides to the tray when closed.

package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
)

// bgLauncher starts Plex HTPC as a process (Instances) whose window shows
// after delay without being brought forward (activate false), and that keeps
// running when its window closes.
type bgLauncher struct {
	desk     *fake.Desktop
	delay    time.Duration
	activate bool

	mu    sync.Mutex
	alive map[string]bool
}

func (l *bgLauncher) Discover(context.Context, string) (applications.Installation, error) {
	return applications.Installation{Installed: true, Version: "1.0", Scope: "user"}, nil
}

func (l *bgLauncher) Launch(_ context.Context, id string, _ []string) (applications.Instance, error) {
	l.mu.Lock()
	l.alive[id] = true
	l.mu.Unlock()
	go func() {
		time.Sleep(l.delay)
		w := l.desk.AddWindow(platform.WindowInfo{PID: 9100, Class: []string{"plexhtpc", "plexhtpc"}})
		if l.activate {
			l.desk.SetActive(w)
		}
	}()
	return applications.Instance{FlatpakID: id, InstanceID: "1", PID: 9100}, nil
}

func (l *bgLauncher) Instances(context.Context) ([]applications.Instance, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []applications.Instance
	for id, on := range l.alive {
		if on {
			out = append(out, applications.Instance{FlatpakID: id, InstanceID: "1", PID: 9100})
		}
	}
	return out, nil
}

func (l *bgLauncher) Kill(context.Context, applications.Instance) error { return nil }

func (l *bgLauncher) exit(id string) {
	l.mu.Lock()
	l.alive[id] = false
	l.mu.Unlock()
}

func setFor(t *testing.T, v *time.Duration, d time.Duration) {
	old := *v
	*v = d
	t.Cleanup(func() { *v = old })
}

func TestASlowFirstStartComesToTheFrontWhenItsWindowShows(t *testing.T) {
	setFor(t, &LaunchObserveTimeout, 150*time.Millisecond)
	setFor(t, &slowStartPoll, 20*time.Millisecond)
	var l *bgLauncher
	h := newHarness(t, func(o *Options) {
		l = &bgLauncher{desk: o.Desktop.(*fake.Desktop), delay: 500 * time.Millisecond, alive: map[string]bool{}}
		o.Launcher = l
	})
	res := h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	if res.Outcome != contract.OutcomeAccepted {
		t.Fatalf("launch: %+v", res)
	}
	// Past LaunchObserveTimeout, before the window: still starting, no error.
	time.Sleep(300 * time.Millisecond)
	if a := appState(h.c.buildState(viewShell), "plex-htpc"); a.LaunchState != "launching" || a.LastError != nil {
		t.Fatalf("while slow: %s %v", a.LaunchState, a.LastError)
	}
	h.eventually("plex brought to the front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	h.eventually("running", func() bool { return appState(h.c.buildState(viewShell), "plex-htpc").LaunchState == "running" })

	// A start whose process exits before any window: failed, saying so.
	l.delay = time.Hour
	h.desk.SetActive(h.shellW)
	for _, w := range mustWindows(h, "plexhtpc") {
		h.desk.RemoveWindow(w)
	}
	l.exit("tv.plex.PlexHTPC")
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	time.Sleep(250 * time.Millisecond)
	l.exit("tv.plex.PlexHTPC")
	h.eventually("failed", func() bool {
		a := appState(h.c.buildState(viewShell), "plex-htpc")
		return a.LaunchState == "failed" && a.LastError != nil && strings.Contains(*a.LastError, "closed before its window showed")
	})
}

func mustWindows(h *harness, class string) []platform.WindowID {
	var out []platform.WindowID
	wins, _ := h.desk.ListWindows(context.Background())
	for _, w := range wins {
		if len(w.Class) > 0 && w.Class[0] == class {
			out = append(out, w.ID)
		}
	}
	return out
}

func TestCloseQuitsAnAppThatHidItsWindow(t *testing.T) {
	setFor(t, &CloseQuitAfter, 50*time.Millisecond)
	var l *bgLauncher
	media := fake.NewMedia()
	player := fake.NewPlayer(clock.Real{}, platform.MediaInfo{Status: "Playing"})
	media.Add("tv.plex.PlexHTPC", player)
	h := newHarness(t, func(o *Options) {
		l = &bgLauncher{desk: o.Desktop.(*fake.Desktop), activate: true, alive: map[string]bool{}}
		o.Launcher = l
		o.Media = media
	})
	player.SetOnQuit(func() { l.exit("tv.plex.PlexHTPC") })
	quits := func() int {
		n := 0
		for _, c := range player.Calls() {
			if c == "Quit" {
				n++
			}
		}
		return n
	}

	// Close from the front: the window goes (hidden), the app keeps
	// running, so it is asked to quit.
	h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	h.eventually("plex in front", func() bool { return strOr(h.c.Target().AppID) == "plex-htpc" })
	expectOutcome(t, h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeDelivered, contract.CodeOK)
	h.eventually("asked to quit", func() bool { return quits() == 1 })

	// Running in the background with no window at all: Close asks it to quit
	// and says so, where it used to say it was not running.
	l.mu.Lock()
	l.alive["tv.plex.PlexHTPC"] = true
	l.mu.Unlock()
	res := h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"}))
	if res.Outcome != contract.OutcomeDelivered || !strings.Contains(res.Message, "running in the background") || quits() != 2 {
		t.Fatalf("close in the background: %+v, quits %d", res, quits())
	}
	// Gone: not running.
	expectOutcome(t, h.submit(h.ctl, h.req("app.close", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeFailed, contract.CodeNoTarget)
}
