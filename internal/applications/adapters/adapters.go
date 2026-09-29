// Package adapters holds the per-application knowledge for the approved
// clients: the three core apps (Plex HTPC, VacuumTube, Moonlight) and the
// optional ones (Spotify, Jellyfin Desktop, RetroArch; hidden while not
// installed, config hide_when_missing), the web apps, and the browsers the
// web apps may run in (Browsers: Chromium, Brave). Per app: the only Flatpak id it may
// launch, the closed list of launch arguments, window matching by WM_CLASS
// (X11) or app_id (Wayland), the action → logical-key map, the MPRIS match, how Home may pause
// it (HomePause) and the pause verification flag. Everything here
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
	SpotifyName    = "spotify"
	JellyfinName   = "jellyfin"
	RetroArchName  = "retroarch"

	// PlexHTPCFlatpakID is the published Flatpak id of Plex HTPC (spec §7.1).
	PlexHTPCFlatpakID = "tv.plex.PlexHTPC"
	// VacuumTubeFlatpakID is the documented Flatpak id of VacuumTube (spec §8).
	VacuumTubeFlatpakID = "rocks.shy.VacuumTube"
	// MoonlightFlatpakID is the Flathub id of Moonlight (game streaming client).
	MoonlightFlatpakID = "com.moonlight_stream.Moonlight"
	// SpotifyFlatpakID is the Flathub id of Spotify (checked on flathub.org).
	SpotifyFlatpakID = "com.spotify.Client"
	// JellyfinFlatpakID is the Flathub id of Jellyfin Desktop, the renamed
	// Jellyfin Media Player (2.0; the old com.github.iwalton3.jellyfin-media-player
	// is end-of-life on Flathub).
	JellyfinFlatpakID = "org.jellyfin.JellyfinDesktop"
	// RetroArchFlatpakID is the Flathub id of RetroArch.
	RetroArchFlatpakID = "org.libretro.RetroArch"
)

// WM_CLASS fragments (lower-cased substring match against any WM_CLASS
// element). UNVERIFIED: taken from documentation and naming conventions, not
// from the target. Replace with the exact WM_CLASS the live probe records in
// tests/compatibility/ once each client is installed there.
var (
	plexClassFragments       = []string{"plex"}       // expected "Plex HTPC" / "plexhtpc"
	vacuumTubeClassFragments = []string{"vacuumtube"} // expected "vacuumtube" / "VacuumTube"
	moonlightClassFragments  = []string{"moonlight"}  // expected "moonlight" / "Moonlight"
	spotifyClassFragments    = []string{"spotify"}    // expected "spotify" / "Spotify"
	jellyfinClassFragments   = []string{"jellyfin"}   // expected "org.jellyfin.JellyfinDesktop" (its .desktop StartupWMClass)
	retroArchClassFragments  = []string{"retroarch"}  // expected "retroarch" / "RetroArch"
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

// spotifyKeys follow Spotify's documented shortcuts
// (support.spotify.com, "Keyboard shortcuts"): Up/Down move through a list and
// Return plays the selected row. Left/Right are left unmapped because Spotify
// documents them as list actions (add to library/queue), not as moving focus;
// there is no documented Back key. Playback goes through MPRIS instead.
var spotifyKeys = map[string]platform.Key{
	"nav.up":   platform.KeyUp,
	"nav.down": platform.KeyDown,
	"select":   platform.KeySelect,
}

// retroArchKeys are RetroArch's default keyboard controls (retroarch.cfg and
// docs.libretro.com "Input and controls"): arrows navigate the menu, x
// confirms (input_player1_a), z goes back (input_player1_b). Escape is never
// sent: it is input_exit_emulator and would quit RetroArch.
var retroArchKeys = map[string]platform.Key{
	"nav.up":    platform.KeyUp,
	"nav.down":  platform.KeyDown,
	"nav.left":  platform.KeyLeft,
	"nav.right": platform.KeyRight,
	"select":    platform.KeyLetterX,
	"back":      platform.KeyLetterZ,
}

// HomePause says how Home may pause an app before bringing the shell forward
// (config home_policy "pause-if-supported"; contracts/actions.md `home`). It
// is data. The router (internal/session/homepause.go) acts only on a pause it
// can check at run time: "mpris" is sent only when the app's own player
// reports Playing, and claimed only when it then reports Paused. A "key" is
// never sent (no readable state to check; a Toggle key could even resume a
// paused app). "page" is the web apps' page pause (session/web.go).
// PauseVerified is the separate, live-evidence flag and stays false.
type HomePause struct {
	// Kind: "none" (keep playing: music, game streams), "mpris" (MPRIS
	// Pause, idempotent), "key" (a documented pause key; recorded, not sent)
	// or "page" (web apps).
	Kind string
	// Key is the pause key for Kind "key".
	Key platform.Key
	// Toggle marks a key that also un-pauses: sent to an already paused app
	// it would resume it, so it needs the app's state first.
	Toggle bool
	// Why explains the choice (docs, diagnostics).
	Why string
}

type app struct {
	name      string
	flatpakID string
	args      []string
	fragments []string
	keys      map[string]platform.Key
	media     string // MPRIS match when it is not the Flatpak id
	home      HomePause
}

// Name implements applications.Adapter.
func (a *app) Name() string { return a.name }

// FlatpakID implements applications.Adapter.
func (a *app) FlatpakID() string { return a.flatpakID }

// ApprovedArgs implements applications.Adapter; the returned slice is a copy.
func (a *app) ApprovedArgs() []string { return append([]string(nil), a.args...) }

// MatchWindow implements applications.Adapter on WM_CLASS (X11) or the
// Wayland app_id; titles are never consulted (spec §6.2).
func (a *app) MatchWindow(w platform.WindowInfo) bool {
	return MatchClass(w.Class, a.fragments) || MatchAppID(w.AppID, a.flatpakID, a.fragments)
}

// KeyFor implements applications.Adapter.
func (a *app) KeyFor(action string) (platform.Key, bool) {
	k, ok := a.keys[action]
	return k, ok
}

// MediaMatch implements applications.Adapter with the Flatpak id, which is
// also the expected MPRIS DesktopEntry.
func (a *app) MediaMatch() string {
	if a.media != "" {
		return a.media
	}
	return a.flatpakID
}

// HomePause returns how Home may pause this app (data; see HomePause).
func (a *app) HomePause() HomePause { return a.home }

// HomePauseOf returns ad's HomePause, or Kind "none" for an adapter that does
// not declare one.
func HomePauseOf(ad applications.Adapter) HomePause {
	if h, ok := ad.(interface{ HomePause() HomePause }); ok {
		return h.HomePause()
	}
	return HomePause{Kind: "none"}
}

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

// MatchAppID reports whether a Wayland app_id names this app: exactly its
// Flatpak id (the usual app_id of a Flatpak'd Wayland client), or, like
// WM_CLASS, containing one of the fragments (an XWayland window's app_id is
// its X11 class). Case-insensitive; an empty app_id never matches.
// UNVERIFIED against the real clients on Wayland
// (docs/decisions/0007-wayland-profile.md).
func MatchAppID(appID, flatpakID string, fragments []string) bool {
	if appID == "" {
		return false
	}
	if flatpakID != "" && strings.EqualFold(appID, flatpakID) {
		return true
	}
	return MatchClass([]string{appID}, fragments)
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
	return &app{name: PlexHTPCName, flatpakID: PlexHTPCFlatpakID, fragments: plexClassFragments, keys: merged(navKeys, plexMediaKeys),
		home: HomePause{Kind: "mpris", Why: "A film should wait while you are Home: MPRIS Pause."}}
}

// VacuumTube returns the VacuumTube adapter: documented fullscreen flags
// (spec §8), navigation only — media.* stays unmapped until verified.
func VacuumTube() applications.Adapter {
	return &app{name: VacuumTubeName, flatpakID: VacuumTubeFlatpakID, args: []string{"--fullscreen", "--no-window-decorations"}, fragments: vacuumTubeClassFragments, keys: merged(navKeys),
		home: HomePause{Kind: "mpris", Why: "A video should wait while you are Home: MPRIS Pause."}}
}

// Moonlight returns the Moonlight adapter: no launch arguments, navigation
// keys for its controller-friendly menus. While a stream runs, Moonlight sends
// keyboard input to the streaming host, so mapped keys reach that host; Home
// still works because it activates the shell window instead of sending keys.
// No MPRIS: media.* stays unmapped.
func Moonlight() applications.Adapter {
	return &app{name: MoonlightName, flatpakID: MoonlightFlatpakID, fragments: moonlightClassFragments, keys: merged(navKeys),
		home: HomePause{Kind: "none", Why: "The game runs on the streaming PC and keys would reach that host, so Home leaves it alone."}}
}

// Spotify returns the Spotify adapter (optional app): no launch arguments,
// Spotify's documented list keys (spotifyKeys). Its MPRIS player is
// org.mpris.MediaPlayer2.spotify, so MediaMatch is "spotify", not the
// Flatpak id (UNVERIFIED on the target). Home leaves music playing.
func Spotify() applications.Adapter {
	return &app{name: SpotifyName, flatpakID: SpotifyFlatpakID, fragments: spotifyClassFragments, keys: merged(spotifyKeys), media: "spotify",
		home: HomePause{Kind: "none", Why: "Music keeps playing while you are Home."}}
}

// Jellyfin returns the Jellyfin Desktop adapter (optional app): starts in its
// TV interface, full screen (`--fullscreen --tv`, from its --help and its
// .desktop actions). Its keyboard map (resources/inputmaps/keyboard.json)
// gives arrows, Return (enter) and Escape (short press: back); media.* stays
// unmapped until verified. Its MPRIS DesktopEntry is the Flatpak id.
func Jellyfin() applications.Adapter {
	return &app{name: JellyfinName, flatpakID: JellyfinFlatpakID, args: []string{"--fullscreen", "--tv"}, fragments: jellyfinClassFragments, keys: merged(navKeys),
		home: HomePause{Kind: "mpris", Why: "A film should wait while you are Home: MPRIS Pause."}}
}

// RetroArch returns the RetroArch adapter (optional app): full screen
// (`--fullscreen`), RetroArch's default menu keys (retroArchKeys). No MPRIS.
// Home's pause is the documented pause key p (input_pause_toggle), a toggle.
func RetroArch() applications.Adapter {
	return &app{name: RetroArchName, flatpakID: RetroArchFlatpakID, args: []string{"--fullscreen"}, fragments: retroArchClassFragments, keys: merged(retroArchKeys),
		home: HomePause{Kind: "key", Key: platform.KeyLetterP, Toggle: true, Why: "A game should wait while you are Home: RetroArch's pause key (p, input_pause_toggle)."}}
}

// Web adapter names (config adapter, contracts/config.md rule 11).
const (
	NetflixName    = "netflix"
	DisneyPlusName = "disney-plus"
	HuluName       = "hulu"
	BrowserName    = "browser"

	// ChromiumFlatpakID is Flathub's Chromium, the default browser of every
	// web adapter (docs/decisions/0010-web-apps-over-cdp-pipe.md).
	ChromiumFlatpakID = "org.chromium.Chromium"
	// BraveFlatpakID is Flathub's Brave (published by Brave Software),
	// the owner's alternative (config apps.browser and
	// apps.streaming_browser; docs/decisions/0013-brave-as-a-browser-choice.md).
	BraveFlatpakID = "com.brave.Browser"
	// WebClassPrefix starts the WM_CLASS Bear Den gives each web adapter's
	// Chromium (--class=BearDenWeb-<adapter>), so windows of different web
	// apps and of other Chromium windows are told apart.
	WebClassPrefix = "BearDenWeb-"
)

// Web modes: a streaming site opens as a full-screen app window (no tabs,
// no address bar); the browser is ordinary Chromium, maximized.
const (
	WebModeApp     = "app"
	WebModeBrowser = "browser"
)

// WebSpec is what makes an adapter a web adapter: Chromium opens a page in
// its own profile and the coordinator drives it through the navigation
// script over the DevTools pipe (internal/applications/web).
type WebSpec struct {
	// Mode is WebModeApp or WebModeBrowser.
	Mode string
	// Hints names the navigation hints file (apps/web-nav/hints/<id>.json);
	// empty means the generic behaviour only.
	Hints string
	// Class is the WM_CLASS class Chromium is started with.
	Class string
}

// webApp is a web adapter: no key map (input goes through the page, never
// XTEST), MPRIS matched on Chromium's desktop entry for Now playing.
type webApp struct {
	app
	web WebSpec
}

// Web returns the adapter's web spec.
func (w *webApp) Web() WebSpec { return w.web }

// WebOf reports whether ad is a web adapter and returns its spec.
func WebOf(ad applications.Adapter) (WebSpec, bool) {
	if w, ok := ad.(interface{ Web() WebSpec }); ok {
		return w.Web(), true
	}
	return WebSpec{}, false
}

// OwnFlatpakIcon reports whether ad's Flatpak is the app itself, so the icon
// that Flatpak exports is the app's own (layout ui.app_icons "app"). False
// for the streaming sites: they run in Chromium and never show its icon; the
// Browser tile is Chromium and may.
func OwnFlatpakIcon(ad applications.Adapter) bool {
	if spec, ok := WebOf(ad); ok {
		return spec.Mode == WebModeBrowser
	}
	return true
}

func newWeb(name, mode, hints string) applications.Adapter {
	class := WebClassPrefix + name
	return &webApp{
		app: app{name: name, flatpakID: ChromiumFlatpakID, fragments: []string{strings.ToLower(class)}, keys: map[string]platform.Key{},
			// Flathub Chromium's MPRIS DesktopEntry is its Flatpak id
			// (UNVERIFIED on the TV); matching the bare "chromium" bus name
			// would also catch Electron apps such as VacuumTube.
			media: ChromiumFlatpakID,
			home:  HomePause{Kind: "page", Why: "A film should wait while you are Home: the site's own pause key, only when the page reports a playing video."}},
		web: WebSpec{Mode: mode, Hints: hints, Class: class},
	}
}

// Browser names (config apps.browser and apps.streaming_browser).
const (
	ChromiumBrowser = "chromium"
	BraveBrowser    = "brave"
	// DefaultBrowser is every web adapter's browser when config names none.
	DefaultBrowser = ChromiumBrowser
)

// BrowserInfo is one browser web adapters may run in: a Flathub Flatpak,
// Chromium-based (the DevTools pipe and the navigation script work the
// same), started by internal/applications/web with Bear Den's own profile
// per app. The table below is the only place a browser's Flatpak id and
// profile settings are written.
type BrowserInfo struct {
	// Name is the config value; Label is what the TV shows.
	Name  string
	Label string
	// FlatpakID is the Flathub id installed (one press, ADR 0011) and run.
	FlatpakID string
	// ProfileRoot is the folder under $XDG_DATA_HOME/bear-den-tv holding
	// this browser's profiles (<ProfileRoot>/<app-id>): each browser keeps
	// its own, never one profile opened by two browsers.
	ProfileRoot string
	// GrantProfileRoot: the Flatpak cannot see $XDG_DATA_HOME, so `flatpak
	// run` gets --filesystem=<that profile root> (only it) for this run.
	GrantProfileRoot bool
	// StreamingUnverified: nothing shows that the streaming sites' Widevine
	// works in this browser's Flatpak; the TV says so next to the choice.
	StreamingUnverified bool
	// LocalState and Preferences are prefs Bear Den writes, before every
	// start, into its own profiles only: <profile>/Local State and
	// <profile>/Default/Preferences. Keys are dotted pref paths.
	LocalState  map[string]any
	Preferences map[string]any
}

// browsers is the table, in the order the TV offers them.
var browsers = []BrowserInfo{
	{Name: ChromiumBrowser, Label: "Chromium", FlatpakID: ChromiumFlatpakID, ProfileRoot: "web"},
	{
		Name: BraveBrowser, Label: "Brave", FlatpakID: BraveFlatpakID, ProfileRoot: "web-brave",
		// The Flatpak's finish-args give it neither home nor xdg-data
		// (flathub/com.brave.Browser, com.brave.Browser.yaml).
		GrantProfileRoot: true,
		// Its launcher exposes Widevine to the sandbox only from the
		// default profile's WidevineCdm folder (cobalt ExposeWidevine), not
		// from a --user-data-dir; never seen playing (ADR 0013).
		StreamingUnverified: true,
		// Widevine opt-in: brave-core kWidevineEnabled, a Local State pref
		// (components/constants/pref_names.h, browser/widevine/widevine_utils.cc).
		// P3A's first-run notice is a Local State pref too (components/p3a/pref_names.h).
		LocalState: map[string]any{
			"brave.widevine_opted_in":       true,
			"brave.p3a.notice_acknowledged": true,
		},
		// Brave's own features stay out of the kiosk profile's way (names
		// from brave-core's pref_names.h files; effect not seen on a TV).
		Preferences: map[string]any{
			"brave.has_seen_brave_welcome_page":                       true,
			"brave.show_fullscreen_reminder":                          false,
			"brave.brave_vpn.show_button":                             false,
			"brave.wallet.show_wallet_icon_on_toolbar":                false,
			"brave.ai_chat.show_toolbar_button":                       false,
			"brave.ai_chat.context_menu_enabled":                      false,
			"brave.rewards.show_brave_rewards_button_in_location_bar": false,
			"brave.new_tab_page.show_rewards":                         false,
			"brave.new_tab_page.show_brave_vpn":                       false,
			"brave.new_tab_page.show_brave_news":                      false,
			"brave.today.should_show_toolbar_button":                  false,
			"brave.ask_widevine_install":                              false,
		},
	},
}

// Browsers returns the browser table (a copy), in the TV's order.
func Browsers() []BrowserInfo { return append([]BrowserInfo(nil), browsers...) }

// BrowserNamed returns the table row for a config browser name.
func BrowserNamed(name string) (BrowserInfo, bool) {
	for _, b := range browsers {
		if b.Name == name {
			return b, true
		}
	}
	return BrowserInfo{}, false
}

// BrowserByFlatpakID returns the table row whose Flatpak id is id.
func BrowserByFlatpakID(id string) (BrowserInfo, bool) {
	for _, b := range browsers {
		if b.FlatpakID == id {
			return b, true
		}
	}
	return BrowserInfo{}, false
}

// IsStreaming reports whether a web spec is a streaming site (an app
// window; apps.streaming_browser) rather than the Browser tile
// (apps.browser).
func (s WebSpec) IsStreaming() bool { return s.Mode == WebModeApp }

// RunsIn reports whether ad may run from the Flatpak flatpakID: its own
// id, or for a web adapter any browser in the table.
func RunsIn(ad applications.Adapter, flatpakID string) bool {
	if ad.FlatpakID() == flatpakID {
		return true
	}
	if _, web := WebOf(ad); web {
		_, ok := BrowserByFlatpakID(flatpakID)
		return ok
	}
	return false
}

// InstallableFlatpakIDs is every Flatpak id Bear Den may install: each
// adapter's own and every browser's, once each.
func (r *Registry) InstallableFlatpakIDs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, a := range r.All() {
		add(a.FlatpakID())
	}
	for _, b := range browsers {
		add(b.FlatpakID)
	}
	return out
}

// Netflix returns the Netflix web adapter (Chromium app window, hints
// netflix.json). Plays at up to 720p in a Linux browser.
func Netflix() applications.Adapter { return newWeb(NetflixName, WebModeApp, "netflix") }

// DisneyPlus returns the Disney+ web adapter.
func DisneyPlus() applications.Adapter { return newWeb(DisneyPlusName, WebModeApp, "disney-plus") }

// Hulu returns the Hulu web adapter.
func Hulu() applications.Adapter { return newWeb(HuluName, WebModeApp, "hulu") }

// Browser returns the Browser adapter: ordinary Chromium in its own
// profile, for keyboard and mouse, with the navigation script too.
func Browser() applications.Adapter { return newWeb(BrowserName, WebModeBrowser, "") }

// Registry resolves config adapter names to adapters.
type Registry struct {
	byName map[string]applications.Adapter
}

// NewRegistry returns a registry holding every approved adapter.
func NewRegistry() *Registry {
	r := &Registry{byName: map[string]applications.Adapter{}}
	for _, a := range []applications.Adapter{PlexHTPC(), VacuumTube(), Moonlight(), Spotify(), Jellyfin(), RetroArch(), Netflix(), DisneyPlus(), Hulu(), Browser()} {
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
