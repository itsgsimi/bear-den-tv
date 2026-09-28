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

// startScript locates scripts/start-session.sh relative to this binary
// (<repo>/build/bin/bear-den-tv → <repo>/scripts/start-session.sh).
func startScript() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	script := filepath.Join(filepath.Dir(exe), "..", "..", "scripts", "start-session.sh")
	script = filepath.Clean(script)
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("start script not found next to this build (%s): %w", script, err)
	}
	return script, nil
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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(desktopEntry(script)), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
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
