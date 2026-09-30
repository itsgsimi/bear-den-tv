// The Browser tile's start page, the coordinator's side
// (internal/applications/web/start.go): the cards it shows are the
// streaming sites that are on and installed, in config order, with the
// owner's labels; a card pressed on it opens that site as its own app, as
// the shell's app.launch would (every check of route applies: lock, on,
// installed), and only from the Browser tile's own start page. Spec:
// docs/operations.md "Streaming sites and the Browser".

package session

import (
	"context"

	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/contract"
)

// startPageWeb is what a web manager offers for the start page
// (web.Manager does; the fakes need not).
type startPageWeb interface {
	SetStartCards(fn func() []web.StartCard)
	OnOpen(fn func(from, appID string))
}

// wireStartPage connects the web manager's start page to the coordinator.
func (c *Coordinator) wireStartPage() {
	sp, ok := c.opts.Web.(startPageWeb)
	if !ok {
		return
	}
	sp.SetStartCards(c.startCards)
	sp.OnOpen(c.openFromStartPage)
}

// startCards are the streaming sites the start page offers: on and
// installed.
func (c *Coordinator) startCards() []web.StartCard {
	var out []web.StartCard
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.opts.Config.Current().Applications {
		if c.isStreaming(a) && a.IsEnabled() && c.appLocked(a.ID).install.Installed {
			out = append(out, web.StartCard{AppID: a.ID, Label: a.Label})
		}
	}
	return out
}

// openFromStartPage opens appID, a streaming site, because a card on the
// start page of from's browser (the Browser tile) was pressed.
func (c *Coordinator) openFromStartPage(from, appID string) {
	fromApp, ok := c.opts.Config.Current().Application(from)
	if !ok {
		return
	}
	if spec, web := c.webSpecForAdapter(fromApp.Adapter); !web || spec.IsStreaming() {
		return // only the Browser tile shows the start page
	}
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok || !c.isStreaming(app) {
		c.log.Warn("session: the start page asked for an app that is not a streaming site", "app", appID)
		return
	}
	req := contract.ActionRequest{Protocol: 1, RequestID: "start-" + randomID(), Target: "shell", Action: contract.ActionAppLaunch, Args: map[string]any{"app_id": appID}}
	res := c.submit(context.Background(), sender{key: shellSender}, req)
	c.log.Info("session: opened from the Browser's start page", "app", appID, "outcome", res.Outcome, "code", res.Code)
}
