// The config upgrade: a config.json written by an older version keeps its
// own applications list, so apps added in later versions (the optional
// apps, the web apps) would never appear, not even in Settings → Add apps.
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
// defaults have it so). Spec: docs/operations.md "Upgrading".

package config

import (
	"slices"

	"bear-den-tv/internal/contract"
)

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
