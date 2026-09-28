// Package adapters holds the per-application knowledge for the three approved
// clients, Plex HTPC, VacuumTube and Moonlight: the only Flatpak id each may
// launch, the closed list of launch arguments, WM_CLASS-based window
// matching, the action → logical-key map, and the pause verification flag. Everything here
// is unverified against the target until the live probe records evidence in
// tests/compatibility/; PauseVerified stays false until then.
package adapters

import (
	"sort"
	"strings"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/platform"
)

// Adapter names as used by config (`adapter` field) and the registry.
const (
	PlexHTPCName   = "plex-htpc"
	VacuumTubeName = "vacuumtube"
	MoonlightName  = "moonlight"

	// PlexHTPCFlatpakID is the published Flatpak id of Plex HTPC (spec §7.1).
	PlexHTPCFlatpakID = "tv.plex.PlexHTPC"
	// VacuumTubeFlatpakID is the documented Flatpak id of VacuumTube (spec §8).
	VacuumTubeFlatpakID = "rocks.shy.VacuumTube"
	// MoonlightFlatpakID is the Flathub id of Moonlight (game streaming client).
	MoonlightFlatpakID = "com.moonlight_stream.Moonlight"
)

// WM_CLASS fragments (lower-cased substring match against any WM_CLASS
// element). UNVERIFIED: taken from documentation and naming conventions, not
// from the target. Replace with the exact WM_CLASS the live probe records in
// tests/compatibility/ once each client is installed there.
var (
	plexClassFragments       = []string{"plex"}       // expected "Plex HTPC" / "plexhtpc"
	vacuumTubeClassFragments = []string{"vacuumtube"} // expected "vacuumtube" / "VacuumTube"
	moonlightClassFragments  = []string{"moonlight"}  // expected "moonlight" / "Moonlight"
)

// navKeys is the action → key map shared by every Leanback-style client.
var navKeys = map[string]platform.Key{
	"nav.up":    platform.KeyUp,
	"nav.down":  platform.KeyDown,
	"nav.left":  platform.KeyLeft,
	"nav.right": platform.KeyRight,
	"select":    platform.KeySelect,
	"back":      platform.KeyBack,
}

// plexMediaKeys are UNVERIFIED: Plex HTPC documents media-key handling, but
// no probe on the target has confirmed XF86AudioPlay/Pause reach it.
var plexMediaKeys = map[string]platform.Key{
	"media.play":  platform.KeyPlay,
	"media.pause": platform.KeyPause,
}

type app struct {
	name      string
	flatpakID string
	args      []string
	fragments []string
	keys      map[string]platform.Key
}

// Name implements applications.Adapter.
func (a *app) Name() string { return a.name }

// FlatpakID implements applications.Adapter.
func (a *app) FlatpakID() string { return a.flatpakID }

// ApprovedArgs implements applications.Adapter; the returned slice is a copy.
func (a *app) ApprovedArgs() []string { return append([]string(nil), a.args...) }

// MatchWindow implements applications.Adapter on WM_CLASS only; titles are
// never consulted (spec §6.2).
func (a *app) MatchWindow(w platform.WindowInfo) bool {
	return MatchClass(w.Class, a.fragments)
}

// KeyFor implements applications.Adapter.
func (a *app) KeyFor(action string) (platform.Key, bool) {
	k, ok := a.keys[action]
	return k, ok
}

// MediaMatch implements applications.Adapter with the Flatpak id, which is
// also the expected MPRIS DesktopEntry.
func (a *app) MediaMatch() string { return a.flatpakID }

// PauseVerified implements applications.Adapter: nothing has been verified on
// the target; flip only with recorded evidence in tests/compatibility/.
func (a *app) PauseVerified() bool { return false }

// Actions lists the actions this adapter maps, sorted, for diagnostics.
func (a *app) Actions() []string {
	out := make([]string, 0, len(a.keys))
	for k := range a.keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MatchClass reports whether any WM_CLASS element contains any fragment,
// case-insensitively.
func MatchClass(class []string, fragments []string) bool {
	for _, c := range class {
		lc := strings.ToLower(c)
		for _, f := range fragments {
			if f != "" && strings.Contains(lc, f) {
				return true
			}
		}
	}
	return false
}

func merged(maps ...map[string]platform.Key) map[string]platform.Key {
	out := map[string]platform.Key{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// PlexHTPC returns the Plex HTPC adapter: no launch arguments (spec §7.1),
// navigation plus unverified media keys.
func PlexHTPC() applications.Adapter {
	return &app{name: PlexHTPCName, flatpakID: PlexHTPCFlatpakID, fragments: plexClassFragments, keys: merged(navKeys, plexMediaKeys)}
}

// VacuumTube returns the VacuumTube adapter: documented fullscreen flags
// (spec §8), navigation only — media.* stays unmapped until verified.
func VacuumTube() applications.Adapter {
	return &app{name: VacuumTubeName, flatpakID: VacuumTubeFlatpakID, args: []string{"--fullscreen", "--no-window-decorations"}, fragments: vacuumTubeClassFragments, keys: merged(navKeys)}
}

// Moonlight returns the Moonlight adapter: no launch arguments, navigation
// keys for its controller-friendly menus. While a stream runs, Moonlight sends
// keyboard input to the streaming host, so mapped keys reach that host; Home
// still works because it activates the shell window instead of sending keys.
// No MPRIS: media.* stays unmapped.
func Moonlight() applications.Adapter {
	return &app{name: MoonlightName, flatpakID: MoonlightFlatpakID, fragments: moonlightClassFragments, keys: merged(navKeys)}
}

// Registry resolves config adapter names to adapters.
type Registry struct {
	byName map[string]applications.Adapter
}

// NewRegistry returns a registry holding every approved adapter.
func NewRegistry() *Registry {
	r := &Registry{byName: map[string]applications.Adapter{}}
	for _, a := range []applications.Adapter{PlexHTPC(), VacuumTube(), Moonlight()} {
		r.byName[a.Name()] = a
	}
	return r
}

// ForName returns the adapter registered under name.
func (r *Registry) ForName(name string) (applications.Adapter, bool) {
	a, ok := r.byName[name]
	return a, ok
}

// Names lists registered adapter names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns the registered adapters in Names order.
func (r *Registry) All() []applications.Adapter {
	names := r.Names()
	out := make([]applications.Adapter, 0, len(names))
	for _, n := range names {
		out = append(out, r.byName[n])
	}
	return out
}

var defaultRegistry = NewRegistry()

// ForName resolves name against the default registry.
func ForName(name string) (applications.Adapter, bool) {
	return defaultRegistry.ForName(name)
}
