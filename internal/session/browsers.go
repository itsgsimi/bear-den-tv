// The web apps' browser (owner decision, docs/decisions/0013-brave-as-a-browser-choice.md):
// IPC apps.browser (TV Settings → Streaming sites) stores config
// apps.browser (the Browser tile) and apps.streaming_browser (the streaming
// sites), each a browser from the adapter table (adapters.Browsers), and in
// the same config write moves every web app's launch.app_id to its
// browser's Flatpak id (config rule 3). Discovery, installs, launches and
// Widevine all follow launch.app_id, so nothing else branches on the
// browser. Apps that moved are re-discovered, and the enabled streaming
// sites get their quiet Widevine run in the new browser. A web app already
// running keeps its browser until it is closed.

package session

import (
	"context"
	"errors"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

// setBrowsers is IPC apps.browser.
func (c *Coordinator) setBrowsers(browser, streaming string) error {
	b, okB := adapters.BrowserNamed(browser)
	s, okS := adapters.BrowserNamed(streaming)
	if !okB || !okS {
		return errors.New("That browser is not one Bear Den runs.")
	}
	var moved []string // app ids whose browser changed
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		apps := config.Apps{AutoUpdate: cfg.AutoUpdate()}
		if cfg.Apps != nil {
			apps = *cfg.Apps
		}
		apps.Browser, apps.StreamingBrowser = b.Name, s.Name
		cfg.Apps = &apps
		moved = moved[:0]
		for i := range cfg.Applications {
			a := &cfg.Applications[i]
			spec, web := c.webSpecForAdapter(a.Adapter)
			if !web {
				continue
			}
			want := b.FlatpakID
			if spec.IsStreaming() {
				want = s.FlatpakID
			}
			if a.Launch.AppID != want {
				a.Launch.AppID = want
				moved = append(moved, a.ID)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if len(moved) == 0 {
		c.publish()
		return nil
	}
	c.mu.Lock()
	for _, id := range moved {
		// Until discovery has looked at the new browser, say nothing about
		// its installation rather than the old browser's.
		rt := c.appLocked(id)
		rt.install, rt.discovered = applications.Installation{Scope: "unknown"}, false
	}
	c.mu.Unlock()
	var stopped []<-chan struct{}
	for _, id := range moved {
		stopped = append(stopped, c.stopDRMPrep(id)) // a quiet run in the old browser
	}
	c.log.Info("session: web apps' browser", "browser", b.Name, "streaming_browser", s.Name, "moved", moved)
	c.publish()
	go func() {
		for _, done := range stopped {
			<-done // so the new browser's run is not skipped as "already preparing"
		}
		seen := map[string]bool{}
		for _, fid := range []string{b.FlatpakID, s.FlatpakID} {
			if !seen[fid] {
				seen[fid] = true
				c.rediscover(context.Background(), fid)
			}
		}
		c.refreshDRM()
		c.prepareEnabledStreaming(s.FlatpakID)
	}()
	return nil
}

// webSpecForAdapter is the web spec of a config adapter name.
func (c *Coordinator) webSpecForAdapter(adapter string) (adapters.WebSpec, bool) {
	ad, ok := c.opts.Adapters.ForName(adapter)
	if !ok {
		return adapters.WebSpec{}, false
	}
	return adapters.WebOf(ad)
}
