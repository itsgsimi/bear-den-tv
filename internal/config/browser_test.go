// Tests for the web apps' browser in config (config.go BrowserName,
// StreamingBrowserName; validate.go launchFlatpakID, validateBrowsers):
// rule 3 for web adapters, contracts/config.md.

package config

import (
	"strings"
	"testing"
)

// Rule 3 for web adapters: each row runs in the browser config apps names
// for it (apps.browser for the Browser tile, apps.streaming_browser for the
// streaming sites; absent means Brave for the tile and Google Chrome for
// the streaming sites), and only table browsers exist: Chromium is not one.
func TestWebBrowserRule(t *testing.T) {
	const brave, chrome, chromium = "com.brave.Browser", "com.google.Chrome", "org.chromium.Chromium"
	for _, c := range []struct {
		name     string
		apps     map[string]any // config apps (nil: absent)
		rows     map[string]string
		wantErr  string // "" = valid
		browser  string
		streamed string
	}{
		{"defaults", nil, nil, "", "brave", "chrome"},
		{"browser tile in chrome", map[string]any{"auto_update": true, "browser": "chrome"}, map[string]string{"browser": chrome}, "", "chrome", "chrome"},
		{"streaming in brave", map[string]any{"auto_update": true, "streaming_browser": "brave"}, map[string]string{"netflix": brave, "disney-plus": brave, "hulu": brave}, "", "brave", "brave"},
		{"row left in brave", map[string]any{"auto_update": true, "browser": "chrome"}, nil, `application "browser" launch.app_id "com.brave.Browser" must be "com.google.Chrome" for adapter browser (apps.browser is chrome)`, "", ""},
		{"one site left behind", map[string]any{"auto_update": true, "streaming_browser": "brave"}, map[string]string{"netflix": brave, "disney-plus": brave}, `(apps.streaming_browser is brave)`, "", ""},
		{"chrome tile without the setting", nil, map[string]string{"browser": chrome}, `must be "com.brave.Browser" for adapter browser (apps.browser is brave)`, "", ""},
		{"not a web adapter", nil, map[string]string{"plex-htpc": brave}, `must be "tv.plex.PlexHTPC"`, "", ""},
		{"unknown browser", map[string]any{"auto_update": true, "browser": "firefox"}, nil, `browser`, "", ""},
		// Chromium is retired: a Flatpak app's row cannot name it either.
		{"chromium on a non-web row", nil, map[string]string{"moonlight": chromium}, `must be "com.moonlight_stream.Moonlight"`, "", ""},
	} {
		raw := mutateJSON(t, defaultRaw(t), func(m map[string]any) {
			if c.apps == nil {
				delete(m, "apps")
			} else {
				m["apps"] = c.apps
			}
			for _, a := range m["applications"].([]any) {
				am := a.(map[string]any)
				if id, ok := c.rows[am["id"].(string)]; ok {
					am["launch"].(map[string]any)["app_id"] = id
				}
			}
		})
		cfg, err := Parse(raw, testRules())
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
		case c.wantErr == "" && (cfg.BrowserName() != c.browser || cfg.StreamingBrowserName() != c.streamed):
			t.Errorf("%s: browsers %s/%s", c.name, cfg.BrowserName(), cfg.StreamingBrowserName())
		}
	}
	// The semantic check alone (the schema's enum aside) refuses a browser
	// outside the table.
	cfg := Defaults()
	cfg.Apps = &Apps{AutoUpdate: true, StreamingBrowser: "firefox"}
	if err := ValidatePortable(cfg, testRules()); err == nil || !strings.Contains(err.Error(), `apps.streaming_browser "firefox" is not a browser Bear Den runs`) {
		t.Fatalf("unknown browser: %v", err)
	}
}
