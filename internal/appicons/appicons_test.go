// Tests for the phone icon resolver: the order (brand, then the installed
// Flatpak's export only with the "app" choice and only when the Flatpak is
// the app itself), and the safety rules (no SVG, size caps, re-encoding).
package appicons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func pngOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

type tree struct {
	f            Finder
	user, system string
}

func newTree(t *testing.T) tree {
	d := t.TempDir()
	tr := tree{user: filepath.Join(d, "user"), system: filepath.Join(d, "system")}
	tr.f = Finder{BrandDir: filepath.Join(d, "brand"), ExportRoots: []string{tr.user, tr.system}}
	return tr
}

func (tr tree) export(t *testing.T, root, size, id, ext string, data []byte) {
	write(t, filepath.Join(root, "icons", "hicolor", size, "apps", id+ext), data)
}

func colourOf(t *testing.T, b []byte) color.RGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("served bytes are not a PNG: %v", err)
	}
	return color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
}

var (
	red   = color.RGBA{255, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
)

func TestResolutionOrder(t *testing.T) {
	plex := App{Adapter: "plex-htpc", FlatpakID: "tv.plex.PlexHTPC", OwnFlatpak: true}
	netflix := App{Adapter: "netflix", FlatpakID: "org.chromium.Chromium", OwnFlatpak: false}
	browser := App{Adapter: "browser", FlatpakID: "org.chromium.Chromium", OwnFlatpak: true}

	tr := newTree(t)
	// Nothing installed, no brand: Bear Den's (ErrNoIcon), either choice.
	for _, choice := range []string{"app", "bear_den", ""} {
		if _, _, err := tr.f.PNG(plex, choice); !errors.Is(err, ErrNoIcon) {
			t.Fatalf("%q with nothing on disk: %v", choice, err)
		}
	}
	// Installed (system export): "app" takes it, "bear_den" does not.
	tr.export(t, tr.system, "128x128", plex.FlatpakID, ".png", pngOf(t, 4, 4, green))
	b, src, err := tr.f.PNG(plex, "app")
	if err != nil || src != SourceFlatpak || colourOf(t, b) != green {
		t.Fatalf("app choice: src %q err %v", src, err)
	}
	if _, _, err := tr.f.PNG(plex, ""); err != nil {
		t.Fatalf("missing choice means app: %v", err)
	}
	if _, _, err := tr.f.PNG(plex, "bear_den"); !errors.Is(err, ErrNoIcon) {
		t.Fatalf("bear_den took the Flatpak icon: %v", err)
	}
	// A per-user export wins over the system one.
	tr.export(t, tr.user, "256x256", plex.FlatpakID, ".png", pngOf(t, 4, 4, blue))
	if b, _, _ := tr.f.PNG(plex, "app"); colourOf(t, b) != blue {
		t.Fatal("the user's export should come first")
	}
	// The owner's brand icon wins over everything, with either choice.
	write(t, filepath.Join(tr.f.BrandDir, "plex-htpc", "icon.png"), pngOf(t, 4, 4, red))
	for _, choice := range []string{"app", "bear_den"} {
		b, src, err := tr.f.PNG(plex, choice)
		if err != nil || src != SourceBrand || colourOf(t, b) != red {
			t.Fatalf("%s: brand not first (src %q err %v)", choice, src, err)
		}
	}

	// Streaming sites never take Chromium's icon; the Browser tile does.
	tr.export(t, tr.system, "128x128", "org.chromium.Chromium", ".png", pngOf(t, 4, 4, green))
	if _, _, err := tr.f.PNG(netflix, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatalf("netflix took Chromium's icon: %v", err)
	}
	if _, src, err := tr.f.PNG(browser, "app"); err != nil || src != SourceFlatpak {
		t.Fatalf("browser: src %q err %v", src, err)
	}
	// A streaming site's own brand icon still counts.
	write(t, filepath.Join(tr.f.BrandDir, "netflix", "icon.png"), pngOf(t, 4, 4, red))
	if _, src, err := tr.f.PNG(netflix, "app"); err != nil || src != SourceBrand {
		t.Fatalf("netflix brand: src %q err %v", src, err)
	}
}

func TestNeverSVGAndSizeCaps(t *testing.T) {
	app := App{Adapter: "spotify", FlatpakID: "com.spotify.Client", OwnFlatpak: true}
	tr := newTree(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	// SVG-only brand and export: nothing is served.
	write(t, filepath.Join(tr.f.BrandDir, "spotify", "icon.svg"), svg)
	tr.export(t, tr.user, "scalable", app.FlatpakID, ".svg", svg)
	// An SVG under a .png name is refused by the decoder, not served.
	tr.export(t, tr.user, "256x256", app.FlatpakID, ".png", svg)
	if b, _, err := tr.f.PNG(app, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatalf("an SVG was served: %q %v", b, err)
	}
	// Over MaxSide pixels: skipped.
	tr.export(t, tr.user, "512x512", app.FlatpakID, ".png", pngOf(t, MaxSide+1, 1, green))
	if _, _, err := tr.f.PNG(app, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatalf("an oversized image was served: %v", err)
	}
	// Over MaxBytes: skipped before decoding.
	big := append(pngOf(t, 2, 2, green), make([]byte, MaxBytes)...)
	tr.export(t, tr.user, "192x192", app.FlatpakID, ".png", big)
	if _, err := Sanitize(filepath.Join(tr.user, "icons", "hicolor", "192x192", "apps", app.FlatpakID+".png")); err == nil {
		t.Fatal("a file over MaxBytes was read")
	}
	if _, _, err := tr.f.PNG(app, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatalf("an over-size file was served: %v", err)
	}
	// A good PNG further down the size list is still found.
	tr.export(t, tr.user, "64x64", app.FlatpakID, ".png", pngOf(t, 4, 4, blue))
	if b, _, err := tr.f.PNG(app, "app"); err != nil || colourOf(t, b) != blue {
		t.Fatalf("the valid PNG: %v", err)
	}
}

// withText inserts a tEXt chunk (metadata) after the PNG header chunk.
func withText(t *testing.T, p []byte, key, value string) []byte {
	t.Helper()
	data := append([]byte(key+"\x00"), value...)
	chunk := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(chunk, uint32(len(data)))
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, data...)
	crc := crc32.ChecksumIEEE(chunk[4:])
	chunk = binary.BigEndian.AppendUint32(chunk, crc)
	const ihdrEnd = 8 + 25 // signature + IHDR chunk
	out := append([]byte{}, p[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, p[ihdrEnd:]...)
}

func TestReencodesAndStripsMetadata(t *testing.T) {
	dir := t.TempDir()
	tagged := withText(t, pngOf(t, 3, 3, red), "Comment", "DEMO-SECRET")
	if _, err := png.Decode(bytes.NewReader(tagged)); err != nil {
		t.Fatalf("test PNG broken: %v", err)
	}
	p := filepath.Join(dir, "a.png")
	write(t, p, tagged)
	out, err := Sanitize(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("DEMO-SECRET")) || bytes.Contains(out, []byte("tEXt")) {
		t.Fatal("metadata survived")
	}
	if colourOf(t, out) != red {
		t.Fatal("pixels changed")
	}
	// A JPEG brand icon comes out as PNG.
	var j bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if err := jpeg.Encode(&j, img, nil); err != nil {
		t.Fatal(err)
	}
	jp := filepath.Join(dir, "b.jpg")
	write(t, jp, j.Bytes())
	out, err = Sanitize(jp)
	if err != nil || !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Fatalf("jpeg: %v", err)
	}
}

func TestCacheReusesThenRereads(t *testing.T) {
	tr := newTree(t)
	now := time.Unix(1_800_000_000, 0)
	c := &Cache{Finder: tr.f, TTL: time.Minute, Now: func() time.Time { return now }}
	app := App{Adapter: "jellyfin", FlatpakID: "org.jellyfin.JellyfinDesktop", OwnFlatpak: true}
	if _, err := c.PNG(app, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatal(err)
	}
	tr.export(t, tr.user, "128x128", app.FlatpakID, ".png", pngOf(t, 2, 2, green))
	if _, err := c.PNG(app, "app"); !errors.Is(err, ErrNoIcon) {
		t.Fatal("the cached answer should hold within the TTL")
	}
	now = now.Add(time.Minute)
	if _, err := c.PNG(app, "app"); err != nil {
		t.Fatalf("after the TTL the install shows: %v", err)
	}
}
