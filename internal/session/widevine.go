// Playback support for the streaming sites: once Chromium is installed, and
// when a streaming site is turned on, the coordinator runs each enabled
// site's profile once, headless, so Flathub Chromium's component updater
// fetches Widevine into it (web.Widevine), and reports
// state.applications[].install.drm: ready, preparing or pending ("Still
// setting up playback support"). Opening the site stops its quiet run
// (Chromium allows one process per profile; the real run fetches the CDM
// too). Spec: contracts/http.md "App installs", ADR 0010 and ADR 0011.

package session

import (
	"context"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
)

// WebDRM is the seam to web.Widevine (a fake in tests).
type WebDRM interface {
	Ready(appID string) bool
	Prepare(ctx context.Context, appID string) (bool, error)
}

// drmState is the playback-support bookkeeping (c.mu).
type drmState struct {
	ready     map[string]bool               // last check per streaming app
	preparing map[string]context.CancelFunc // quiet runs in progress
}

// isStreaming reports whether a config app is a streaming site (a web app
// in app mode; the Browser tile needs no Widevine).
func (c *Coordinator) isStreaming(a configApp) bool {
	ad, ok := c.opts.Adapters.ForName(a.Adapter)
	if !ok {
		return false
	}
	spec, web := adapters.WebOf(ad)
	return web && spec.Mode == adapters.WebModeApp
}

// drmForLocked is an app's install.drm ("" when it does not apply). c.mu
// is held.
func (c *Coordinator) drmForLocked(a configApp, installed bool) string {
	if c.opts.DRM == nil || !installed || !c.isStreaming(a) {
		return ""
	}
	switch {
	case c.drm.preparing[a.ID] != nil:
		return contract.DRMPreparing
	case c.drm.ready[a.ID]:
		return contract.DRMReady
	}
	return contract.DRMPending
}

// refreshDRM re-reads which streaming profiles have Widevine (a glob per
// app; the apps watch loop calls it) and publishes a change.
func (c *Coordinator) refreshDRM() {
	if c.opts.DRM == nil {
		return
	}
	now := map[string]bool{}
	for _, a := range c.opts.Config.Current().Applications {
		if c.isStreaming(a) {
			now[a.ID] = c.opts.DRM.Ready(a.ID)
		}
	}
	c.mu.Lock()
	changed := len(now) != len(c.drm.ready)
	for id, r := range now {
		changed = changed || c.drm.ready[id] != r
	}
	c.drm.ready = now
	c.mu.Unlock()
	if changed {
		c.publish()
	}
}

// prepareDRM runs the quiet first run for each app in turn, skipping apps
// that are ready or already preparing.
func (c *Coordinator) prepareDRM(appIDs []string) {
	if c.opts.DRM == nil || len(appIDs) == 0 {
		return
	}
	go func() {
		for _, id := range appIDs {
			if c.opts.DRM.Ready(id) {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			c.mu.Lock()
			if c.drm.preparing == nil {
				c.drm.preparing = map[string]context.CancelFunc{}
			}
			if c.drm.preparing[id] != nil {
				c.mu.Unlock()
				cancel()
				continue
			}
			c.drm.preparing[id] = cancel
			c.mu.Unlock()
			c.publish()
			c.log.Info("session: preparing playback support", "app", id)
			ok, err := c.opts.DRM.Prepare(ctx, id)
			cancel()
			c.mu.Lock()
			delete(c.drm.preparing, id)
			c.mu.Unlock()
			c.log.Info("session: playback support", "app", id, "ready", ok, "err", err)
			c.refreshDRM()
			c.publish()
		}
	}()
}

// stopDRMPrep ends appID's quiet run (the site is opening for real).
func (c *Coordinator) stopDRMPrep(appID string) {
	c.mu.Lock()
	cancel := c.drm.preparing[appID]
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// prepareEnabledStreaming prepares the enabled streaming sites that use
// flatpakID ("" = any) and are installed.
func (c *Coordinator) prepareEnabledStreaming(flatpakID string) {
	var ids []string
	c.mu.Lock()
	for _, a := range c.opts.Config.Current().Applications {
		if (flatpakID == "" || a.Launch.AppID == flatpakID) && a.IsEnabled() && c.isStreaming(a) && c.appLocked(a.ID).install.Installed {
			ids = append(ids, a.ID)
		}
	}
	c.mu.Unlock()
	c.prepareDRM(ids)
}
