// Tests for the autostart desktop entry (autostart.go).

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("/home/u/bear-den-tv/scripts/start-session.sh")
	for _, want := range []string{"[Desktop Entry]", "Type=Application", "Exec=/home/u/bear-den-tv/scripts/start-session.sh --watch", "Terminal=false"} {
		if !strings.Contains(e, want) {
			t.Fatalf("entry missing %q:\n%s", want, e)
		}
	}
	if q := desktopEntry("/home/a b/start.sh"); !strings.Contains(q, `Exec="/home/a b/start.sh" --watch`) {
		t.Fatalf("path with space not quoted:\n%s", q)
	}
}

func TestAutostartFileHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cfg")
	if got := autostartFile(); got != "/tmp/cfg/autostart/bear-den-tv.desktop" {
		t.Fatal(got)
	}
}

// touch creates the files (and their folders) with mode 0755.
func touch(t *testing.T, files ...string) {
	t.Helper()
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// enableFor runs `autostart enable` as if the binary were exe and returns
// the start script it chose and the entry it wrote.
func enableFor(t *testing.T, exe string) (string, string) {
	t.Helper()
	script, err := startScriptFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "autostart", "bear-den-tv.desktop")
	if err := enableAutostart(script, path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return script, string(raw)
}

func TestAutostartCheckoutLayout(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	exe := filepath.Join(repo, "build", "bin", "bear-den-tv")
	want := filepath.Join(repo, "scripts", "start-session.sh")
	touch(t, exe, want)
	script, entry := enableFor(t, exe)
	if script != want || !strings.Contains(entry, "Exec="+want+" --watch\n") {
		t.Fatalf("checkout: script %q, entry:\n%s", script, entry)
	}
}

// The .deb layout: /usr/bin/bear-den-tv → /usr/lib/bear-den-tv/start-session.sh.
func TestAutostartInstalledLayout(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "usr")
	exe := filepath.Join(prefix, "bin", "bear-den-tv")
	want := filepath.Join(prefix, "lib", "bear-den-tv", "start-session.sh")
	touch(t, exe, want)
	script, entry := enableFor(t, exe)
	if script != want || !strings.Contains(entry, "Exec="+want+" --watch\n") {
		t.Fatalf("installed: script %q, entry:\n%s", script, entry)
	}
}

func TestAutostartNoStartScript(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bin", "bear-den-tv")
	touch(t, exe)
	_, err := startScriptFor(exe)
	if err == nil || !strings.Contains(err.Error(), "checkout") || !strings.Contains(err.Error(), "installed") {
		t.Fatalf("want an error naming both layouts, got %v", err)
	}
}
