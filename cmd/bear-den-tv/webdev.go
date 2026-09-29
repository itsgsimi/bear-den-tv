// `dev --dev-browser PATH`: the real web app manager (internal/applications/web)
// running a Chromium binary directly, with a window on the fake desktop
// standing in for Chromium's so the coordinator sees the app in front.
// Development only; the TV runs Flathub Chromium through `flatpak run`.

package main

import (
	"context"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
)

type devBrowser struct {
	*web.Manager
	desk *fake.Desktop
}

func (d devBrowser) Launch(ctx context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error) {
	inst, err := d.Manager.Launch(ctx, app, spec)
	if err == nil {
		w := d.desk.AddWindow(platform.WindowInfo{PID: inst.PID, Class: []string{"dev.browser", spec.Class}, Title: app.Label})
		d.desk.SetActive(w)
	}
	return inst, err
}
