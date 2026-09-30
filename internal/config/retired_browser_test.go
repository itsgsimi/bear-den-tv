// Tests for moving a config.json off the retired browser, Flathub Chromium
// (upgrade.go upgradeRetiredBrowser, Store.UpgradeBrowsers; ADR 0014).
// testdata/config.chromium-era.json is the state a TV was in before:
// apps.browser and apps.streaming_browser "chromium", every web row's
// launch.app_id org.chromium.Chromium, the streaming sites off.

package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func openLogged(t *testing.T, raw []byte) (*Store, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	s, err := Open(Options{Dir: dir, Rules: testRules(), Logger: slog.New(slog.NewTextHandler(&buf, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	return s, &buf
}

func TestChromiumEraConfigMovesToChromeAndBrave(t *testing.T) {
	raw := mustRead(t, "testdata/config.chromium-era.json")
	s, logs := openLogged(t, raw)
	if s.Report().Source != SourceConfig || s.Revision() != 42 {
		t.Fatalf("the Chromium-era config did not load as itself: %+v rev %d", s.Report(), s.Revision())
	}
	moved, err := s.UpgradeBrowsers()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apps", "netflix", "disney-plus", "hulu", "browser"}; !reflect.DeepEqual(moved, want) {
		t.Fatalf("moved %v, want %v", moved, want)
	}
	c := s.Current()
	if c.Revision != 43 || c.BrowserName() != "brave" || c.StreamingBrowserName() != "chrome" {
		t.Fatalf("rev %d, browsers %s/%s", c.Revision, c.BrowserName(), c.StreamingBrowserName())
	}
	for id, want := range map[string]string{"netflix": "com.google.Chrome", "disney-plus": "com.google.Chrome", "hulu": "com.google.Chrome", "browser": "com.brave.Browser", "plex-htpc": "tv.plex.PlexHTPC"} {
		a, ok := c.Application(id)
		if !ok || a.Launch.AppID != want {
			t.Errorf("%s runs %q, want %q", id, a.Launch.AppID, want)
		}
		if a.Web != nil && id != "browser" && a.IsEnabled() {
			t.Errorf("%s was turned on by the upgrade", id)
		}
	}
	// Written through the normal path: config.json, its history entry and
	// the last-known-good copy say Chrome and Brave, never Chromium.
	for _, p := range []string{s.Path(), s.historyPath(43), s.LKGPath()} {
		if got := string(mustRead(t, p)); strings.Contains(got, "hromium") || !strings.Contains(got, `"com.google.Chrome"`) {
			t.Errorf("%s still names Chromium or lacks chrome: %s", filepath.Base(p), got)
		}
	}
	if !strings.Contains(logs.String(), "moved the web apps off Chromium") || !strings.Contains(logs.String(), "netflix") {
		t.Errorf("the move was not logged: %s", logs)
	}
	// Idempotent: nothing more now, nor after a restart.
	if again, err := s.UpgradeBrowsers(); again != nil || err != nil || s.Revision() != 43 {
		t.Fatalf("second run: %v %v rev %d", again, err, s.Revision())
	}
	s2, _ := openLogged(t, mustRead(t, s.Path()))
	if again, err := s2.UpgradeBrowsers(); again != nil || err != nil || s2.Revision() != 43 {
		t.Fatalf("after a restart: %v %v rev %d", again, err, s2.Revision())
	}
}

// An owner who had already chosen Brave for the streaming sites keeps it;
// only what named Chromium moves (the Browser tile, to Brave).
func TestRetiredBrowserKeepsTheOwnersOtherChoice(t *testing.T) {
	raw := strings.Replace(string(mustRead(t, "testdata/config.chromium-era.json")), `"streaming_browser": "chromium"`, `"streaming_browser": "brave"`, 1)
	raw = strings.Replace(raw, `"adapter":"netflix","launch":{"kind":"flatpak","app_id":"org.chromium.Chromium"`, `"adapter":"netflix","launch":{"kind":"flatpak","app_id":"com.brave.Browser"`, 1)
	s, _ := openLogged(t, []byte(raw))
	moved, err := s.UpgradeBrowsers()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apps", "disney-plus", "hulu", "browser"}; !reflect.DeepEqual(moved, want) {
		t.Fatalf("moved %v, want %v", moved, want)
	}
	c := s.Current()
	if c.StreamingBrowserName() != "brave" || c.BrowserName() != "brave" {
		t.Fatalf("browsers %s/%s", c.BrowserName(), c.StreamingBrowserName())
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu", "browser"} {
		if a, _ := c.Application(id); a.Launch.AppID != "com.brave.Browser" {
			t.Errorf("%s runs %s", id, a.Launch.AppID)
		}
	}
}

// A config.json that did not load is never rewritten from what did (the
// last-known-good copy of the Chromium era is read, moved in memory only),
// and Parse, which doctor and `apps` use, reads the old file too.
func TestRetiredBrowserOnlyWritesALoadedConfig(t *testing.T) {
	dir := t.TempDir()
	brokenRaw := `{"schema_version": 1, "revision": 3`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(brokenRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LKGFileName), mustRead(t, "testdata/config.chromium-era.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(Options{Dir: dir, Rules: testRules()})
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := s.Load(); err != nil || rep.Source != SourceLKG || s.Current().StreamingBrowserName() != "chrome" {
		t.Fatalf("recovery load: %+v %v %s", rep, err, s.Current().StreamingBrowserName())
	}
	if moved, err := s.UpgradeBrowsers(); moved != nil || err != nil {
		t.Fatalf("recovery: %v %v", moved, err)
	}
	if got := mustRead(t, s.Path()); string(got) != brokenRaw {
		t.Fatalf("the broken config.json was rewritten: %s", got)
	}
	c, err := Parse(mustRead(t, "testdata/config.chromium-era.json"), testRules())
	if err != nil || c.BrowserName() != "brave" {
		t.Fatalf("Parse: %v %s", err, c.BrowserName())
	}
	// A document that is not an object is still refused, with Parse's reason.
	if _, err := Parse([]byte(`["chromium"]`), testRules()); err == nil {
		t.Fatal("a JSON array parsed")
	}
}
