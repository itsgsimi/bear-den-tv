// Tests for `bear-den-tv autostart` (autostart.go). The entry, the layouts
// and enable/disable are tested in internal/platform/autostart.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"bear-den-tv/internal/platform/autostart"
)

// disable removes the entry, and a second disable (nothing there) is fine;
// status works either way; unknown words are a usage error.
func TestCmdAutostartDisableAndStatus(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	path := filepath.Join(cfg, "autostart", "bear-den-tv.desktop")
	if err := autostart.Enable("/opt/bear-den-tv/start-session.sh", path); err != nil {
		t.Fatal(err)
	}
	if err := cmdAutostart([]string{"status"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdAutostart([]string{"disable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("entry still there after disable: %v", err)
	}
	if err := cmdAutostart([]string{"disable"}); err != nil {
		t.Fatalf("second disable: %v", err)
	}
	if err := cmdAutostart(nil); err != nil {
		t.Fatal(err)
	}
	if err := cmdAutostart([]string{"on"}); err == nil {
		t.Fatal("unknown word accepted")
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
