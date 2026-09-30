# apps/tv-shell/: the TV shell (Qt 6.8, QML + C++)

`bear-den-tv-shell` is everything on the TV between apps: Home, Settings,
pairing, dialogs, themes and bears. It owns presentation and the focus graph
and nothing else. The coordinator starts and supervises it and talks to it over
the private socket in [`contracts/ipc.md`](../../contracts/ipc.md). It never
launches apps, reads config files or listens on the network.

The whole shell is one static QML module `BearDen` (library `beardenshell`,
[`CMakeLists.txt`](CMakeLists.txt)). The executable and the tests link the same
module, so tests exercise the shipped QML.

## C++ types exposed to QML

| QML name | Class | Role |
|---|---|---|
| `Session` (singleton) | [`SessionModel`](src/SessionModel.h) | the latest state snapshot (incl. `weather`, `power`, `plex`, `achievements`, `apps`, `onboarding` (`{completed}`) and `autostart` (`{enabled, available, reason?}`), empty when absent; app tiles carry `installState`/`installProgress` from `applications[].install`), validated by `validateSnapshot`/`validateLayout` then applied whole (rejected snapshots keep the previous state); `application(id)`, `layoutForEdit()`, rails as `sections` |
| `Nav` (singleton) | [`Navigator`](src/Navigator.h) | the one input path: key events and coordinator `input` become named actions; `apply(action)` returns the `input_result`; `reportFocus(section, item, scrollX)`, `noteAtRoot()`, `screen`, `textFieldFocused`; `focusReported` carries `text_field` and is re-emitted when only that changes |
| `Shell` (singleton) | [`ShellController`](src/ShellController.h) | the IPC bridge. Methods: `launchApp`, `closeApp`, `issuePairing(pass)` (`""` a family phone, `tonight`/`24h`/`7d` a guest pass), `cancelPairing`, `revokeDevice`, `configureRemote(enabled, interface)`, `updateLayout(layout)`, `setPlayback(adapter, setting, value)`, `setNowPlaying(enabled)` (IPC `remote.now_playing`), `setAppEnabled(appId, enabled)` (IPC `app.enable`, Apps → Streaming sites), `installInfo(appId)`, `installApp(appId)`, `cancelInstall(appId)` (IPC `app.install_info`, `app.install`, `app.install_cancel`; answers in `installReplied(type, appId, ok, error, data)`, progress in `state.applications[].install`), `setAutoUpdate(enabled)` (IPC `apps.configure`), `setBrowsers(browser, streamingBrowser)` (IPC `apps.browser`, Apps → Streaming sites), `setCEC(enabled, volumeTarget)` (IPC `cec.configure`), `completeOnboarding()` (IPC `onboarding.complete`), `setAutostart(enabled)` (IPC `autostart.configure`; answers in `setupReplied(type, ok, error)`, a refusal also raises `requestFailed`), `weatherSearch(query)` (answer in `weatherPlaces`/`weatherSearchOk`/`weatherSearchError`/`weatherSearching`), `weatherConfigure(enabled, place, units, scene)` (place `null` keeps the stored one; answer in `weatherConfigured(ok, error)`), `setSleepTimer(minutes)` (0 cancels) and `screenOff()` (the `power.sleep_timer` and `display.off` actions), `powerActivity()` (IPC `power.activity`), `plexSignIn`, `plexCancel`, `plexChooseServer(id)`, `plexChooseLibraries(ids)`, `plexSignOut` (IPC `plex.*`; answer in `plexReplied(type, ok, error)`, the flow itself in `Session.plex`), `setAchievements(enabled)`, `resetAchievements()`, `achievementsCelebrated(ids)`, `achievementEvent(event)` (IPC `achievements.*`, Den badges), `answerConfirm`, `exitShell`, `flatpakIdFor`, `ownIconFlatpakIdFor` (the Flatpak whose exported icon is the app's own; empty for the streaming sites), `appArt`, `lanInterfaces`. Properties: connection state (`connectionState`, `connected`, `rejectReason`, `attempt`), `offline`, `devBuild`, `version`, `startScreen`, `launchingAppId`, `screensaverSeconds`, and the check overrides `bearsSeconds`, `bearsAct`, `restSeconds`, `monthOverride`, `lightningSeconds` |
| `Theme` (singleton) | [`Theme`](src/Theme.h) | design tokens from `layout.ui` and window size: colours, `scale`, type and tile sizes, `ms()`/`duration`, `reducedMotion`, `resting`, `screensaver`, `tintFor(id)` |
| `Themes` (singleton) | [`ThemeRegistry`](src/ThemeRegistry.h) | installed theme packages: `list`, `get(id)`, `canonical(id)`, `ornament()`, `reload()` ([`themes/AGENTS.md`](../../themes/AGENTS.md)) |
| `FocusMemory` (singleton) | [`FocusMemory`](src/FocusMemory.h) | remembered item, index and scroll per section; `lastSectionId` |
| `QrCode` | [`QrRenderer`](src/QrRenderer.h) | paints `pairing.qr_modules`; the coordinator encodes |
| `RoundedImage` | [`RoundedImage`](src/RoundedImage.h) | artwork backdrop without shaders (local files and qrc only); `pixelSize` > 1 composes it in whole art pixels, once per source and size (the hero in the Pixel art style) |
| (uncreatable) | `SectionsModel`, `ItemsModel`, `IpcClient` | id-preserving rail models; the socket client (offline mode records sent messages) |

QML singletons: [`Apps.qml`](qml/Apps.qml) (per-adapter copy and brand
colours), [`Badges.qml`](qml/Badges.qml) (Den badge names, hints and art by id), [`Help.qml`](qml/Help.qml) (one plain sentence of help per settings row id: Settings, Themes, Apps, setup) and [`World.qml`](qml/World.qml) (the active theme as components see
it), marked `QT_QML_SINGLETON_TYPE` in CMake.

## main.cpp flags

[`src/main.cpp`](src/main.cpp): `--dev` (honours `BDTV_SHELL_SOCKET`),
`--fixture PATH` (offline: render a state snapshot, no coordinator),
`--screen home|apps|themes|settings|pairing|devices|diagnostics|remote-setup|playback|advanced-playback|weather|plex|badges|onboarding` (any screen in ShellRoot's `screens` map; also `onboarding-0` … `onboarding-5` for one setup step, and the old names `streaming` and `add-apps`, which open the Apps page at that section), `--windowed`,
`--size WxH` (default 1920x1080), `--screenshot PATH`, `--screenshot-after MS`
(default 1500), `--screenshot-every MS` (`--dev` only), `--exit-after MS`.
Environment variables:

| Variable | Effect |
|---|---|
| `BDTV_FPS_LOG=1` | prints frames per second every 5 s; a resting shell prints 0 |
| `BDTV_THEMES_DIR` | your themes folder instead of `$XDG_DATA_HOME/bear-den-tv/themes` |
| `BDTV_BEARS_SECONDS`, `BDTV_BEARS_ACT` | fixed gap between bear visits; which act (`walk\|peek\|hop\|parade\|chase`) |
| `BDTV_REST_SECONDS` | seconds without input before resting (default 45) |
| `BDTV_SCREENSAVER_SECONDS` | screensaver delay override |
| `BDTV_MONTH` | pretend it is month 1–12 (featured-panel seasons) |
| `BDTV_LIGHTNING_SECONDS` | fixed gap between thunder's lightning flashes (`Shell.lightningSeconds`; 0 = flash after flash), to check the startled bears |
| `BDTV_SHELL_SOCKET` | the coordinator socket (with `--dev`) |
The WM_CLASS comes from the application name `bear-den-tv-shell`, which the
coordinator matches (`ShellClassFragments` in `internal/session`). On Wayland
the shell runs through XWayland (the toolchain's Qt has no wayland platform
plugin), and a wlroots compositor reports that class as its app_id
([ADR 0007](../../docs/decisions/0007-wayland-profile.md)).

## QML files

| Group | Files |
|---|---|
| Root and focus graph | `Main.qml` (window), [`ShellRoot.qml`](qml/ShellRoot.qml) (screen stack, dialogs, rest/screensaver timers, `handle(action)`) |
| Screens | `HomeScreen`, `AppsScreen` (the Apps pill: the installed apps as cards, OK opens one; Add apps, one card per missing Flatpak from `Session.applications[].install` (the web apps one card per browser), OK opens the install card, cards show progress; Streaming sites, one toggle per web app (the entries that carry `enabled`; OK sends `Shell.setAppEnabled` → IPC `app.enable`, and a site turned on while its browser is missing opens the install card), then "Browser tile uses" / "Streaming sites use" from `Session.apps.browsers` (◀ ▶ or OK send `Shell.setBrowsers` → IPC `apps.browser`; a `streaming_unverified` browser says so on the streaming row); Remove apps, one card per installed Flatpak while `app.uninstall` is listed, OK opens `RemoveCard` (Cancel focused, the size it frees from `install.installed_bytes`, which apps it turns off from `Shell.flatpakIdFor`, Remove / Remove and delete its data → `Shell.uninstallApp` → IPC `app.uninstall`; a system-wide install is refused with OK only; "Removing …" while `install.state` is `removing`, then it closes and a toast says so); Keep apps up to date (`Shell.setAutoUpdate`); the focused entry's help and app notes beside; `focusSection("add-apps"\|"streaming")`; reports `settings`), `ThemesScreen` (the Themes pill: `ThemePicker`'s live-preview theme cards, then Art style, Style, App icons and Den badges; reports `settings`), `SettingsScreen` (categories, see [Settings rows](#settings-rows)), `OnboardingScreen` (first-run setup, six steps, see [First-run setup](#first-run-setup); reports `setup`), `RemoteSetupScreen`, `PairingScreen` ("Who is it for?": family phone or guest pass; the address and guest note on a surface panel, `pairAddressPanel`), `DevicesScreen` (guests with a badge and the time left), `DiagnosticsScreen`, `PlaybackScreen`, `AdvancedPlaybackScreen`, `WeatherScreen` (Settings → Home screen → Weather: toggles, search field, places; reports `settings`), `PlexScreen` (Settings → Playback → Plex: sign in with a code and QR, choose a server and libraries, sign out; follows `Session.plex.status`; leaving mid-flow sends `plex.cancel`; reports `settings`), `BadgesScreen` (Themes → Den badges: the shelf of Den badges, 8 per row, then Counting and Reset badges (asks first); reports `settings`), `ConnectingScreen`, `LockedScreen`, `Screensaver` |
| Dialogs and overlays | `ConfirmDialog`, `MessageDialog`, `InstallCard` (OK on a "Not installed" tile, Add apps, a streaming site turned on without its browser: name, icon, size from `app.install_info`, "From Flathub", Install focused / Not now; progress from state pushes; Back hides it and the install carries on; Cancel install; without installs it says why with OK only; ShellRoot opens the app when done if the owner is still on its card or tile, only that app and only when it is on; `openFor(id, forApp)`: opened from the app's own tile, row or setup card, Install also sends `enable` so the coordinator turns that app on when installed, never from a shared browser card of Add apps), `LaunchOverlay` ("Opening …" while `Shell.launchingAppId` is set; after `Shell.launchObserved` it stays up, still, behind the app until a snapshot has the shell in front again or Home, so the app's first unpainted frames never show Bear Den's page), `ErrorBanner`, `Toast`, `SleepWarning` (the sleep timer's last minute: "Going to sleep in 1 minute", the cub dozing; while it shows, or while `Session.power.display` is `off`, `ShellRoot.handle` swallows keys and calls `Shell.powerActivity()`), `BadgeCelebration` (a new Den badge on Home: the medal on a card with confetti on `World.beat`; only while Home is in front with no dialog or screensaver, so a badge earned behind an app waits; then IPC `achievements.celebrated`) |
| Home pieces | `Header` (brand, pills Home · Apps · Themes · Pair phone · Settings, status, weather chip, clock; with large text the brand shows only the bear mark, then the pills drop their icons and narrow, so they never reach the status), `NavPill` (optional icon), `StatusChip`, `HeroPanel` (an app's notes as a "Good to know" line; a simple panel for the Add apps tile), `Rail`, `AppTile`, `AddAppsTile` (the last tile of the first apps rail while something can be installed: kind `add-apps`, added by `SessionModel::rebuildSections`, naming the first two missing apps, those without a tile first), `AppIcon`, `ContentCard`, `SetupCard`, `BrandBackdrop`, `DemoBadge`, `ProgressBar`, `FocusFrame` |
| Screen pieces | `ScreenFrame`, `SettingsRow` (optional leading icon; kinds `link`, `choice`, `toggle`, `danger`, `info`), `FocusButton`, `KeyHints`, `BadgeMedal` (one Den badge medal, pixel PNG or classic SVG), `UiIcon` (one of Bear Den's UI icons, `assets/pixel/icon-<name>.png` / `assets/classic/icon-<name>.svg`, [`docs/THEMES.md` → UI icons](../../docs/THEMES.md#ui-icons)), `HelpPanel` (the focused entry's name, its help from `Help.qml` and an app's notes), `ThemePicker` (the theme cards: ◀ ▶ previews with `Session.previewUi`, OK applies, `cancel()` returns) |
| Theme engine | `World` (incl. the pixel grid `World.px` and the weather override `World.weather*`), `Wallpaper`, `WeatherSky` (weather veil + lightning, sets `World.flashing`), `SceneWeather` (a corner scene's reactions to the weather, declared as data per look), `Ambient`, `Ornament`, `CornerDecor`, `FocusDecor`, `HeroDecor`, `CampScene`, `MoonScene`, `CampfireScene` |
| Featured panel | `HeroPanel` (typing, row reveal, cub behind OK, seasons), `HeroScene` (the app's room, generated `HeroRig.js`), `HeroLife` (visiting and dozing bears); per-app data in `Apps.stage()` |
| Pixel art | `PixelBox` (rounded-`Rectangle` stand-in with stair corners), `PixelSprite` (a sprite/frame from `assets/pixel/`); art generated by [`tools/pixelart`](../../tools/pixelart/) ([`docs/THEMES.md` → Pixel art](../../docs/THEMES.md#pixel-art)) |
| Bears | `BearMark`, `BearHead`, `BearPuppet` (+ generated `BearRig.js`), `BearVisitors`, `DenMascot`, `DenFamily` |
| Classic twins ([ADR 0006](../../docs/decisions/0006-classic-art-style.md)) | `AmbientClassic`, `BearPuppetClassic`, `CampSceneClassic`, `MoonSceneClassic`, `CampfireSceneClassic`, `DenFamilyClassic`: the same properties as the pixel component, drawn smooth |

## Animation and performance gates

The floor is a 2-core Celeron at 120 Hz. Rules:
[`docs/THEMES.md` → Performance rules](../../docs/THEMES.md#performance-rules).
The convention is one `alive` property that every timer and animation binds to:

```qml
readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting
                              && !Theme.screensaver && Session.target.kind === "shell"
```

(`CampScene.qml`, `DenFamily.qml`, `HeroDecor.qml`; `FocusDecor.qml` keeps its
slow focused motion while resting.) Anything that loops moves on the shared
heartbeat, not its own Timer or a looping animation:

```qml
Connections {
    target: World
    enabled: root.alive
    function onBeat(dt) { root.phase += dt }   // 20 beats/s awake, 4 resting
}
```

Separate timers each trigger their own frames and their rates add up; the
heartbeat puts every change in one frame. Canvas repaints only when inputs
change (each repaint costs one extra frame), no shaders. `make perf` fails
when resting or the screensaver draws over budget. Durations go through
`Theme.ms()`/`Theme.duration` so reduced motion collapses them to 0.
`Theme.resting` is set by ShellRoot after 45 s without input.

## CMake file lists

- **QML:** every QML/JS file is listed **explicitly** in `BDTV_QML_FILES` in
  [`CMakeLists.txt`](CMakeLists.txt). A new file that is not listed is
  simply missing at runtime. QML is addressed as
  `qrc:/qt/qml/BearDen/<File>.qml`.
- **Pixel art and ornaments:** `assets/pixel/*.png` and
  `assets/ornaments/*.{png,svg}` are **globbed** (`CONFIGURE_DEPENDS`) into
  `BDTV_ASSETS`, addressed as `qrc:/qt/qml/BearDen/assets/...`. Regenerating
  art with [`tools/pixelart`](../../tools/pixelart/README.md) needs no list edit.
- **Theme packages:** `themes/**` is globbed into `:/themes/<id>/`.
- **C++:** new files go in `SOURCES`.

## Add a screen

1. Write `qml/<Name>Screen.qml`: an `Item` with `enter()`,
   `navigate(action)` returning `true` when it consumed the action, and
   optionally `report()`, `opened()` (called only on a fresh visit, not
   when a page it opened pops back: start at the top there) and `leave()`;
   it reports focus with `Nav.reportFocus(...)`.
2. List it in `BDTV_QML_FILES` in [`CMakeLists.txt`](CMakeLists.txt).
3. In [`ShellRoot.qml`](qml/ShellRoot.qml): instantiate it in the `safe`
   item with the same opacity/`y` pattern as its siblings and register it in
   the `screens` map.
4. Open it with `openScreen(name)` from another screen (a Settings `link`
   row, see below).
5. If its name is not one of the contract's `shell.screen` values, map it in
   `navScreenName()` (`remote-setup` and `onboarding` report `setup`;
   `playback` and `advanced-playback` report `diagnostics`; `apps`,
   `themes`, `weather`, `plex` and `badges` report `settings`). A name that
   should land on a section of another screen goes in ShellRoot's `aliases`.
6. `--screen <name>` in `main.cpp` then works for it too.

Back pops the stack; at Home it is a reported no-op, never an exit.

Done when: `make test-shell` passes with a test that opens it, and
`scripts/sandbox.sh shot --screen <name>` shows it (you looked).

## Settings rows

[`SettingsScreen.qml`](qml/SettingsScreen.qml) has two levels: a list of
categories (a card with a Bear Den icon each; section `settings-categories`,
items `phones`, `display`, `home`, `playback`, `power`, `about`) and, after
OK, that category's rows (section `settings`, items the row ids). Back from
a row's page returns to its row, from a category to that category, and from
the categories Home; opening Settings from Home shows the categories on the
one last used. Rows are data: `allRows` holds every row by id as
`{id, kind, label, description, value}` with `kind` one of `link`,
`choice`, `toggle`, `danger`, `info`, rendered by `SettingsRow.qml`, and
`categories` lists each category's row ids. Beside the list, `HelpPanel`
shows the focused row's help from [`Help.qml`](qml/Help.qml) (one table by
row id; engine code only looks ids up).

| Category | Rows (ids) |
|---|---|
| Phones & remote | `remote`, `pairing`, `devices`, `now-playing` (toggle, `Shell.setNowPlaying`) |
| Display & accessibility | `text`, `density`, `margin`, `motion`, `contrast` |
| Home screen | `hero`, `clock`, `weather` |
| Playback | `playback`, `advanced-playback`, `plex` |
| Power & TV | `sleep` (◀ ▶ over Off, 15 … 120 min, `Shell.setSleepTimer`), `screen-off` (`Shell.screenOff()` 0.8 s after OK, only while `display.off` is available), `cec` (`Shell.setCEC`, showing `Session.cec.reason` when no adapter is usable), `cec-volume` (only while CEC is available and on), `autostart` (Start with this PC: `Shell.setAutostart` → IPC `autostart.configure`, showing `Session.autostart`; absent from an older coordinator; says why when unavailable and sends nothing) |
| About | `version` (`info`), `diagnostics`, `setup-again` (opens the first-run setup), `exit` |

Where the rows of the old single list went (every one reachable exactly
once, `everyFormerSettingIsReachableOnceWithHelp`): `background` (Theme),
`style`, `art`, `app-icons` and `badges` are on the **Themes** page
(`ThemesScreen`; the theme is the card strip, the badges a card that opens
`BadgesScreen`); `add-apps`, `streaming` and `auto-update` are on the
**Apps** page (`AppsScreen`: the Add apps cards, the streaming site and
browser rows, Keep apps up to date); every other row is in the category
above.

1. Add the row to `allRows` and its id to a category's `rows` (the order
   there is the order on screen). Copy goes through `qsTr`.
2. `choice`: a `case` in `change(row, delta)` (◀ ▶). `toggle` or `link`: a
   `case` in `activate(row)` (OK). A `link` opens a screen with
   `openScreen(name)`.
3. Its one-line help: an entry in [`Help.qml`](qml/Help.qml) (and the same
   words in the phone's `SETTING_HELP` if the phone shows the setting).
4. Appearance settings are layout fields: edit through
   `editUi(u => u.<field> = ...)`, which sends `Shell.updateLayout`; the
   coordinator validates and persists. A new `ui` field is a contract change
   ([`contracts/AGENTS.md`](../../contracts/AGENTS.md#change-a-contract)).
5. **Update [`tests/tst_shell.cpp`](tests/tst_shell.cpp).** Tests reach a
   row with `toSettingsRow(id)`: it asks the screen which category holds the
   row (`categoryOf(id)`), goes there with `toSettingsCategory(category)`
   (Back to the categories, down to it, OK), to the category's top, then
   down until that row, so a new row shifts nothing. `openSettings()` opens
   Settings from the top bar; `openFromHeader(pill)` any pill.
   `everyFormerSettingIsReachableOnceWithHelp` walks every category, the
   Themes page and the Apps page with the D-pad and fails when a former row
   is missing or twice, or an entry has no help. A snapshot rebuilds the
   rows; the list keeps the focused row in view (`onModelChanged`).

Done when: `make test-shell` passes and
`scripts/sandbox.sh shot --screen settings` shows the row (you looked).

## First-run setup

[`OnboardingScreen.qml`](qml/OnboardingScreen.qml) opens over Home when the
first snapshot that carries `state.onboarding` says `completed: false`
(`ShellRoot.checkOnboarding`, once; an older coordinator sends none and a
later snapshot never reopens it), and from Settings → About → Run setup
again. Six steps with progress dots: Welcome, Your look (Pixel or Classic,
then the `ThemePicker` cards), Your apps (one-press cards that open the
install card; a "Streaming sites" card turns Netflix, Disney+ and Hulu on
(`Apps.isStreamingSite`) and offers their browser's install card), Phone
remote (plain-language consent, the network to allow it on through
`Shell.configureRemote`, then Pair a phone opens `PairingScreen`), Extras
(Plex, Weather, Start with this PC through `Shell.setAutostart`) and Done
(Go Home sends `Shell.completeOnboarding()` → IPC `onboarding.complete`).
Every step has Skip; Back returns to the step you came from, and on the
welcome leaves the setup (it shows again at the next start). Nothing is
installed, enabled or stored until OK on that thing. Sections report as
`onboarding-<step>`. Tests: `onboardingOnFirstRunSkipAndComplete`.
Screenshots: `scripts/sandbox.sh shot --installs --screen onboarding-<step>`.

## App tiles and branding

Supported apps are a closed set keyed by **adapter name** (`plex-htpc`,
`vacuumtube`, `moonlight`, the optional `spotify`, `jellyfin`,
`retroarch`, and the web apps `netflix`, `disney-plus`, `hulu`, `browser`;
tiles are skipped while `hidden`); engine code never
branches on a display name. `Apps.note(adapter)` is a short line a tile shows
while the app has not been opened since Bear Den started (the streaming
sites' "Up to 720p"); the web apps reuse existing rooms (cinema, theatre,
cabin, nook).
For a new app, after [`internal/AGENTS.md` → Add an app](../../internal/AGENTS.md#add-an-app):

| Where | What |
|---|---|
| [`qml/Apps.qml`](qml/Apps.qml) | `tagline`, `about`, `hint` (a how-to line on the featured panel) and `brand` (`{top, bottom, glow}`) per adapter, and `stage` (its room: draw it in `tools/pixelart/hero.py` and `tools/classicart/hero.py`); without a brand the tile falls back to the item's tint |
| [`qml/AppTile.qml`](qml/AppTile.qml) | the peeking cub's `prop` ornament per adapter (`popcorn`, `remote`, `controller`, `heart`), with its fit per ornament; the ornament is a PNG in `assets/ornaments/` (draw it in `tools/pixelart/ornaments.py`; assets are globbed into the build) |
| `ShellController::flatpakIdFor` in [`src/ShellController.cpp`](src/ShellController.cpp) | adapter → Flatpak id; for a web adapter its browser's (`Session.apps.browser` or `streaming_browser` looked up in `Session.apps.browsers`; without them the defaults, Google Chrome for the streaming sites and Brave for the Browser tile, them); used for the exported Flatpak icon and to show a shared Flatpak once in `AddAppsScreen.qml` |
| `Theme::tintFor` in [`src/Theme.cpp`](src/Theme.cpp) | a fixed hue per **application id** (`plex-htpc`, `youtube`, `moonlight`); other ids hash to a hue |
| [`qml/Apps.qml`](qml/Apps.qml) `browserOf`, `browserLabel`, `installName`, `installWhy` | the browser a web adapter runs in (its `Session.apps.browsers` entry), and what an install of the adapter fetches when it is not the app itself, and why (the web apps: their browser's label, "Browser for Netflix, Disney+, Hulu" or "The Browser tile's browser") |

Artwork: Bear Den bundles its own original app icons, never third-party
logos ([`docs/THEMES.md` → App icons](../../docs/THEMES.md#app-icons)).
`Shell.appArt(adapter, classic, icons)` returns `{icon, logo, background,
iconSource}`; the icon comes, in order, from the owner's brand folder
`~/.local/share/bear-den-tv/brand/<adapter>/` (`icon|logo|background` +
`.svg|.png|.jpg|.webp`; logo and background come only from there); with
`icons` `"app"` (`Theme.appIcons`, layout `ui.app_icons`, the default:
Themes → App icons → App's own) the icon the installed Flatpak exports
when that Flatpak is the app itself (`ownIconFlatpakIdFor`: never
Chrome's for the streaming sites, the Browser tile may show its browser's; an app that is
not installed has no export); Bear Den's own icon
(`assets/pixel/app-<adapter>.png`, or with `classic`
`assets/classic/app-<adapter>.svg`); `AppIcon` draws a monogram when there
is none. Results are cached for a minute. Every place an app is shown goes
through `AppIcon` (tiles, the featured panel and its room's screen, the
launch overlay, the install card, Add apps). A new app needs its icon
in `tools/pixelart/appicons.py` and `tools/classicart/appicons.py`
(`appArtResolutionOrder` checks every adapter has both).

## Change the look

Most look changes are not QML changes. Start in
[`docs/THEMES.md` → Make it yours](../../docs/THEMES.md#make-it-yours):

| Change | Where |
|---|---|
| Colours, decorations, particles, corner scene, wallpaper | a theme package ([`themes/AGENTS.md`](../../themes/AGENTS.md)); no QML |
| Pixel art (ornaments, bears, scenes, rooms, weather icons) | [`tools/pixelart`](../../tools/pixelart/README.md), then `make shell` |
| Classic art (Winter's classic world, the rooms, weather icons, seasonal art, worlds' classic animated layers) | [`tools/classicart`](../../tools/classicart/README.md), then `make shell`; the restored SVGs in `assets/*.svg` and `assets/ornaments/*.svg` are edited by hand |
| A component's look in one art style | branch on `World.pixel` / `World.classic`; big differences live in `<Name>Classic.qml` beside `<Name>.qml` (bears, scenes, particles). Keep the other style's path unchanged ([ADR 0006](../../docs/decisions/0006-classic-art-style.md)) |
| A new decoration style, particle kind or scene | [`docs/THEMES.md` → Extending the engine](../../docs/THEMES.md#extending-the-engine-new-building-blocks) |
| Colours and sizes of the chrome (type scale, tile sizes, `scale`) | `Theme` ([`src/Theme.h`](src/Theme.h)) |

Engine QML reads the active theme only through `World`
([`World.qml`](qml/World.qml)) and never checks a theme id.

## Tests

[`tests/tst_shell.cpp`](tests/tst_shell.cpp) loads the real `BearDen` module
offscreen from [`tests/fixtures/state.demo.json`](tests/fixtures/state.demo.json)
(offline `ShellController`, animations forced off) and drives it through
`Nav.apply`, exactly like coordinator input:

- `act("nav.right")` applies an action and processes events; `goHome()`,
  `toHeader()`, `toFavorites()`, `toFavoriteTile(id)`, `openFromHeader(pill)`,
  `openSettings()`, `toSettingsCategory(id)` and `toSettingsRow(id)` walk to
  a known start; `shellScreen()` is ShellRoot's own screen name (`Nav.screen`
  is the contract's).
- `fixture()` returns the demo snapshot; tests modify a copy and call
  `SessionModel::instance()->applySnapshot(...)`, then restore it.
- `shot(name)` saves a PNG when `BDTV_SCREENSHOT_DIR` is set.

`pairing.guest-demo.json` is a DEMO guest invitation with a real QR for the Pair screen test.
The fixture's apps, sections and device ids (`plex-htpc`, `youtube`,
`plex-continue`, `demo-1`, `dev_a1`) are asserted by name. The shell does not
run the `contracts/fixtures` files; its schema checks are in `SessionModel`.

## Render offscreen and look

The quick way ([`scripts/sandbox.sh`](../../scripts/sandbox.sh); run `make shell`
first after a QML change, the script only builds a missing binary):

```sh
scripts/sandbox.sh shot --screen settings --theme forest   # prints the PNG path
make shots                                                 # every theme × main screens
```

The same by hand:

```sh
make shell
QT_QPA_PLATFORM=offscreen build/bin/bear-den-tv-shell \
  --fixture apps/tv-shell/tests/fixtures/state.demo.json --windowed --size 1920x1080 \
  --screen settings --screenshot /tmp/shell.png --screenshot-after 2600 --exit-after 3400
```

Then open the PNG. For a theme, see
[`docs/THEMES.md` → Preview and validate](../../docs/THEMES.md#preview-and-validate).

## Checks

```sh
make test-shell     # configure build/tv-shell with tests, build, ctest offscreen
make lint           # includes qmllint (all_qmllint); configures build/tv-shell first if it is missing
make shell          # release binary into build/bin/
make perf           # frames and CPU per phase of Home, offscreen on two cores
```
