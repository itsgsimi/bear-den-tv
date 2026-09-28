// Tests for the DEMO fixtures provider (fixtures.go).

package fixtures

import (
	"context"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"bear-den-tv/internal/providers"
)

func TestFixturesDeterministicDemoItems(t *testing.T) {
	p := New("")
	ctx := context.Background()
	if err := p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{providers.KindContinueWatching, providers.KindRecentlyAdded, providers.KindCollection} {
		a, next, err := p.FetchItems(ctx, kind, providers.SectionConfig{Kind: kind}, "")
		if err != nil || next != "" || len(a) == 0 {
			t.Fatalf("%s: %v %q %d", kind, err, next, len(a))
		}
		b, _, _ := p.FetchItems(ctx, kind, providers.SectionConfig{Kind: kind}, "")
		for i := range a {
			if a[i].ID != b[i].ID || a[i].Title != b[i].Title {
				t.Fatalf("%s item %d not deterministic", kind, i)
			}
			if !a[i].Demo || !IsDemoID(a[i].ID) || a[i].Artwork != nil || a[i].OpenAction != providers.OpenApp {
				t.Fatalf("%s item %+v", kind, a[i])
			}
			if a[i].Progress != nil && (*a[i].Progress < 0 || *a[i].Progress > 1) {
				t.Fatalf("progress out of range: %v", *a[i].Progress)
			}
		}
	}
	items, _, _ := p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{Limit: 2}, "")
	if len(items) != 2 || items[0].Progress == nil {
		t.Fatalf("limit/progress: %+v", items)
	}
	if _, _, err := p.FetchItems(ctx, "applications", providers.SectionConfig{}, ""); err == nil {
		t.Fatal("unsupported kind must error")
	}
	if path, err := p.ResolveArtwork(ctx, items[0]); err != nil || path != "" {
		t.Fatalf("no artwork dir: %q %v", path, err)
	}
	if a := p.ResolveOpenAction(ctx, items[0]); a.Kind != providers.OpenApp || a.AppID != AppID {
		t.Fatalf("open action: %+v", a)
	}
	if s, m := p.Status(); s != providers.StatusReady || m == "" {
		t.Fatalf("status: %s %q", s, m)
	}
	secs, _ := p.ListSections(ctx)
	if len(secs) != 3 {
		t.Fatalf("sections: %+v", secs)
	}
}

func TestFixturesArtworkPlaceholders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "art")
	p := New(dir)
	ctx := context.Background()
	items, _, _ := p.FetchItems(ctx, providers.KindRecentlyAdded, providers.SectionConfig{}, "")
	path, err := p.ResolveArtwork(ctx, items[0])
	if err != nil || filepath.Dir(path) != dir {
		t.Fatalf("artwork: %q %v", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || format != "png" || cfg.Width != artworkWidth || cfg.Height != artworkHeight {
		t.Fatalf("placeholder: %v %s %+v", err, format, cfg)
	}
	again, _ := p.ResolveArtwork(ctx, items[0])
	if again != path {
		t.Fatal("artwork path must be stable")
	}
	other, _ := p.ResolveArtwork(ctx, items[1])
	if other == path {
		t.Fatal("distinct items get distinct placeholders")
	}
}
