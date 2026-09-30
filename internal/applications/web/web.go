// Package web runs Bear Den's web apps (streaming sites and the Browser
// tile): a Chromium-based browser from Flathub (Google Chrome for the
// streaming sites and Brave for the Browser tile by default; the adapter
// table's browsers, config apps.browser and apps.streaming_browser), full
// screen, one profile per app and browser under
// $XDG_DATA_HOME/bear-den-tv/<profile root>/<app-id> (web-chrome/ for
// Chrome, web-brave/ for Brave; web/ was retired Chromium's and is left
// alone; prefs.go seeds a browser's own settings there and nowhere else),
// each app its own window class and process tree, controlled over the DevTools
// protocol on --remote-debugging-pipe, with the navigation script
// (apps/web-nav) injected into an isolated world of every page. Phones never
// reach this package with anything but named actions: page addresses come
// from the owner's config.json (contracts/config.md rule 11), the script
// and its hints are embedded in the binary, and the only trusted input it
// sends the browser is a click inside the page's viewport, a key from
// AllowedKeys, the phone's text into a focused field, or a pointer move,
// click or wheel. Decision and security model:
// docs/decisions/0010-web-apps-over-cdp-pipe.md,
// docs/decisions/0013-brave-as-a-browser-choice.md,
// docs/decisions/0014-google-chrome-for-streaming-brave-for-browser.md,
// docs/security.md.
package web

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

// BlankPage is what the Browser opens without a start page: the browser's own
// empty page, with no search engine and nothing loaded from the network.
const BlankPage = "about:blank"

// WorldName is the isolated world the navigation script runs in; the page's
// own scripts cannot reach it, and the report binding exists only there.
const WorldName = "bearden"

// ReportBinding is the function the script calls to push page status.
const ReportBinding = "__bdtvReport"

// AllowedKeys is the closed set of keys Bear Den presses for a page, with
// the DevTools key description of each (contracts/web-hints.schema.json
// $defs/key). A page or hint asking for anything else is refused.
var AllowedKeys = map[string]KeyInfo{
	" ":          {Key: " ", Code: "Space", VK: 32, Text: " "},
	"k":          {Key: "k", Code: "KeyK", VK: 75, Text: "k"},
	"j":          {Key: "j", Code: "KeyJ", VK: 74, Text: "j"},
	"l":          {Key: "l", Code: "KeyL", VK: 76, Text: "l"},
	"f":          {Key: "f", Code: "KeyF", VK: 70, Text: "f"},
	"ArrowLeft":  {Key: "ArrowLeft", Code: "ArrowLeft", VK: 37},
	"ArrowRight": {Key: "ArrowRight", Code: "ArrowRight", VK: 39},
	"Escape":     {Key: "Escape", Code: "Escape", VK: 27},
	"Enter":      {Key: "Enter", Code: "Enter", VK: 13, Text: "\r"},
}

// KeyInfo describes one key for Input.dispatchKeyEvent.
type KeyInfo struct {
	Key  string
	Code string
	VK   int
	Text string
}

// MaxKeys bounds how many keys one action may press (a 5-minute seek at 10 s
// a press is 30).
const MaxKeys = 30

var appIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ProfileRoot is the folder holding browser b's profiles,
// $XDG_DATA_HOME/bear-den-tv/<b.ProfileRoot>: "web-chrome" for Google
// Chrome, "web-brave" for Brave ("web" held retired Chromium's), so no
// profile is ever opened by two browsers. A path with ":" is refused: `flatpak run
// --filesystem=` would read what follows as an access mode.
func ProfileRoot(dataHome string, b adapters.BrowserInfo) (string, error) {
	if dataHome == "" || !filepath.IsAbs(dataHome) {
		return "", errors.New("web: the data directory must be an absolute path")
	}
	if strings.Contains(dataHome, ":") {
		return "", errors.New("web: the data directory may not contain \":\"")
	}
	if !appIDPattern.MatchString(b.ProfileRoot) {
		return "", fmt.Errorf("web: %q is not a browser Bear Den runs", b.FlatpakID)
	}
	return filepath.Join(dataHome, "bear-den-tv", b.ProfileRoot), nil
}

// ProfileDir is the app's own profile in browser b: cookies and sign-ins
// stay there, apart from every other web app and from any personal browser.
func ProfileDir(dataHome string, b adapters.BrowserInfo, appID string) (string, error) {
	if !appIDPattern.MatchString(appID) {
		return "", fmt.Errorf("web: invalid application id %q", appID)
	}
	root, err := ProfileRoot(dataHome, b)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, appID), nil
}

// BrowserOf is the browser a web app's config row runs in: its
// launch.app_id, which config rule 3 ties to apps.browser (the Browser
// tile) or apps.streaming_browser (the streaming sites). Anything outside
// the adapter table's browsers is refused.
func BrowserOf(app config.Application) (adapters.BrowserInfo, error) {
	b, ok := adapters.BrowserByFlatpakID(app.Launch.AppID)
	if !ok {
		return adapters.BrowserInfo{}, fmt.Errorf("web: %q is not a browser Bear Den runs", app.Launch.AppID)
	}
	return b, nil
}

// StartURL returns the page an app opens, re-checked against rule 11 even
// though config validation already did: "" (browser) becomes BlankPage.
func StartURL(app config.Application) (string, error) {
	spec, ok := config.DefaultAdapters[app.Adapter]
	if !ok || spec.Web == nil {
		return "", fmt.Errorf("web: %s is not a web adapter", app.Adapter)
	}
	raw := app.WebURL()
	if raw == "" {
		if spec.Web.URLRequired {
			return "", fmt.Errorf("web: %s needs a page address", app.Adapter)
		}
		return BlankPage, nil
	}
	if err := config.CheckWebURL(raw, spec.Web.Hosts); err != nil {
		return "", fmt.Errorf("web: page address %w", err)
	}
	return raw, nil
}

// BrowserArgs is the browser's argument list for one web app (the same for
// Google Chrome and Brave, both Chromium-based: Brave's help center lists
// every switch used here; Brave's Flatpak wrapper appends
// --no-default-browser-check itself): its own profile, the DevTools pipe
// (never a port), no first-run questions, the adapter's window class (its
// own WM_CLASS, so each streaming site is its own window even though they
// share one Chrome install), and either a full-screen app window on the
// page or the ordinary browser, maximized, on the start page. The page
// address is last and, for app mode, glued to --app= so it can never read
// as a flag.
func BrowserArgs(spec adapters.WebSpec, profile, url string) []string {
	args := []string{
		"--user-data-dir=" + profile,
		"--remote-debugging-pipe",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=" + spec.Class,
	}
	if spec.Mode == adapters.WebModeApp {
		return append(args, "--start-fullscreen", "--app="+url)
	}
	return append(args, "--start-maximized", url)
}

// FlatpakArgv is the full command line through Flatpak: `flatpak run` in
// the foreground (it execs bubblewrap and keeps inherited fds 3 and 4, the
// pipe), for a browser whose Flatpak cannot see $XDG_DATA_HOME
// (GrantProfileRoot) access to its own profile root and nothing more, for
// this run only, then the browser's Flatpak id and its own arguments.
// A browser outside the adapter table is refused.
func FlatpakArgv(dataHome string, b adapters.BrowserInfo, chromiumArgs []string) ([]string, error) {
	known, ok := adapters.BrowserByFlatpakID(b.FlatpakID)
	if !ok || known.Name != b.Name {
		return nil, fmt.Errorf("web: %q is not a browser Bear Den runs", b.FlatpakID)
	}
	argv := []string{"flatpak", "run"}
	if known.GrantProfileRoot {
		root, err := ProfileRoot(dataHome, known)
		if err != nil {
			return nil, err
		}
		argv = append(argv, "--filesystem="+root)
	}
	argv = append(argv, known.FlatpakID)
	return append(argv, chromiumArgs...), nil
}
