// Tests for theme package loading and validation (themes.go; spec
// contracts/theme.schema.json).

package themes

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/contract"
)

func builtIns(t *testing.T) (fs.FS, fs.FS) {
	t.Helper()
	b, err := fs.Sub(bdtv.Themes, "themes")
	if err != nil {
		t.Fatal(err)
	}
	o, err := fs.Sub(bdtv.Ornaments, "apps/tv-shell/assets/ornaments")
	if err != nil {
		t.Fatal(err)
	}
	return b, o
}

func TestEveryBuiltInThemeIsValid(t *testing.T) {
	b, o := builtIns(t)
	reg, problems := Load(b, nil, o)
	if len(problems) > 0 {
		t.Fatalf("built-in themes must validate: %v", problems)
	}
	var ids []string
	for _, s := range reg.List() {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "den,forest,midnight,campfire,winter" {
		t.Fatalf("order = %v", ids)
	}
	// Old layout values still work.
	if reg.Canonical("den-gradient") != "den" || reg.Canonical("charcoal") != "campfire" || reg.Canonical("nope") != "den" {
		t.Fatal("aliases/fallback broken")
	}
}

func TestAppearanceForPhones(t *testing.T) {
	b, o := builtIns(t)
	reg, _ := Load(b, nil, o)
	a := reg.Appearance(contract.UI{Background: "charcoal", Theme: "den-dark", Accent: "#E8A45C"})
	if a.Background != "campfire" || a.Name != "Campfire" || a.Focus.Style != "embers" || !a.Focus.TipUpright {
		t.Fatalf("appearance = %+v %+v", a, a.Focus)
	}
	if a.Focus.Tip != "/themes/_ornaments/flame.png" || a.Phone.Backdrop != "/themes/campfire/backdrop.png" || a.Phone.Particles != "embers" || !a.Pixel {
		t.Fatalf("urls = %+v %+v", a.Focus, a.Phone)
	}
	w := reg.Appearance(contract.UI{Background: "winter", Theme: "den-dark", Accent: "#B8D8F0"})
	if w.Focus.Tip != "/themes/winter/snowflake.png" || w.Phone.Backdrop != "/themes/winter/backdrop.png" || w.Phone.Veil != "#0B1220" {
		t.Fatalf("winter uses its own ornament and backdrop: %+v %+v", w.Focus, w.Phone)
	}
	p := reg.Appearance(contract.UI{Background: "forest", Theme: "plain-dark", Accent: "#8DBF7F"})
	if p.Focus.Style != "" || p.Phone.Particles != "none" || p.Palette["stem"] == "" {
		t.Fatalf("plain keeps colours, drops decorations: %+v %+v", p.Focus, p.Phone)
	}
	if len(a.Themes) != 5 {
		t.Fatalf("themes list = %v", a.Themes)
	}
}

func TestAssetsExposeOnlyThemeImages(t *testing.T) {
	b, o := builtIns(t)
	reg, _ := Load(b, nil, o)
	assets := reg.Assets()
	for _, ok := range []string{"campfire/backdrop.png", "campfire/rain.png", "winter/snowflake.png", "_ornaments/daisy.png"} {
		if _, err := fs.Stat(assets, ok); err != nil {
			t.Fatalf("%s should be served: %v", ok, err)
		}
	}
	for _, bad := range []string{"den/theme.json", "den", "../embed.go", "_ornaments/../../../go.mod", "nope/x.png", "den/sub/x.png"} {
		if _, err := assets.Open(bad); err == nil {
			t.Fatalf("%s must not be served", bad)
		}
	}
}

func TestInvalidPackagesAreSkippedWithReasons(t *testing.T) {
	_, o := builtIns(t)
	user := fstest.MapFS{
		"ok/theme.json":        {Data: []byte(`{"schema":1,"id":"ok","name":"OK","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"focus":{"style":"vine","tip":"heart"}}`)},
		"wrongid/theme.json":   {Data: []byte(`{"schema":1,"id":"other","name":"X","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"}}`)},
		"missing/theme.json":   {Data: []byte(`{"schema":1,"id":"missing","name":"X","accent":"#123456","wallpaper":{"image":"nope.jpg","top":"#000000","bottom":"#111111"}}`)},
		"orn/theme.json":       {Data: []byte(`{"schema":1,"id":"orn","name":"X","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"focus":{"style":"vine","tip":"unicorn"}}`)},
		"typo/theme.json":      {Data: []byte(`{"schema":1,"id":"typo","name":"X","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"focsu":{}}`)},
		"notatheme/readme.txt": {Data: []byte("hi")},
	}
	reg, problems := Load(nil, user, o)
	if reg.Get("ok") == nil || reg.Get("ok").ID != "ok" {
		t.Fatal("valid owner theme should load")
	}
	got := map[string]string{}
	for _, p := range problems {
		got[p.Dir] = strings.Join(p.Errors, " | ")
	}
	for dir, want := range map[string]string{"wrongid": "folder name", "missing": "missing", "orn": "unicorn", "typo": "focsu"} {
		if !strings.Contains(got[dir], want) {
			t.Fatalf("%s: problems = %q, want mention of %q (all: %v)", dir, got[dir], want, got)
		}
	}
	if _, ok := got["notatheme"]; ok {
		t.Fatal("folders without theme.json are ignored, not reported")
	}
}

func TestUnknownIDReloadsOwnerThemes(t *testing.T) {
	b, o := builtIns(t)
	user := fstest.MapFS{}
	reg, _ := Load(b, user, o)
	reg.reload = func() (*Registry, []Problem) {
		user["autumn/theme.json"] = &fstest.MapFile{Data: []byte(`{"schema":1,"id":"autumn","name":"Autumn","accent":"#D98C3F","wallpaper":{"top":"#3A2416","bottom":"#120B07"}}`)}
		return Load(b, user, o)
	}
	old := ReloadEvery
	ReloadEvery = 0
	t.Cleanup(func() { ReloadEvery = old })
	a := reg.Appearance(contract.UI{Background: "autumn", Theme: "den-dark", Accent: "#D98C3F"})
	if a.Background != "autumn" || a.Name != "Autumn" {
		t.Fatalf("a theme added after startup should load on first use: %+v", a)
	}
}

func TestPerformanceStyleIsPlainForPhones(t *testing.T) {
	b, o := builtIns(t)
	reg, _ := Load(b, nil, o)
	p := reg.Appearance(contract.UI{Background: "forest", Theme: "performance", Accent: "#8DBF7F"})
	if p.Focus == nil || p.Focus.Style != "" || p.Phone == nil || p.Phone.Particles != "none" {
		t.Fatalf("performance must drop decorations like Plain: %+v %+v", p.Focus, p.Phone)
	}
}

// pixelTheme is a pixel-art owner theme with one sprite.
func pixelTheme(sheet string) fstest.MapFS {
	return fstest.MapFS{
		"pix/theme.json": {Data: []byte(`{"schema":1,"id":"pix","name":"Pix","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111","sprites":[{"sheet":"` + sheet + `","frames":4,"fps":8,"x":10,"y":200}]}}`)},
		"pix/bg.png":     {Data: []byte("png")},
		"pix/fire.png":   {Data: []byte("png")},
	}
}

func TestPixelWallpaperWithSprites(t *testing.T) {
	_, o := builtIns(t)
	reg, problems := Load(nil, pixelTheme("fire.png"), o)
	if len(problems) > 0 {
		t.Fatalf("pixel theme should load: %v", problems)
	}
	th := reg.Get("pix")
	want := Sprite{Sheet: "fire.png", Frames: 4, FPS: 8, X: 10, Y: 200}
	if !th.Wallpaper.Pixel || len(th.Wallpaper.Sprites) != 1 || th.Wallpaper.Sprites[0] != want {
		t.Fatalf("wallpaper = %+v", th.Wallpaper)
	}
	if a := reg.Appearance(contract.UI{Background: "pix", Theme: "den-dark", Accent: "#123456"}); !a.Pixel {
		t.Fatal("appearance.pixel should follow wallpaper.pixel")
	}
	if _, err := fs.Stat(reg.Assets(), "pix/fire.png"); err != nil {
		t.Fatalf("sprite sheets are served like other theme images: %v", err)
	}
	plain, _ := Load(nil, fstest.MapFS{"autumn/theme.json": {Data: []byte(`{"schema":1,"id":"autumn","name":"Autumn","accent":"#D98C3F","wallpaper":{"top":"#3A2416","bottom":"#120B07"}}`)}}, o)
	if plain.Appearance(contract.UI{Background: "autumn", Theme: "den-dark", Accent: "#123456"}).Pixel {
		t.Fatal("a theme without wallpaper.pixel is not pixel art")
	}
	// Every built-in world is pixel art.
	b, _ := builtIns(t)
	builtin, _ := Load(b, nil, o)
	for _, id := range []string{"den", "forest", "midnight", "campfire", "winter"} {
		if !builtin.Appearance(contract.UI{Background: id, Theme: "den-dark", Accent: "#123456"}).Pixel {
			t.Fatalf("built-in %s should be pixel art", id)
		}
	}
}

func TestMissingSpriteSheetIsAProblem(t *testing.T) {
	_, o := builtIns(t)
	_, problems := Load(nil, pixelTheme("nope.png"), o)
	if len(problems) != 1 || !strings.Contains(problems[0].String(), "nope.png") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestBuiltInOrnamentMayBePNG(t *testing.T) {
	orn := fstest.MapFS{"acorn.png": {Data: []byte("png")}, "leaf.svg": {Data: []byte("<svg/>")}}
	user := fstest.MapFS{
		"pngo/theme.json": {Data: []byte(`{"schema":1,"id":"pngo","name":"P","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"focus":{"style":"vine","tip":"acorn","extras":["leaf"]}}`)},
	}
	reg, problems := Load(nil, user, orn)
	if len(problems) > 0 {
		t.Fatalf("a .png built-in ornament should resolve: %v", problems)
	}
	a := reg.Appearance(contract.UI{Background: "pngo", Theme: "den-dark", Accent: "#123456"})
	if a.Focus.Tip != "/themes/_ornaments/acorn.png" {
		t.Fatalf("tip = %q", a.Focus.Tip)
	}
	for _, ok := range []string{"_ornaments/acorn.png", "_ornaments/leaf.svg"} {
		if _, err := fs.Stat(reg.Assets(), ok); err != nil {
			t.Fatalf("%s should be served: %v", ok, err)
		}
	}
}

// Art style (layout ui.art_style): Classic takes the theme's classic backdrop
// (or its classic wallpaper), drawn smooth, and the SVG ornament first; Pixel
// keeps the PNG. A theme without classic art keeps its own backdrop.
func TestClassicArtStyleForPhones(t *testing.T) {
	orn := fstest.MapFS{"acorn.png": {Data: []byte("png")}, "acorn.svg": {Data: []byte("<svg/>")}}
	user := fstest.MapFS{
		"both/theme.json": {Data: []byte(`{"schema":1,"id":"both","name":"B","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111"},"phone":{"backdrop":"phone.png"},"focus":{"style":"vine","tip":"acorn"},"classic":{"wallpaper":{"image":"bg.jpg"},"phone":{"backdrop":"phone.jpg"}}}`)},
		"both/bg.png":     {Data: []byte("png")},
		"both/phone.png":  {Data: []byte("png")},
		"both/bg.jpg":     {Data: []byte("jpg")},
		"both/phone.jpg":  {Data: []byte("jpg")},
		"wall/theme.json": {Data: []byte(`{"schema":1,"id":"wall","name":"W","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111"},"classic":{"wallpaper":{"image":"bg.svg"}}}`)},
		"wall/bg.png":     {Data: []byte("png")},
		"wall/bg.svg":     {Data: []byte("<svg/>")},
		"one/theme.json":  {Data: []byte(`{"schema":1,"id":"one","name":"O","accent":"#123456","wallpaper":{"image":"bg.png","pixel":true,"top":"#000000","bottom":"#111111"}}`)},
		"one/bg.png":      {Data: []byte("png")},
		"bad/theme.json":  {Data: []byte(`{"schema":1,"id":"bad","name":"X","accent":"#123456","wallpaper":{"top":"#000000","bottom":"#111111"},"classic":{"wallpaper":{"image":"gone.jpg"}}}`)},
	}
	reg, problems := Load(nil, user, orn)
	if len(problems) != 1 || !strings.Contains(problems[0].String(), "gone.jpg") {
		t.Fatalf("only the missing classic wallpaper should be a problem: %v", problems)
	}
	ui := func(id, art string) contract.UI {
		return contract.UI{Background: id, Theme: "den-dark", Accent: "#123456", ArtStyle: art}
	}
	cases := []struct {
		id, art, backdrop, tip, style string
		pixel                         bool
	}{
		{"both", "", "/themes/both/phone.png", "/themes/_ornaments/acorn.png", "pixel", true},
		{"both", "pixel", "/themes/both/phone.png", "/themes/_ornaments/acorn.png", "pixel", true},
		{"both", "classic", "/themes/both/phone.jpg", "/themes/_ornaments/acorn.svg", "classic", false},
		{"wall", "classic", "/themes/wall/bg.svg", "", "classic", false},
		{"one", "classic", "/themes/one/bg.png", "", "classic", true},
	}
	for _, c := range cases {
		a := reg.Appearance(ui(c.id, c.art))
		if a.Phone.Backdrop != c.backdrop || a.Pixel != c.pixel || a.ArtStyle != c.style || (c.tip != "" && a.Focus.Tip != c.tip) {
			t.Errorf("%s/%q: backdrop %q pixel %v style %q tip %q", c.id, c.art, a.Phone.Backdrop, a.Pixel, a.ArtStyle, a.Focus.Tip)
		}
	}
	if _, err := fs.Stat(reg.Assets(), "both/bg.jpg"); err != nil {
		t.Fatalf("classic art should be served: %v", err)
	}
}
