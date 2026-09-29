// Widevine for the streaming sites: whether a web app's browser profile
// has the CDM (<profile>/WidevineCdm/*/manifest.json, the one
// machine-checkable sign), and the quiet first run that lets the browser's
// own component updater fetch it into that profile, per user, without
// root. Bear Den never downloads or copies the CDM itself. Measured in an
// ubuntu:24.04 container: Flathub Chromium 154 fetched Widevine 4.10.3050.0
// into a fresh profile 65 s after starting, headless or in a window. Brave
// fetches it only once opted in (brave.widevine_opted_in, which SeedPrefs
// writes before the run); never measured, and its Flatpak may not load a
// CDM from Bear Den's profile (ADR 0013). Spec:
// docs/decisions/0010-web-apps-over-cdp-pipe.md,
// docs/decisions/0011-per-user-flathub-installs.md and
// docs/decisions/0013-brave-as-a-browser-choice.md.

package web

import (
	"context"
	"fmt"
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

// WidevineReady reports whether the profile holds a Widevine CDM that
// Chromium's component updater installed.
func WidevineReady(profile string) bool {
	m, _ := filepath.Glob(filepath.Join(profile, "WidevineCdm", "*", "manifest.json"))
	return len(m) > 0
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
}

// Ready reports whether appID's profile in the browser browserID (a
// Flatpak id from the adapter table) has Widevine.
func (w *Widevine) Ready(appID, browserID string) bool {
	b, ok := adapters.BrowserByFlatpakID(browserID)
	if !ok {
		return false
	}
	p, err := ProfileDir(w.DataHome, b, appID)
	return err == nil && WidevineReady(p)
}

// Prepare starts the browser browserID headless on appID's profile (its
// prefs seeded first) and waits until Widevine appears, the timeout passes
// or ctx ends; then the browser is stopped. It reports whether the profile
// has Widevine afterwards.
func (w *Widevine) Prepare(ctx context.Context, appID, browserID string) (bool, error) {
	b, ok := adapters.BrowserByFlatpakID(browserID)
	if !ok {
		return false, fmt.Errorf("web: %q is not a browser Bear Den runs", browserID)
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
