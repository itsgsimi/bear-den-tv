// Package fixtures is the --dev-fixtures ContentProvider: deterministic
// DEMO-labeled items (demo: true) with no network access. Artwork is a small
// generated PNG placeholder per item when an artwork directory is given,
// otherwise nil. Nothing here reaches a real Plex server or account.
package fixtures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/providers"
)

// Name is the contract content.provider value for demo rows.
const Name = "fixtures"

// AppID is the application id demo items open (applications[].id).
const AppID = "plex-htpc"

const (
	artworkWidth  = 640
	artworkHeight = 360
)

// Provider serves DEMO rows. Safe for concurrent use.
type Provider struct {
	dir string
	mu  sync.Mutex
}

// New returns a fixtures provider. artworkDir receives generated PNG
// placeholders; "" disables artwork (items carry artwork: null).
func New(artworkDir string) *Provider { return &Provider{dir: artworkDir} }

// Name implements providers.ContentProvider.
func (p *Provider) Name() string { return Name }

// Connect implements providers.ContentProvider; fixtures always connect.
func (p *Provider) Connect(ctx context.Context) error { return nil }

// ListSections implements providers.ContentProvider.
func (p *Provider) ListSections(ctx context.Context) ([]providers.SectionDescriptor, error) {
	return []providers.SectionDescriptor{
		{Kind: providers.KindContinueWatching, Title: "Continue Watching (DEMO)"},
		{Kind: providers.KindRecentlyAdded, Title: "Recently Added (DEMO)"},
		{Kind: providers.KindCollection, Key: "demo-collection", Title: "Demo Collection"},
	}, nil
}

// FetchItems implements providers.ContentProvider. Ids are deterministic
// ("demo:<kind>:<n>"); cursors are not used.
// demoShow is one fictional title. Every item is DEMO-labeled; none of these
// names refer to real media.
type demoShow struct {
	title    string
	detail   string // episode or genre line
	minutes  int    // runtime
	progress float64
}

var (
	continueWatching = []demoShow{
		{"Northern Lights", "S1 · E3 · The Long Night", 52, 0.42},
		{"The Quiet Harbor", "Drama · 2024", 118, 0.18},
		{"Mountain Kitchen", "S2 · E7 · Wild Garlic", 28, 0.76},
		{"Paper Planes", "Documentary · 2023", 94, 0.55},
		{"Cedar Lake", "S3 · E1 · Thaw", 47, 0.08},
		{"Salt & Stone", "S1 · E9 · Low Tide", 41, 0.63},
	}
	recentlyAdded = []demoShow{
		{"River Stories", "Documentary · 2025", 88, 0},
		{"Night Market", "Comedy · 2025", 102, 0},
		{"The Long Walk Home", "Drama · 2024", 126, 0},
		{"Winter Birds", "Nature · 2025", 61, 0},
		{"Copper Coast", "Mystery · 2025", 97, 0},
		{"Lanterns", "Animation · 2024", 84, 0},
		{"Field Notes", "S1 · 8 episodes", 30, 0},
		{"High Meadow", "Family · 2023", 99, 0},
	}
	collection = []demoShow{
		{"Harbor Lights", "Collection · Coastal", 110, 0},
		{"Fjord", "Collection · Coastal", 92, 0},
		{"Tidewater", "Collection · Coastal", 101, 0},
		{"Sea Glass", "Collection · Coastal", 87, 0},
		{"Lighthouse Keeper", "Collection · Coastal", 115, 0},
	}
)

func runtimeText(minutes int) string {
	if minutes >= 60 {
		return fmt.Sprintf("%d h %d min", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%d min", minutes)
}

// FetchItems implements providers.ContentProvider with deterministic DEMO items.
func (p *Provider) FetchItems(ctx context.Context, sectionKind string, cfg providers.SectionConfig, cursor string) ([]contract.ContentItem, string, error) {
	var shows []demoShow
	switch sectionKind {
	case providers.KindContinueWatching:
		shows = continueWatching
	case providers.KindRecentlyAdded:
		shows = recentlyAdded
	case providers.KindCollection:
		shows = collection
	default:
		return nil, "", fmt.Errorf("fixtures: unsupported section kind %q", sectionKind)
	}
	count := len(shows)
	if cfg.Limit > 0 && cfg.Limit < count {
		count = cfg.Limit
	}
	items := make([]contract.ContentItem, 0, count)
	for n, s := range shows[:count] {
		subtitle := s.detail + " · " + runtimeText(s.minutes)
		var progress *float64
		if s.progress > 0 {
			v := s.progress
			progress = &v
			left := int(float64(s.minutes) * (1 - s.progress))
			subtitle = s.detail + " · " + runtimeText(left) + " left"
		}
		items = append(items, contract.ContentItem{
			ID:         fmt.Sprintf("demo:%s:%d", sectionKind, n+1),
			Title:      s.title,
			Subtitle:   subtitle,
			Artwork:    nil,
			Progress:   progress,
			OpenAction: providers.OpenApp,
			Demo:       true,
		})
	}
	return items, "", nil
}

// ResolveArtwork implements providers.ContentProvider: a generated 16:9
// landscape per item (sky, sun, layered ridges, water), cached as PNG.
func (p *Provider) ResolveArtwork(ctx context.Context, item contract.ContentItem) (string, error) {
	if p.dir == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte(item.ID))
	name := hex.EncodeToString(sum[:8]) + "-v2.png"
	path := filepath.Join(p.dir, name)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return "", err
	}
	img := paintLandscape(sum)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// palettes are dusk/dawn/night/day sky pairs plus a ridge tint.
var palettes = [][3]color.RGBA{
	{{0x1d, 0x2b, 0x53, 255}, {0xff, 0x9a, 0x5a, 255}, {0x2a, 0x1e, 0x3a, 255}}, // dusk
	{{0x0b, 0x13, 0x2b, 255}, {0x3a, 0x5a, 0x9a, 255}, {0x0d, 0x1a, 0x2e, 255}}, // night
	{{0x5b, 0x8d, 0xc9, 255}, {0xf6, 0xd8, 0xa8, 255}, {0x2f, 0x4a, 0x3a, 255}}, // day
	{{0x3b, 0x1f, 0x4a, 255}, {0xf2, 0x6d, 0x6d, 255}, {0x26, 0x14, 0x2e, 255}}, // sunset
	{{0x14, 0x3d, 0x3a, 255}, {0xc9, 0xe4, 0xb4, 255}, {0x10, 0x2a, 0x22, 255}}, // forest dawn
	{{0x24, 0x24, 0x3e, 255}, {0xe0, 0xb0, 0x6a, 255}, {0x1a, 0x1a, 0x28, 255}}, // amber
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

func mix(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return color.RGBA{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t), 255}
}

func paintLandscape(seed [32]byte) *image.RGBA {
	w, h := artworkWidth, artworkHeight
	pal := palettes[int(seed[0])%len(palettes)]
	top, horizon, ridge := pal[0], pal[1], pal[2]
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sunX := float64(w) * (0.2 + float64(seed[1]%60)/100)
	sunY := float64(h) * (0.28 + float64(seed[2]%20)/100)
	sunR := float64(h) * (0.07 + float64(seed[3]%6)/100)
	water := seed[4]%2 == 0
	waterLine := int(float64(h) * 0.78)
	// Ridge profiles: a few summed sines per layer, farther layers lighter.
	ridgeY := func(layer int, x float64) float64 {
		base := float64(h) * (0.48 + 0.1*float64(layer))
		a1 := float64(seed[5+layer]%30+15) / 1000 * float64(h) * 3
		f1 := float64(seed[8+layer]%5+2) / float64(w) * 3.1
		f2 := float64(seed[11+layer]%9+5) / float64(w) * 3.1
		return base - a1*math.Sin(x*f1+float64(seed[14+layer])) - a1*0.4*math.Sin(x*f2+float64(seed[17+layer]))
	}
	for y := 0; y < h; y++ {
		sky := mix(top, horizon, math.Pow(float64(y)/float64(h)*1.25, 1.6))
		for x := 0; x < w; x++ {
			c := sky
			dx, dy := float64(x)-sunX, float64(y)-sunY
			if d := math.Hypot(dx, dy); d < sunR {
				c = mix(horizon, color.RGBA{255, 244, 220, 255}, 0.85)
			} else if d < sunR*3 {
				c = mix(c, horizon, 0.35*(1-(d-sunR)/(sunR*2)))
			}
			for layer := 0; layer < 3; layer++ {
				if float64(y) >= ridgeY(layer, float64(x)) {
					depth := float64(layer) / 2
					c = mix(mix(horizon, ridge, 0.55), mix(ridge, color.RGBA{6, 8, 10, 255}, 0.6), depth)
				}
			}
			if water && y >= waterLine {
				// Reflection band with gentle ripples.
				ry := waterLine - (y - waterLine)
				ref := mix(top, horizon, math.Pow(float64(ry)/float64(h)*1.25, 1.6))
				if int(float64(y)+3*math.Sin(float64(x)/9))%7 == 0 {
					ref = mix(ref, color.RGBA{255, 255, 255, 255}, 0.12)
				}
				c = mix(ref, color.RGBA{8, 12, 16, 255}, 0.45+0.4*float64(y-waterLine)/float64(h-waterLine))
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func (p *Provider) ResolveOpenAction(ctx context.Context, item contract.ContentItem) providers.OpenAction {
	return providers.OpenAction{Kind: providers.OpenApp, AppID: AppID, Reason: "demo fixtures never open real media"}
}

// Status implements providers.ContentProvider.
func (p *Provider) Status() (string, string) {
	return providers.StatusReady, "DEMO content (--dev-fixtures)"
}

// IsDemoID reports whether an item id was produced by this provider.
func IsDemoID(id string) bool { return strings.HasPrefix(id, "demo:") }
