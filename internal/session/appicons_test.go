// Tests for the coordinator's phone icon source (appicons.go): adapter ids
// from the adapter table only, the layout's ui.app_icons choice, and the
// streaming sites never taking Chromium's icon.
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
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
)

func TestAppIconForPhones(t *testing.T) {
	root := t.TempDir()
	finder := appicons.Finder{BrandDir: filepath.Join(root, "brand"), ExportRoots: []string{filepath.Join(root, "exports")}}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tv.plex.PlexHTPC", "org.chromium.Chromium"} {
		p := filepath.Join(root, "exports", "icons", "hicolor", "128x128", "apps", id+".png")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, func(o *Options) { o.IconFinder = &finder })
	ctx := context.Background()

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
		t.Fatalf("not installed: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "netflix"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("netflix took Chromium's icon: %v", err)
	}
	if _, err := h.c.AppIcon(ctx, "browser"); err != nil {
		t.Fatalf("the Browser tile may use Chromium's icon: %v", err)
	}
	if _, err := h.c.opts.Config.Update(func(c *config.Config) error {
		c.UI.AppIcons = contract.AppIconsBearDen
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.AppIcon(ctx, "plex-htpc"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("Bear Den style still served the Flatpak icon: %v", err)
	}
	// Without a finder (tests, a session without one) nothing is served.
	bare := newHarness(t)
	if _, err := bare.c.AppIcon(ctx, "plex-htpc"); !errors.Is(err, remote.ErrNoIcon) {
		t.Fatalf("no finder: %v", err)
	}
}
