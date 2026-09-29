// Tests for the config upgrade (upgrade.go): a config.json from an older
// version (testdata/config.three-apps-v0.json, shaped like a TV that has
// run since Plex, YouTube and Moonlight were the only apps, with its own
// layout and settings) gains exactly the default apps it lacks, in the
// defaults' state, appended to its "favorites" section, through the normal
// write path; nothing else changes, and a second run writes nothing.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func openWith(t *testing.T, raw []byte) *Store {
	t.Helper()
	dir := t.TempDir()
	if raw != nil {
		if err := os.WriteFile(filepath.Join(dir, FileName), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(Options{Dir: dir, Rules: testRules()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpgradeAddsOnlyTheMissingDefaultApps(t *testing.T) {
	raw, err := os.ReadFile("testdata/config.three-apps-v0.json")
	if err != nil {
		t.Fatal(err)
	}
	s := openWith(t, raw)
	before := s.Current()
	if s.Report().Source != SourceConfig || before.Revision != 17 {
		t.Fatalf("the old config did not load as itself: %+v rev %d", s.Report(), before.Revision)
	}

	added, err := s.UpgradeApps()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"spotify", "jellyfin", "retroarch", "netflix", "disney-plus", "hulu", "browser"}
	if !reflect.DeepEqual(added, want) {
		t.Fatalf("added %v, want %v", added, want)
	}
	after := s.Current()
	if after.Revision != 18 {
		t.Fatalf("revision %d, want 18", after.Revision)
	}
	// The owner's rows first, untouched and in order; then the defaults' rows
	// exactly as the defaults have them.
	if !reflect.DeepEqual(after.Applications[:3], before.Applications) {
		t.Fatalf("existing rows changed:\n%+v\n%+v", after.Applications[:3], before.Applications)
	}
	defaults := Defaults()
	for i, id := range want {
		d, _ := defaults.Application(id)
		a := after.Applications[3+i]
		if a.Launch.Args == nil { // Clone's nil is Marshal's []
			a.Launch.Args = []string{}
		}
		got, _ := json.Marshal(a)
		exp, _ := json.Marshal(d)
		if string(got) != string(exp) {
			t.Errorf("%s: %s, want the default %s", id, got, exp)
		}
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu"} {
		if a, _ := after.Application(id); a.IsEnabled() {
			t.Errorf("%s is on; streaming sites stay off until the owner turns them on", id)
		}
	}
	for _, id := range []string{"spotify", "jellyfin", "retroarch", "browser"} {
		if a, _ := after.Application(id); !a.HideWhenMissing {
			t.Errorf("%s shows a tile while missing", id)
		}
	}
	// Only the favorites section gains the new ids, at its end.
	if got := after.Sections[0].ApplicationIDs; !reflect.DeepEqual(got, append([]string{"youtube", "plex-htpc", "moonlight"}, want...)) {
		t.Fatalf("favorites %v", got)
	}
	// Everything else is the owner's, unchanged.
	strip := func(c Config) Config {
		c = c.Clone()
		c.Revision, c.Applications = 0, nil
		c.Sections[0].ApplicationIDs = nil
		return c
	}
	if !reflect.DeepEqual(strip(after), strip(before)) {
		b, _ := json.Marshal(strip(before))
		a, _ := json.Marshal(strip(after))
		t.Fatalf("settings changed:\n%s\n%s", b, a)
	}
	// Written like any change: config.json, history, last-known-good.
	for _, p := range []string{s.Path(), s.historyPath(18), s.LKGPath()} {
		onDisk, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Parse(onDisk, testRules())
		if err != nil || c.Revision != 18 || len(c.Applications) != 10 {
			t.Fatalf("%s: rev %d, %d apps, %v", p, c.Revision, len(c.Applications), err)
		}
	}

	// Idempotent: nothing more to add, nothing written.
	again, err := s.UpgradeApps()
	if err != nil || again != nil || s.Revision() != 18 {
		t.Fatalf("second run: %v %v rev %d", again, err, s.Revision())
	}
	if _, err := os.Stat(s.historyPath(19)); !os.IsNotExist(err) {
		t.Fatalf("second run wrote revision 19: %v", err)
	}
	// And after a restart.
	s2 := openWith(t, mustRead(t, s.Path()))
	if again, err := s2.UpgradeApps(); err != nil || again != nil || s2.Revision() != 18 {
		t.Fatalf("after a restart: %v %v rev %d", again, err, s2.Revision())
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The owner's own row for an adapter stands for it (web adapters may appear
// once), a new web row runs in the browser the config chose, and a fresh
// install (all defaults) or a config.json that did not load is left alone.
func TestUpgradeRespectsOwnRowsBrowsersAndRecovery(t *testing.T) {
	c := Defaults()
	c.Applications = slices.DeleteFunc(c.Applications, func(a Application) bool { return a.Adapter == "netflix" || a.Adapter == "browser" })
	for i := range c.Sections {
		c.Sections[i].ApplicationIDs = slices.DeleteFunc(c.Sections[i].ApplicationIDs, func(id string) bool { return id == "netflix" || id == "browser" })
	}
	// The owner's own Netflix row, under another id.
	own := Application{ID: "films", Label: "Films", Adapter: "netflix", Launch: Launch{Kind: "flatpak", AppID: "com.brave.Browser", Args: []string{}},
		HomePolicy: "pause-if-supported", RemoteEnabled: true, Web: &Web{URL: "https://www.netflix.com/"}}
	c.Applications = append(c.Applications, own)
	c.Apps = &Apps{AutoUpdate: true, Browser: "brave", StreamingBrowser: "brave"}
	for i := range c.Applications {
		if c.Applications[i].Launch.AppID == "org.chromium.Chromium" {
			c.Applications[i].Launch.AppID = "com.brave.Browser"
		}
	}
	raw, err := Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := openWith(t, raw)
	if s.Report().Source != SourceConfig {
		t.Fatalf("did not load: %+v", s.Report())
	}
	added, err := s.UpgradeApps()
	if err != nil || !reflect.DeepEqual(added, []string{"browser"}) {
		t.Fatalf("added %v %v", added, err)
	}
	if b, _ := s.Current().Application("browser"); b.Launch.AppID != "com.brave.Browser" {
		t.Fatalf("the new Browser row runs in %s, not the chosen Brave", b.Launch.AppID)
	}
	if _, ok := s.Current().Application("netflix"); ok {
		t.Fatal("a second Netflix row was added beside the owner's")
	}

	// A fresh install: already every default.
	fresh := openWith(t, nil)
	if added, err := fresh.UpgradeApps(); added != nil || err != nil || fresh.Revision() != 1 {
		t.Fatalf("fresh install: %v %v rev %d", added, err, fresh.Revision())
	}
	// config.json unusable, running on an old last-known-good copy:
	// config.json is not rewritten from it.
	dir := t.TempDir()
	brokenRaw := `{"schema_version": 1, "revision": 3`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(brokenRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LKGFileName), mustRead(t, "testdata/config.three-apps-v0.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken, err := Open(Options{Dir: dir, Rules: testRules()})
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := broken.Load(); err != nil || rep.Source != SourceLKG {
		t.Fatalf("recovery load: %+v %v", rep, err)
	}
	if added, err := broken.UpgradeApps(); added != nil || err != nil || broken.Revision() != 17 {
		t.Fatalf("recovery: %v %v rev %d", added, err, broken.Revision())
	}
	if got := mustRead(t, broken.Path()); string(got) != brokenRaw {
		t.Fatalf("the broken config.json was rewritten: %s", got)
	}
}
