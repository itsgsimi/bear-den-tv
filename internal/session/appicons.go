// Phone tile icons (contracts/http.md#app-icons): the coordinator side of
// GET /api/v1/apps/{adapter}/icon. The adapter must be in the adapter table;
// the choice comes from layout ui.app_icons; the files are found and
// sanitised by internal/appicons, cached for a minute.

package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"bear-den-tv/internal/appicons"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
)

// appIconTTL is how long an answer is reused; an install or a brand file
// shows on phones within it.
const appIconTTL = time.Minute

type iconCache struct {
	once  sync.Once
	cache *appicons.Cache
}

// AppIcon implements remote.AppIconSource.
func (c *Coordinator) AppIcon(_ context.Context, adapter string) ([]byte, error) {
	ad, ok := c.opts.Adapters.ForName(adapter)
	if !ok {
		return nil, remote.ErrUnknownApp
	}
	if c.opts.IconFinder == nil {
		return nil, remote.ErrNoIcon
	}
	c.icons.once.Do(func() {
		c.icons.cache = &appicons.Cache{Finder: *c.opts.IconFinder, TTL: appIconTTL, Now: c.clock.Now}
	})
	cfg := c.opts.Config.Current()
	choice := cfg.Layout().UI.IconsOf()
	if !c.adapterInstalled(cfg.Applications, adapter) {
		// Not installed (as the tiles say): Bear Den's icon, whatever
		// export may linger; the owner's brand icon still wins.
		choice = contract.AppIconsBearDen
	}
	// The Flatpak the app runs from: its row's (the Browser tile's browser
	// is the owner's choice), when the adapter table allows it.
	fid := ad.FlatpakID()
	for _, a := range cfg.Applications {
		if a.Adapter == adapter && adapters.RunsIn(ad, a.Launch.AppID) {
			fid = a.Launch.AppID
			break
		}
	}
	app := appicons.App{Adapter: ad.Name(), FlatpakID: fid, OwnFlatpak: adapters.OwnFlatpakIcon(ad)}
	png, err := c.icons.cache.PNG(app, choice)
	if errors.Is(err, appicons.ErrNoIcon) {
		return nil, remote.ErrNoIcon
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", remote.ErrNoIcon, err)
	}
	return png, nil
}

// adapterInstalled reports whether discovery has seen an application of
// this adapter installed (state.applications[].installed).
func (c *Coordinator) adapterInstalled(apps []configApp, adapter string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range apps {
		if a.Adapter == adapter && c.appLocked(a.ID).install.Installed {
			return true
		}
	}
	return false
}
