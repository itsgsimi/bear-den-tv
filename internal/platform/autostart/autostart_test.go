// Tests for the autostart entry (autostart.go): its text, where it lives,
// which start script each install layout finds, and enable/disable/enabled.

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopEntry(t *testing.T) {
	e := DesktopEntry("/home/u/bear-den-tv/scripts/start-session.sh")
	for _, want := range []string{"[Desktop Entry]", "Type=Application", "Exec=/home/u/bear-den-tv/scripts/start-session.sh --watch", "Terminal=false"} {
		if !strings.Contains(e, want) {
			t.Fatalf("entry missing %q:\n%s", want, e)
		}
	}
	if q := DesktopEntry("/home/a b/start.sh"); !strings.Contains(q, `Exec="/home/a b/start.sh" --watch`) {
		t.Fatalf("path with space not quoted:\n%s", q)
	}
}

func TestFileHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cfg")
	if got := File(); got != "/tmp/cfg/autostart/bear-den-tv.desktop" {
		t.Fatal(got)
	}
}

func TestFileDefaultsToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := File(), filepath.Join(home, ".config", "autostart", "bear-den-tv.desktop"); got != want {
		t.Fatalf("File() = %q, want %q", got, want)
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

// enableFor enables autostart as if the binary were exe and returns the
// start script it chose and the entry it wrote.
func enableFor(t *testing.T, exe string) (string, string) {
	t.Helper()
	script, err := StartScriptFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "autostart", "bear-den-tv.desktop")
	if err := Enable(script, path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return script, string(raw)
}

func TestCheckoutLayout(t *testing.T) {
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
func TestInstalledLayout(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "usr")
	exe := filepath.Join(prefix, "bin", "bear-den-tv")
	want := filepath.Join(prefix, "lib", "bear-den-tv", "start-session.sh")
	touch(t, exe, want)
	script, entry := enableFor(t, exe)
	if script != want || !strings.Contains(entry, "Exec="+want+" --watch\n") {
		t.Fatalf("installed: script %q, entry:\n%s", script, entry)
	}
}

func TestNoStartScript(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bin", "bear-den-tv")
	touch(t, exe)
	_, err := StartScriptFor(exe)
	if err == nil || !strings.Contains(err.Error(), "checkout") || !strings.Contains(err.Error(), "installed") {
		t.Fatalf("want an error naming both layouts, got %v", err)
	}
}

func TestEnableDisableEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autostart", "bear-den-tv.desktop")
	if Enabled(path) {
		t.Fatal("enabled before any entry was written")
	}
	if err := Disable(path); err != nil {
		t.Fatalf("disable with no entry: %v", err)
	}
	if err := Enable("/opt/x/start-session.sh", path); err != nil {
		t.Fatal(err)
	}
	if !Enabled(path) {
		t.Fatal("not enabled after Enable")
	}
	if err := Disable(path); err != nil {
		t.Fatal(err)
	}
	if Enabled(path) {
		t.Fatal("still enabled after Disable")
	}
}
