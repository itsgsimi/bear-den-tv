// Sign-ins on the TV (docs/operations.md "Sign-ins on the TV"; IPC
// link.open). An app Bear Den opened asks the desktop to open a web page,
// most often to sign in (Spotify's Log in); `bear-den-tv open-url`, the
// desktop's handler for http and https links, hands it here. While Bear Den
// or one of its apps is in front, the link opens in the Browser tile, where
// the remote's arrows and the phone's keyboard work, and the app that asked
// is kept open (opening an app normally closes the others). When the page
// hands its result back to the app on this machine (a navigation to a
// loopback address, web.Manager.OnLoopback), Bear Den closes that tab and
// brings the app back. Anything else, the link goes to the desktop's own
// browser (the command does that when the reply says not handled).

package session

import (
	"context"
	"net/url"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

// linkWeb is what a web manager offers for links (web.Manager does; the
// fakes need not).
type linkWeb interface {
	LaunchAt(ctx context.Context, app config.Application, spec adapters.WebSpec, link string) (applications.Instance, error)
	OpenTab(ctx context.Context, appID, link string) (string, error)
	CloseTab(ctx context.Context, appID, targetID string) error
	OnLoopback(fn func(appID, targetID, url string))
}

// signIn is a link Bear Den opened on the TV, until its page hands back.
type signIn struct {
	browser  string // the Browser tile's app id
	tab      string // the tab opened for it; "" when the browser was started on the link
	returnTo string // the app in front when the link came ("" = Bear Den's own screens)
	started  bool   // the browser was started for this link
	at       time.Time
}

// SignInFor is how long Bear Den keeps an opened link's app open and waits
// for its page to hand back.
var SignInFor = 15 * time.Minute

// signInSettle is how long the page that handed back stays before its tab
// closes (the app already has its answer: the browser fetched that address).
var signInSettle = 1500 * time.Millisecond

// wireLinks connects the web manager's loopback reports to the coordinator.
func (c *Coordinator) wireLinks() {
	if lw, ok := c.opts.Web.(linkWeb); ok {
		lw.OnLoopback(c.onLoopback)
	}
}

// openLink opens link on the TV when Bear Den or one of its apps is in
// front and the Browser tile can show it. It reports whether it did, and
// why not (never with the link, which may carry a sign-in code).
func (c *Coordinator) openLink(ctx context.Context, link string) (bool, string) {
	lw, ok := c.opts.Web.(linkWeb)
	if !ok || c.opts.Web == nil {
		return false, "links cannot open on the TV in this session"
	}
	if err := web.CheckLink(link); err != nil {
		return false, "not a web link"
	}
	_, target, _ := c.current()
	if target.Kind != "app" && target.Kind != "shell" {
		return false, "Bear Den is not in front"
	}
	browser, ok := c.linkBrowser()
	if !ok {
		return false, "the Browser tile is not installed or is turned off"
	}
	s := &signIn{browser: browser.ID, at: c.clock.Now()}
	if target.Kind == "app" && strOr(target.AppID) != browser.ID {
		s.returnTo = strOr(target.AppID)
	}
	if c.opts.Web.Running(browser.ID) {
		tab, err := lw.OpenTab(ctx, browser.ID, link)
		if err != nil {
			c.log.Warn("session: a link could not open in the Browser", "err", err)
			return false, "the Browser could not open it"
		}
		s.tab = tab
	} else {
		s.started = true
		c.mu.Lock()
		c.launchLink[browser.ID] = link
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.signIn = s
	c.mu.Unlock()
	host := ""
	if u, err := url.Parse(link); err == nil {
		host = u.Host
	}
	c.log.Info("session: opening a link on the TV", "browser", browser.ID, "host", host, "return_to", s.returnTo, "new_browser", s.started)
	// Bring the Browser forward, or start it on the link (doLaunch), as the
	// shell's app.launch would; the app waiting for the sign-in stays open
	// (closeOtherApps). The reply does not wait for the window.
	go func() {
		req := contract.ActionRequest{Protocol: 1, RequestID: "link-" + randomID(), Target: "shell", Action: contract.ActionAppLaunch, Args: map[string]any{"app_id": browser.ID}}
		res := c.submit(context.Background(), sender{key: shellSender}, req)
		if res.Outcome == contract.OutcomeFailed {
			c.log.Warn("session: the Browser did not open for a link", "code", res.Code)
			c.mu.Lock()
			if c.signIn == s {
				c.signIn = nil
			}
			delete(c.launchLink, browser.ID)
			c.mu.Unlock()
		}
	}()
	return true, ""
}

// linkBrowser is the Browser tile (the web app that is not a streaming
// site), when it is on and installed.
func (c *Coordinator) linkBrowser() (config.Application, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.opts.Config.Current().Applications {
		spec, isWeb := c.webSpecForAdapter(a.Adapter)
		if !isWeb || spec.IsStreaming() || !a.IsEnabled() || !c.appLocked(a.ID).install.Installed {
			continue
		}
		return a, true
	}
	return config.Application{}, false
}

// keptForSignInLocked reports whether appID waits for a sign-in Bear Den
// opened (closeOtherApps leaves it open). Caller holds c.mu.
func (c *Coordinator) keptForSignInLocked(appID string) bool {
	s := c.signIn
	return s != nil && s.returnTo == appID && c.clock.Since(s.at) < SignInFor
}

// onLoopback: a tab of browser appID reached a loopback address. When it is
// the sign-in Bear Den opened, the tab closes and the app that asked comes
// back.
func (c *Coordinator) onLoopback(appID, targetID, _ string) {
	c.mu.Lock()
	s := c.signIn
	if s == nil || s.browser != appID || c.clock.Since(s.at) >= SignInFor || (s.tab != "" && s.tab != targetID) {
		c.mu.Unlock()
		return
	}
	c.signIn = nil
	c.mu.Unlock()
	<-c.clock.After(signInSettle)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if lw, ok := c.opts.Web.(linkWeb); ok {
		if err := lw.CloseTab(ctx, appID, targetID); err != nil {
			c.log.Warn("session: the sign-in tab could not be closed", "err", err)
		}
	}
	label := ""
	if s.returnTo != "" {
		if app, ok := c.opts.Config.Current().Application(s.returnTo); ok {
			label = app.Label
		}
	}
	c.log.Info("session: a sign-in on the TV handed back", "browser", appID, "return_to", s.returnTo)
	if label == "" {
		c.Notify("success", "Signed in.")
		return
	}
	c.Notify("success", "Signed in. Back to "+label+".")
	req := contract.ActionRequest{Protocol: 1, RequestID: "link-back-" + randomID(), Target: "shell", Action: contract.ActionAppLaunch, Args: map[string]any{"app_id": s.returnTo}}
	res := c.submit(context.Background(), sender{key: shellSender}, req)
	if res.Outcome == contract.OutcomeFailed {
		c.log.Warn("session: could not return to the app after a sign-in", "app", s.returnTo, "code", res.Code)
	}
}
