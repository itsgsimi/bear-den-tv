// Package appicons finds the icon a phone tile shows when it is not Bear
// Den's own drawing, and makes it safe to serve (contracts/http.md#app-icons;
// docs/THEMES.md → App icons). The TV shell resolves its own icons the same
// way in ShellController::appArt.
//
// Order: the owner's brand icon (brand/<adapter>/icon.png|.jpg|.jpeg), then,
// when the choice is "app" and the app's Flatpak is the app itself, the PNG
// its installed Flatpak exports (user exports first, then system). Nothing
// else: Bear Den's own icons are bundled in the phone, which draws them when
// this package has none (ErrNoIcon). An export exists only while its Flatpak
// is installed, so an app that is not installed never takes this path.
//
// Safety: SVG is never read or served (there is no rasteriser here); a file
// over MaxBytes or MaxSide pixels is skipped; PNG and JPEG are decoded and
// re-encoded as PNG, which validates them and drops any metadata.
package appicons

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bear-den-tv/internal/contract"
)

const (
	// MaxBytes is the largest icon file read (1 MiB).
	MaxBytes = 1 << 20
	// MaxSide is the largest width or height decoded.
	MaxSide = 1024
)

// ErrNoIcon means the phone should draw Bear Den's own icon.
var ErrNoIcon = errors.New("no icon of the app's own; use Bear Den's")

// exportSizes are the hicolor sizes tried, best phone size first. Only PNG:
// Flatpak exports a PNG at 128 or 256 for most apps.
var exportSizes = []string{"256x256", "128x128", "512x512", "192x192", "96x96", "64x64", "48x48"}

// brandNames are the brand-folder files a phone may be served (no SVG, no
// WebP: the standard library decodes neither).
var brandNames = []string{"icon.png", "icon.jpg", "icon.jpeg"}

// App is what the resolver needs to know about one adapter.
type App struct {
	// Adapter is the adapter name (config adapter; the brand folder name).
	Adapter string
	// FlatpakID is the Flatpak the adapter launches.
	FlatpakID string
	// OwnFlatpak is true when that Flatpak is the app itself, so its icon is
	// the app's own. False for the streaming sites, which run in Google Chrome.
	OwnFlatpak bool
}

// Finder knows where icons live.
type Finder struct {
	// BrandDir is $XDG_DATA_HOME/bear-den-tv/brand.
	BrandDir string
	// ExportRoots are Flatpak "exports/share" directories, user first.
	ExportRoots []string
}

// DefaultFinder uses the owner's data directory and the standard Flatpak
// export locations.
func DefaultFinder() Finder {
	data := os.Getenv("XDG_DATA_HOME")
	home, _ := os.UserHomeDir()
	if data == "" && home != "" {
		data = filepath.Join(home, ".local", "share")
	}
	f := Finder{ExportRoots: []string{"/var/lib/flatpak/exports/share"}}
	if data != "" {
		f.BrandDir = filepath.Join(data, "bear-den-tv", "brand")
	}
	if home != "" {
		f.ExportRoots = append([]string{filepath.Join(home, ".local", "share", "flatpak", "exports", "share")}, f.ExportRoots...)
	}
	return f
}

// Source names where an icon came from.
type Source string

// Sources, in resolution order.
const (
	SourceBrand   Source = "brand"
	SourceFlatpak Source = "flatpak"
)

// Candidate is one file to try.
type Candidate struct {
	Source Source
	Path   string
}

// Candidates lists the files to try for app under the choice ("app" or
// "bear_den"), in order. Files that do not exist are left out.
func (f Finder) Candidates(app App, choice string) []Candidate {
	var out []Candidate
	if f.BrandDir != "" && app.Adapter != "" && filepath.Base(app.Adapter) == app.Adapter {
		for _, n := range brandNames {
			p := filepath.Join(f.BrandDir, app.Adapter, n)
			if regular(p) {
				out = append(out, Candidate{SourceBrand, p})
			}
		}
	}
	if choice == contract.AppIconsBearDen || !app.OwnFlatpak || app.FlatpakID == "" || filepath.Base(app.FlatpakID) != app.FlatpakID {
		return out
	}
	for _, root := range f.ExportRoots {
		for _, size := range exportSizes {
			p := filepath.Join(root, "icons", "hicolor", size, "apps", app.FlatpakID+".png")
			if regular(p) {
				out = append(out, Candidate{SourceFlatpak, p})
			}
		}
	}
	return out
}

// regular reports whether p is (or links to) a regular file.
func regular(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// PNG returns the first candidate that sanitises, re-encoded as PNG, and its
// source; ErrNoIcon when none does.
func (f Finder) PNG(app App, choice string) ([]byte, Source, error) {
	for _, c := range f.Candidates(app, choice) {
		if b, err := Sanitize(c.Path); err == nil {
			return b, c.Source, nil
		}
	}
	return nil, "", ErrNoIcon
}

// Sanitize reads one PNG or JPEG of at most MaxBytes and MaxSide pixels and
// returns it re-encoded as PNG. Anything else (SVG included) is an error.
func Sanitize(path string) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > MaxBytes {
		return nil, fmt.Errorf("icon %s: not a regular file of at most %d bytes", filepath.Base(path), MaxBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(fh, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("icon %s: larger than %d bytes", filepath.Base(path), MaxBytes)
	}
	var decodeConfig func(io.Reader) (image.Config, error)
	var decode func(io.Reader) (image.Image, error)
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		decodeConfig, decode = png.DecodeConfig, png.Decode
	case bytes.HasPrefix(raw, []byte("\xff\xd8\xff")):
		decodeConfig, decode = jpeg.DecodeConfig, jpeg.Decode
	default:
		return nil, fmt.Errorf("icon %s: not a PNG or JPEG", filepath.Base(path))
	}
	cfg, err := decodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxSide || cfg.Height > MaxSide {
		return nil, fmt.Errorf("icon %s: %dx%d is outside 1..%d pixels", filepath.Base(path), cfg.Width, cfg.Height, MaxSide)
	}
	img, err := decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Cache remembers answers for TTL, so a phone opening its tiles does not
// decode every icon each time. Safe for concurrent use.
type Cache struct {
	Finder Finder
	TTL    time.Duration
	Now    func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	png []byte
	err error
	at  time.Time
}

// PNG is Finder.PNG through the cache.
func (c *Cache) PNG(app App, choice string) ([]byte, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	key := fmt.Sprintf("%s|%s|%s|%v", app.Adapter, app.FlatpakID, choice, app.OwnFlatpak)
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now().Sub(e.at) < c.TTL {
		c.mu.Unlock()
		return e.png, e.err
	}
	c.mu.Unlock()
	b, _, err := c.Finder.PNG(app, choice)
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	c.entries[key] = cacheEntry{png: b, err: err, at: now()}
	c.mu.Unlock()
	return b, err
}
