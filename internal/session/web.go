// Web apps in the coordinator: routing nav/select/back/text/media and the
// touchpad's pointer actions to a web app's page (internal/applications/web)
// after verifying its window is the foreground, their capabilities, the
// pointer rate limits, Home's pause for web pages, and TV Settings →
// Streaming sites (IPC app.enable). Spec: contracts/actions.md ("Web apps",
// "Pointer"), docs/decisions/0010-web-apps-over-cdp-pipe.md.

package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

// WebApps is the seam to the web app manager (web.Manager; a fake in tests).
type WebApps interface {
	Launch(ctx context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error)
	// Running reports a live DevTools connection to the app.
	Running(appID string) bool
	// Status is what the app's current page last reported.
	Status(appID string) (web.Status, bool)
	Apply(ctx context.Context, appID, action string, args map[string]any) (web.Outcome, error)
	Pointer(ctx context.Context, appID, action string, args map[string]any) error
	PauseIfPlaying(ctx context.Context, appID string) (bool, error)
	Close(ctx context.Context, appID string, force bool) error
	// Watch registers the coordinator's change callback.
	Watch(fn func())
}

// backendWeb is the capability backend for web app input.
const backendWeb = "web-cdp"

// webSpecFor returns the web spec of appID's adapter, if it is a web app.
func (c *Coordinator) webSpecFor(appID string) (adapters.WebSpec, bool) {
	ad, ok := c.adapterFor(appID)
	if !ok {
		return adapters.WebSpec{}, false
	}
	return adapters.WebOf(ad)
}

// isWebAdapter reports whether a config adapter name is a web adapter.
func (c *Coordinator) isWebAdapter(name string) bool {
	ad, ok := c.opts.Adapters.ForName(name)
	if !ok {
		return false
	}
	_, web := adapters.WebOf(ad)
	return web
}

// webInputCapLocked is the capability of a web app in front for nav,
// select, back, text, media and pointer actions. c.mu is held.
func (c *Coordinator) webInputCapLocked(action, appID, label string, desk map[string]platform.Capability) contract.Capability {
	switch {
	case c.opts.Web == nil:
		return unavailable("Web apps are not available in this session.")
	case !desk[platform.CapObserveForeground].Available:
		return unavailable("This desktop cannot confirm " + label + " is in front, so Bear Den will not send it input.")
	case !c.opts.Web.Running(appID):
		return unavailable("Bear Den is not connected to " + label + ". Close it and open it again from Home.")
	}
	st, _ := c.opts.Web.Status(appID)
	switch action {
	case contract.ActionTextSubmit:
		if !st.TextField {
			return unavailable("No text field is focused.")
		}
	case contract.ActionMediaPlay, contract.ActionMediaPause, contract.ActionMediaSeek:
		if st.Video == "" || st.Video == "none" {
			return unavailable("No video on this page.")
		}
	}
	cp := available(backendWeb)
	if contract.IsNav(action) {
		cp.Holdable = true
	}
	return cp
}

// pointerCapsLocked fills the pointer capabilities: only a web app in
// front, never anything else. c.mu is held.
func (c *Coordinator) pointerCapsLocked(caps map[string]contract.Capability, desk map[string]platform.Capability) {
	for _, a := range []string{contract.ActionPointerMove, contract.ActionPointerClick, contract.ActionPointerScroll} {
		cp := unavailable("The touchpad works only in web apps.")
		if c.target.Kind == "app" {
			appID := strOr(c.target.AppID)
			if _, ok := c.webSpecFor(appID); ok {
				cp = c.webInputCapLocked(a, appID, c.target.Label, desk)
				cp.Holdable = false
			}
		}
		caps[a] = cp
	}
}

// verifyForeground re-reads the active window right before input reaches a
// page: the same rule as an XTEST key (platform.DesktopAdapter.DeliverKey).
func (c *Coordinator) verifyForeground(ctx context.Context, win platform.WindowID) error {
	fg, err := c.opts.Desktop.ObserveForeground(ctx)
	if err != nil || !fg.Known || win == 0 || fg.Window.ID != win {
		return platform.ErrNotForeground
	}
	return nil
}

// webFailure maps a web manager error to a failed result.
func (c *Coordinator) webFailure(req contract.ActionRequest, label string, err error) contract.ActionResult {
	var ref *web.RefusedError
	switch {
	case errors.As(err, &ref):
		return c.fail(req, contract.CodeUnsupported, ref.Reason)
	case errors.Is(err, web.ErrNotRunning):
		return c.fail(req, contract.CodeNoTarget, "Bear Den is not connected to "+label+". Close it and open it again from Home.")
	case errors.Is(err, web.ErrNoPage):
		return c.fail(req, contract.CodeBusy, label+" is still loading its page.")
	case errors.Is(err, context.DeadlineExceeded):
		return c.fail(req, contract.CodeTimeout, label+" did not answer in time.")
	}
	return c.fail(req, contract.CodeInternal, label+" could not be reached.")
}

// routeWeb delivers nav, select, back, text.submit and media to the web app
// in front. The caller checked the capability.
func (c *Coordinator) routeWeb(ctx context.Context, req contract.ActionRequest, target contract.Target) contract.ActionResult {
	c.mu.Lock()
	win := c.targetWindow
	c.mu.Unlock()
	if err := c.verifyForeground(ctx, win); err != nil {
		return c.fail(req, contract.CodeTargetUnfocused, target.Label+" lost focus before the input was sent.")
	}
	out, err := c.opts.Web.Apply(ctx, strOr(target.AppID), req.Action, req.Args)
	if err != nil {
		return c.webFailure(req, target.Label, err)
	}
	defer c.publish() // the page's text field or video state may have changed
	if out.Observed {
		return c.result(req, contract.OutcomeObserved, out.Detail)
	}
	return c.result(req, contract.OutcomeDelivered, out.Detail)
}

// doPointer routes a touchpad action: rate limit, capability, foreground.
func (c *Coordinator) doPointer(ctx context.Context, s sender, req contract.ActionRequest, target contract.Target) contract.ActionResult {
	if !c.pointerAllow(s.key, req.Action) {
		return c.fail(req, contract.CodeRateLimited, fmt.Sprintf("Too many %s in one second.", req.Action))
	}
	cp, code := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, code, cp.Reason)
	}
	if req.Target != "active" && req.Target != strOr(target.AppID) {
		return c.fail(req, contract.CodeTargetUnfocused, "That application is not in the foreground.")
	}
	c.mu.Lock()
	win := c.targetWindow
	c.mu.Unlock()
	if err := c.verifyForeground(ctx, win); err != nil {
		return c.fail(req, contract.CodeTargetUnfocused, target.Label+" lost focus before the pointer moved.")
	}
	if err := c.opts.Web.Pointer(ctx, strOr(target.AppID), req.Action, req.Args); err != nil {
		return c.webFailure(req, target.Label, err)
	}
	return c.result(req, contract.OutcomeDelivered, nil)
}

// pointerBucket is one sender's allowance for one pointer action.
type pointerBucket struct {
	tokens float64
	last   time.Time
}

// pointerAllow is a token bucket per sender and pointer action on the
// coordinator clock: contract.PointerRates per second, bursts of one second.
func (c *Coordinator) pointerAllow(senderKey, action string) bool {
	rate := float64(contract.PointerRates[action])
	if rate <= 0 {
		return false
	}
	now := c.clock.Now()
	key := senderKey + "\x00" + action
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pointerBuckets == nil {
		c.pointerBuckets = map[string]*pointerBucket{}
	}
	b := c.pointerBuckets[key]
	if b == nil {
		b = &pointerBucket{tokens: rate, last: now}
		c.pointerBuckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * rate
	if b.tokens > rate {
		b.tokens = rate
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// pauseWebForHome is Home's pause for a web app in front (home_policy
// pause-if-supported): the site's own pause key, only when the page reports
// a playing video, only into the verified foreground. It returns the detail
// for Home's result.
func (c *Coordinator) pauseWebForHome(ctx context.Context, target contract.Target) map[string]any {
	appID := strOr(target.AppID)
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok || app.HomePolicy != "pause-if-supported" || c.opts.Web == nil {
		return nil
	}
	if _, isWeb := c.webSpecFor(appID); !isWeb {
		return nil
	}
	c.mu.Lock()
	win := c.targetWindow
	c.mu.Unlock()
	if c.verifyForeground(ctx, win) != nil {
		return map[string]any{"paused": false}
	}
	paused, err := c.opts.Web.PauseIfPlaying(ctx, appID)
	if err != nil {
		c.log.Info("session: web app not paused for Home", "app", appID, "err", err)
		return map[string]any{"paused": false}
	}
	return map[string]any{"paused": paused}
}

// setAppEnabled is IPC app.enable (TV Settings → Streaming sites): turns a
// web app on or off in config.json. Anything but a web app fails closed.
func (c *Coordinator) setAppEnabled(_ context.Context, appID string, enabled bool) error {
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok {
		return errors.New("That application is not registered.")
	}
	if !c.isWebAdapter(app.Adapter) {
		return errors.New("Only streaming sites and the browser can be turned off here.")
	}
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		for i := range cfg.Applications {
			if cfg.Applications[i].ID == appID {
				v := enabled
				cfg.Applications[i].Enabled = &v
			}
		}
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	if enabled && c.isStreaming(app) {
		// Chromium already installed: fetch Widevine for this site now
		// (widevine.go); with Chromium missing the install card follows.
		c.prepareEnabledStreaming(app.Launch.AppID)
	}
	return nil
}
