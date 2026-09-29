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
	choice := c.opts.Config.Current().Layout().UI.IconsOf()
	app := appicons.App{Adapter: ad.Name(), FlatpakID: ad.FlatpakID(), OwnFlatpak: adapters.OwnFlatpakIcon(ad)}
	png, err := c.icons.cache.PNG(app, choice)
	if errors.Is(err, appicons.ErrNoIcon) {
		return nil, remote.ErrNoIcon
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", remote.ErrNoIcon, err)
	}
	return png, nil
}
