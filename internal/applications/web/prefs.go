// A browser's own settings in Bear Den's profiles: before every start (a web
// app, or Widevine's quiet run) the browser row's LocalState prefs go into
// <profile>/Local State and its Preferences into <profile>/Default/Preferences
// (adapters.BrowserInfo; for Chrome no first-run, default-browser, sign-in
// to Chrome, password, autofill or translate prompts, and a clean exit so no
// "Restore pages?" bubble; for Brave the Widevine opt-in and quieter kiosk
// defaults), and for a browser that bundles Widevine (Chrome) an empty file
// at <profile>/WidevineCdm, as the Flatpak's own launcher makes for its
// default profile, so the bundled CDM is used and none is fetched into the
// profile. Only a profile Bear Den owns is touched:
// the path comes from ProfileDir (under $XDG_DATA_HOME/bear-den-tv), and a
// symbolic link anywhere from the profile root down is refused, so a
// profile cannot be pointed at a personal browser's folder. Other keys in
// those files are kept. Spec: docs/decisions/0013-brave-as-a-browser-choice.md,
// docs/decisions/0014-google-chrome-for-streaming-brave-for-browser.md.

package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"bear-den-tv/internal/applications/adapters"
)

// Profile file names inside a browser profile (Chromium's layout).
const (
	LocalStateFile  = "Local State"
	PreferencesFile = "Preferences"
	defaultProfile  = "Default"
	// WidevineFile is the component-updated CDM's folder name in a profile;
	// BlockProfileWidevine puts an empty file there instead.
	WidevineFile = "WidevineCdm"
)

// SeedPrefs writes browser b's prefs into appID's profile and returns the
// profile directory (created if missing). A browser with no prefs writes
// nothing.
func SeedPrefs(dataHome string, b adapters.BrowserInfo, appID string) (string, error) {
	root, err := ProfileRoot(dataHome, b)
	if err != nil {
		return "", err
	}
	profile, err := ProfileDir(dataHome, b, appID)
	if err != nil {
		return "", err
	}
	if err := ownedDir(root); err != nil {
		return "", err
	}
	if err := ownedDir(profile); err != nil {
		return "", err
	}
	if b.BlockProfileWidevine {
		if err := blockFile(filepath.Join(profile, WidevineFile)); err != nil {
			return "", err
		}
	}
	if len(b.LocalState) > 0 {
		if err := mergePrefs(filepath.Join(profile, LocalStateFile), b.LocalState); err != nil {
			return "", err
		}
	}
	if len(b.Preferences) > 0 {
		dir := filepath.Join(profile, defaultProfile)
		if err := ownedDir(dir); err != nil {
			return "", err
		}
		if err := mergePrefs(filepath.Join(dir, PreferencesFile), b.Preferences); err != nil {
			return "", err
		}
	}
	return profile, nil
}

// ownedDir makes dir (0700) if missing and refuses anything that is not a
// plain directory: a symbolic link could send Bear Den's writes into a
// profile it does not own.
func ownedDir(dir string) error {
	fi, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("web: profile: %w", err)
		}
		fi, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("web: profile: %w", err)
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("web: %s is not a folder Bear Den owns", dir)
	}
	return nil
}

// blockFile makes an empty file at path (0600) unless something is there
// already; a symbolic link is refused, a folder (a CDM fetched before) is
// left as it is.
func blockFile(path string) error {
	fi, err := os.Lstat(path)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("web: %s is not a file Bear Den owns", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("web: profile: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("web: profile: %w", err)
	}
	return f.Close()
}

// mergePrefs sets each dotted pref path in the JSON file at path, keeping
// every other key; a file that is not a JSON object is replaced (the
// browser resets a broken prefs file itself). Written through a temporary
// file and a rename, mode 0600.
func mergePrefs(path string, prefs map[string]any) error {
	if fi, err := os.Lstat(path); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("web: %s is not a file Bear Den owns", path)
	}
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(raw, &doc) != nil || doc == nil {
			doc = map[string]any{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("web: profile: %w", err)
	}
	keys := make([]string, 0, len(prefs))
	for k := range prefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		setPref(doc, strings.Split(k, "."), prefs[k])
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := path + ".bdtv-tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("web: profile: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("web: profile: %w", err)
	}
	return nil
}

// setPref sets doc[a][b]...[z] = v, replacing any non-object on the way.
func setPref(doc map[string]any, path []string, v any) {
	for _, p := range path[:len(path)-1] {
		next, ok := doc[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			doc[p] = next
		}
		doc = next
	}
	doc[path[len(path)-1]] = v
}
