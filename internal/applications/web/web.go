// Package web runs Bear Den's web apps (streaming sites and the Browser
// tile): Chromium from Flathub, full screen, one profile per app under
// $XDG_DATA_HOME/bear-den-tv/web/<app-id>, controlled over the DevTools
// protocol on --remote-debugging-pipe, with the navigation script
// (apps/web-nav) injected into an isolated world of every page. Phones never
// reach this package with anything but named actions: page addresses come
// from the owner's config.json (contracts/config.md rule 11), the script
// and its hints are embedded in the binary, and the only trusted input it
// sends Chromium is a click inside the page's viewport, a key from
// AllowedKeys, the phone's text into a focused field, or a pointer move,
// click or wheel. Decision and security model:
// docs/decisions/0010-web-apps-over-cdp-pipe.md, docs/security.md.
package web

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

// BlankPage is what the Browser opens without a start page: Chromium's own
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

// ProfileDir is the app's own Chromium profile: cookies and sign-ins stay
// there, apart from every other web app and from any personal browser.
func ProfileDir(dataHome, appID string) (string, error) {
	if !appIDPattern.MatchString(appID) {
		return "", fmt.Errorf("web: invalid application id %q", appID)
	}
	if dataHome == "" || !filepath.IsAbs(dataHome) {
		return "", errors.New("web: the data directory must be an absolute path")
	}
	return filepath.Join(dataHome, "bear-den-tv", "web", appID), nil
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

// ChromiumArgs is Chromium's argument list for one web app: its own profile,
// the DevTools pipe (never a port), no first-run questions, the adapter's
// window class, and either a full-screen app window on the page or the
// ordinary browser, maximized, on the start page. The page address is last
// and, for app mode, glued to --app= so it can never read as a flag.
func ChromiumArgs(spec adapters.WebSpec, profile, url string) []string {
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
// pipe) with Flathub Chromium's id and nothing else before Chromium's own
// arguments.
func FlatpakArgv(chromiumArgs []string) []string {
	return append([]string{"flatpak", "run", adapters.ChromiumFlatpakID}, chromiumArgs...)
}
