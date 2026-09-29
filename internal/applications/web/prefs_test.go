// Tests for a browser's own settings in Bear Den's profiles (prefs.go):
// Brave's rows land in its profile's Local State and Default/Preferences
// as nested keys beside what is already there; Chromium's profile gets no
// file at all; and a profile, profile root or prefs file that is a
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
	// Chromium's profiles get nothing written.
	cp, err := SeedPrefs(data, browser(t, "chromium"), "netflix")
	if err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(cp); len(entries) != 0 {
		t.Fatalf("Chromium's profile got files: %v", entries)
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
