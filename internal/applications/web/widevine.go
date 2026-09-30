// Widevine for the streaming sites, per the browser table
// (adapters.BrowserInfo):
//   - Google Chrome (BundledWidevine) ships the CDM inside its Flatpak:
//     Flathub's apply_extra unpacks Google's package, WidevineCdm/ included,
//     into files/extra. Ready means that bundled manifest.json is there in
//     the installed Flatpak (per user or system-wide); there is no quiet run,
//     and SeedPrefs keeps the browser on that copy (BlockProfileWidevine).
//   - Brave fetches it into the profile (<profile>/WidevineCdm/*/manifest.json,
//     the one machine-checkable sign) once opted in (brave.widevine_opted_in,
//     which SeedPrefs writes), so a quiet headless first run lets its own
//     component updater fetch it, per user, without root. Never measured, and
//     its Flatpak may not load a CDM from Bear Den's profile (ADR 0013).
// Bear Den never downloads or copies the CDM itself. Spec:
// docs/decisions/0010-web-apps-over-cdp-pipe.md,
// docs/decisions/0011-per-user-flathub-installs.md,
// docs/decisions/0013-brave-as-a-browser-choice.md and
// docs/decisions/0014-google-chrome-for-streaming-brave-for-browser.md.

package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"bear-den-tv/internal/applications/adapters"
)

// Defaults for Widevine.Prepare.
const (
	DefaultPrepareTimeout = 5 * time.Minute
	defaultPreparePoll    = 5 * time.Second
	prepareStopGrace      = 5 * time.Second
)

// PrepareArgs is the browser's argument list for the quiet first run: the
// app's own profile, headless (no window, no DevTools channel of any kind),
// no first-run questions, a blank page. Nothing else.
func PrepareArgs(profile string) []string {
	return []string{
		"--user-data-dir=" + profile,
		"--headless=new",
		"--no-first-run",
		"--no-default-browser-check",
		BlankPage,
	}
}

// WidevineReady reports whether the profile holds a Widevine CDM that the
// browser's component updater installed.
func WidevineReady(profile string) bool {
	m, _ := filepath.Glob(filepath.Join(profile, "WidevineCdm", "*", "manifest.json"))
	return len(m) > 0
}

// SystemFlatpakDir is where system-wide Flatpaks live.
const SystemFlatpakDir = "/var/lib/flatpak"

// BundledWidevinePath is where a Flatpak that bundles the CDM keeps its
// manifest under one Flatpak installation (dir: $XDG_DATA_HOME/flatpak or
// /var/lib/flatpak): the active deployment's files/extra/WidevineCdm.
func BundledWidevinePath(dir, flatpakID string) string {
	return filepath.Join(dir, "app", flatpakID, "current", "active", "files", "extra", "WidevineCdm", "manifest.json")
}

// flatpakDirs are the Flatpak installations checked for a bundled CDM:
// FlatpakDirs, else the user's ($XDG_DATA_HOME/flatpak) and the system's.
func (w *Widevine) flatpakDirs() []string {
	if len(w.FlatpakDirs) > 0 {
		return w.FlatpakDirs
	}
	return []string{filepath.Join(w.DataHome, "flatpak"), SystemFlatpakDir}
}

// bundledReady reports whether an installed copy of the Flatpak carries
// its own CDM.
func (w *Widevine) bundledReady(flatpakID string) bool {
	for _, dir := range w.flatpakDirs() {
		if fi, err := os.Stat(BundledWidevinePath(dir, flatpakID)); err == nil && fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// Widevine checks and prepares the streaming sites' profiles.
type Widevine struct {
	// DataHome is $XDG_DATA_HOME (absolute), as for the Manager.
	DataHome string
	// Starter starts the browser (FlatpakStarter in production).
	Starter Starter
	// Timeout bounds one quiet run (DefaultPrepareTimeout); Poll is how
	// often the profile is checked (5 s).
	Timeout time.Duration
	Poll    time.Duration
	// Signal stops the run's process group; nil uses kill(-pid).
	Signal func(pid int, sig syscall.Signal) error
	// FlatpakDirs are the Flatpak installations to look in for a bundled
	// CDM (tests); nil means $XDG_DATA_HOME/flatpak and /var/lib/flatpak.
	FlatpakDirs []string
}

// Ready reports whether appID can play protected video in the browser
// browserID (a Flatpak id from the adapter table): the browser's bundled
// CDM is installed, or appID's profile has one.
func (w *Widevine) Ready(appID, browserID string) bool {
	b, ok := adapters.BrowserByFlatpakID(browserID)
	if !ok {
		return false
	}
	if b.BundledWidevine {
		return w.bundledReady(b.FlatpakID)
	}
	p, err := ProfileDir(w.DataHome, b, appID)
	return err == nil && WidevineReady(p)
}

// Prepare starts the browser browserID headless on appID's profile (its
// prefs seeded first) and waits until Widevine appears, the timeout passes
// or ctx ends; then the browser is stopped. It reports whether the profile
// has Widevine afterwards. A browser that bundles the CDM is never started:
// its answer is Ready's.
func (w *Widevine) Prepare(ctx context.Context, appID, browserID string) (bool, error) {
	b, ok := adapters.BrowserByFlatpakID(browserID)
	if !ok {
		return false, fmt.Errorf("web: %q is not a browser Bear Den runs", browserID)
	}
	if b.BundledWidevine {
		return w.bundledReady(b.FlatpakID), nil
	}
	profile, err := ProfileDir(w.DataHome, b, appID)
	if err != nil {
		return false, err
	}
	if WidevineReady(profile) {
		return true, nil
	}
	if _, err := SeedPrefs(w.DataHome, b, appID); err != nil {
		return false, err
	}
	timeout, poll := w.Timeout, w.Poll
	if timeout <= 0 {
		timeout = DefaultPrepareTimeout
	}
	if poll <= 0 {
		poll = defaultPreparePoll
	}
	proc, err := w.Starter.Start(ctx, b, PrepareArgs(profile), nil)
	if err != nil {
		return false, err
	}
	defer w.stop(proc)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return WidevineReady(profile), ctx.Err()
		case <-deadline.C:
			return WidevineReady(profile), nil
		case <-proc.Done():
			return WidevineReady(profile), nil
		case <-tick.C:
			if WidevineReady(profile) {
				return true, nil
			}
		}
	}
}

// stop ends the quiet run: SIGTERM to its process group, SIGKILL after a
// grace period.
func (w *Widevine) stop(proc Process) {
	signal := w.Signal
	if signal == nil {
		signal = func(pid int, sig syscall.Signal) error { return syscall.Kill(-pid, sig) }
	}
	select {
	case <-proc.Done():
		return
	default:
	}
	_ = signal(proc.PID(), syscall.SIGTERM)
	select {
	case <-proc.Done():
	case <-time.After(prepareStopGrace):
		_ = signal(proc.PID(), syscall.SIGKILL)
	}
}
