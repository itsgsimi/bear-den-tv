// Package autostart owns the user's XDG autostart entry for Bear Den TV
// (docs/operations.md → Autostart): where it lives, which start script it
// runs, what it says, and writing, removing and checking it. Its caller is
// `bear-den-tv autostart enable|disable|status` (cmd/bear-den-tv/autostart.go);
// `bear-den-tv shortcut` reuses StartScript and ExecQuote. Nothing here runs
// without the owner asking.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File is the XDG autostart entry the desktop session runs at login:
// $XDG_CONFIG_HOME/autostart/bear-den-tv.desktop, else
// ~/.config/autostart/bear-den-tv.desktop.
func File() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "bear-den-tv.desktop")
}

// StartScript locates start-session.sh for this binary (os.Executable with
// symlinks resolved); see StartScriptFor.
func StartScript() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return StartScriptFor(exe)
}

// StartScriptFor finds start-session.sh relative to the binary at exe, in the
// two layouts Bear Den runs from (docs/operations.md → Packaging):
//
//	checkout:  <repo>/build/bin/bear-den-tv → <repo>/scripts/start-session.sh
//	installed: <prefix>/bin/bear-den-tv     → <prefix>/lib/bear-den-tv/start-session.sh
//	           (the .deb: /usr/bin → /usr/lib/bear-den-tv)
//
// The checkout wins when both exist. Neither is an error naming both paths.
func StartScriptFor(exe string) (string, error) {
	dir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(dir, "..", "..", "scripts", "start-session.sh"),
		filepath.Join(dir, "..", "lib", "bear-den-tv", "start-session.sh"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return filepath.Clean(c), nil
		}
	}
	return "", fmt.Errorf("start script not found for %s: looked for %s (checkout) and %s (installed)",
		exe, filepath.Clean(candidates[0]), filepath.Clean(candidates[1]))
}

// ExecQuote quotes an Exec path with spaces per the Desktop Entry spec.
func ExecQuote(path string) string {
	if strings.ContainsAny(path, " \t") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}

// DesktopEntry renders the autostart entry for script. Exec paths with spaces
// are quoted per the Desktop Entry spec.
func DesktopEntry(script string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Bear Den TV",
		"Comment=TV home screen and phone remote (coordinator + shell)",
		"Exec=" + ExecQuote(script) + " --watch",
		"Icon=video-display",
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")
}

// Enable writes the autostart entry for script to path atomically.
func Enable(script, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(DesktopEntry(script)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Disable removes the entry at path. A missing entry is not an error.
func Disable(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Enabled reports whether an entry exists at path.
func Enabled(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
