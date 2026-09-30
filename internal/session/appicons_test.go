// Tests for the coordinator's phone icon source (appicons.go): adapter ids
// from the adapter table only, the layout's ui.app_icons choice, installed
// apps only (as the tiles say), and the streaming sites never taking
// its browser's icon.
package session

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"bear-den-tv/internal/appicons"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
)

func TestAppIconForPhones(t *testing.T) {
	root := t.TempDir()
	finder := appicons.Finder{BrandDir: filepath.Join(root, "brand"), ExportRoots: []string{filepath.Join(root, "exports")}}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	put := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Exports for Plex (installed), and for Moonlight, Brave and Chrome,
	// which the harness starts missing (a lingering export must not show).
	for _, id := range []string{adapters.PlexHTPCFlatpakID, adapters.MoonlightFlatpakID, adapters.BraveFlatpakID, adapters.ChromeFlatpakID} {
		put(filepath.Join(root, "exports", "icons", "hicolor", "128x128", "apps", id+".png"))
	}
	h, fi, sl := installHarness(t, func(o *Options) { o.IconFinder = &finder })
	ctx := context.Background()
	h.eventually("discovery", func() bool { return h.c.adapterInstalled(h.c.opts.Config.Current().Applications, "plex-htpc") })

	if _, err := h.c.AppIcon(ctx, "not-an-app"); !errors.Is(err, remote.ErrUnknownApp) {
		t.Fatalf("unknown adapter: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "youtube"); !errors.Is(err, remote.ErrUnknownApp) {
		t.Fatalf("an application id is not an adapter name: %v", err)
	}
	if b, err := h.c.AppIcon(ctx, "plex-htpc"); err != nil || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Fatalf("plex (installed, default choice): %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "vacuumtube"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("installed without an export: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "moonlight"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("not installed, lingering export: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "browser"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("Brave not installed: %v", err)
	}
	// Installed now: the icons follow once discovery sees it.
	for _, a := range []struct{ app, flatpak string }{{"moonlight", adapters.MoonlightFlatpakID}, {"browser", adapters.BraveFlatpakID}, {"netflix", adapters.ChromeFlatpakID}} {
		if r := h.shellSend(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: "i-" + a.app, AppID: a.app}, "i-"+a.app); !r.OK {
			t.Fatalf("app.install %s refused: %+v", a.app, r)
		}
		sl.setScope(a.flatpak, "user")
		fi.set(a.flatpak, install.Status{State: contract.InstallDone, Progress: 100})
	}
	h.eventually("rediscovery", func() bool {
		apps := h.c.opts.Config.Current().Applications
		return h.c.adapterInstalled(apps, "moonlight") && h.c.adapterInstalled(apps, "browser") && h.c.adapterInstalled(apps, "netflix")
	})
	if _, err := h.c.AppIcon(ctx, "moonlight"); err != nil {
		t.Fatalf("moonlight installed: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "browser"); err != nil {
		t.Fatalf("the Browser tile may use its browser's icon: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "netflix"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("netflix took Chrome's icon: %v", err)
	}
	// The owner's brand icon wins even for an app that is not installed.
	put(filepath.Join(root, "brand", "retroarch", "icon.png"))
	if _, err := h.c.AppIcon(ctx, "retroarch"); err != nil {
		t.Fatalf("brand icon: %v", err)
	}
	// Bear Den style: no Flatpak icon, the brand one stays.
	if _, err := h.c.opts.Config.Update(func(c *config.Config) error {
		c.UI.AppIcons = contract.AppIconsBearDen
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.AppIcon(ctx, "plex-htpc"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("Bear Den style still served the Flatpak icon: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "retroarch"); err != nil {
		t.Fatalf("brand icon under Bear Den style: %v", err)
	}
	// Without a finder (tests, a session without one) nothing is served.
	bare := newHarness(t)
	if _, err := bare.c.AppIcon(ctx, "plex-htpc"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("no finder: %v", err)
	}
}
