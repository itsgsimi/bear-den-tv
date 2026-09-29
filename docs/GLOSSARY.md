# Glossary

Words used across Bear Den TV, in alphabetical order. Each entry says where the
thing lives. How the pieces fit together: [`HOW_IT_WORKS.md`](HOW_IT_WORKS.md).

**Action.** A named command a phone (or the TV's own keys) sends, such as
`nav.left`, `select`, `home` or `app.launch`. Phones send only names from this
list, never keycodes or commands. Spec: [`contracts/actions.md`](../contracts/actions.md).

**Adapter.** Per-app knowledge in a table: the Flatpak id, allowed launch
arguments, how to find its window, and which key each action becomes.
[`internal/applications/adapters/adapters.go`](../internal/applications/adapters/adapters.go).

**Bears.** The default style's cast: the mascot, the bear in the featured
panel and the visiting bears (walk, peek, hop, parade, chase). Off in Plain,
Performance, reduced motion and while an app is in front.
[`BearVisitors.qml`](../apps/tv-shell/qml/BearVisitors.qml), [`THEMES.md`](THEMES.md).

**Box tiers.** `entry`, `standard` or `high`: how much the TV box's CPU can do,
worked out from cores and clock. Picks each app's playback settings.
`Classify` in [`internal/applications/tuning/tuning.go`](../internal/applications/tuning/tuning.go);
[`APP_PERFORMANCE.md`](APP_PERFORMANCE.md).

**Capability.** Whether an action is available for what is in front right now,
and through which backend, or why not. Unverified capabilities are refused.
`capabilities` in [`contracts/state.schema.json`](../contracts/state.schema.json);
built in [`internal/session/state.go`](../internal/session/state.go).

**Context epoch** (`context_epoch`). A number that goes up every time the
target changes. Actions carry the epoch the phone last saw; a stale one is
refused (`failed/stale_epoch`), except `home`, `app.launch` and `shell.restart`.
[`contracts/actions.md`](../contracts/actions.md), [`internal/session/coordinator.go`](../internal/session/coordinator.go).

**Contract.** A shared shape (JSON Schema, spec and fixtures) that the Go,
C++ and TypeScript code all validate. [`contracts/`](../contracts/README.md).

**Coordinator.** The Go process `bear-den-tv session`. It owns the session:
which app is in front, launching and closing apps, routing actions, pairing,
permissions, config and the LAN service. [`internal/`](../internal/AGENTS.md),
[`cmd/bear-den-tv/main.go`](../cmd/bear-den-tv/main.go).

**DEMO / fixtures.** Two meanings. *Contract fixtures* are example messages in
[`contracts/fixtures/`](../contracts/fixtures/) that every validator checks.
*Demo content* is fake titles and weather shown only with `--dev-fixtures`, and
always labelled DEMO ([`internal/providers/fixtures/`](../internal/providers/fixtures/),
[`DemoBadge.qml`](../apps/tv-shell/qml/DemoBadge.qml)).

**Den badges.** Playful badges ("Movie Night", "Night Owl", ...) earned from
a few local counters on the TV: TV Settings → Badges, the phone's Badges tab.
Only ids, counts and days are kept, never what was watched
([ADR 0009](decisions/0009-den-badges-local-counters.md),
[`internal/achievements`](../internal/achievements/achievements.go)).

**Dev mode.** `bear-den-tv dev` (or `make dev`): the coordinator with a fake
desktop and a loopback-only remote, for working on a workstation. Shown to
clients as `dev_mode` in the state. [`cmd/bear-den-tv/session.go`](../cmd/bear-den-tv/session.go).

**Device permissions.** What a paired phone may do: `controller` (drive the
TV) ⊂ `layout_editor` (also edit the home layout) ⊂ `owner` (everything).
Granted on the TV only. [`contracts/http.md`](../contracts/http.md).

**Guest pass.** A time-limited pairing for a visitor's phone: permission
`guest` only (remote control without closing apps, settings or power), ending
"tonight" at 04:00, after 24 hours or after 7 days, when the phone is removed
automatically. Issued on the TV or with `bear-den-tv pair --guest`.
[`contracts/http.md`](../contracts/http.md#guest-passes).

**Doctor.** `bear-den-tv doctor`: JSON diagnostics of the running coordinator.
[`internal/doctor/doctor.go`](../internal/doctor/doctor.go).

**Focus memory.** The shell remembers the focused item (by id) in each
section, so Home comes back where you left it.
[`apps/tv-shell/src/FocusMemory.h`](../apps/tv-shell/src/FocusMemory.h).

**Foreground.** The window actually in front on the X11 desktop, as seen by
the desktop adapter. Unknown foreground means actions are refused.
`target.observed` in [`contracts/state.schema.json`](../contracts/state.schema.json).

**Hold / lease.** Holding a direction on the phone. The coordinator runs a
short lease that repeats the move and stops when renewals stop, the socket
closes or the target changes. Only `nav.*` can be held.
[`internal/actions/holds.go`](../internal/actions/holds.go), [`contracts/actions.md`](../contracts/actions.md).

**Layout.** The owner's home screen setup: `ui` options (theme, style, text
and tile size, …) and the list of sections.
[`contracts/layout.schema.json`](../contracts/layout.schema.json).

**Locked.** The TV's desktop is behind its lock screen. Bear Den refuses
every action (`failed/locked`) and never bypasses the lock.
[`LockedScreen.qml`](../apps/tv-shell/qml/LockedScreen.qml).

**Onboarding.** Turning on the phone remote on the TV: choose a network
interface and consent to LAN exposure. Nothing listens on the LAN before this.
[`RemoteSetupScreen.qml`](../apps/tv-shell/qml/RemoteSetupScreen.qml),
[`security.md`](security.md).

**Ornament.** A small decoration image (a daisy, a snowflake, an ember) that a
theme names; the theme can ship its own file of the same name to restyle it.
[`Ornament.qml`](../apps/tv-shell/qml/Ornament.qml).

**Outcome.** The result of an action: `accepted` (taken on), `delivered`
(sent to the app or window), `observed` (the effect was verified) or `failed`
(with a reason code). `delivered` is never reported as `observed`.
[`contracts/actions.md`](../contracts/actions.md).

**Pairing.** Linking a phone: the TV shows a QR code and a six-digit code, the
phone claims it and gets a device session.
[`internal/pairing/pairing.go`](../internal/pairing/pairing.go),
[`PairingScreen.qml`](../apps/tv-shell/qml/PairingScreen.qml).

**Performance style.** A style with no bears, decorations or animations, the
lightest on the TV. Forces reduced motion on. `style` in
[`apps/tv-shell/src/Theme.h`](../apps/tv-shell/src/Theme.h).

**Phone remote.** The TypeScript (Preact) web app on the phone, served by the
coordinator and embedded in its binary. [`apps/remote-web/`](../apps/remote-web/AGENTS.md).

**Plain style.** Keeps the theme's wallpaper and colours but drops the
decorations and bears. [`World.qml`](../apps/tv-shell/qml/World.qml).

**Protocol.** The contract version number, today `1`. Additive changes keep
it; breaking ones bump it. [ADR 0004](decisions/0004-contract-versioning.md).

**Rail.** One horizontal row of tiles on Home, drawn from a section.
[`Rail.qml`](../apps/tv-shell/qml/Rail.qml).

**Reduced motion.** A setting that stops animations; the heartbeat stops too.
`reducedMotion` in [`apps/tv-shell/src/Theme.h`](../apps/tv-shell/src/Theme.h).

**Resting.** The shell's calm state after a while without input (45 s by
default, `BDTV_REST_SECONDS`): the heartbeat slows from 20 to 4 beats a second.
`Theme.resting`, set in [`ShellRoot.qml`](../apps/tv-shell/qml/ShellRoot.qml).

**Sandbox.** The real TV shell run offscreen on demo data to take screenshots
and measure frames, without a TV. [`scripts/sandbox.sh`](../scripts/sandbox.sh),
[`operations.md`](operations.md).

**Screensaver.** Covers the screen after a longer idle time; everything stops
drawing. `Theme.screensaver`, [`Screensaver.qml`](../apps/tv-shell/qml/Screensaver.qml).

**Section.** An entry in the layout that becomes a rail: an id, a title and a
`kind` (`applications` or a Plex row).
`sections` in [`contracts/layout.schema.json`](../contracts/layout.schema.json).

**Shell (TV shell).** The Qt 6 QML/C++ home screen `bear-den-tv-shell`:
everything on the TV between apps. It owns presentation and focus only.
The coordinator starts and supervises it. [`apps/tv-shell/`](../apps/tv-shell/AGENTS.md).

**Target.** Two meanings. (1) In the state: what the coordinator believes is
in front (`shell`, `app`, `unknown`, `locked` or `none`),
`target` in [`contracts/state.schema.json`](../contracts/state.schema.json).
(2) In scripts: your TV machine, reached over ssh (see *target.env*).

**target.env.** An untracked file at the repo root with your TV's ssh address
(`BDTV_TARGET`), read by the `*-target.sh` scripts. Copy
[`target.env.example`](../target.env.example). Never commit it.

**Theme package.** A folder with `theme.json` and art (wallpaper, ornaments),
no code. Built-ins are in [`themes/`](../themes/AGENTS.md); your own go in
`~/.local/share/bear-den-tv/themes/`. [`THEMES.md`](THEMES.md).

**Tuning.** Writing each app's playback settings (hardware decoding,
resolution, bitrate) to suit the box's tier and display.
`bear-den-tv apps detect`; [`internal/applications/tuning/`](../internal/applications/tuning/tuning.go),
[`APP_PERFORMANCE.md`](APP_PERFORMANCE.md).

**Touchpad.** The phone's pointer pad for web apps: drag moves a pointer on
the page, tap clicks, two fingers scroll (`pointer.*` actions). Only while a
web app is in front, never on a guest pass. [`touchpad.tsx`](../apps/remote-web/src/views/touchpad.tsx),
[`contracts/actions.md`](../contracts/actions.md#web-apps).

**Web app.** A website Bear Den runs as an app (Netflix, Disney+, Hulu, the
Browser tile): Flathub Chromium with its own profile, driven over the DevTools
pipe with the navigation script (`apps/web-nav`). [`internal/applications/web`](../internal/applications/web/web.go),
[ADR 0010](decisions/0010-web-apps-over-cdp-pipe.md).

**Weather.** Optional local weather from Open-Meteo, off by default: a chip by
the clock and, if chosen, rain or snow in the Home scene. Phones never see it.
[`internal/weather/`](../internal/weather/), [`WeatherScreen.qml`](../apps/tv-shell/qml/WeatherScreen.qml).

**World / World.beat.** `World` is the QML singleton for the active theme.
`World.beat` is its heartbeat: one signal that every looping decoration moves
on (20 a second awake, 4 resting, none with reduced motion or the screensaver).
[`World.qml`](../apps/tv-shell/qml/World.qml).
