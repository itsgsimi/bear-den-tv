// Package themes loads theme packages for the coordinator: a folder <id>/
// with theme.json (contracts/theme.schema.json) and its art. It is the Go twin
// of the TV shell's ThemeRegistry (apps/tv-shell/src/ThemeRegistry.h): the
// shell draws themes; this package validates them (`bear-den-tv themes
// validate`), tells phones how to look (state.appearance), and serves theme
// art to phones under /themes/.
//
// Sources, in order (a later source overrides an earlier one with the same id):
//  1. built-in packages embedded from the repository's themes/ folder;
//  2. the owner's folder, $BDTV_THEMES_DIR or $XDG_DATA_HOME/bear-den-tv/themes.
//
// A package that fails validation is skipped and reported, never half-loaded.
// Guide: docs/THEMES.md.
package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/contract"
)

// Fallback is the theme used for unknown ids.
const Fallback = "den"

// builtInOrder is the display order of the built-in themes; others follow by id.
var builtInOrder = []string{"den", "forest", "midnight", "campfire", "winter"}

var (
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	ornamentPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	assetPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}\.(svg|png|jpg|jpeg|webp)$`)
)

// Manifest is theme.json (the fields the coordinator uses; the schema is the
// full definition).
type Manifest struct {
	Schema      int      `json:"schema"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Accent      string   `json:"accent"`
	Wallpaper   struct {
		Image   string   `json:"image"`
		Top     string   `json:"top"`
		Bottom  string   `json:"bottom"`
		Pixel   bool     `json:"pixel"`
		Sprites []Sprite `json:"sprites"`
	} `json:"wallpaper"`
	Palette map[string]string `json:"palette"`
	Focus   struct {
		Style        string   `json:"style"`
		Tip          string   `json:"tip"`
		TipUpright   bool     `json:"tip_upright"`
		Extras       []string `json:"extras"`
		ExtrasBottom []string `json:"extras_bottom"`
	} `json:"focus"`
	Panel *struct {
		Tip          string   `json:"tip"`
		Extras       []string `json:"extras"`
		ExtrasBottom []string `json:"extras_bottom"`
	} `json:"panel"`
	Ambient struct {
		Kind string `json:"kind"`
	} `json:"ambient"`
	Bears struct {
		Hat   string `json:"hat"`
		Carry string `json:"carry"`
		Chase string `json:"chase"`
	} `json:"bears"`
	Heading string `json:"heading"`
	Phone   struct {
		Backdrop  string `json:"backdrop"`
		Veil      string `json:"veil"`
		Particles string `json:"particles"`
	} `json:"phone"`
	Classic *struct {
		Wallpaper *struct {
			Image   string   `json:"image"`
			Sprites []Sprite `json:"sprites"`
		} `json:"wallpaper"`
		Phone *struct {
			Backdrop string `json:"backdrop"`
		} `json:"phone"`
	} `json:"classic"`
}

// Sprite is an animated layer over a wallpaper (pixel art or classic): a sheet of frames
// side by side in one row, played at FPS, placed at X,Y in the image's pixels.
type Sprite struct {
	Sheet  string `json:"sheet"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

// Theme is a loaded, valid package.
type Theme struct {
	Manifest
	BuiltIn bool
	files   fs.FS // the package folder
}

// Problem is a package that was skipped.
type Problem struct {
	Dir    string
	Errors []string
}

func (p Problem) String() string { return p.Dir + ": " + strings.Join(p.Errors, "; ") }

// Registry is the set of loaded themes. It is safe for concurrent use; when
// asked about an id it does not know (a theme the owner just added), it
// reloads its sources, at most every ReloadEvery.
type Registry struct {
	mu        sync.RWMutex
	themes    map[string]*Theme
	aliases   map[string]string
	order     []string
	ornaments fs.FS // built-in ornaments: <name>.svg or <name>.png
	reload    func() (*Registry, []Problem)
	lastLoad  time.Time
}

// ReloadEvery bounds how often an unknown id triggers a reload.
var ReloadEvery = 10 * time.Second

// UserDir is where owners add themes.
func UserDir() string {
	if d := os.Getenv("BDTV_THEMES_DIR"); d != "" {
		return d
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, _ := os.UserHomeDir()
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "bear-den-tv", "themes")
}

// LoadDefault loads the built-in themes and the owner's folder.
func LoadDefault() (*Registry, []Problem) {
	load := func() (*Registry, []Problem) {
		builtIn, _ := fs.Sub(bdtv.Themes, "themes")
		orn, _ := fs.Sub(bdtv.Ornaments, "apps/tv-shell/assets/ornaments")
		var user fs.FS
		if st, err := os.Stat(UserDir()); err == nil && st.IsDir() {
			user = os.DirFS(UserDir())
		}
		return Load(builtIn, user, orn)
	}
	r, problems := load()
	r.reload = load
	return r, problems
}

// known reports whether id (or alias) is loaded, reloading once in a while
// when it is not.
func (r *Registry) known(id string) bool {
	r.mu.RLock()
	_, ok := r.themes[id]
	if !ok {
		_, ok = r.aliases[id]
	}
	stale := r.reload != nil && time.Since(r.lastLoad) > ReloadEvery
	r.mu.RUnlock()
	if ok || !stale {
		return ok
	}
	fresh, _ := r.reload()
	r.mu.Lock()
	r.themes, r.aliases, r.order, r.ornaments = fresh.themes, fresh.aliases, fresh.order, fresh.ornaments
	r.lastLoad = time.Now()
	_, ok = r.themes[id]
	if !ok {
		_, ok = r.aliases[id]
	}
	r.mu.Unlock()
	return ok
}

// Load reads every package folder in builtIn, then user (either may be nil).
func Load(builtIn, user, ornaments fs.FS) (*Registry, []Problem) {
	r := &Registry{themes: map[string]*Theme{}, aliases: map[string]string{}, ornaments: ornaments, lastLoad: time.Now()}
	var problems []Problem
	var others []string
	for _, src := range []struct {
		root    fs.FS
		builtIn bool
	}{{builtIn, true}, {user, false}} {
		if src.root == nil {
			continue
		}
		entries, err := fs.ReadDir(src.root, ".")
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir, _ := fs.Sub(src.root, e.Name())
			t, errs := LoadPackage(dir, e.Name(), ornaments)
			if len(errs) > 0 {
				if _, err := fs.Stat(dir, "theme.json"); err == nil {
					problems = append(problems, Problem{Dir: e.Name(), Errors: errs})
				}
				continue
			}
			t.BuiltIn = src.builtIn
			if _, exists := r.themes[t.ID]; !exists && !contains(builtInOrder, t.ID) {
				others = append(others, t.ID)
			}
			r.themes[t.ID] = t
			for _, a := range t.Aliases {
				r.aliases[a] = t.ID
			}
		}
	}
	for _, id := range builtInOrder {
		if _, ok := r.themes[id]; ok {
			r.order = append(r.order, id)
		}
	}
	sort.Strings(others)
	r.order = append(r.order, others...)
	return r, problems
}

// LoadPackage validates one package folder named folder. It checks the manifest
// against the schema, the id against the folder name, and that every file and
// ornament it names exists (in the folder, or among the built-in ornaments).
func LoadPackage(dir fs.FS, folder string, ornaments fs.FS) (*Theme, []string) {
	raw, err := fs.ReadFile(dir, "theme.json")
	if err != nil {
		return nil, []string{"theme.json: " + err.Error()}
	}
	if err := contract.ValidateTheme(raw); err != nil {
		var ve *contract.ValidationError
		if errors.As(err, &ve) {
			return nil, ve.Details
		}
		return nil, []string{err.Error()}
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, []string{err.Error()}
	}
	var errs []string
	if m.ID != folder {
		errs = append(errs, fmt.Sprintf("id %q must equal the folder name %q", m.ID, folder))
	}
	files := []string{m.Wallpaper.Image, m.Phone.Backdrop}
	for _, s := range m.Wallpaper.Sprites {
		files = append(files, s.Sheet)
	}
	if c := m.Classic; c != nil {
		if c.Wallpaper != nil {
			files = append(files, c.Wallpaper.Image)
			for _, s := range c.Wallpaper.Sprites {
				files = append(files, s.Sheet)
			}
		}
		if c.Phone != nil {
			files = append(files, c.Phone.Backdrop)
		}
	}
	for _, f := range files {
		if f == "" {
			continue
		}
		if !assetPattern.MatchString(f) {
			errs = append(errs, fmt.Sprintf("file %q is badly named", f))
		} else if _, err := fs.Stat(dir, f); err != nil {
			errs = append(errs, fmt.Sprintf("file %q is missing", f))
		}
	}
	names := []string{m.Focus.Tip, m.Bears.Hat, m.Heading}
	names = append(names, m.Focus.Extras...)
	names = append(names, m.Focus.ExtrasBottom...)
	if m.Panel != nil {
		names = append(names, m.Panel.Tip)
		names = append(names, m.Panel.Extras...)
		names = append(names, m.Panel.ExtrasBottom...)
	}
	if m.Bears.Carry != "stick" {
		names = append(names, m.Bears.Carry)
	}
	switch m.Bears.Chase {
	case "", "firefly", "leaf", "star", "ember":
	default:
		names = append(names, m.Bears.Chase)
	}
	for _, n := range names {
		if n != "" && ornamentFile(dir, ornaments, n, false) == "" {
			errs = append(errs, fmt.Sprintf("ornament %q: no %s.svg or %s.png in the theme folder, and no built-in ornament of that name", n, n, n))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return &Theme{Manifest: m, files: dir}, nil
}

// ornamentFile finds an ornament: "<name>.png|svg" in the theme folder, or
// "_ornaments/<name>.png|svg" (built-in); "" when neither exists. The art
// style picks which kind comes first: the pixel PNG, or the Classic SVG.
func ornamentFile(dir, ornaments fs.FS, name string, classic bool) string {
	if !ornamentPattern.MatchString(name) {
		return ""
	}
	exts := []string{".png", ".svg"}
	if classic {
		exts = []string{".svg", ".png"}
	}
	for _, ext := range exts {
		if dir != nil {
			if _, err := fs.Stat(dir, name+ext); err == nil {
				return name + ext
			}
		}
	}
	if ornaments != nil {
		for _, ext := range exts {
			if _, err := fs.Stat(ornaments, name+ext); err == nil {
				return "../_ornaments/" + name + ext
			}
		}
	}
	return ""
}

// Canonical returns the id for an id or alias, or Fallback.
func (r *Registry) Canonical(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.canonicalLocked(id)
}

func (r *Registry) canonicalLocked(id string) string {
	if _, ok := r.themes[id]; ok {
		return id
	}
	if a, ok := r.aliases[id]; ok {
		return a
	}
	if _, ok := r.themes[Fallback]; ok || len(r.order) == 0 {
		return Fallback
	}
	return r.order[0]
}

// Get returns the theme for an id or alias (the fallback when unknown), or nil.
func (r *Registry) Get(id string) *Theme {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.themes[r.canonicalLocked(id)]
}

// List names the installed themes in display order.
func (r *Registry) List() []contract.ThemeSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked()
}

func (r *Registry) listLocked() []contract.ThemeSummary {
	out := make([]contract.ThemeSummary, 0, len(r.order))
	for _, id := range r.order {
		t := r.themes[id]
		out = append(out, contract.ThemeSummary{ID: id, Name: t.Name, Accent: t.Accent})
	}
	return out
}

// URL of a theme file for phones ("/themes/<id>/<file>"), resolving ornament
// names; "" when absent.
func (r *Registry) ornamentURL(t *Theme, name string, classic bool) string {
	f := ornamentFile(t.files, r.ornaments, name, classic)
	if f == "" {
		return ""
	}
	return path.Clean("/themes/" + t.ID + "/" + f)
}

// Appearance is how a phone should look for this layout.ui: the theme's
// name, colours, tile decoration, backdrop and particles, plus the list of
// installed themes. Plain ("plain-dark") and Performance keep colours and drop decorations.
func (r *Registry) Appearance(ui contract.UI) contract.Appearance {
	r.known(ui.Background) // a theme added since startup loads here
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.themes[r.canonicalLocked(ui.Background)]
	classic := ui.Classic()
	a := contract.Appearance{Background: ui.Background, Theme: ui.Theme, Accent: ui.Accent, ArtStyle: contract.ArtPixel, Themes: r.listLocked()}
	if classic {
		a.ArtStyle = contract.ArtClassic
	}
	if t == nil {
		return a
	}
	a.Background = t.ID
	a.Name = t.Name
	a.Pixel = t.Wallpaper.Pixel
	a.Palette = t.Palette
	plain := ui.Theme == "plain-dark" || ui.Theme == "performance"
	style := t.Focus.Style
	if plain || style == "none" {
		style = ""
	}
	a.Focus = &contract.AppearanceFocus{Style: style, TipUpright: t.Focus.TipUpright}
	if style != "" && t.Focus.Tip != "" {
		a.Focus.Tip = r.ornamentURL(t, t.Focus.Tip, classic)
	}
	particles := t.Phone.Particles
	if particles == "" {
		particles = t.Ambient.Kind
	}
	if plain {
		particles = "none"
	}
	backdrop := t.Phone.Backdrop
	if backdrop == "" {
		backdrop = t.Wallpaper.Image
	}
	// Classic: the theme's classic backdrop, else its classic wallpaper; a
	// theme without classic art keeps its main one (drawn as it is).
	if c := t.Classic; classic && c != nil {
		switch {
		case c.Phone != nil && c.Phone.Backdrop != "":
			backdrop, a.Pixel = c.Phone.Backdrop, false
		case c.Wallpaper != nil && c.Wallpaper.Image != "":
			backdrop, a.Pixel = c.Wallpaper.Image, false
		}
	}
	veil := t.Phone.Veil
	if veil == "" {
		veil = t.Wallpaper.Bottom
	}
	a.Phone = &contract.AppearancePhone{Veil: veil, Particles: particles}
	if backdrop != "" {
		a.Phone.Backdrop = "/themes/" + t.ID + "/" + backdrop
	}
	return a
}

// Assets is a read-only file tree for phones: "<id>/<file>" for image files
// of loaded themes (wallpaper, backdrop, sprite sheets, ornaments) and
// "_ornaments/<name>.svg|png" for the built-in ornaments.
// Nothing else (manifests, other files, other folders) is reachable.
func (r *Registry) Assets() fs.FS { return assetFS{r} }

type assetFS struct{ r *Registry }

func (a assetFS) Open(name string) (fs.File, error) {
	a.r.mu.RLock()
	defer a.r.mu.RUnlock()
	if !fs.ValidPath(name) {
		return nil, fs.ErrNotExist
	}
	dir, file, ok := strings.Cut(name, "/")
	if !ok || strings.Contains(file, "/") || !assetPattern.MatchString(file) {
		return nil, fs.ErrNotExist
	}
	if dir == "_ornaments" {
		if a.r.ornaments == nil || !(strings.HasSuffix(file, ".svg") || strings.HasSuffix(file, ".png")) {
			return nil, fs.ErrNotExist
		}
		return a.r.ornaments.Open(file)
	}
	t, ok := a.r.themes[dir]
	if !ok || !idPattern.MatchString(dir) {
		return nil, fs.ErrNotExist
	}
	return t.files.Open(file)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
