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
| `Session` (singleton) | [`SessionModel`](src/SessionModel.h) | the latest state snapshot (incl. `weather`, `power` and `plex`, empty when absent), validated by `validateSnapshot`/`validateLayout` then applied whole (rejected snapshots keep the previous state); `application(id)`, `layoutForEdit()`, rails as `sections` |
| `Nav` (singleton) | [`Navigator`](src/Navigator.h) | the one input path: key events and coordinator `input` become named actions; `apply(action)` returns the `input_result`; `reportFocus(section, item, scrollX)`, `noteAtRoot()`, `screen`, `textFieldFocused`; `focusReported` carries `text_field` and is re-emitted when only that changes |
| `Shell` (singleton) | [`ShellController`](src/ShellController.h) | the IPC bridge. Methods: `launchApp`, `closeApp`, `issuePairing(pass)` (`""` a family phone, `tonight`/`24h`/`7d` a guest pass), `cancelPairing`, `revokeDevice`, `configureRemote(enabled, interface)`, `updateLayout(layout)`, `setPlayback(adapter, setting, value)`, `setNowPlaying(enabled)` (IPC `remote.now_playing`), `weatherSearch(query)` (answer in `weatherPlaces`/`weatherSearchOk`/`weatherSearchError`/`weatherSearching`), `weatherConfigure(enabled, place, units, scene)` (place `null` keeps the stored one; answer in `weatherConfigured(ok, error)`), `setSleepTimer(minutes)` (0 cancels) and `screenOff()` (the `power.sleep_timer` and `display.off` actions), `powerActivity()` (IPC `power.activity`), `plexSignIn`, `plexCancel`, `plexChooseServer(id)`, `plexChooseLibraries(ids)`, `plexSignOut` (IPC `plex.*`; answer in `plexReplied(type, ok, error)`, the flow itself in `Session.plex`), `answerConfirm`, `exitShell`, `flatpakIdFor`, `appArt`, `lanInterfaces`. Properties: connection state (`connectionState`, `connected`, `rejectReason`, `attempt`), `offline`, `devBuild`, `version`, `startScreen`, `launchingAppId`, `screensaverSeconds`, and the check overrides `bearsSeconds`, `bearsAct`, `restSeconds`, `monthOverride`, `lightningSeconds` |
| `Theme` (singleton) | [`Theme`](src/Theme.h) | design tokens from `layout.ui` and window size: colours, `scale`, type and tile sizes, `ms()`/`duration`, `reducedMotion`, `resting`, `screensaver`, `tintFor(id)` |
| `Themes` (singleton) | [`ThemeRegistry`](src/ThemeRegistry.h) | installed theme packages: `list`, `get(id)`, `canonical(id)`, `ornament()`, `reload()` ([`themes/AGENTS.md`](../../themes/AGENTS.md)) |
| `FocusMemory` (singleton) | [`FocusMemory`](src/FocusMemory.h) | remembered item, index and scroll per section; `lastSectionId` |
| `QrCode` | [`QrRenderer`](src/QrRenderer.h) | paints `pairing.qr_modules`; the coordinator encodes |
| `RoundedImage` | [`RoundedImage`](src/RoundedImage.h) | artwork backdrop without shaders (local files and qrc only); `pixelSize` > 1 composes it in whole art pixels, once per source and size (the hero in the Pixel art style) |
| (uncreatable) | `SectionsModel`, `ItemsModel`, `IpcClient` | id-preserving rail models; the socket client (offline mode records sent messages) |

QML singletons: [`Apps.qml`](qml/Apps.qml) (per-adapter copy and brand
colours) and [`World.qml`](qml/World.qml) (the active theme as components see
it), marked `QT_QML_SINGLETON_TYPE` in CMake.

## main.cpp flags

[`src/main.cpp`](src/main.cpp): `--dev` (honours `BDTV_SHELL_SOCKET`),
`--fixture PATH` (offline: render a state snapshot, no coordinator),
`--screen home|settings|pairing|devices|diagnostics|remote-setup|playback|advanced-playback|weather|plex` (any screen ShellRoot registers), `--windowed`,
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
| Screens | `HomeScreen`, `SettingsScreen`, `RemoteSetupScreen`, `PairingScreen` ("Who is it for?": family phone or guest pass), `DevicesScreen` (guests with a badge and the time left), `DiagnosticsScreen`, `PlaybackScreen`, `AdvancedPlaybackScreen`, `WeatherScreen` (Settings → Weather: toggles, search field, places; reports `settings`), `PlexScreen` (Settings → Plex: sign in with a code and QR, choose a server and libraries, sign out; follows `Session.plex.status`; leaving mid-flow sends `plex.cancel`; reports `settings`), `ConnectingScreen`, `LockedScreen`, `Screensaver` |
| Dialogs and overlays | `ConfirmDialog`, `MessageDialog`, `AppUnavailableDialog`, `LaunchOverlay`, `ErrorBanner`, `Toast`, `SleepWarning` (the sleep timer's last minute: "Going to sleep in 1 minute", the cub dozing; while it shows, or while `Session.power.display` is `off`, `ShellRoot.handle` swallows keys and calls `Shell.powerActivity()`) |
| Home pieces | `Header` (brand, pills, status, weather chip, clock), `NavPill`, `StatusChip`, `HeroPanel`, `Rail`, `AppTile`, `AppIcon`, `ContentCard`, `SetupCard`, `BrandBackdrop`, `DemoBadge`, `ProgressBar`, `FocusFrame` |
| Screen pieces | `ScreenFrame`, `SettingsRow`, `FocusButton`, `KeyHints` |
| Theme engine | `World` (incl. the pixel grid `World.px` and the weather override `World.weather*`), `Wallpaper`, `WeatherSky` (weather veil + lightning, sets `World.flashing`), `SceneWeather` (a corner scene's reactions to the weather, declared as data per look), `Ambient`, `Ornament`, `CornerDecor`, `FocusDecor`, `HeroDecor`, `CampScene`, `MoonScene`, `CampfireScene` |
| Featured panel | `HeroPanel` (typing, row reveal, cub behind OK, seasons), `HeroScene` (the app's room, generated `HeroRig.js`), `HeroLife` (visiting and dozing bears); per-app data in `Apps.stage()` |
| Pixel art | `PixelBox` (rounded-`Rectangle` stand-in with stair corners), `PixelSprite` (a sprite/frame from `assets/pixel/`); art generated by [`tools/pixelart`](../../tools/pixelart/) ([`docs/THEMES.md` → Pixel art](../../docs/THEMES.md#pixel-art)) |
| Bears | `BearMark`, `BearHead`, `BearPuppet` (+ generated `BearRig.js`), `BearVisitors`, `DenMascot`, `DenFamily` |

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
   optionally `report()`; it reports focus with `Nav.reportFocus(...)`.
2. List it in `BDTV_QML_FILES` in [`CMakeLists.txt`](CMakeLists.txt).
3. In [`ShellRoot.qml`](qml/ShellRoot.qml): instantiate it in the `safe`
   item with the same opacity/`y` pattern as its siblings and register it in
   the `screens` map.
4. Open it with `openScreen(name)` from another screen (a Settings `link`
   row, see below).
5. If its name is not one of the contract's `shell.screen` values, map it in
   `navScreenName()` (`remote-setup` reports `setup`; `playback` and
   `advanced-playback` report `diagnostics`; `weather` reports `settings`).
6. `--screen <name>` in `main.cpp` then works for it too.

Back pops the stack; at Home it is a reported no-op, never an exit.

Done when: `make test-shell` passes with a test that opens it, and
`scripts/sandbox.sh shot --screen <name>` shows it (you looked).

## Settings rows

[`SettingsScreen.qml`](qml/SettingsScreen.qml) is data-driven: `rows` is an
array of `{id, kind, label, description, value}` with `kind` one of `link`,
`choice`, `toggle`, `danger`, rendered by `SettingsRow.qml`.

1. Add the row to `rows` in the place it should appear. Copy goes through
   `qsTr`.
2. `choice`: a `case` in `change(row, delta)` (◀ ▶). `toggle` or `link`: a
   `case` in `activate(row)` (OK). A `link` opens a screen with
   `openScreen(name)`.
3. Appearance settings are layout fields: edit through
   `editUi(u => u.<field> = ...)`, which sends `Shell.updateLayout`; the
   coordinator validates and persists. A new `ui` field is a contract change
   ([`contracts/AGENTS.md`](../../contracts/AGENTS.md#change-a-contract)).
4. **Update [`tests/tst_shell.cpp`](tests/tst_shell.cpp).** The rows today,
   from the top: `remote`, `pairing`, `devices`, `now-playing` (a toggle
   that sends `remote.now_playing` through `Shell.setNowPlaying`, showing
   `Session.remote.now_playing`), `text`, `density`, `background` (Theme),
   `style`, `art`, `margin`, `motion`, `contrast`, `hero`, `clock`,
   `weather`, `playback`, `advanced-playback`, `diagnostics`, `sleep` (◀ ▶
   over Off, 15 … 120 min through `Shell.setSleepTimer`, showing
   `Session.power.sleep_minutes`), `screen-off` (`Shell.screenOff()` 0.8 s
   after OK, so the key's release does not wake the display; only while
   `display.off` is available), `plex` (opens Settings → Plex), `exit`. A snapshot rebuilds `rows`; the
   list keeps the focused row in view (`onModelChanged`).
   `sleepRowSetsTheTimerAndScreenOff` walks 18 rows down to `sleep`.
   `headerPillsOpenScreensAndBackReturns` walks 17 rows down from `remote`
   to reach `diagnostics` (and 2 to `devices`),
   `advancedPlaybackSendsPlaybackSet` 16 to `advanced-playback`,
   `weatherScreenSearchesAndConfigures` 14 to `weather`, and
   `nowPlayingRowTogglesTheSetting` 3 to `now-playing`, and
   `plexScreenDrivesSignIn` goes to the bottom and one up to `plex` (it sits
   just above `exit`). A row inserted above any of them shifts those counts.

Done when: `make test-shell` passes and
`scripts/sandbox.sh shot --screen settings` shows the row (you looked).

## App tiles and branding

Supported apps are a closed set keyed by **adapter name** (`plex-htpc`,
`vacuumtube`, `moonlight`, and the optional `spotify`, `jellyfin`,
`retroarch`, whose tiles are skipped while `hidden`); engine code never
branches on a display name.
For a new app, after [`internal/AGENTS.md` → Add an app](../../internal/AGENTS.md#add-an-app):

| Where | What |
|---|---|
| [`qml/Apps.qml`](qml/Apps.qml) | `tagline`, `about`, `hint` (a how-to line on the featured panel) and `brand` (`{top, bottom, glow}`) per adapter, and `stage` (its room: draw it in `tools/pixelart/hero.py` and `tools/classicart/hero.py`); without a brand the tile falls back to the item's tint |
| [`qml/AppTile.qml`](qml/AppTile.qml) | the peeking cub's `prop` ornament per adapter (`popcorn`, `remote`, `controller`, `heart`), with its fit per ornament; the ornament is a PNG in `assets/ornaments/` (draw it in `tools/pixelart/ornaments.py`; assets are globbed into the build) |
| `ShellController::flatpakIdFor` in [`src/ShellController.cpp`](src/ShellController.cpp) | adapter → Flatpak id; used for the exported Flatpak icon and the install hint in `AppUnavailableDialog.qml` |
| `Theme::tintFor` in [`src/Theme.cpp`](src/Theme.cpp) | a fixed hue per **application id** (`plex-htpc`, `youtube`, `moonlight`); other ids hash to a hue |

Artwork: Bear Den bundles its own original app icons, never third-party
logos ([`docs/THEMES.md` → App icons](../../docs/THEMES.md#app-icons)).
`Shell.appArt(adapter, classic)` returns `{icon, logo, background,
iconSource}`; the icon comes, in order, from the owner's brand folder
`~/.local/share/bear-den-tv/brand/<adapter>/` (`icon|logo|background` +
`.svg|.png|.jpg|.webp`; logo and background come only from there), Bear
Den's own icon (`assets/pixel/app-<adapter>.png`, or with `classic`
`assets/classic/app-<adapter>.svg`), the icon the installed Flatpak exports,
then icons cached by `bear-den-tv artwork fetch`; `AppIcon` draws a monogram
when there is none. Results are cached for a minute. A new app needs its icon
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
  `toHeader()`, `toFavorites()` walk to a known start.
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
make lint           # includes qmllint (all_qmllint) once build/tv-shell exists
make shell          # release binary into build/bin/
make perf           # frames and CPU per phase of Home, offscreen on two cores
```
