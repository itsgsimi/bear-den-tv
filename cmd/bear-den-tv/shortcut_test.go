// Tests for launcher installation (shortcut.go).

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUserDir(t *testing.T) {
	raw := "# comment\nXDG_DOWNLOAD_DIR=\"$HOME/Downloads\"\nXDG_DESKTOP_DIR=\"$HOME/Schreibtisch\"\n"
	if got, ok := parseUserDir(raw, "/home/u", "XDG_DESKTOP_DIR"); !ok || got != "/home/u/Schreibtisch" {
		t.Fatalf("got %q %v", got, ok)
	}
	// $HOME itself means "no desktop folder" (xdg-user-dirs).
	if got, ok := parseUserDir(`XDG_DESKTOP_DIR="$HOME/"`, "/home/u", "XDG_DESKTOP_DIR"); !ok || got != "" {
		t.Fatalf("home as desktop: got %q %v", got, ok)
	}
	if got, ok := parseUserDir(`XDG_DESKTOP_DIR="/data/desk"`, "/home/u", "XDG_DESKTOP_DIR"); !ok || got != "/data/desk" {
		t.Fatalf("absolute: got %q %v", got, ok)
	}
	if _, ok := parseUserDir("", "/home/u", "XDG_DESKTOP_DIR"); ok {
		t.Fatal("absent key reported present")
	}
}

func TestInstallLaunchers(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "repo", "scripts", "start-session.sh")
	icon := filepath.Join(root, "repo", "apps", "remote-web", "static", "icons", "bear-den.svg")
	for _, f := range []string{script, icon} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var trusted []string
	orig := trustLauncher
	trustLauncher = func(path string, _ []byte) error { trusted = append(trusted, path); return nil }
	t.Cleanup(func() { trustLauncher = orig })

	menu := filepath.Join(root, "share", "applications", launcherName)
	desk := filepath.Join(root, "Desktop")
	if err := os.Mkdir(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	written, warn, err := installLaunchers(script, menu, desk)
	if err != nil || warn != nil || len(written) != 2 {
		t.Fatalf("written=%v warn=%v err=%v", written, warn, err)
	}
	raw, err := os.ReadFile(filepath.Join(desk, launcherName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Name=Bear Den TV", "Exec=" + script + " --watch", "Icon=" + icon, "Terminal=false"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("launcher missing %q:\n%s", want, raw)
		}
	}
	if st, _ := os.Stat(filepath.Join(desk, launcherName)); st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("desktop launcher not executable: %v", st.Mode())
	}
	if len(trusted) != 1 || trusted[0] != filepath.Join(desk, launcherName) {
		t.Fatalf("trusted = %v, want only the desktop launcher", trusted)
	}

	// No desktop folder: menu entry only.
	written, _, err = installLaunchers(script, menu, "")
	if err != nil || len(written) != 1 || written[0] != menu {
		t.Fatalf("no desktop: written=%v err=%v", written, err)
	}
}
