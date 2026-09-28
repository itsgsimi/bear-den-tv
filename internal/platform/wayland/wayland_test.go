// Tests that the Wayland adapter reports every capability unavailable
// (wayland.go).

package wayland

import (
	"context"
	"errors"
	"testing"

	"bear-den-tv/internal/platform"
)

func TestEverythingUnavailable(t *testing.T) {
	var a platform.DesktopAdapter = New()
	if a.Name() != "wayland-limited" || a.DisplaySession() != "wayland" {
		t.Fatal(a.Name(), a.DisplaySession())
	}
	caps := a.Capabilities()
	for _, k := range []string{platform.CapObserveForeground, platform.CapActivate, platform.CapInput, platform.CapLockObservation, platform.CapPhysicalHome} {
		c, ok := caps[k]
		if !ok || c.Available || c.Reason == "" || c.Backend != Name {
			t.Fatalf("%s: %+v", k, c)
		}
	}
	ctx := context.Background()
	if err := a.DeliverKey(ctx, 1, platform.KeyUp); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if fg, err := a.ObserveForeground(ctx); fg.Known || !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(fg, err)
	}
	if _, err := a.WatchForeground(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.Activate(ctx, 1); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.RequestClose(ctx, 1); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := a.ListWindows(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}
