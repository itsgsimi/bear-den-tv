// Tests for the autostart desktop entry (autostart.go).

package main

import (
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
