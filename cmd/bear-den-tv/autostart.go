// `bear-den-tv autostart`: write or remove the XDG autostart entry
// (docs/operations.md).

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// autostartFile is the XDG autostart entry the desktop session runs at login.
func autostartFile() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "bear-den-tv.desktop")
}

// startScript locates start-session.sh for this binary (os.Executable with
// symlinks resolved); see startScriptFor.
func startScript() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return startScriptFor(exe)
}

// startScriptFor finds start-session.sh relative to the binary at exe, in the
// two layouts Bear Den runs from (docs/operations.md → Packaging):
//
//	checkout:  <repo>/build/bin/bear-den-tv → <repo>/scripts/start-session.sh
//	installed: <prefix>/bin/bear-den-tv     → <prefix>/lib/bear-den-tv/start-session.sh
//	           (the .deb: /usr/bin → /usr/lib/bear-den-tv)
//
// The checkout wins when both exist. Neither is an error naming both paths.
func startScriptFor(exe string) (string, error) {
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

// desktopEntry renders the autostart entry for script. Exec paths with spaces
// are quoted per the Desktop Entry spec.
func desktopEntry(script string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Bear Den TV",
		"Comment=TV home screen and phone remote (coordinator + shell)",
		"Exec=" + execQuote(script) + " --watch",
		"Icon=video-display",
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")
}

// enableAutostart writes the autostart entry for script to path atomically.
func enableAutostart(script, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(desktopEntry(script)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cmdAutostart installs or removes the login autostart entry. Owner-run only:
// it is never enabled implicitly.
func cmdAutostart(args []string) error {
	path := autostartFile()
	switch {
	case len(args) == 1 && args[0] == "enable":
		script, err := startScript()
		if err != nil {
			return err
		}
		if err := enableAutostart(script, path); err != nil {
			return err
		}
		fmt.Printf("autostart enabled: %s → %s --watch\n", path, script)
	case len(args) == 1 && args[0] == "disable":
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Println("autostart disabled")
	case len(args) == 0 || (len(args) == 1 && args[0] == "status"):
		if raw, err := os.ReadFile(path); err == nil {
			fmt.Printf("enabled (%s)\n%s", path, raw)
		} else {
			fmt.Println("disabled")
		}
	default:
		return errors.New("usage: bear-den-tv autostart enable|disable|status")
	}
	return nil
}
