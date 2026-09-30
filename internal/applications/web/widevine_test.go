// Tests for the Widevine check and the quiet first run (widevine.go): the
// exact browser arguments (headless, the app's own profile, no DevTools
// port or pipe), stopping the browser once the CDM appears, on timeout and on
// cancel; and Google Chrome's bundled copy, for which nothing runs.

package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
)

type wvProc struct {
	done chan struct{}
	once sync.Once
}

func (p *wvProc) PID() int              { return 4242 }
func (p *wvProc) Done() <-chan struct{} { return p.done }
func (p *wvProc) end()                  { p.once.Do(func() { close(p.done) }) }

// wvStarter is a browser whose component updater writes the CDM after
// `after` (never when zero).
type wvStarter struct {
	after    time.Duration
	mu       sync.Mutex
	args     [][]string
	browsers []string
	proc     *wvProc
}

// cr is the browser that fetches Widevine into its profile with a quiet
// run: Brave (Google Chrome bundles it instead).
const cr = adapters.BraveFlatpakID

func (s *wvStarter) Start(_ context.Context, b adapters.BrowserInfo, args []string, extra []*os.File) (Process, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.browsers = append(s.browsers, b.FlatpakID)
	if len(extra) != 0 {
		return nil, fmt.Errorf("the quiet run got %d extra fds", len(extra))
	}
	s.args = append(s.args, args)
	s.proc = &wvProc{done: make(chan struct{})}
	if s.after > 0 {
		profile := args[0][len("--user-data-dir="):]
		go func() {
			time.Sleep(s.after)
			dir := filepath.Join(profile, "WidevineCdm", "4.10.3050.0")
			_ = os.MkdirAll(dir, 0o700)
			_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"WidevineCdm"}`), 0o600)
		}()
	}
	return s.proc, nil
}

func newWidevine(t *testing.T, s *wvStarter, timeout time.Duration) (*Widevine, *[]syscall.Signal) {
	var mu sync.Mutex
	sigs := []syscall.Signal{}
	return &Widevine{
		DataHome: t.TempDir(), Starter: s, Timeout: timeout, Poll: 5 * time.Millisecond,
		Signal: func(pid int, sig syscall.Signal) error {
			mu.Lock()
			sigs = append(sigs, sig)
			mu.Unlock()
			if pid != 4242 {
				t.Errorf("signalled pid %d", pid)
			}
			s.proc.end()
			return nil
		},
	}, &sigs
}

func TestPrepareRunsHeadlessUntilWidevineAppears(t *testing.T) {
	s := &wvStarter{after: 30 * time.Millisecond}
	w, sigs := newWidevine(t, s, 5*time.Second)
	if w.Ready("netflix", cr) {
		t.Fatal("ready before anything ran")
	}
	ok, err := w.Prepare(context.Background(), "netflix", cr)
	if err != nil || !ok || !w.Ready("netflix", cr) {
		t.Fatalf("Prepare = %v, %v; ready %v", ok, err, w.Ready("netflix", cr))
	}
	profile := filepath.Join(w.DataHome, "bear-den-tv", "web-brave", "netflix")
	want := fmt.Sprint([][]string{{"--user-data-dir=" + profile, "--headless=new", "--no-first-run", "--no-default-browser-check", "about:blank"}})
	if fmt.Sprint(s.args) != want {
		t.Fatalf("args %v\nwant %v", s.args, want)
	}
	if fmt.Sprint(*sigs) != fmt.Sprint([]syscall.Signal{syscall.SIGTERM}) {
		t.Fatalf("the browser was not stopped once ready: %v", *sigs)
	}
	if st, err := os.Stat(profile); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("profile %v %v", st, err)
	}
	// Ready already: nothing runs again.
	if ok, _ := w.Prepare(context.Background(), "netflix", cr); !ok || len(s.args) != 1 {
		t.Fatalf("ran again: %v", s.args)
	}
	// Other profiles are separate.
	if w.Ready("hulu", cr) {
		t.Fatal("hulu shares netflix's profile")
	}
}

func TestPrepareGivesUpAndStops(t *testing.T) {
	s := &wvStarter{} // never fetches
	w, sigs := newWidevine(t, s, 40*time.Millisecond)
	ok, err := w.Prepare(context.Background(), "hulu", cr)
	if ok || err != nil {
		t.Fatalf("Prepare = %v, %v", ok, err)
	}
	if len(*sigs) == 0 {
		t.Fatal("the browser was left running after the timeout")
	}
	s2 := &wvStarter{}
	w2, sigs2 := newWidevine(t, s2, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if ok, err := w2.Prepare(ctx, "disney-plus", cr); ok || err == nil {
		t.Fatalf("cancelled Prepare = %v, %v", ok, err)
	}
	if len(*sigs2) == 0 {
		t.Fatal("the browser was left running after a cancel")
	}
	if _, err := w2.Prepare(context.Background(), "../escape", cr); err == nil {
		t.Fatal("a bad app id reached a path")
	}
}

// Brave: the quiet run uses Brave's own profile root (never Chrome's
// profile) and finds its Widevine opt-in already in Local State, which
// Bear Den wrote there first; a browser outside the table never starts.
func TestPrepareInBraveSeedsTheOptInFirst(t *testing.T) {
	s := &wvStarter{after: 30 * time.Millisecond}
	w, _ := newWidevine(t, s, 5*time.Second)
	br := adapters.BraveFlatpakID
	ok, err := w.Prepare(context.Background(), "netflix", br)
	if err != nil || !ok || !w.Ready("netflix", br) {
		t.Fatalf("Prepare = %v, %v", ok, err)
	}
	if w.Ready("netflix", adapters.ChromeFlatpakID) {
		t.Fatal("Brave's profile counted for Chrome")
	}
	profile := filepath.Join(w.DataHome, "bear-den-tv", "web-brave", "netflix")
	if fmt.Sprint(s.browsers) != "["+br+"]" || s.args[0][0] != "--user-data-dir="+profile {
		t.Fatalf("browsers %v args %v", s.browsers, s.args)
	}
	raw, err := os.ReadFile(filepath.Join(profile, "Local State"))
	if err != nil || !strings.Contains(string(raw), `"widevine_opted_in":true`) {
		t.Fatalf("Local State %s %v", raw, err)
	}
	if _, err := w.Prepare(context.Background(), "hulu", "org.example.Browser"); err == nil || len(s.args) != 1 {
		t.Fatalf("an unknown browser ran: %v %v", err, s.args)
	}
}

// Google Chrome bundles the CDM: ready means the installed Flatpak carries
// files/extra/WidevineCdm/manifest.json (per user or system-wide), the
// same for every streaming site; nothing is ever started for it, and a
// copy in a profile does not count.
func TestChromeWidevineIsTheBundledCopy(t *testing.T) {
	s := &wvStarter{after: time.Millisecond}
	w, _ := newWidevine(t, s, time.Second)
	user, system := filepath.Join(w.DataHome, "flatpak"), t.TempDir()
	w.FlatpakDirs = []string{user, system}
	ch := adapters.ChromeFlatpakID
	// A component-updated copy in the profile is not Chrome's answer.
	stray := filepath.Join(w.DataHome, "bear-den-tv", "web-chrome", "netflix", "WidevineCdm", "4.10.3050.0")
	if err := os.MkdirAll(stray, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stray, "manifest.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if w.Ready("netflix", ch) {
		t.Fatal("ready without Chrome installed")
	}
	if ok, err := w.Prepare(context.Background(), "netflix", ch); ok || err != nil || len(s.args) != 0 {
		t.Fatalf("Prepare = %v, %v; started %v", ok, err, s.args)
	}
	for _, dir := range []string{system, user} {
		m := BundledWidevinePath(dir, ch)
		if err := os.MkdirAll(filepath.Dir(m), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(m, []byte(`{"name":"WidevineCdm"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, app := range []string{"netflix", "disney-plus", "hulu"} {
			if !w.Ready(app, ch) {
				t.Fatalf("%s not ready with %s", app, m)
			}
			if ok, err := w.Prepare(context.Background(), app, ch); !ok || err != nil {
				t.Fatalf("Prepare %s = %v, %v", app, ok, err)
			}
		}
		if len(s.args) != 0 {
			t.Fatalf("Chrome was started: %v", s.args)
		}
		_ = os.Remove(m)
	}
	if want := filepath.Join(user, "app", ch, "current", "active", "files", "extra", "WidevineCdm", "manifest.json"); BundledWidevinePath(user, ch) != want {
		t.Fatalf("bundled path %s", BundledWidevinePath(user, ch))
	}
	// Without FlatpakDirs: the user's installation under DataHome, then the system's.
	w.FlatpakDirs = nil
	if got := w.flatpakDirs(); fmt.Sprint(got) != fmt.Sprint([]string{user, SystemFlatpakDir}) {
		t.Fatalf("flatpak dirs %v", got)
	}
}
