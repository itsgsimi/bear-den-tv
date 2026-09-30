// Tests for a browser's own settings in Bear Den's profiles (prefs.go):
// Brave's rows land in its profile's Local State and Default/Preferences
// as nested keys beside what is already there; Google Chrome's rows land
// in its own profile only, with the empty WidevineCdm file that keeps it on
// its bundled CDM; and a profile, profile root or prefs file that is a
// symbolic link (for example into a personal browser's folder) is refused
// with nothing written through it.

package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bear-den-tv/internal/applications/adapters"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

func at(m map[string]any, path ...string) any {
	var v any = m
	for _, p := range path {
		mm, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = mm[p]
	}
	return v
}

func TestSeedPrefsWritesBraveRowsIntoItsOwnProfile(t *testing.T) {
	data := t.TempDir()
	brave := browser(t, "brave")
	profile, err := ProfileDir(data, brave, "netflix")
	if err != nil {
		t.Fatal(err)
	}
	// A profile Brave has used: its own keys must survive.
	if err := os.MkdirAll(filepath.Join(profile, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(profile, "Local State"), []byte(`{"browser":{"enabled_labs_experiments":[]},"brave":{"other":1}}`), 0o600)
	_ = os.WriteFile(filepath.Join(profile, "Default", "Preferences"), []byte(`{"profile":{"name":"Person 1"}}`), 0o600)

	got, err := SeedPrefs(data, brave, "netflix")
	if err != nil || got != profile {
		t.Fatalf("SeedPrefs = %q, %v", got, err)
	}
	ls := readJSON(t, filepath.Join(profile, "Local State"))
	if at(ls, "brave", "dark_mode") != float64(1) {
		t.Fatalf("Brave's browser UI is not set to dark: %v", at(ls, "brave", "dark_mode"))
	}
	if at(ls, "brave", "widevine_opted_in") != true || at(ls, "brave", "other") != float64(1) || at(ls, "browser", "enabled_labs_experiments") == nil {
		t.Fatalf("Local State %v", ls)
	}
	prefs := readJSON(t, filepath.Join(profile, "Default", "Preferences"))
	for k, v := range brave.Preferences {
		if got := at(prefs, splitDots(k)...); got != v {
			t.Errorf("%s = %v, want %v", k, got, v)
		}
	}
	if at(prefs, "profile", "name") != "Person 1" {
		t.Fatalf("the profile's own keys were lost: %v", prefs)
	}
	if st, _ := os.Stat(filepath.Join(profile, "Local State")); st.Mode().Perm() != 0o600 {
		t.Fatalf("Local State mode %v", st.Mode())
	}
}

// Google Chrome: no first-run, default-browser, sign-in-to-Chrome, password,
// autofill or translate prompts and a clean exit, written into Bear Den's
// Chrome profile for that app only (never Brave's, never another app's,
// never Chrome's own default profile under ~/.var/app), nothing in Local
// State, and an empty WidevineCdm file (a symbolic link there is refused,
// a folder left alone).
func TestSeedPrefsWritesChromeRowsIntoItsOwnProfile(t *testing.T) {
	data := t.TempDir()
	chrome := browser(t, "chrome")
	for k, want := range map[string]any{
		"browser.check_default_browser":  false,
		"signin.allowed":                 false,
		"credentials_enable_service":     false,
		"credentials_enable_autosignin":  false,
		"translate.enabled":              false,
		"browser.has_seen_welcome_page":  true,
		"profile.exit_type":              "Normal",
		"autofill.credit_card_enabled":   false,
		"signin.allowed_on_next_startup": false,
	} {
		if chrome.Preferences[k] != want {
			t.Errorf("chrome pref %s = %v, want %v", k, chrome.Preferences[k], want)
		}
	}
	profile, err := SeedPrefs(data, chrome, "netflix")
	if err != nil || profile != filepath.Join(data, "bear-den-tv", "web-chrome", "netflix") {
		t.Fatalf("SeedPrefs = %q, %v", profile, err)
	}
	prefs := readJSON(t, filepath.Join(profile, "Default", "Preferences"))
	for k, v := range chrome.Preferences {
		if got := at(prefs, splitDots(k)...); got != v {
			t.Errorf("%s = %v, want %v", k, got, v)
		}
	}
	if _, err := os.Stat(filepath.Join(profile, "Local State")); !os.IsNotExist(err) {
		t.Errorf("Chrome's Local State was written: %v", err)
	}
	wv, err := os.Lstat(filepath.Join(profile, WidevineFile))
	if err != nil || !wv.Mode().IsRegular() || wv.Size() != 0 || wv.Mode().Perm() != 0o600 {
		t.Fatalf("WidevineCdm %v %v", wv, err)
	}
	// Nothing anywhere else: one profile folder under web-chrome, nothing
	// under web-brave, and no Chrome folder outside bear-den-tv.
	entries, _ := os.ReadDir(filepath.Join(data, "bear-den-tv"))
	if len(entries) != 1 || entries[0].Name() != "web-chrome" {
		t.Fatalf("bear-den-tv holds %v", entries)
	}
	if apps, _ := os.ReadDir(filepath.Join(data, "bear-den-tv", "web-chrome")); len(apps) != 1 {
		t.Fatalf("web-chrome holds %v", apps)
	}
	if top, _ := os.ReadDir(data); len(top) != 1 {
		t.Fatalf("data home holds %v", top)
	}
	// Again: kept as it is.
	if _, err := SeedPrefs(data, chrome, "netflix"); err != nil {
		t.Fatal(err)
	}
	// A CDM folder fetched before is left alone.
	hulu, _ := ProfileDir(data, chrome, "hulu")
	if err := os.MkdirAll(filepath.Join(hulu, WidevineFile, "4.10.3050.0"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedPrefs(data, chrome, "hulu"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(hulu, WidevineFile, "4.10.3050.0")); err != nil || !fi.IsDir() {
		t.Fatalf("the fetched CDM folder was touched: %v", err)
	}
	// A symbolic link at WidevineCdm is refused.
	disney, _ := ProfileDir(data, chrome, "disney-plus")
	if err := os.MkdirAll(disney, 0o700); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(disney, WidevineFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedPrefs(data, chrome, "disney-plus"); err == nil {
		t.Fatal("a symbolic link at WidevineCdm was accepted")
	}
	if left, _ := os.ReadDir(elsewhere); len(left) != 0 {
		t.Fatalf("written through the link: %v", left)
	}
}

func splitDots(k string) []string {
	var out []string
	start := 0
	for i := 0; i < len(k); i++ {
		if k[i] == '.' {
			out = append(out, k[start:i])
			start = i + 1
		}
	}
	return append(out, k[start:])
}

func TestSeedPrefsWritesOnlyInProfilesBearDenOwns(t *testing.T) {
	brave := browser(t, "brave")
	// A personal Brave profile elsewhere, which Bear Den must never touch.
	personal := t.TempDir()
	personalState := filepath.Join(personal, "Local State")
	if err := os.WriteFile(personalState, []byte(`{"mine":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	untouched := func(what string) {
		t.Helper()
		raw, _ := os.ReadFile(personalState)
		if string(raw) != `{"mine":true}` {
			t.Fatalf("%s: the personal profile was written: %s", what, raw)
		}
		if entries, _ := os.ReadDir(personal); len(entries) != 1 {
			t.Fatalf("%s: files appeared in the personal profile: %v", what, entries)
		}
	}
	cases := map[string]func(data string){
		// The profile itself is a link to the personal profile.
		"profile link": func(data string) {
			root, _ := ProfileRoot(data, brave)
			_ = os.MkdirAll(root, 0o700)
			_ = os.Symlink(personal, filepath.Join(root, "netflix"))
		},
		// The whole profile root is a link.
		"root link": func(data string) {
			_ = os.MkdirAll(filepath.Join(data, "bear-den-tv"), 0o700)
			_ = os.Symlink(personal, filepath.Join(data, "bear-den-tv", "web-brave"))
		},
		// Local State in Bear Den's profile links to the personal one.
		"file link": func(data string) {
			p, _ := ProfileDir(data, brave, "netflix")
			_ = os.MkdirAll(p, 0o700)
			_ = os.Symlink(personalState, filepath.Join(p, "Local State"))
		},
	}
	for name, setup := range cases {
		data := t.TempDir()
		setup(data)
		if _, err := SeedPrefs(data, brave, "netflix"); err == nil {
			t.Errorf("%s: seeded through a link", name)
		}
		untouched(name)
	}
	if _, err := SeedPrefs(t.TempDir(), adapters.BrowserInfo{Name: "x", FlatpakID: "org.example.X", ProfileRoot: "../../etc"}, "netflix"); err == nil {
		t.Error("a profile root outside bear-den-tv accepted")
	}
}
