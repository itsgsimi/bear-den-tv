// The Browser tile's start page: when the Browser opens without a start page
// of its own (config applications[].web absent), Bear Den writes start.html
// (embedded) into the browser's own profile root,
// $XDG_DATA_HOME/bear-den-tv/<profile root>/start/index.html (0600 in a
// 0700 folder, through a temporary file and a rename, never through a
// symbolic link), and opens it as a file:// page: the profile root is the
// one folder `flatpak run` grants the browser. The page loads nothing from
// the network. It shows a card per streaming site that is on and installed
// (StartCard, from the coordinator), a note about the phone's touchpad and
// an address box that works with the phone's text entry. A card is a link
// to "#open-<app-id>": the coordinator sees that same-document navigation
// of its own start page over the DevTools pipe (OpenRequest) and opens that
// site as its own app. The navigation script drives the page like any
// other. Spec: docs/operations.md "Streaming sites and the Browser".

package web

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"bear-den-tv/internal/applications/adapters"
)

//go:embed start.html
var startHTML string

var startTemplate = template.Must(template.New("start").Parse(startHTML))

// StartFragment starts the fragment of a start page card's link: "#open-" and
// the application id.
const StartFragment = "#open-"

// StartCard is one streaming site on the start page: its config id and the
// label the owner gave it.
type StartCard struct {
	AppID string
	Label string
}

// StartPagePath is browser b's start page, under its own profile root.
func StartPagePath(dataHome string, b adapters.BrowserInfo) (string, error) {
	root, err := ProfileRoot(dataHome, b)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "start", "index.html"), nil
}

// StartPageURL is the file:// address of a start page path.
func StartPageURL(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// WriteStartPage writes browser b's start page with cards (ids that are not
// application ids are left out) and returns its file:// address.
func WriteStartPage(dataHome string, b adapters.BrowserInfo, cards []StartCard) (string, error) {
	path, err := StartPagePath(dataHome, b)
	if err != nil {
		return "", err
	}
	root := filepath.Dir(filepath.Dir(path))
	if err := ownedDir(root); err != nil {
		return "", err
	}
	if err := ownedDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	if fi, err := os.Lstat(path); err == nil && !fi.Mode().IsRegular() {
		return "", fmt.Errorf("web: %s is not a file Bear Den owns", path)
	}
	var ok []StartCard
	for _, c := range cards {
		if appIDPattern.MatchString(c.AppID) && strings.TrimSpace(c.Label) != "" {
			ok = append(ok, c)
		}
	}
	var buf bytes.Buffer
	if err := startTemplate.Execute(&buf, struct{ Cards []StartCard }{ok}); err != nil {
		return "", err
	}
	tmp := path + ".bdtv-tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return "", fmt.Errorf("web: start page: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("web: start page: %w", err)
	}
	return StartPageURL(path), nil
}

// OpenRequest reports whether navURL is the start page startURL with a
// card's fragment, and which application the card names. Anything else (another
// page, another fragment, a malformed id) is not a request.
func OpenRequest(startURL, navURL string) (string, bool) {
	if startURL == "" {
		return "", false
	}
	rest, ok := strings.CutPrefix(navURL, startURL+StartFragment)
	if !ok || !appIDPattern.MatchString(rest) {
		return "", false
	}
	return rest, true
}
