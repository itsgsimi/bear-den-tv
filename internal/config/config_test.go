// Tests for config defaults, validation, the store and layout apply (spec
// contracts/config.md).

package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

func defaultRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mutateJSON(t *testing.T, raw []byte, f func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	f(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var testIfaces = StaticInterfaces{{Name: "lo", Loopback: true}, {Name: "enp1s0"}, {Name: "docker0", Virtual: true}}

func testRules() Rules { return Rules{Interfaces: testIfaces} }

func TestDefaultsMatchFixture(t *testing.T) {
	raw := defaultRaw(t)
	cfg, err := Parse(raw, testRules())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Revision != 1 || len(cfg.Applications) != 3 || cfg.Applications[1].Launch.Args[0] != "--fullscreen" || cfg.Applications[2].Adapter != "moonlight" {
		t.Fatalf("defaults decoded wrongly: %+v", cfg)
	}
	if err := Validate(cfg, testRules()); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticRejections(t *testing.T) {
	base := defaultRaw(t)
	apps := func(m map[string]any) []any { return m["applications"].([]any) }
	cases := map[string]struct {
		mutate func(m map[string]any)
		want   string
	}{
		"duplicate app id": {func(m map[string]any) {
			a := apps(m)
			a[1].(map[string]any)["id"] = "plex-htpc"
		}, "duplicate application id"},
		"reserved app id": {func(m map[string]any) {
			apps(m)[0].(map[string]any)["id"] = "shell"
		}, "reserved"},
		"duplicate section id": {func(m map[string]any) {
			s := m["sections"].([]any)
			s[1].(map[string]any)["id"] = "favorites"
		}, "duplicate section id"},
		"dangling ref": {func(m map[string]any) {
			s := m["sections"].([]any)[0].(map[string]any)
			s["application_ids"] = []any{"plex-htpc", "netflix"}
		}, "unknown application"},
		"disallowed arg": {func(m map[string]any) {
			l := apps(m)[0].(map[string]any)["launch"].(map[string]any)
			l["args"] = []any{"--fullscreen"}
		}, "not approved"},
		"wrong flatpak id": {func(m map[string]any) {
			l := apps(m)[0].(map[string]any)["launch"].(map[string]any)
			l["app_id"] = "org.evil.App"
		}, "must be"},
		"token leak": {func(m map[string]any) {
			m["plex_content"].(map[string]any)["token"] = "x"
		}, "credential key"},
		"nested password leak": {func(m map[string]any) {
			m["device"].(map[string]any)["display_name"] = "ok"
			m["plex_content"].(map[string]any)["library_ids"] = []any{"a"}
			m["remote"].(map[string]any)["https"].(map[string]any)["password"] = "x"
		}, "credential key"},
		"remote without consent": {func(m map[string]any) {
			r := m["remote"].(map[string]any)
			r["enabled"] = true
			r["interfaces"] = []any{"enp1s0"}
		}, "lan_consent"},
		"plex without server": {func(m map[string]any) {
			p := m["plex_content"].(map[string]any)
			p["enabled"] = true
			p["connection_ref"] = "ref"
		}, "server_url"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw := mutateJSON(t, base, tc.mutate)
			_, err := Parse(raw, testRules())
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestSchemaVersionAndCorruptJSON(t *testing.T) {
	raw := mutateJSON(t, defaultRaw(t), func(m map[string]any) { m["schema_version"] = 2 })
	_, err := Parse(raw, testRules())
	if !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("want ErrSchemaVersion, got %v", err)
	}
	if _, err := Parse([]byte("{\"schema_version\":1,"), testRules()); err == nil {
		t.Fatal("corrupt JSON accepted")
	}
	if _, err := Parse([]byte("{}"), testRules()); !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("missing schema_version: %v", err)
	}
}

func TestHostRules(t *testing.T) {
	cfg := Defaults()
	cfg.Onboarding.LANConsent = true
	cfg.Remote.Enabled = true
	cfg.Remote.Interfaces = []string{"wlan9"}
	err := Validate(cfg, testRules())
	if err == nil || !strings.Contains(err.Error(), "interface wlan9 not present") {
		t.Fatalf("missing interface: %v", err)
	}
	cfg.Remote.Interfaces = []string{"lo"}
	if err := Validate(cfg, testRules()); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("loopback: %v", err)
	}
	cfg.Remote.Interfaces = []string{"docker0"}
	if err := Validate(cfg, testRules()); err == nil || !strings.Contains(err.Error(), "virtual") {
		t.Fatalf("virtual: %v", err)
	}
	cfg.Remote.Interfaces = []string{"enp1s0"}
	if err := Validate(cfg, testRules()); err != nil {
		t.Fatalf("valid interface rejected: %v", err)
	}
	cfg.Remote.Transport = "https"
	if err := Validate(cfg, testRules()); err == nil || !strings.Contains(err.Error(), "certificate_file") {
		t.Fatalf("https without certs: %v", err)
	}
	cert, key := "/nonexistent/cert.pem", "/nonexistent/key.pem"
	cfg.Remote.HTTPS = HTTPSFiles{CertificateFile: &cert, PrivateKeyFile: &key}
	if err := Validate(cfg, testRules()); err == nil || !strings.Contains(err.Error(), "unusable") {
		t.Fatalf("unreadable certs: %v", err)
	}
	ok := testRules()
	ok.LoadKeyPair = func(c, k string) error { return nil }
	if err := Validate(cfg, ok); err != nil {
		t.Fatalf("parseable certs rejected: %v", err)
	}
	if !cfg.Remote.LayoutEditingOverHTTP() == false {
		t.Fatal("https must not honor http_layout_editing")
	}
}

func newStore(t *testing.T, dir string, fc *clock.Fake) *Store {
	t.Helper()
	s, err := Open(Options{Dir: dir, Clock: fc, Rules: testRules(), ConfirmTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadInitializesAndRecovers(t *testing.T) {
	dir := t.TempDir()
	fc := clock.NewFake(time.Unix(1, 0))
	s := newStore(t, dir, fc)
	rep, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Initialized || rep.Source != SourceDefaults || rep.Notification != nil {
		t.Fatalf("first run report %+v", rep)
	}
	if _, err := os.Stat(s.LKGPath()); err != nil {
		t.Fatal("LKG not written on init")
	}
	if _, err := s.Update(func(c *Config) error { c.Device.DisplayName = "Den"; return nil }); err != nil {
		t.Fatal(err)
	}
	lkgBefore, _ := os.ReadFile(s.LKGPath())

	// A bad write from outside must never replace the last-known-good copy.
	if err := os.WriteFile(s.Path(), []byte("{\"schema_version\":1, garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2 := newStore(t, dir, fc)
	rep, err = s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != SourceLKG || rep.Notification == nil || rep.Notification.Kind != "error" {
		t.Fatalf("recovery report %+v", rep)
	}
	if s2.Current().Device.DisplayName != "Den" || s2.Revision() != 2 {
		t.Fatalf("LKG content not loaded: %+v", s2.Current().Device)
	}
	lkgAfter, _ := os.ReadFile(s.LKGPath())
	if string(lkgBefore) != string(lkgAfter) {
		t.Fatal("bad config.json replaced the last-known-good copy")
	}

	// Semantically invalid config.json is also skipped.
	bad := mutateJSON(t, lkgBefore, func(m map[string]any) {
		m["sections"].([]any)[0].(map[string]any)["application_ids"] = []any{"ghost"}
	})
	_ = os.WriteFile(s.Path(), bad, 0o600)
	s3 := newStore(t, dir, fc)
	rep, _ = s3.Load()
	if rep.Source != SourceLKG {
		t.Fatalf("semantic failure not recovered: %+v", rep)
	}
	lkgAfter, _ = os.ReadFile(s.LKGPath())
	if string(lkgBefore) != string(lkgAfter) {
		t.Fatal("invalid config.json replaced the last-known-good copy")
	}

	// Both files unusable -> defaults with a persistent notification, nothing written.
	_ = os.WriteFile(s.LKGPath(), []byte("nope"), 0o600)
	s4 := newStore(t, dir, fc)
	rep, _ = s4.Load()
	if rep.Source != SourceDefaults || rep.Notification == nil || rep.Initialized {
		t.Fatalf("defaults fallback report %+v", rep)
	}
	if got, _ := os.ReadFile(s.Path()); string(got) != string(bad) {
		t.Fatal("fallback to defaults rewrote config.json")
	}
}

func TestLoadWithMissingInterfaceKeepsConfigButBlocksRemote(t *testing.T) {
	dir := t.TempDir()
	fc := clock.NewFake(time.Unix(1, 0))
	s := newStore(t, dir, fc)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := s.Current()
	cfg.Onboarding.LANConsent = true
	cfg.Remote.Enabled = true
	cfg.Remote.Interfaces = []string{"usb0"}
	cfg.Revision = 5
	raw, _ := Marshal(cfg)
	_ = os.WriteFile(s.Path(), raw, 0o600)
	s2 := newStore(t, dir, fc)
	rep, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != SourceConfig || rep.RemoteBlocked == "" || !strings.Contains(rep.RemoteBlocked, "usb0 not present") {
		t.Fatalf("report %+v", rep)
	}
	if s2.Revision() != 5 {
		t.Fatal("user configuration was discarded")
	}
	_, err = s2.UpdateRemote(func(c *Config) error { return nil })
	if err == nil {
		t.Fatal("UpdateRemote accepted a missing interface")
	}
}

func TestSchemaVersionMismatchLoadsLKG(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir, clock.NewFake(time.Unix(1, 0)))
	_, _ = s.Load()
	raw := mutateJSON(t, defaultRaw(t), func(m map[string]any) { m["schema_version"] = 7 })
	_ = os.WriteFile(s.Path(), raw, 0o600)
	s2 := newStore(t, dir, clock.NewFake(time.Unix(1, 0)))
	rep, _ := s2.Load()
	if rep.Source != SourceLKG || len(rep.Problems) == 0 || !strings.Contains(rep.Problems[0], "schema_version") {
		t.Fatalf("report %+v", rep)
	}
}

func TestHistoryKeepsTwenty(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir, clock.NewFake(time.Unix(1, 0)))
	_, _ = s.Load()
	for i := 0; i < 25; i++ {
		if _, err := s.Update(func(c *Config) error { c.UI.ClockEnabled = !c.UI.ClockEnabled; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	revs := s.HistoryRevisions()
	if len(revs) != 20 || revs[0] != 7 || revs[19] != 26 {
		t.Fatalf("history revisions %v", revs)
	}
	if _, err := os.Stat(filepath.Join(dir, HistoryDir, "26.json")); err != nil {
		t.Fatal(err)
	}
}

func TestApplyLayoutConflictPendingRollbackAndConfirm(t *testing.T) {
	dir := t.TempDir()
	fc := clock.NewFake(time.Unix(1, 0))
	changes := 0
	s, _ := Open(Options{Dir: dir, Clock: fc, Rules: testRules(), ConfirmTimeout: 30 * time.Second, OnChange: func() { changes++ }})
	_, _ = s.Load()
	layout := s.Current().Layout()
	layout.UI.ClockEnabled = false
	if _, err := s.ApplyLayout(99, layout, "web"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("want revision conflict, got %v", err)
	}
	res, err := s.ApplyLayout(1, layout, "web")
	if err != nil || res.Pending || res.Revision != 2 {
		t.Fatalf("plain change: %+v %v", res, err)
	}
	if changes != 1 {
		t.Fatalf("OnChange calls = %d", changes)
	}

	risky := s.Current().Layout()
	risky.UI.TextScale = 1.9
	res, err = s.ApplyLayout(2, risky, "web")
	if err != nil || !res.Pending || res.Revision != 3 {
		t.Fatalf("risky change: %+v %v", res, err)
	}
	p := s.Pending()
	if p == nil || p.Revision != 3 || p.PreviousRevision != 2 || p.ExpiresInS != 30 || p.Source != "web" {
		t.Fatalf("pending %+v", p)
	}
	if _, err := s.ApplyLayout(3, risky, "web"); !errors.Is(err, ErrPendingChange) {
		t.Fatalf("second change during pending: %v", err)
	}
	fc.Advance(29 * time.Second)
	if s.Pending() == nil || s.Pending().ExpiresInS != 1 {
		t.Fatalf("pending should still be live: %+v", s.Pending())
	}
	fc.Advance(time.Second)
	if s.Pending() != nil {
		t.Fatal("pending not rolled back on timeout")
	}
	if cur := s.Current(); cur.Revision != 4 || cur.UI.TextScale != 1.0 {
		t.Fatalf("rollback state rev=%d scale=%v", cur.Revision, cur.UI.TextScale)
	}

	res, _ = s.ApplyLayout(4, risky, "tv")
	if err := s.Confirm(999); !errors.Is(err, ErrNoPending) {
		t.Fatalf("confirm wrong revision: %v", err)
	}
	if err := s.Confirm(res.Revision); err != nil {
		t.Fatal(err)
	}
	fc.Advance(time.Minute)
	if cur := s.Current(); cur.Revision != 5 || cur.UI.TextScale != 1.9 {
		t.Fatalf("confirmed change lost: rev=%d scale=%v", cur.Revision, cur.UI.TextScale)
	}

	calm := s.Current().Layout()
	calm.UI.TextScale = 1.0
	_, _ = s.ApplyLayout(5, calm, "web")
	risky2 := calm
	risky2.UI.SafeMarginPercent = 8
	res, _ = s.ApplyLayout(6, risky2, "web")
	if err := s.Cancel(res.Revision); err != nil {
		t.Fatal(err)
	}
	if cur := s.Current(); cur.UI.SafeMarginPercent != 3 || cur.Revision != 8 {
		t.Fatalf("cancel state %+v", cur.UI)
	}
	if fc.Pending() != 0 {
		t.Fatal("timer left armed after cancel")
	}
}

func TestDisablingLastAppSectionIsRisky(t *testing.T) {
	cur := Defaults().Layout()
	next := Defaults().Layout()
	next.Sections[0].Enabled = false
	if !IsRisky(cur, next) {
		t.Fatal("disabling the only application section must be risky")
	}
	next.Sections[0].Enabled = true
	next.Sections[1].Enabled = false
	if IsRisky(cur, next) {
		t.Fatal("disabling a plex section is not risky")
	}
}

func TestUndoResetExportImport(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir, clock.NewFake(time.Unix(1, 0)))
	_, _ = s.Load()
	if _, err := s.Undo(); !errors.Is(err, ErrNoUndo) {
		t.Fatalf("undo on fresh store: %v", err)
	}
	layout := s.Current().Layout()
	layout.Sections[0].Title = "Favourites"
	layout.Sections[1].Enabled = true
	if _, err := s.ApplyLayout(1, layout, "web"); err != nil {
		t.Fatal(err)
	}
	rev, err := s.Undo()
	if err != nil || rev != 3 || s.Current().Sections[0].Title != "Your Apps" {
		t.Fatalf("undo rev=%d err=%v title=%q", rev, err, s.Current().Sections[0].Title)
	}
	rev, _ = s.Undo()
	if rev != 4 || s.Current().Sections[0].Title != "Favourites" {
		t.Fatal("second undo should redo")
	}
	if _, err := s.Reset("plex-continue"); err != nil {
		t.Fatal(err)
	}
	if s.Current().Sections[1].Enabled || s.Current().Sections[0].Title != "Favourites" {
		t.Fatal("section reset touched the wrong section")
	}
	if _, err := s.Reset("nope"); !errors.Is(err, ErrUnknownSection) {
		t.Fatalf("reset unknown: %v", err)
	}
	if _, err := s.Reset(""); err != nil {
		t.Fatal(err)
	}
	if s.Current().Sections[0].Title != "Your Apps" {
		t.Fatal("full reset did not restore defaults")
	}

	cfg := s.Current()
	ref := "secret-ref"
	url := "http://192.168.1.50:32400"
	_, err = s.Update(func(c *Config) error {
		c.PlexContent = PlexContent{Enabled: true, ConnectionRef: &ref, ServerURL: &url}
		c.Remote.AllowedHosts = []string{"den.local"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg
	out, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, forbidden := range []string{"remote", "devices", "token", "secret-ref", "192.168", "flatpak", "den.local", "launch"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("export leaks %q: %s", forbidden, text)
		}
	}
	var doc Portable
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Layout.Sections[0].ApplicationIDs = append(doc.Layout.Sections[0].ApplicationIDs, "netflix")
	doc.Layout.UI.TextScale = 1.8
	imported, _ := json.Marshal(doc)
	pv, err := s.ImportPreview(imported)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.UnresolvedApplicationIDs) != 1 || pv.UnresolvedApplicationIDs[0] != "netflix" || !pv.Risky {
		t.Fatalf("preview %+v", pv)
	}
	if len(pv.Changes) != 2 {
		t.Fatalf("changes %v", pv.Changes)
	}
	if s.Revision() != 7 {
		t.Fatalf("import preview wrote a revision: %d", s.Revision())
	}
	if _, err := s.ImportPreview([]byte(`{"format":"other"}`)); err == nil {
		t.Fatal("foreign document accepted")
	}
}

func TestLayoutValidationAgainstApplications(t *testing.T) {
	cfg := Defaults()
	l := cfg.Layout()
	l.Sections[0].ApplicationIDs = []string{"ghost"}
	if err := ValidateLayoutAgainst(cfg, l); err == nil {
		t.Fatal("dangling reference accepted")
	}
	l = cfg.Layout()
	l.Sections = append(l.Sections, contract.Section{ID: "favorites", Title: "Dup", Kind: "applications", Enabled: true, ApplicationIDs: []string{}})
	if err := ValidateLayoutAgainst(cfg, l); err == nil {
		t.Fatal("duplicate section accepted")
	}
	l = cfg.Layout()
	l.UI.Accent = "red"
	if err := ValidateLayoutAgainst(cfg, l); err == nil {
		t.Fatal("structural error accepted")
	}
}

func TestWriteFileAtomicLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	if err := writeFileAtomic(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(p, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
	got, _ := os.ReadFile(p)
	if string(got) != "b" {
		t.Fatal("content not replaced")
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
}

// Playback overrides are optional, validated structurally, deep-copied, pruned
// when emptied, and survive a restart (a fresh store loading the same file).
func TestPlaybackOverridesPersist(t *testing.T) {
	if strings.Contains(string(defaultRaw(t)), "playback") {
		t.Fatal("the defaults carry no playback overrides")
	}
	with := func(overrides any) []byte {
		return mutateJSON(t, defaultRaw(t), func(m map[string]any) { m["playback"] = map[string]any{"overrides": overrides} })
	}
	cfg, err := Parse(with(map[string]any{"moonlight": map[string]any{"fps": "60", "resolution": "1080p"}}), testRules())
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.PlaybackOverrides("moonlight"); got["fps"] != "60" || got["resolution"] != "1080p" {
		t.Fatalf("overrides = %v", got)
	}
	for name, bad := range map[string]any{
		"adapter key": map[string]any{"Moon light": map[string]any{"fps": "60"}},
		"setting id":  map[string]any{"moonlight": map[string]any{"FPS": "60"}},
		"value":       map[string]any{"moonlight": map[string]any{"fps": "rm -rf /"}},
		"value type":  map[string]any{"moonlight": map[string]any{"fps": 60}},
		"not a map":   map[string]any{"moonlight": "60"},
	} {
		if _, err := Parse(with(bad), testRules()); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Parse(mutateJSON(t, defaultRaw(t), func(m map[string]any) { m["playback"] = map[string]any{"extra": true} }), testRules()); err == nil {
		t.Error("unknown playback key accepted")
	}

	clone := cfg.Clone()
	clone.SetPlaybackOverride("moonlight", "fps", "120")
	if cfg.PlaybackOverrides("moonlight")["fps"] != "60" {
		t.Fatal("Clone must deep-copy the overrides")
	}

	dir := t.TempDir()
	s := newStore(t, dir, clock.NewFake(time.Unix(1, 0)))
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(func(c *Config) error { c.SetPlaybackOverride("vacuumtube", "codecs", "vp9"); return nil }); err != nil {
		t.Fatal(err)
	}
	again := newStore(t, dir, clock.NewFake(time.Unix(2, 0)))
	if _, err := again.Load(); err != nil {
		t.Fatal(err)
	}
	if got := again.Current().PlaybackOverrides("vacuumtube"); got["codecs"] != "vp9" {
		t.Fatalf("after restart: %v", got)
	}
	if _, err := again.Update(func(c *Config) error { c.SetPlaybackOverride("vacuumtube", "codecs", ""); return nil }); err != nil {
		t.Fatal(err)
	}
	if again.Current().Playback != nil {
		t.Fatalf("back to automatic prunes the empty maps: %+v", again.Current().Playback)
	}
	if raw, _ := os.ReadFile(again.Path()); strings.Contains(string(raw), "playback") {
		t.Fatalf("config.json keeps no empty playback object:\n%s", raw)
	}
}

// The weather block is optional (absent = off), validated structurally and
// semantically (rule 10: at most 2 decimals), and deep-copied by Clone.
func TestWeatherBlock(t *testing.T) {
	base := defaultRaw(t)
	absent := mutateJSON(t, base, func(m map[string]any) { delete(m, "weather") })
	cfg, err := Parse(absent, testRules())
	if err != nil {
		t.Fatalf("a file without weather must keep loading: %v", err)
	}
	if w := cfg.WeatherSettings(); w.Enabled || w.Place != nil || w.Units != UnitsCelsius || !w.Scene {
		t.Fatalf("absent weather = %+v, want off/celsius/scene", w)
	}
	place := func(lat, lon float64) map[string]any {
		return map[string]any{"name": "Zagreb", "region": "City of Zagreb", "country": "Croatia", "latitude": lat, "longitude": lon}
	}
	good := mutateJSON(t, base, func(m map[string]any) {
		m["weather"] = map[string]any{"enabled": true, "place": place(45.81, 15.98), "units": "fahrenheit", "scene": false}
	})
	if _, err := Parse(good, testRules()); err != nil {
		t.Fatalf("valid weather rejected: %v", err)
	}
	for name, f := range map[string]func(m map[string]any){
		"enabled without place": func(m map[string]any) { m["weather"].(map[string]any)["enabled"] = true },
		"precise latitude":      func(m map[string]any) { m["weather"].(map[string]any)["place"] = place(45.815, 15.98) },
		"precise longitude":     func(m map[string]any) { m["weather"].(map[string]any)["place"] = place(45.81, 15.9819) },
		"unknown units":         func(m map[string]any) { m["weather"].(map[string]any)["units"] = "kelvin" },
		"a url":                 func(m map[string]any) { m["weather"].(map[string]any)["url"] = "https://example.com" },
	} {
		if _, err := Parse(mutateJSON(t, base, f), testRules()); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// ValidatePortable alone (the Update path) refuses what the schema would.
	c := Defaults()
	c.Weather = &Weather{Enabled: true, Units: UnitsCelsius}
	if err := ValidatePortable(c, testRules()); err == nil {
		t.Error("enabled without a place passed ValidatePortable")
	}
	c.Weather.Place = &contract.WeatherPlace{Name: "X", Latitude: 1.234, Longitude: 2}
	if err := ValidatePortable(c, testRules()); err == nil || !strings.Contains(err.Error(), "2 decimals") {
		t.Errorf("precise coordinates passed ValidatePortable: %v", err)
	}
	if RoundCoordinate(45.8150) != 45.82 && RoundCoordinate(45.8150) != 45.81 {
		t.Errorf("RoundCoordinate(45.815) = %v", RoundCoordinate(45.815))
	}
	if RoundCoordinate(-15.98765) != -15.99 || !TwoDecimals(RoundCoordinate(45.123456)) {
		t.Error("RoundCoordinate does not round to 2 decimals")
	}

	cfg2, _ := Parse(good, testRules())
	clone := cfg2.Clone()
	clone.Weather.Place.Name = "Elsewhere"
	clone.Weather.Units = UnitsCelsius
	if cfg2.Weather.Place.Name != "Zagreb" || cfg2.Weather.Units != UnitsFahrenheit {
		t.Fatal("Clone aliases the weather block")
	}
}

// remote.now_playing is optional (absent = on), a boolean, on in the product
// default, and deep-copied by Clone.
func TestRemoteNowPlaying(t *testing.T) {
	if !Defaults().Remote.ShowNowPlaying() || Defaults().Remote.NowPlaying == nil {
		t.Fatal("the product default must write now_playing: true")
	}
	base := defaultRaw(t)
	absent := mutateJSON(t, base, func(m map[string]any) { delete(m["remote"].(map[string]any), "now_playing") })
	cfg, err := Parse(absent, testRules())
	if err != nil {
		t.Fatalf("a file without remote.now_playing must keep loading: %v", err)
	}
	if !cfg.Remote.ShowNowPlaying() {
		t.Fatal("absent remote.now_playing must mean on")
	}
	off, err := Parse(mutateJSON(t, base, func(m map[string]any) { m["remote"].(map[string]any)["now_playing"] = false }), testRules())
	if err != nil || off.Remote.ShowNowPlaying() {
		t.Fatalf("now_playing false: err=%v show=%v", err, off.Remote.ShowNowPlaying())
	}
	if _, err := Parse(mutateJSON(t, base, func(m map[string]any) { m["remote"].(map[string]any)["now_playing"] = "yes" }), testRules()); err == nil {
		t.Fatal("a non-boolean now_playing was accepted")
	}
	clone := off.Clone()
	*clone.Remote.NowPlaying = true
	if off.Remote.ShowNowPlaying() {
		t.Fatal("Clone aliases remote.now_playing")
	}
}

// cec is optional (absent = off, PC volume), its volume_target is pc or tv
// (also on the Update path, which skips the schema), and Clone copies it.
func TestCECBlock(t *testing.T) {
	base := defaultRaw(t)
	cfg, err := Parse(base, testRules())
	if err != nil {
		t.Fatal(err)
	}
	if s := cfg.CECSettings(); s.Enabled || s.VolumeTarget != "pc" || s.TVVolume() {
		t.Fatalf("absent cec must be off with PC volume: %+v", s)
	}
	on, err := Parse(mutateJSON(t, base, func(m map[string]any) { m["cec"] = map[string]any{"enabled": true, "volume_target": "tv"} }), testRules())
	if err != nil || !on.CECSettings().TVVolume() {
		t.Fatalf("cec on, tv: err=%v %+v", err, on.CECSettings())
	}
	if _, err := Parse(mutateJSON(t, base, func(m map[string]any) { m["cec"] = map[string]any{"enabled": true, "volume_target": "soundbar"} }), testRules()); err == nil {
		t.Fatal("volume_target soundbar was accepted")
	}
	bad := on.Clone()
	bad.CEC.VolumeTarget = "soundbar"
	if err := ValidatePortable(bad, testRules()); err == nil || !strings.Contains(err.Error(), "cec.volume_target") {
		t.Fatalf("ValidatePortable accepted volume_target soundbar: %v", err)
	}
	if on.CEC.VolumeTarget != "tv" {
		t.Fatal("Clone aliases the cec block")
	}
	off := CEC{Enabled: false, VolumeTarget: "tv"}
	if off.TVVolume() {
		t.Fatal("volume_target tv must not apply while CEC is off")
	}
}
