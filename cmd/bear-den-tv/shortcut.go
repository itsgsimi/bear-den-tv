// `bear-den-tv shortcut`: install the menu and desktop launchers
// (docs/operations.md).

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Desktop and app-menu launchers: `bear-den-tv shortcut enable` adds a
// "Bear Den TV" icon to the desktop and the applications menu. It runs
// start-session.sh --watch, which starts Bear Den or restarts it if it is
// already running (e.g. to come back after Settings → Exit Bear Den TV).

const launcherName = "bear-den-tv.desktop"

// menuEntryFile is the applications-menu entry.
func menuEntryFile() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "applications", launcherName)
}

// desktopDir is the user's desktop folder (XDG_DESKTOP_DIR, else ~/Desktop),
// or "" when there is none: it does not exist, or user-dirs sets it to $HOME,
// which the xdg-user-dirs spec uses to mean "no desktop folder".
func desktopDir() string {
	home, _ := os.UserHomeDir()
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	dir := filepath.Join(home, "Desktop")
	if raw, err := os.ReadFile(filepath.Join(cfg, "user-dirs.dirs")); err == nil {
		if v, ok := parseUserDir(string(raw), home, "XDG_DESKTOP_DIR"); ok {
			dir = v
		}
	}
	if dir == "" {
		return ""
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

// parseUserDir reads key from a user-dirs.dirs file. ok is false when the key
// is absent; the value is "" when it is $HOME itself or not absolute.
func parseUserDir(raw, home, key string) (string, bool) {
	for _, line := range strings.Split(raw, "\n") {
		v, found := strings.CutPrefix(strings.TrimSpace(line), key+"=")
		if !found {
			continue
		}
		v = strings.Trim(v, `"`)
		if rest, rel := strings.CutPrefix(v, "$HOME"); rel {
			v = home + rest
		}
		if !filepath.IsAbs(v) || filepath.Clean(v) == filepath.Clean(home) {
			return "", true
		}
		return filepath.Clean(v), true
	}
	return "", false
}

// execQuote quotes an Exec path with spaces per the Desktop Entry spec.
func execQuote(path string) string {
	if strings.ContainsAny(path, " \t") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}

// launcherIcon is the bear mark shipped with the phone remote, next to the
// start script in the checkout; a stock icon when it is missing.
func launcherIcon(script string) string {
	icon := filepath.Join(filepath.Dir(script), "..", "apps", "remote-web", "static", "icons", "bear-den.svg")
	if _, err := os.Stat(icon); err != nil {
		return "video-display"
	}
	return filepath.Clean(icon)
}

// launcherEntry renders the desktop/menu launcher for script.
func launcherEntry(script, icon string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Bear Den TV",
		"GenericName=TV home screen",
		"Comment=Open the Bear Den TV home screen (restarts it if it is already running)",
		"Exec=" + execQuote(script) + " --watch",
		"Icon=" + icon,
		"Terminal=false",
		"StartupNotify=false",
		"Categories=AudioVideo;Video;TV;",
		"",
	}, "\n")
}

// trustLauncher marks a desktop launcher as trusted so a double-click runs it
// without an "untrusted launcher" prompt: xfdesktop 4.18 compares
// metadata::xfce-exe-checksum with the file's SHA-256, and GNOME/Nemo read
// metadata::trusted. Tests replace it.
var trustLauncher = func(path string, data []byte) error {
	sum := sha256.Sum256(data)
	env := os.Environ()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		rt := os.Getenv("XDG_RUNTIME_DIR")
		if rt == "" {
			rt = fmt.Sprintf("/run/user/%d", os.Getuid())
		}
		env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(rt, "bus"))
	}
	for _, kv := range [][2]string{{"metadata::xfce-exe-checksum", hex.EncodeToString(sum[:])}, {"metadata::trusted", "true"}} {
		cmd := exec.Command("gio", "set", path, kv[0], kv[1])
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("gio set %s: %v %s", kv[0], err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil { // WriteFile's mode is masked by umask
		return err
	}
	return os.Rename(tmp, path)
}

// installLaunchers writes the menu entry and, when deskDir is set, a trusted
// executable desktop launcher. It returns the files written and a warning
// when the desktop launcher could not be marked trusted.
func installLaunchers(script, menuFile, deskDir string) (written []string, warn error, err error) {
	data := []byte(launcherEntry(script, launcherIcon(script)))
	if err := writeAtomic(menuFile, data, 0o644); err != nil {
		return nil, nil, err
	}
	written = append(written, menuFile)
	if deskDir == "" {
		return written, nil, nil
	}
	desk := filepath.Join(deskDir, launcherName)
	if err := writeAtomic(desk, data, 0o755); err != nil {
		return written, nil, err
	}
	written = append(written, desk)
	if err := trustLauncher(desk, data); err != nil {
		warn = fmt.Errorf("the desktop may ask once whether to trust the launcher (%v)", err)
	}
	return written, warn, nil
}

// cmdShortcut installs or removes the launchers. Owner-run only.
func cmdShortcut(args []string) error {
	menu, desk := menuEntryFile(), desktopDir()
	files := []string{menu}
	if desk != "" {
		files = append(files, filepath.Join(desk, launcherName))
	}
	switch {
	case len(args) == 1 && args[0] == "enable":
		script, err := startScript()
		if err != nil {
			return err
		}
		written, warn, err := installLaunchers(script, menu, desk)
		if err != nil {
			return err
		}
		for _, f := range written {
			fmt.Println("shortcut:", f)
		}
		if desk == "" {
			fmt.Println("no desktop folder; added to the applications menu only")
		}
		if warn != nil {
			fmt.Println("note:", warn)
		}
	case len(args) == 1 && args[0] == "disable":
		for _, f := range files {
			if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		fmt.Println("shortcuts removed")
	case len(args) == 0 || (len(args) == 1 && args[0] == "status"):
		for _, f := range files {
			if _, err := os.Stat(f); err == nil {
				fmt.Println("present:", f)
			} else {
				fmt.Println("absent: ", f)
			}
		}
	default:
		return errors.New("usage: bear-den-tv shortcut enable|disable|status")
	}
	return nil
}
