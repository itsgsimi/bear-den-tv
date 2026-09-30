// The config upgrade: a config.json written by an older version keeps its
// own applications list, so apps added in later versions (the optional
// apps, the web apps) would never appear, not even in Apps → Add apps.
// When the coordinator starts it calls Store.UpgradeApps: every application
// of the built-in defaults (contracts/fixtures/config.default.valid.json)
// whose id and adapter the config does not have yet is appended exactly as
// the defaults have it (optional apps hide_when_missing, streaming sites
// off, a web row in the browser config apps names for it), and its id is
// appended to the application sections the defaults put it in, when the
// config has a section with that id. Nothing that is already there changes:
// no row, order, enabled flag, layout or setting. The write goes through
// the normal path (revision + 1, config.history, last-known-good); a
// second run adds nothing. The config cannot say "the owner removed this
// app", so an app removed by hand comes back (hidden or off where the
// defaults have it so).
//
// The retired browser (ADR 0014): Flathub Chromium is no longer a browser
// Bear Den runs. A config.json of that era (apps.browser or
// apps.streaming_browser "chromium", web rows with launch.app_id
// org.chromium.Chromium) is moved while it is parsed (upgradeRetiredBrowser,
// before the schema check, which no longer knows "chromium"): the streaming
// sites to Google Chrome, the Browser tile to Brave, or to the browser the
// config already names when that is not Chromium. When the coordinator
// starts, Store.UpgradeBrowsers writes the moved config through the normal
// path and logs which apps moved; a second run finds nothing. Profiles in
// the old Chromium folder are left in place, and Chromium itself is never
// uninstalled. Spec: docs/operations.md "Upgrading".

package config

import (
	"encoding/json"
	"slices"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
)

// The retired browser (ADR 0014): its config name and Flatpak id are known
// here only, to move an older config.json off it.
const (
	RetiredBrowserName      = "chromium"
	RetiredBrowserFlatpakID = "org.chromium.Chromium"
)

// UpgradeBrowsers writes a config.json that was moved off the retired
// browser while it loaded (revision + 1, through the normal path) and
// returns the application ids that moved (nil when none did; then nothing
// is written). Only a config.json that loaded is written.
func (s *Store) UpgradeBrowsers() ([]string, error) {
	s.mu.Lock()
	moved := s.retiredMoved
	s.retiredMoved = nil
	s.mu.Unlock()
	if s.Report().Source != SourceConfig || len(moved) == 0 {
		return nil, nil
	}
	if _, err := s.update(func(*Config) error { return nil }, false, false); err != nil {
		return nil, err
	}
	s.opts.Logger.Info("config: moved the web apps off Chromium (no longer used)", "apps", moved, "browser", s.Current().BrowserName(), "streaming_browser", s.Current().StreamingBrowserName(), "revision", s.Revision())
	return moved, nil
}

// upgradeRetiredBrowser moves a raw config document off the retired
// browser: apps.browser and apps.streaming_browser "chromium" become the
// defaults (brave, chrome), and every web row whose launch.app_id is
// Chromium's moves to the browser apps names for it. It returns the
// document (raw itself when nothing matched) and the moved rows' ids (plus
// "apps" when only the setting changed). A document that is not a JSON
// object is returned as it is, for Parse to refuse.
func upgradeRetiredBrowser(raw []byte) ([]byte, []string) {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return raw, nil
	}
	var moved []string
	names := map[bool]string{false: adapters.DefaultTileBrowser, true: adapters.DefaultStreamingBrowser}
	if apps, ok := doc["apps"].(map[string]any); ok {
		for key, streaming := range map[string]bool{"browser": false, "streaming_browser": true} {
			switch v, _ := apps[key].(string); v {
			case RetiredBrowserName:
				apps[key] = names[streaming]
				moved = append(moved, "apps")
			case "":
			default:
				names[streaming] = v
			}
		}
	}
	rows, _ := doc["applications"].([]any)
	for _, r := range rows {
		row, _ := r.(map[string]any)
		launch, _ := row["launch"].(map[string]any)
		adapter, _ := row["adapter"].(string)
		if launch == nil || launch["app_id"] != RetiredBrowserFlatpakID {
			continue
		}
		ad, ok := adapters.ForName(adapter)
		if !ok {
			continue
		}
		spec, web := adapters.WebOf(ad)
		if !web {
			continue
		}
		b, ok := adapters.BrowserNamed(names[spec.IsStreaming()])
		if !ok {
			continue // an unknown browser name: Parse says so
		}
		launch["app_id"] = b.FlatpakID
		id, _ := row["id"].(string)
		moved = append(moved, id)
	}
	if len(moved) == 0 {
		return raw, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw, nil
	}
	return out, slices.Compact(moved)
}

// UpgradeApps adds the default applications the running config lacks and
// returns their ids (nil when nothing was missing; then nothing is written).
// Only a config.json that loaded is upgraded: running on the
// last-known-good copy or the defaults, config.json stays as it is.
func (s *Store) UpgradeApps() ([]string, error) {
	if s.Report().Source != SourceConfig {
		return nil, nil
	}
	if missing := missingDefaults(s.Current()); len(missing) == 0 {
		return nil, nil
	}
	var added []string
	if _, err := s.update(func(c *Config) error {
		added = upgradeApps(c)
		return nil
	}, false, false); err != nil {
		return nil, err
	}
	s.opts.Logger.Info("config: added the apps this version knows", "apps", added, "revision", s.Revision())
	return added, nil
}

// missingDefaults lists the default applications c has neither by id nor by
// adapter (an owner's own row for an adapter stands for it; web adapters
// may appear only once, rule 11).
func missingDefaults(c Config) []Application {
	ids, adapters := map[string]bool{}, map[string]bool{}
	for _, a := range c.Applications {
		ids[a.ID], adapters[a.Adapter] = true, true
	}
	var out []Application
	for _, d := range Defaults().Applications {
		if !ids[d.ID] && !adapters[d.Adapter] {
			out = append(out, d)
		}
	}
	return out
}

// upgradeApps appends the missing default rows and section memberships to
// c and returns the added ids.
func upgradeApps(c *Config) []string {
	missing := missingDefaults(*c)
	if len(missing) == 0 {
		return nil
	}
	added := make([]string, 0, len(missing))
	for _, d := range missing {
		// A web row runs in the browser this config chose (rule 3).
		if want, _ := launchFlatpakID(*c, d.Adapter, DefaultAdapters[d.Adapter]); want != "" {
			d.Launch.AppID = want
		}
		c.Applications = append(c.Applications, d)
		added = append(added, d.ID)
	}
	for _, ds := range Defaults().Sections {
		i := slices.IndexFunc(c.Sections, func(s contract.Section) bool { return s.ID == ds.ID })
		if i < 0 || c.Sections[i].Kind != ds.Kind {
			continue // the owner's layout has no such section: leave it
		}
		for _, id := range ds.ApplicationIDs {
			if slices.Contains(added, id) && !slices.Contains(c.Sections[i].ApplicationIDs, id) {
				c.Sections[i].ApplicationIDs = append(c.Sections[i].ApplicationIDs, id)
			}
		}
	}
	return added
}
