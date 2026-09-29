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
// streaming sites; absent means Chromium), and only table browsers exist.
func TestWebBrowserRule(t *testing.T) {
	const brave = "com.brave.Browser"
	for _, c := range []struct {
		name     string
		apps     map[string]any // config apps (nil: absent)
		rows     map[string]string
		wantErr  string // "" = valid
		browser  string
		streamed string
	}{
		{"defaults", nil, nil, "", "chromium", "chromium"},
		{"browser tile in brave", map[string]any{"auto_update": true, "browser": "brave"}, map[string]string{"browser": brave}, "", "brave", "chromium"},
		{"streaming in brave", map[string]any{"auto_update": true, "streaming_browser": "brave"}, map[string]string{"netflix": brave, "disney-plus": brave, "hulu": brave}, "", "chromium", "brave"},
		{"row left in chromium", map[string]any{"auto_update": true, "browser": "brave"}, nil, `application "browser" launch.app_id "org.chromium.Chromium" must be "com.brave.Browser" for adapter browser (apps.browser is brave)`, "", ""},
		{"one site left behind", map[string]any{"auto_update": true, "streaming_browser": "brave"}, map[string]string{"netflix": brave, "disney-plus": brave}, `(apps.streaming_browser is brave)`, "", ""},
		{"brave without the setting", nil, map[string]string{"browser": brave}, `must be "org.chromium.Chromium" for adapter browser (apps.browser is chromium)`, "", ""},
		{"not a web adapter", nil, map[string]string{"plex-htpc": brave}, `must be "tv.plex.PlexHTPC"`, "", ""},
		{"unknown browser", map[string]any{"auto_update": true, "browser": "firefox"}, nil, `browser`, "", ""},
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
