# Bear Den TV implementation status

This page says what exists today, what is tested and what has been seen working on your TV.
Those are three separate claims:

- **Implemented**: the code is in the tree.
- **Automatically tested**: a named test exercises it and runs in `make test` (or `scripts/e2e-target.sh` for the live suite).
- **Live on the TV**: a commit message or doc records it working on the real TV. If not, the row says "not yet seen on the TV".

Requirement ids come from [`ACCEPTANCE_MATRIX.md`](../bear-den-tv-materials/ACCEPTANCE_MATRIX.md).
Commands are explained in [`operations.md`](operations.md).

## Current checkpoint

- Updated: 2026-09-23, branch `coordinator-shell-remote` at commit `552aefd`.
- Milestones: the control loop (M0), the shell and phone remote (M1) and the app adapters (M2) work on the TV. Customization (M3) is partly built. Packaging (M4) and the release checks (M5) are not done. See [`PLAN.md`](PLAN.md) for the original milestone list.
- Development happens on a workstation with no display. The TV is a small Linux Mint 21.3 Xfce/X11 box (the reference box, see [`AGENTS.md`](../AGENTS.md)).
- The TV's details (host, user) live in an untracked `target.env`; see [`operations.md`](operations.md).

### What runs today

- The coordinator, the TV shell and the phone remote run together on the TV, started at login by `bear-den-tv autostart` with a watchdog.
- A phone pairs with a QR code or a six-digit code, then drives the shell and the three apps: Plex HTPC, VacuumTube (YouTube) and Moonlight.
- The live end-to-end suite passed 5/5 on the TV on 2026-09-22 (commit `f087d7b`): pair, navigate, launch each app, keys, Home with focus restored, close.
- Pixel-art worlds (ADR 0005), bears, the featured panel, playback tuning and weather are built. Several of these are not yet seen on the TV; see the table.

### Screenshots

- [`docs/screenshots/readme/`](screenshots/readme/) shows the current pixel-art look (sandbox renders).
- The dated folders [`2026-09-22-tv-shell/`](screenshots/2026-09-22-tv-shell/) and [`2026-09-22-remote/`](screenshots/2026-09-22-remote/) show the **pre-pixel-art UI** (vector vines, rounded cards). They are kept as history; they no longer match what the TV draws.
- Exception: `2026-09-22-tv-shell/offline-diagnostics.png` was re-rendered on 2026-09-23 in the sandbox from the demo fixture (to drop a real LAN address), so it shows the pixel-art look. `offline-devices-confirm.png` was removed because it showed a real phone name.

### Known test gaps

Checked in the tree on 2026-09-23:

- `internal/remote` (HTTP/WebSocket server, auth, rate limits) has no tests. Only its subpackage `internal/remote/mdns` has `mdns_test.go`.
- `internal/doctor` has no tests.
- `internal/providers/plex` has no tests (it has a `testdata` folder but no `_test.go`).
- The shell supervisor's crash/restart path has no test; `internal/shellipc/supervisor_test.go` only checks the environment allow-list.
- The phone remote has unit tests only (Vitest); no browser tests.
- `make lint` reports qmllint warnings without failing.

## Requirement status

| Id | Implemented | Automatically tested | Live on the TV | Remaining |
|---|---|---|---|---|
| UI-01..04 | Yes. Shell screens in `apps/tv-shell/qml` (Home, Settings, Playback, Advanced playback, Weather, Pair a phone, Paired phones, Diagnostics, Locked, dialogs), focus graph, text/tile size, margins, high-contrast focus, reduced motion, Performance style | `apps/tv-shell/tests/tst_shell.cpp` (focus by item id, Back at root, Home closes dialogs, refresh/reorder keeps focus, locked hides everything, screensaver, large text) | Shell navigation seen in the e2e run (2026-09-22). The pixel-art look is not yet seen on the TV | Couch readability check of the pixel-art UI |
| CFG-01 | Yes. TV settings rows; phone layout editor (`apps/remote-web/src/views/editor.tsx`) | `internal/config/config_test.go`; shell `tst_shell.cpp`. The phone editor has no browser test | Not yet seen on the TV | Browser test for the editor |
| CFG-02 | Yes. Apply/undo/reset, timed confirm with rollback, revision conflicts (`internal/config`) | `TestApplyLayoutConflictPendingRollbackAndConfirm`, `TestUndoResetExportImport`, `TestHistoryKeepsTwenty` in `internal/config/config_test.go` | Not yet seen on the TV | |
| CFG-03 | Yes. Schema and semantic validation, last-known-good (LKG) recovery, atomic writes | `TestSemanticRejections`, `TestSchemaVersionAndCorruptJSON`, `TestSchemaVersionMismatchLoadsLKG`, `TestLoadInitializesAndRecovers`, `TestWriteFileAtomicLeavesNoTemp` | Not yet seen on the TV | |
| CFG-04 | Partly. Export/import of config (`TestUndoResetExportImport`); theme packages are data only with an asset allow-list (`internal/themes`) | `internal/config/config_test.go`, `internal/themes/themes_test.go` | Not yet seen on the TV | Security tests for unsafe archives/images |
| NET-01 | Yes. The LAN listener stays off until consent and an interface are chosen (`bear-den-tv remote enable --interface IF --accept-lan-exposure`, or onboarding on the TV) | `TestLoadWithMissingInterfaceKeepsConfigButBlocksRemote` (config only). `internal/remote` has no tests | Not yet seen on the TV | Socket tests for `internal/remote` |
| NET-02 | Partly. QR and code pairing work. `internal/remote/mdns` exists but nothing imports it yet, so mDNS is not published | `internal/pairing/pairing_test.go`; `internal/remote/mdns/mdns_test.go` | QR/code pairing used by the e2e run (2026-09-22) | Wire mDNS into the server |
| AUTH-01 | Yes. Invitation expiry, attempt limits, source rate limit (`internal/pairing`, `internal/remote/ratelimit.go`) | `TestClaimExpiry`, `TestWrongCodeAndAttemptsExhaustion`, `TestSourceRateLimit` in `internal/pairing/pairing_test.go` | Not yet seen on the TV as a negative test | HTTP/WebSocket negative suite |
| AUTH-02 | Yes. Per-action permissions in `internal/session` | `TestPermissionsAndRedaction` in `internal/session/session_test.go` | Not yet seen on the TV | |
| AUTH-03 | Yes. Revocation; locked session refuses and redacts | `TestRevocationKillsAuthenticate` (pairing), `TestLockedRefusesEverythingAndRedacts` (session), `internal/platform/lock/lock_test.go` | Fail-closed while locked seen on the TV (2026-09-22, [validation report](VALIDATION_REPORT.md)) | Socket-close test in `internal/remote` |
| AUTH-04 | Partly. Host rules (`internal/config`); request checks in `internal/remote` not verified in detail | `TestHostRules` (config). The server itself has no tests | Not yet seen on the TV | Security suite for `internal/remote` |
| AUTH-05 | Partly. `trusted-lan-http` and `https` transport options exist | none | Not yet seen on the TV | Transport-mode tests, real phone trust test |
| REM-01..04 | Yes. Dedup and hold leases (`internal/actions`), routing and epochs (`internal/session`), phone UI (`apps/remote-web`) | `internal/actions/actions_test.go`; `TestShellNavigationObservedAndEpochRules`, `TestHoldRepeatsIntoShell`; Vitest `apps/remote-web/tests/unit/state.spec.ts`, `hold.spec.ts` | One-step navigation (tap and short hold) in the e2e run (2026-09-22) | Two-phone test; disconnect-during-hold experiment |
| APP-01..02 | Yes. Flatpak discovery and launch, one app at a time, X11 window matching (`internal/applications`, `internal/platform/x11`) | `adapters_test.go`, `flatpak_test.go`, `TestOpeningAnotherAppClosesThePreviousOneAndNoDoubleLaunch`, `TestLaunchThenKeysGoToVerifiedApp`; `tests/e2e/target_test.go` (live only) | e2e 5/5 on the TV (2026-09-22): each app launched, verified in front and fullscreen | |
| APP-03..04 | Launch, keys and Home work | `tests/e2e/target_test.go` | Launch, keys and Home with focus restore seen in the e2e run. Browsing and playing real content from the phone is not recorded | Play real content in Plex and VacuumTube from the phone |
| APP-05 | Yes. Unknown foreground gets no input; Home escapes | `TestUnknownForegroundFailsClosedAndHomeEscapes` | Not yet seen on the TV as a negative test | |
| APP-06 | Partly. MPRIS and audio probes (`internal/platform/mpris`, `audio`); `delivered` is kept separate from `observed` | `mpris_test.go`, `audio_test.go`, `TestShellNavigationObservedAndEpochRules` | Not yet seen on the TV | Live capability probes per app |
| PLEX-01..02 | Code only. `internal/providers/plex` exists but nothing imports it; content rows come from the DEMO fixtures provider behind `--dev-fixtures` | none for Plex (`internal/providers/feed_test.go` covers the feed with fixtures) | Not yet seen on the TV | Wire the Plex provider; tests |
| REL-01 | Yes. Shell supervisor restarts a crashed shell, respects intentional exit; `Pdeathsig` stops orphaned shells; coordinator watchdog in `scripts/start-session.sh --watch` | `internal/shellipc/supervisor_test.go` covers only the environment; `cmd/bear-den-tv/autostart_test.go` | Watchdog seen on the TV: `kill -9` of the coordinator, back in about 1 s with one shell (2026-09-22) | Tests for the crash/restart path |
| REL-02 | Lock detection only | `internal/platform/lock/lock_test.go` | Lock seen on the TV; TV off/on, sleep and network changes not tried | |
| PORT-01..02 | Partly. `packaging/nfpm.yaml`, `packaging/bundle-qt.sh`; no `uninstall` subcommand | none | Not run | Clean install/remove test; a Wayland report |
| OPS-01 | Yes. `bear-den-tv doctor`, `docs/operations.md` | none (`internal/doctor` has no tests) | `doctor` is used by `scripts/deploy-target.sh` on every deploy | Tests for `internal/doctor` |
| PERF-01 | Partly. `make perf` sandbox, `scripts/measure-target.sh`, `BDTV_FPS_LOG=1` | `make perf` fails when resting or the screensaver draws over budget | Home CPU on the TV fell from 60% to about 7-11% of a core (2026-09-22, commit `6c5d9af`). The pixel-art build is not measured on the TV | Startup and action latency numbers |

Features outside the matrix:

| Feature | Implemented | Automatically tested | Live on the TV |
|---|---|---|---|
| Local weather (commits `669da70`, `bc0d2a1`) | Yes. Off by default. `internal/weather` polls Open-Meteo every 30 min with coordinates rounded to 2 decimals. Settings → Weather, `bear-den-tv weather`, a header chip and optional rain/snow/fog on Home | `internal/weather/weather_test.go`, `internal/session/weather_test.go`, `TestWeatherBlock` (config), `tests/contract/fixtures_test.go`, shell `weatherChipShowsTemperature`, `badWeatherRejectsSnapshot`, `weatherScreenSearchesAndConfigures` | The TV's coordinator fetched real geocoding and a forecast (2026-09-23, `bc0d2a1`). The chip and scene on screen are not yet seen on the TV; not measured there |
| Playback tuning and tiers | Yes. `bear-den-tv apps detect`, Settings → Playback and Advanced playback (`internal/applications/tuning`) | `tuning/detect_test.go`, `settings_test.go`, `tuning_test.go`; `TestPlaybackTuningAfterStartupAndOnExit`, `TestPlaybackSetStoresTheOverrideAndRetunes`; shell `advancedPlaybackSendsPlaybackSet` | An early `apps detect` dry run matched the TV's hardware. Tiers, automatic apply and Advanced playback not yet seen on the TV |
| Art style: Pixel / Classic (ADR 0006) | Yes. `layout.ui.art_style` (Settings → Art style, the phone's Layout); theme `classic` block; Classic twins of every pixel piece on the TV (`*Classic.qml`, `assets/*.svg`, `assets/classic/`, `tools/classicart`) and the phone (`data-art`) | `internal/themes` `TestClassicArtStyleForPhones`; shell `artStyleResolvesThemes`, `classicDrawsSmooth`, `classicComponents`, `classicArtAssets`; Vitest `art-style.spec.ts`; fixture `layout.art-style-bad.invalid.json` | Not yet seen on the TV |
| Theme packages and pixel art (ADR 0005) | Yes. `themes/`, `ThemeRegistry`, `internal/themes`, `tools/pixelart` | `internal/themes/themes_test.go`; shell `themeWallpaperSprites`, `pixelArtBuildingBlocks`, `heroPanelLife`; Vitest `vines.spec.ts` | Not yet seen on the TV |
| Autostart and desktop shortcut | Yes. `bear-den-tv autostart`, `bear-den-tv shortcut` | `cmd/bear-den-tv/autostart_test.go`, `shortcut_test.go` | Autostart and watchdog seen on the TV. The shortcut was installed but not yet double-clicked |
| Close an app from the phone | Yes | `TestCloseAppFromPhone`; e2e close step | Close seen in the e2e run (2026-09-22) |

## Blockers and permissions

- The TV screen locks when idle. Visual checks need someone to unlock it in person. Agents never bypass the lock.
- Nothing listens on the LAN until the owner gives consent on the TV or runs `bear-den-tv remote enable`.
- No system packages: the toolchain is user-space.

## Next steps

1. Look at the pixel-art UI, bears and weather on the TV, and measure it with `scripts/measure-target.sh 20`.
2. Tests for `internal/remote` (HTTP/WebSocket negative suite), `internal/doctor`, `internal/providers/plex` and the supervisor's restart path.
3. Wire mDNS and the Plex provider.
4. Browser tests for the phone remote.
5. Packaging and an `uninstall` path.

## History

Older notes, kept as written at the time (dates are 2026). They describe the UI before pixel art and may name things that have since changed.

### Session 3 decisions (2026-09-22 to 2026-09-23)

- `contracts/action.schema.json` `$defs.target` changed from `oneOf` to `anyOf`: "active"/"shell" also match the appId pattern, so `oneOf` rejected every valid request under a spec-compliant validator (Ajv). Semantics unchanged; no protocol bump.
- Target resolution: the shell window is recognized by the connected shell's pid or WM_CLASS containing `bear-den-tv-shell`; external apps by adapter WM_CLASS fragments (still UNVERIFIED on the target). External window titles never reach phones.
- Shell singletons (`Theme`, `Nav`, `Session`, `FocusMemory`, `Shell`) have private constructors: Qt 6.8 prefers a public default constructor over `static create()` for `QML_SINGLETON` (qqmlprivate.h), which silently gave QML separate instances from the C++ wiring.
- QML sizes use `N * Theme.fontUnit` / `N * Theme.scale` (notifiable properties) instead of `Theme.fs()`/`px()` invokables so text-size and resolution changes re-evaluate bindings.
- Shell IPC client unwraps the coordinator's `action_result {result: …}` envelope and reports generic `result` replies under the originating request type.
- App artwork: Bear Den bundles no brand assets. The shell resolves, in order, the owner's brand folder (`$XDG_DATA_HOME/bear-den-tv/brand/<adapter>/{logo,icon,background}.{svg,png,jpg,webp}`), the icon the installed Flatpak exports, then icons cached by the explicit `bear-den-tv artwork fetch` (Flathub only, PNG-validated, 1 MiB cap); otherwise a monogram. Official wordmarks (Plex, YouTube) go in the brand folder from the owners' brand portals; they are not committed.
- The hero backdrop uses a QPainter item instead of `MultiEffect`: the shader path rendered nothing offscreen and is untested on the Haswell target.
- Moonlight added as a third approved adapter (`moonlight` → `com.moonlight_stream.Moonlight`, no launch args, nav keys only, `leave-running` Home policy) in Go, config schema/defaults, and the shell. Additive config enum value; no protocol bump.
- `bear-den-tv session --dev-listen 127.0.0.1:PORT` serves the phone remote only on loopback (testing over an SSH tunnel); the consent-gated LAN listener stays off while it is set.
- Visual identity: new Bear Den mark (hand-built SVG of the owner-chosen "dad bear" head; `apps/tv-shell/assets/bear-mark.svg`, phone icons generated from it). Wallpapers are the owner's generated images (Den = cave mouth, Charcoal = smoke, Forest = misty pines, Midnight = moonlit lake), bundled as 2560×1440 JPEG (sources arrived at 1672×941 and were upscaled). Plex has a bundled default backdrop (projector image); owner brand-folder art still takes precedence. A top scrim keeps the header legible over bright wallpaper detail.
- Autostart (owner-authorized 2026-09-22): `bear-den-tv autostart enable` writes `~/.config/autostart/bear-den-tv.desktop`, which runs `scripts/start-session.sh --watch` at every desktop login. The watchdog restarts the coordinator after a crash (1 s → 30 s backoff) and ends on a clean stop/SIGTERM. The supervised shell gets `Pdeathsig: SIGTERM` so a crashed coordinator never leaves an orphaned full-screen shell (found live on the target: before the fix the old shell survived `kill -9` of its coordinator).
- Homey/live visual pass: shared vector ornament set (`apps/tv-shell/assets/ornaments/`, mirrored in `apps/remote-web/static/art/`), cub and sleeping-bear variants of the mark. TV: fireflies and a slow wallpaper drift (paused while an app is in front or with reduced motion), time-of-day greeting, leafy hero corners, rail sprigs, den mascot on Connecting/Locked/empty home, pairing celebration, paw loader, screen transitions, and vines that grow around the focused card's corners (Canvas/QPainter, no shaders). The lock screen now draws nothing private behind it. Remote: den backdrop, fireflies, bear status, press ripples, held-direction pulse, result feedback, vine growth on app tiles, pairing celebration; all motion gated on `prefers-reduced-motion`. Evidence: docs/screenshots/2026-09-22-tv-shell/live-homey-*, live-vines-*, docs/screenshots/2026-09-22-remote/.
- Video decode on the Haswell target: Plex (23.08) and Moonlight (24.08) runtimes ship the Haswell `i965` VA-API driver; VacuumTube's 25.08 runtime ships only `iHD` (Gen8+), and Flathub has no separate i965 extension for 25.08, so YouTube decodes in software. With owner approval VacuumTube's own `h264ify` setting was enabled (config backed up) so YouTube serves H.264 instead of VP9/AV1, which is far cheaper to decode. Haswell hardware decodes H.264 only (no HEVC/VP9/AV1).
- Deploy lessons: `make shell` installs by copy + rename (in-place `cp` onto the running binary failed with "Text file busy" and silently left an old shell), skips the test binary on deploy builds (target incremental build ~20 s instead of ~8 min), and target builds run niced; the Go coordinator is built on the workstation and copied.
- Close app from the phone (owner request 2026-09-22): the remote shows "Close <app>" for the app in front, or the one left running behind Home, and asks for a second tap within 3 s. `app.close` is now offered whenever an app is running, not only in front. A normal close asks every mapped window of the app to close and, for 4 s, also closes windows the app opens in reply, because closing Moonlight's stream window only ends the stream and brings its host list back. A relaunch inside that window cancels the follow-through. The same close path is used when opening another app. Tests: `TestCloseAppFromPhone` (fails with the follow-through disabled), `closableApp` unit test; checked in headless Chromium against `bear-den-tv dev`.
- Desktop shortcut (owner request 2026-09-22): `bear-den-tv shortcut enable|disable|status` writes a "Bear Den TV" launcher to the applications menu and, if the desktop folder exists, an executable launcher on the desktop that runs `scripts/start-session.sh --watch`. That script starts Bear Den, or restarts it if it is running. xfdesktop 4.18 prompts before running launchers unless `metadata::xfce-exe-checksum` matches the file's SHA-256, so the command sets that (and `metadata::trusted` for GNOME/Nemo) via `gio`. Installed on the reference box, where the entry passes `desktop-file-validate` and the saved checksum matches the file. Not yet double-clicked on the TV.
- Deploy script (owner request 2026-09-22): `scripts/deploy-target.sh [--now|--no-restart|--dry-run]`. It builds the Go binaries (with the phone remote) here, syncs the checkout, and installs the binaries on the target by copy + rename. It rebuilds the Qt shell on the target only when a content hash of `apps/tv-shell` changed, with `nice 19`, `ionice` idle and 2 jobs. It restarts with `start-session.sh --watch`, then fails loudly unless: the running coordinator's `/proc/<pid>/exe` hash matches the build, `doctor` reports the shell connected and running, and `BDTV_TARGET_HEALTH_CMD` (if set) passes. If `doctor` shows an app in front, it installs the binaries only: no shell build and no restart, unless `--now`.
- Design pass (owner request 2026-09-22, from photos of the TV):
  - App tiles and the app hero use each app's own colours (`Apps.brand`: Plex charcoal/amber, YouTube dark/red, Moonlight slate) around its official icon, with a faint oversized copy of the icon as texture (`BrandBackdrop`, a Canvas painted once per size change).
  - Official icons are drawn as-is instead of inside a white box.
  - The bundled Plex projector backdrop is removed; the owner's brand folder still overrides.
  - Tiles are 16:9 (368×207), with a 34 px rail gap and 1.05 focus scale.
  - The focus ring is a single ring 3 px outside the element; the breathing second ring and the twinkling sparkle are gone.
  - Vines are slimmer, with five small leaves that all point outward so nothing covers a card, and no amber bud (it read as stray dots between tiles).
  - The rail scroll area is wider so a focused tile's ring and vines are not clipped.
  - The hero drops the Flatpak/version chips (only Not installed / Running / On screen / errors remain), uses shorter copy, and shows the primary action as "[OK] Open YouTube" / "Switch to YouTube".
  - The launch overlay shows the app's brand card.
  - The mouse pointer is hidden over the fullscreen shell.
- YouTube's icon: VacuumTube exports its own icon, so the official YouTube icon (Wikimedia Commons "YouTube full-color icon (2017)", flattened to one `<svg>` because Qt SVG Tiny ignores nested `<svg>`) is installed in the owner's brand folder on the reference box (`~/.local/share/bear-den-tv/brand/vacuumtube/icon.svg`). It is not committed, consistent with the no-bundled-brand-assets rule.
- Vines and bears (owner feedback 2026-09-22: "the vines look really weak now, the little flower at the end is gone"):
  - Vines rebuilt, now on the top-right and bottom-left corners, clear of the app icon and the rail heading. The stem tapers from 4.8 to 1.6 px, the leaves are two-tone with midribs and unfurl as the stem passes (large ones outward, small ones tucked against the corner), two tendrils curl off, and a daisy (from the owner's ornament sheet) blooms at the hooked tip.
  - Two bugs found while doing this: mirrored corners used the frame's width as the pivot (inside `Scale {}` a bare `width` resolves to the document root), and the focused rail cell now lifts above its neighbours so its vines are not covered.
  - Peekaboo: 1.4 s after focus settles on an app tile, a cub peeks over its top edge with paws on the rim, holding popcorn (Plex), a remote (YouTube) or a controller (Moonlight). It blinks every few seconds and ducks when focus moves.
  - Den family: the owner's bear family (dad = the mark, mama with lashes and a daisy, the cub) peeks out of the den in Home's empty bottom-right corner, lit by a warm lantern glow. They blink, and the cub waves every 11–18 s. From 22:00 to 06:00 they sleep and "z"s drift up. Shown only while a single rail leaves room.
  - Launch overlay: a cub hops along, leaving paw prints. Screensaver: static "z Z" above the sleeping bear.
  - New assets: `bear-mama(-sleep).svg`, `bear-cub-sleep.svg`, and `ornaments/{daisy,paw-grip,popcorn,controller,remote}.svg`.
  - All motion runs only while awake, with the shell in front and without reduced motion.
- Themed worlds (owner request 2026-09-22: "take the vine thing up to 11; each background a theme for the entire TV and remote UI"):
  - Each background is a world with its own focus decoration (`CornerDecor` styles), Home-panel corners (`HeroDecor`, just inside the panel's edge), backdrop life (`Ambient`) and corner scene. There is also a Plain style.
    - Den: vines with a daisy and blossoms, fireflies, the den family.
    - Forest: fern fronds with a fiddlehead, mushrooms and berries, drifting leaves, mama and the cub in a lantern-lit tent between pines.
    - Midnight: a constellation traced star by star with a crescent moon, twinkling stars and shooting stars, and the cub asleep on a hanging moon mobile.
    - Campfire (`charcoal`): an ember trail with coals and a flame, rising sparks and embers, and dad and the cub toasting marshmallows.
    - Plain (layout `ui.theme: "plain-dark"`, a new enum value) keeps the world's wallpaper and colours and drops every decoration.
  - Settings has Theme (which also applies the world's accent) and Style rows. Paired phones mirror the world through a new optional `state.appearance` ({background, theme, accent}; not sensitive), added to the schema, Go, TS and the phone fixture. The remote recolours its accent, swaps its backdrop, turns fireflies into leaves, stars or embers, and draws the matching tile decoration.
- Vines, per owner feedback: they grow over 2.3 s, keep adding blossoms, stars or sparks while focus stays (3 s, then 6 s apart, up to six), and fade out instead of shrinking to a dot. Neighbouring tiles ease 16 px aside. While Bear Den rests, the focused decoration keeps living at ~6 fps. Everything stops for the screensaver (`Theme.screensaver`), behind apps, and with reduced motion. Canvas antialiasing is on (smooth tile corners), and the tile watermark stays clear of the rounded corners.
- Deploys: the shell is built here (ADR 0003) and shipped as a binary. A full deploy is build + copy + restart + verify, with nothing compiled on the TV.
- Visiting bears (owner request 2026-09-22: "animate them to come into frame, walk by, wave, jump on the app windows… only on Home, Settings and Pair phone"). `BearVisitors` + `BearPuppet` (a head on a body with arms and legs, posed by the director). The acts are walk-by with a wave, peek from the bottom edge, hop onto a non-focused app tile and dance, family parade, and a chase after the world's firefly, leaf, star or ember. Each world dresses the bears (daisy, beanie, nightcap, toasting stick). Visits come every 20–50 s awake, 45–105 s resting, and play at 25 fps only while a bear is on screen. They stop on any other screen, dialogs, the launch overlay, apps in front, the screensaver, reduced motion and Plain. Test knobs: `BDTV_BEARS_SECONDS`, `BDTV_BEARS_ACT`. Checked with offscreen frame sequences (walk-by, chase, parade, tile hop); not yet on the TV.
- Documentation: `docs/HOW_IT_WORKS.md` (architecture, app control, input routing, safety, performance, deploy, troubleshooting), `docs/THEMES_AND_BEARS.md` (since rewritten as `docs/THEMES.md`), and a refreshed `docs/operations.md`.
- App performance on small devices (owner request 2026-09-22):
  - `bear-den-tv apps probe|tune [--apply] [--json]` (`internal/applications/tuning`).
  - The probe asks each app's own Flatpak runtime which hardware decoders GStreamer can build: VA-API, NVDEC, V4L2 and Quick Sync. Vulkan is excluded because it registers decoders without checking the driver; software decoders never count. The probe is vendor-agnostic and per-runtime, because runtimes ship different drivers.
  - Rules: Moonlight streams a codec this box decodes in hardware, at most 1080p60 on small boxes, fullscreen with vsync and frame pacing. Plex HTPC gets a managed `mpv.conf` block (`hwdec=auto-safe`, plus `profile=fast` on small boxes). VacuumTube gets a codec filter matching hardware support, pause on blur, fullscreen, no AI-upscaled streams on small boxes, and low-memory mode at 4 GiB or less.
  - `--apply` writes only while the app is closed, backs up each file, and is idempotent.
  - Dry run on reference box: Plex and Moonlight decode H.264 in hardware, VacuumTube none (its 25.08 runtime lacks a Haswell driver). The plan is H.264 plus frame pacing for Moonlight, the mpv block for Plex, and three YouTube settings. Not applied (owner approval pending). Unit tests cover the parser, per-app rules on Haswell-like and modern capability sets, the editors (INI, mpv block, order-preserving JSON), and apply with backup.
- Theme packages, a plugin-style framework (owner request 2026-09-22: "themes should almost act like a plugin system that is very easy to design for"):
  - A theme is a folder: `theme.json` (`contracts/theme.schema.json`) plus art. The five built-ins live in `themes/<id>/`. Owner themes go in `$XDG_DATA_HOME/bear-den-tv/themes/<id>/` (or `$BDTV_THEMES_DIR`) and need no rebuild.
  - The shell loads them with `ThemeRegistry` (the QML `Themes` singleton; `World.qml` exposes the active one). The coordinator loads them with `internal/themes`, which sends phones `state.appearance` and serves theme images under `/themes/`.
  - Aliases keep old layout values working (`den-gradient` → `den`, `charcoal` → `campfire`).
  - `bear-den-tv themes list|validate <dir>|path`. Guide: `docs/THEMES.md`.
  - Tested:
    - Go (`internal/themes`): built-ins valid and ordered, aliases, appearance per theme and in Plain, the asset allow-list, invalid packages reported and skipped, and owner themes found on reload.
    - Shell: `tst_shell`.
    - Offscreen renders of all five themes and Plain, on the TV and on the phone.
  - Live: installed on the reference box (commit `1210e67`); not yet seen running there, because the restart waited for YouTube to close.
  - Fix (this session): the shell's environment allow-list dropped `BDTV_THEMES_DIR`, so the TV and phones could load themes from different folders. It is now passed through (`internal/shellipc/supervisor_test.go`).
- Playback detection test and automatic tuning (owner request 2026-09-22: "a test that determines this and applies the best settings… warn about what resolutions to expect… caveats"). This supersedes the "owner approval pending" note above; the owner asked for automatic application.
  - `bear-den-tv apps detect [--apply] [--json] [--tier T]`, and the coordinator runs the same test 20 s after startup.
  - It applies plans to closed apps and marks running ones *pending*. A pending app is tuned when it closes, with a toast.
  - Opt out with `startup.tune_apps: false` (new optional config field).
  - Results go to `state.playback` and the new **Settings → Playback** screen: per-app status, GPU codecs, expectations, caveats and changes.
- Box tiers (owner feedback 2026-09-22: "even small modern devices can handle a lot, this little celeron might be worst case scenario"):
  - "Small" (≤ 4 cores) is replaced by `entry` / `standard` / `high`, computed from logical CPUs and the sum of their top clocks.
  - The Celeron 2955U is `entry`, the floor. N100-class boxes are `standard` and keep each app's full-quality defaults. Desktops are `high` and also decode 4K YouTube in software when the GPU can't.
  - Moonlight's frame rate is now the owner's choice, capped only at the TV output's refresh. On 120 Hz TVs the report suggests 120 fps. Resolution is capped at the output mode, and at 1080p on entry boxes or with H.264-only decoding.
  - The 120 Hz caveat is now an info note, shown on entry boxes only. Capable boxes on a 4K ≥ 50 Hz TV are told they can drive 4K.
  - Override with `BDTV_TIER`.
  - Tested: unit tests for `Classify` (reference and published-spec examples), clock reading (cpufreq and cpuinfo), the override, and each tier's plans, expectations and notes. Each new rule was broken on purpose to check that a test fails.
  - Live: `apps detect` ran on the reference box with the earlier rules and matched the hardware. The tier rules and automatic application have not run there yet.
- Settings → Advanced playback (owner request: every setting Bear Den changes adjustable by hand, only choices the box can handle, surviving restarts):
  - Implemented: a per-app setting catalog in `internal/applications/tuning/settings.go` (Moonlight codec/frame rate/resolution/frame pacing, YouTube codecs/AI upscaling/pause behind Home/low-memory, Plex scaling/hardware decoding), options filtered by hardware decoders, tier and display. Plans take overrides and fall back to Auto with a note when one is no longer offered. New optional `config.json` field `playback.overrides` (additive, schema_version and protocol stay 1), `state.playback.apps[].settings`, shell IPC `playback.set` (validated against the offered options, fails closed with a reason), and the TV screen `AdvancedPlaybackScreen.qml`. `apps detect` honours the stored choices too.
  - Moonlight's automatic resolution and frame rate are now always written (not only lowered), so returning a row to Auto takes effect; YouTube switches go back off after an override is removed.
  - With `startup.tune_apps: false` choices are stored and shown but not written.
  - Automatically tested: Go (`tuning/settings_test.go`: catalog on the reference box and modern boxes, overrides honoured/refused, Retune applies or leaves files alone; `config`: overrides parse, schema rejections, deep copy, persistence across a store reload, pruning; `session`: `playback.set` stores, re-plans, refuses unoffered values/settings/apps, returns to Auto, respects tune_apps=false), shell (`tst_shell` opens the screen from the fixture and checks the `playback.set` frames), TS contract test on the fixture and config schema. Each new rule was broken on purpose to check that a test fails.
  - Not live-validated: not deployed to the reference box.
- Performance sandbox and one heartbeat (owner request 2026-09-22: "setup a sandbox here to also test and measure its performance"):
  - `make perf` (`scripts/perf-sandbox.sh`) runs the real shell here offscreen on the demo fixture, pinned to two cores under a CPU quota. It reports frames per second and CPU for awake, resting and screensaver, and fails when resting (> 12 fps) or the screensaver (> 1 fps) draws over budget. `BDTV_REST_SECONDS` shortens the 45 s rest for it.
  - What it found: the wallpaper drift and the particles were free-running animations that drew the whole screen at the display rate while awake, and would do so at 120 Hz on the TV. The focused decoration kept ticking in Plain, where it is invisible. The screensaver bear glided for 2.2 s every 15 s. Each decoration's own timer triggered its own frames.
  - Fix: `World.beat(dt)` is one heartbeat (20 beats/s awake, 4 resting, full speed while a bear visits). Particles, the focus and panel decorations, the wallpaper drift and bear visits all move on it. The screensaver bear jumps instead of gliding. The decoration stops in Plain.
  - Sandbox, before → after: awake 81–89 → 49 fps; resting 30 → 10.7; screensaver 9 → 0.1; Plain resting 12 → 0.
  - Live on the reference box (2026-09-22, Den theme, Home, `scripts/measure-target.sh`): the shell used 60% of one core awake before; after, 6.8% awake (15 s) and 11.2% resting (20 s; a bear visit may have fallen in the window).
  - Not yet: corner scenes (camp, moon, campfire) and the den mascot still use looping animations while awake.
- Bears with bodies, focus glint, Performance style (owner requests 2026-09-23):
  - Campfire scene: dad and the cub are whole bears (BearPuppet gained `sitting`, `reach`, `asleep`, `stickLength`) sitting on logs and toasting marshmallows in the flame. Moon scene: the cub sleeps curled in the crescent, in the theme's nightcap. Den and tent keep peeking heads (those bears are inside). Both scenes now move on the heartbeat.
  - Focus ring glint on app tiles and content cards (`FocusFrame.glint`): a spark in the theme's bloom colour laps the ring when focus lands, then drifts slowly; stops while resting; none in Plain/Performance.
  - Performance style (`layout.ui.theme: "performance"`, Settings → Style): Plain plus reduced motion forced on. No bear art apart from the logo, nothing animates on the TV or the phone. Contract enum widened (ADR 0004); Go test for phone appearance. Sandbox: 0 fps awake and resting.
  - Checked with sandbox screenshots and `make perf` (campfire awake 41 fps, resting 8); not yet seen on the TV.
- Pixel art everywhere (owner request 2026-09-23, after the Sprite Fusion pixel-scene article; ADR 0005):
  - Implemented: the pixel grid `World.px`; pixel worlds for all five built-ins (480×270 backdrops with animated sprite layers: Campfire rainy night with a far tent fire, smoke and puddle ripples; Den cave mouth at dusk with hanging vines, lanterns and mist; Forest misty dawn pines with light shafts and a glinting creek; Midnight moonlit lake with the Milky Way and a shimmering moon path; Winter snowy cabin under an aurora); pixel bears (heads, 12 poses with blinks, the 16×16 mark) driven through the existing puppet API; 25 pixel ornaments; pixel corner scenes (now 440×330); the corner engine, brand backdrops and particles painted on the grid; `PixelBox` replacing rounded rectangles (tiles, panels, pills, rows, dialogs, focus ring and its glint); the phone remote in pixel art (backdrop, bears, vines on a grid, square particles and chrome, PNG PWA icons).
  - Contract: `wallpaper.pixel`, `wallpaper.sprites` (theme schema, Go, C++), `state.appearance.pixel` (schema, fixture, Go, TS); built-in ornaments may be PNG (embed, Go, C++). Additive, protocol 1.
  - Art source: `tools/pixelart` (Python stdlib), `python3 tools/pixelart/build.py [--preview]`.
  - Automatically tested: Go `internal/themes` (pixel wallpaper and sprites load, missing sheet is a problem, PNG built-in ornaments resolve and are served, every built-in world is pixel art); shell `themeWallpaperSprites` and `pixelArtBuildingBlocks` (PixelBox stairs, bear pose selection incl. negative walk phases, PNG ornaments draw as pixel art); phone `vines.spec.ts`. Each new rule was broken on purpose and a test failed.
  - Checked with sandbox screenshots of every world on Home (one rail, scenes showing), Settings, Pair phone, and phone screenshots (pair, campfire, midnight). `make perf` on the sandbox: campfire awake 41 fps / 21% CPU (software rendering of the full-screen rain), resting 8.1 fps / 1%, screensaver 0.1 fps; den awake 9% CPU.
  - Not yet seen on the TV; not deployed. Not measured on the TV.
  - Known gaps: the phone's pair screen shows the den mound at a larger pixel size than the cub; the phone celebration burst still rotates its pieces for about a second.
- Featured panel life (owner request 2026-09-23, "implement most of these"):
  - Implemented: per-app pixel rooms behind the icon (Plex cinema, YouTube cabin TV with intro static and a time-of-day window, Moonlight arcade; `Apps.stage()`, `tools/pixelart/hero.py`, `HeroScene.qml`); parallax of up to two art pixels with the rail position; typed title with a block cursor and row-by-row reveal; a bear that walks onto the panel and reacts to the app; a dozing bear while resting that stretches on wake; the cub peeking over the OK button after 7 s; pumpkins in October and snow on the panel in December (`BDTV_MONTH` override); the remote's secret code starts the bear parade (its final OK is consumed, found by the test: otherwise it launched the focused app and cancelled the parade).
  - Automatically tested: `heroPanelLife` (every adapter's room exists in the generated rig, the cabin intro plays once, the secret code starts a parade), both rules broken on purpose to see the test fail.
  - Checked with sandbox screenshots: cinema with the popcorn cub, cabin with dad waving the remote and December snow, arcade with the controller cub and October pumpkins, the dozing bear on Den. `make perf`: awake 42 fps (campfire 22% / den 9% of a core), resting 8.5 fps / 1%, screensaver 0.3–0.6 fps (the same 0.3 with the panel features switched off; budget 1).
  - Not done: weather in the scene (needs a decision on outbound internet and a location setting), achievements (needs stored data), the Continue Watching path and pixelated posters (wait for live Plex rows).
  - Not yet seen on the TV; not deployed.
- Art style: Pixel / Classic (owner request 2026-09-23; ADR 0006): Classic has to match everything pixel art does.
  - Step 1, the setting: `layout.ui.art_style` (optional, missing means pixel) and `state.appearance.art_style` (schema, fixtures, Go, C++, TS); a theme's optional `classic` block (wallpaper image and sprites, phone backdrop); ornaments resolve PNG first in Pixel and SVG first in Classic; Settings → Art style on the TV and a picker in the phone's Layout editor; `scripts/sandbox.sh shot --classic`.
  - Automatically tested: Go `TestClassicArtStyleForPhones`; shell `artStyleResolvesThemes`; the invalid-value fixture. Each rule broken on purpose; a test failed.
  - Step 2: `PixelBox` draws a smooth rounded `Rectangle` in Classic; the four original worlds' pictures and the original SVG ornaments, bears and mark restored beside the pixel art (test `classicDrawsSmooth`).
  - Step 3: Classic bears (`BearPuppetClassic.qml`, the jointed SVG rig with the pixel puppet's properties), heads and mark; `*Classic.qml` corner scenes at the pixel scenes' size; the corner engine, focus ring and spark, brand backdrop and particles (`AmbientClassic.qml`, incl. soft rain) drawn smooth (test `classicComponents`).
  - Step 4: `tools/classicart` draws Winter's classic world and phone backdrop, the featured panel's rooms (cinema, cabin by time of day with intro static, arcade) with glow layers, 10 weather icons, the pumpkin, snowcap and sleep "z"; HeroScene/HeroPanel/HeroLife/Header/WeatherScreen use them in Classic (test `classicArtAssets`). The wallpaper plays `classic.wallpaper.sprites`.
  - Phone: `data-art` switches rounded chrome, SVG art, round particles, smooth animations and curved vines (`ClassicCorner`); Pixel output byte-identical to before, Classic identical to the pre-pixel look (Vitest `art-style.spec.ts`).
  - Checked with sandbox screenshots of every world in both styles and phone screenshots; not yet seen on the TV; not measured there.

### Early commands run (2026-09-22)

| Command | Environment | Result | Evidence |
|---|---|---|---|
| `python3 tools/validate_materials.py` | workstation | PASS (materials package only) | stdout |
| `scripts/bootstrap-toolchain.sh` | workstation + target | PASS: go1.24.13, cmake 4.4.3, Qt 6.8.4 | `/tmp/bdtv-bootstrap*.log` |
| `QT_QPA_PLATFORM=offscreen qml6 smoke.qml` | workstation | PASS (PNG rendered) | /tmp/qtsmoke/out.png |
| `QT_QPA_PLATFORM=xcb qml6 smoke.qml` on `:0` | target | Window created, NOT mapped by xfwm4 (seat on greeter); GLX deadlock, `xcb_egl` initialises | `tests/compatibility/2026-09-22-reference-box-target.json` |
| `go vet ./...` | workstation | PASS (all packages) | — |
| `go test -count=1 ./...` | workstation | PASS (all suites; remote/shellipc have no tests yet) | — |
| `go test -race -count=1 ./...` (+ `-count=5` session/actions) | workstation | PASS, incl. new internal/actions and internal/session suites | — |
| `npm run lint && npm run test:unit && npm run build` (apps/remote-web) | workstation | PASS: 38 tests (fixtures vs schemas with Ajv, reducer, hold leases); dist built | — |
| `bear-den-tv dev --no-shell --dev-fixtures --dev-listen 127.0.0.1:18787` + curl pair/claim/actions | workstation | PASS (manual smoke; see Running behavior) | — |
| `cmake --build build/tv-shell && ctest` (Debug, `QT_QPA_PLATFORM=offscreen`) | workstation | PASS: tst_shell 13/13 | docs/screenshots/2026-09-22-tv-shell/offline-*.png |
| `bear-den-tv artwork fetch` | workstation | PASS: cached the Flathub icons of Plex HTPC and VacuumTube in ~/.cache/bear-den-tv/brand | stdout |
| `bear-den-tv dev --no-shell --dev-fixtures` + shell + phone nav (rich tiles) | workstation | PASS: content rails from the coordinator feed with artwork; hero backdrop; official icons on app tiles | docs/screenshots/2026-09-22-tv-shell/live-rich-*.png |
| `bear-den-tv dev --no-shell` + `bear-den-tv-shell --dev` (offscreen) + scripted phone HTTP | workstation | PASS: pair, nav/select/back observed with focus detail, TV Pair screen shows live QR/code, TV-initiated launch → app target, phone home → shell + focus restored | docs/screenshots/2026-09-22-tv-shell/live-*.png |
| `QT_QPA_PLATFORM=offscreen bear-den-tv dev --shell-binary build/tv-shell/bear-den-tv-shell` | workstation | PASS: supervisor starts shell, "session: shell connected" | stderr |
| `DISPLAY=:0 XAUTHORITY=~/.Xauthority bdtv-probe` (built on target) | target `:0` | PASS: x11-ewmh-xtest activate/observe/input AVAILABLE; EWMH + XTEST, all keys resolved; windows map (`xfdesktop`, panel); lock=locked (light-locker idle); no HDMI sink; Plex/VacuumTube not installed | `tests/compatibility/2026-09-22-reference-box-live-probe.json` |
