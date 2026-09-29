# Glossary

Words used across Bear Den TV, in alphabetical order. Each entry says where the
thing lives. How the pieces fit together: [`HOW_IT_WORKS.md`](HOW_IT_WORKS.md).

**Action.** A named command a phone (or the TV's own keys) sends, such as
`nav.left`, `select`, `home` or `app.launch`. Phones send only names from this
list, never keycodes or commands. Spec: [`contracts/actions.md`](../contracts/actions.md).

**Adapter.** Per-app knowledge in a table: the Flatpak id, allowed launch
arguments, how to find its window, and which key each action becomes.
[`internal/applications/adapters/adapters.go`](../internal/applications/adapters/adapters.go).

**App icons (setting).** Which icon an app's tile shows: *App's own* (the
default: the icon the installed Flatpak exports, when that Flatpak is the app
itself) or *Bear Den style* (Bear Den's own drawing). The owner's brand folder
wins either way. TV Settings → App icons, the phone's Layout; layout
`ui.app_icons`. [ADR 0012](decisions/0012-app-icons-apps-own-by-default.md),
[`THEMES.md` → App icons](THEMES.md#app-icons).

**Art style.** *Pixel* (the default: everything on one pixel grid) or
*Classic* (smooth drawings and pictures), independent of the theme and the
style. TV Settings → Art style; layout `ui.art_style`.
[ADR 0005](decisions/0005-pixel-art.md), [ADR 0006](decisions/0006-classic-art-style.md).

**Badges.** See *Den badges*.

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

**CDP pipe (DevTools pipe).** How the coordinator controls Chromium:
`--remote-debugging-pipe` gives it the Chrome DevTools Protocol on two
private file descriptors that only it and that Chromium hold. No port, no
socket, nothing another program can attach to.
[`internal/applications/web/cdp.go`](../internal/applications/web/cdp.go),
[ADR 0010](decisions/0010-web-apps-over-cdp-pipe.md).

**CEC (HDMI-CEC).** A slow control bus inside the HDMI cable. With a CEC
device (`/dev/cec*`, usually a USB adapter) and the setting on, Bear Den
turns the TV on and to standby, switches it to Bear Den's input and can
press its volume keys. Off by default; tested against a fake TV only.
[`internal/platform/cec`](../internal/platform/cec/cec.go),
[ADR 0008](decisions/0008-hdmi-cec.md), [`operations.md`](operations.md#tv-control-over-hdmi-cec).

**Context epoch** (`context_epoch`). A number that goes up every time the
target changes. Actions carry the epoch the phone last saw; a stale one is
refused (`failed/stale_epoch`), except the escapes `home`, `app.launch` and
`shell.restart`, the power actions `power.sleep_timer`, `display.off` and
`tv.power`, and `app.install`/`app.install_cancel`, which never touch the
window in front (`IgnoresStaleEpoch`).
[`contracts/actions.md`](../contracts/actions.md), [`internal/session/coordinator.go`](../internal/session/coordinator.go).

**Contract.** A shared shape (JSON Schema, spec and fixtures) that the Go,
C++ and TypeScript code all validate. [`contracts/`](../contracts/README.md).

**Coordinator.** The Go process `bear-den-tv session`. It owns the session:
which app is in front, launching and closing apps, routing actions, pairing,
permissions, config and the LAN service. [`internal/`](../internal/AGENTS.md),
[`cmd/bear-den-tv/main.go`](../cmd/bear-den-tv/main.go).

**Corner scene.** The small living picture in a corner of Home that a
theme picks (the den family, a camp, the moon, a campfire); it reacts to the
local weather. [`THEMES.md` → Corner scenes](THEMES.md#corner-scenes-scene).

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
TV) ⊂ `layout_editor` (also edit the home layout) ⊂ `owner` (everything,
including app installs and the list of phones). A guest pass holds `guest`
alone, never with another. Granted on the TV only.
[`contracts/http.md`](../contracts/http.md).

**Doctor.** `bear-den-tv doctor`: JSON diagnostics of the running coordinator.
[`internal/doctor/doctor.go`](../internal/doctor/doctor.go).

**Featured panel (hero).** The big panel at the top of Home that describes
the focused tile, with the app's room behind its icon and a visiting bear.
[`HeroPanel.qml`](../apps/tv-shell/qml/HeroPanel.qml),
[`THEMES.md` → The featured panel](THEMES.md#the-featured-panel).

**Flathub.** The Flatpak app store. Bear Den installs only from it, per
user (`flatpak --user`), only apps in its adapter table, and only on an
owner's press. [ADR 0011](decisions/0011-per-user-flathub-installs.md).

**Focus memory.** The shell remembers the focused item (by id) in each
section, so Home comes back where you left it.
[`apps/tv-shell/src/FocusMemory.h`](../apps/tv-shell/src/FocusMemory.h).

**Foreground.** The window actually in front on the desktop, as seen by the
desktop adapter (X11 `_NET_ACTIVE_WINDOW`; on wlroots Wayland the activated
toplevel). Unknown foreground means actions are refused.
`target.observed` in [`contracts/state.schema.json`](../contracts/state.schema.json).

**Guest pass.** A time-limited pairing for a visitor's phone: permission
`guest` only (remote control without closing apps, settings, power, installs
or the touchpad), ending "tonight" at 04:00, after 24 hours or after 7 days,
when the phone is removed automatically. Issued on the TV or with
`bear-den-tv pair --guest tonight|24h|7d`.
[`contracts/http.md`](../contracts/http.md#guest-passes), `contract.GuestActions`.

**hide_when_missing.** A config field on an application row: while its
Flatpak is not installed, the state marks the app `hidden` and neither the
TV nor phones show its tile. The optional apps and the web apps ship with it.
[`contracts/config.md`](../contracts/config.md), [`contracts/http.md`](../contracts/http.md#optional-apps-stateapplicationshidden).

**Hold / lease.** Holding a direction on the phone. The coordinator runs a
short lease that repeats the move and stops when renewals stop, the socket
closes or the target changes. Only `nav.*` can be held.
[`internal/actions/holds.go`](../internal/actions/holds.go), [`contracts/actions.md`](../contracts/actions.md).

**Home pause.** What Home does to a playing app before bringing the home
screen forward: pause it through its own MPRIS player only when that pause
can be verified (Plex HTPC, VacuumTube, Jellyfin), the page's own pause key
for a web app, nothing for the others. `HomePause` in
[`adapters.go`](../internal/applications/adapters/adapters.go),
[`internal/session/homepause.go`](../internal/session/homepause.go).

**Install card.** The TV dialog that opens from a "Not installed" tile,
Settings → Add apps, or a streaming site turned on without Chromium: the
app, its download and disk size, Install / Not now, then progress. It says
why when installs are unavailable (for example no Flatpak).
[`InstallCard.qml`](../apps/tv-shell/qml/InstallCard.qml),
[`operations.md` → App installs](operations.md#app-installs).

**Keyring (Secret Service).** The desktop's password store (for example
gnome-keyring). Bear Den keeps the Plex token there and nowhere else; without
a usable keyring, Plex sign-in fails closed.
[`internal/secrets`](../internal/secrets/secrets.go).

**Layout.** The owner's home screen setup: `ui` options (theme, style, text
and tile size, …) and the list of sections.
[`contracts/layout.schema.json`](../contracts/layout.schema.json).

**Locked.** The TV's desktop is behind its lock screen. Bear Den refuses
every action (`failed/locked`) and never bypasses the lock.
[`LockedScreen.qml`](../apps/tv-shell/qml/LockedScreen.qml).

**MPRIS.** The D-Bus interface desktop media players expose (title,
position, play/pause). Bear Den uses it for media buttons, Now playing and
Home pause, and only for an app's own player: the one whose bus name is
owned by a process of that app's Flatpak (or a web app's own browser).
[`internal/platform/mpris`](../internal/platform/mpris/).

**Now playing.** The phone card showing what the app in front, or the app
left playing behind Home, is playing (title, progress, play/pause), read
from its MPRIS player, or for Plex HTPC from the owner's Plex server
(read-only); for controller
phones and guest passes, never while locked, off with TV Settings → Now
playing on phones. [`contracts/http.md`](../contracts/http.md#now-playing-statenow_playing).

**Onboarding.** Turning on the phone remote on the TV: choose a network
interface and consent to LAN exposure. Nothing listens on the LAN before this.
[`RemoteSetupScreen.qml`](../apps/tv-shell/qml/RemoteSetupScreen.qml),
[`security.md`](security.md).

**Optional app.** An app Bear Den supports but does not expect: Spotify,
Jellyfin Desktop and RetroArch. Its tile appears only once its Flatpak is
installed (`hide_when_missing`), and Settings → Add apps offers it.
[`internal/applications/adapters`](../internal/applications/adapters/adapters.go).

**Ornament.** A small decoration image (a daisy, a snowflake, an ember) that a
theme names; the theme can ship its own file of the same name to restyle it.
[`Ornament.qml`](../apps/tv-shell/qml/Ornament.qml).

**Outcome.** The result of an action: `accepted` (taken on), `delivered`
(sent to the app or window), `observed` (the effect was verified) or `failed`
(with a reason code). `delivered` is never reported as `observed`.
[`contracts/actions.md`](../contracts/actions.md).

**Owner phone.** A phone with the `owner` permission: it may also manage
paired phones, restart the shell and install apps (`contract.OwnerActions`).
See *Device permissions*.

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

**Plex rows.** Continue Watching and Recently Added from the owner's Plex
server on Home, after signing the TV in (Settings → Plex, `bear-den-tv plex`).
Selecting one opens Plex HTPC. [`internal/plexlink`](../internal/plexlink/plexlink.go),
[`operations.md` → Plex](operations.md#plex).

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
`kind` (`applications`, or a Plex row: `plex-continue-watching`,
`plex-recently-added`, `plex-collection`).
`sections` in [`contracts/layout.schema.json`](../contracts/layout.schema.json).

**Shell (TV shell).** The Qt 6 QML/C++ home screen `bear-den-tv-shell`:
everything on the TV between apps. It owns presentation and focus only.
The coordinator starts and supervises it. [`apps/tv-shell/`](../apps/tv-shell/AGENTS.md).

**Sleep timer.** After 15 to 120 minutes: pauses a verified player, goes
Home and turns the display off (DPMS), with a warning a minute before; any
press wakes the display. Phone Remote → Sleep, TV Settings → Sleep timer.
[`internal/session/power.go`](../internal/session/power.go),
[`operations.md`](operations.md#sleep-timer-and-screen-off).

**Streaming sites.** Netflix, Disney+ and Hulu as web apps, off by default
and turned on per site in TV Settings → Streaming sites. They play at up to
about 720p in a Linux browser. See *Web app*.

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

**Touchpad.** The phone's pointer pad for web apps: drag moves a pointer on
the page, tap clicks, two fingers scroll (`pointer.*` actions). Only while a
web app is in front, never on a guest pass. [`touchpad.tsx`](../apps/remote-web/src/views/touchpad.tsx),
[`contracts/actions.md`](../contracts/actions.md#web-apps).

**Tuning.** Writing each app's playback settings (hardware decoding,
resolution, bitrate) to suit the box's tier and display.
`bear-den-tv apps detect`; [`internal/applications/tuning/`](../internal/applications/tuning/tuning.go),
[`APP_PERFORMANCE.md`](APP_PERFORMANCE.md).

**Wayland profile.** What Bear Den does on a Wayland desktop: on wlroots
compositors it sees and activates windows (`wayland-wlr`), elsewhere it says
why it cannot (`wayland-limited`); remote keys never reach apps there.
[ADR 0007](decisions/0007-wayland-profile.md), [`internal/platform/wayland`](../internal/platform/wayland/wayland.go).

**Weather.** Optional local weather from Open-Meteo, off by default: a chip by
the clock and, if chosen, rain or snow in the Home scene. Phones never see it.
[`internal/weather/`](../internal/weather/), [`WeatherScreen.qml`](../apps/tv-shell/qml/WeatherScreen.qml).

**Web app.** A website Bear Den runs as an app (Netflix, Disney+, Hulu, the
Browser tile): Flathub Chromium with its own profile, driven over the
DevTools pipe with the navigation script (web-nav). The three streaming sites
are off until the owner turns them on (Settings → Streaming sites).
[`internal/applications/web`](../internal/applications/web/web.go),
[ADR 0010](decisions/0010-web-apps-over-cdp-pipe.md).

**Web-nav (navigation script).** The TypeScript script injected into
every page of a web app, in an isolated world the site cannot see: it moves a
focus ring between links and cards with the D-pad and reports what it did;
the coordinator performs the trusted clicks and keys it asks for, from a
closed list. [`apps/web-nav/`](../apps/web-nav/AGENTS.md).

**Widevine.** The DRM module the streaming services need for playback.
Flathub Chromium fetches it into each profile itself; after Chromium is
installed Bear Den starts each enabled site's profile once, headless, so it
can. Whether the sites then play on the TV is unverified.
[`internal/applications/web/widevine.go`](../internal/applications/web/widevine.go).

**World / World.beat.** `World` is the QML singleton for the active theme.
`World.beat` is its heartbeat: one signal that every looping decoration moves
on (20 a second awake, 4 resting, none with reduced motion or the screensaver).
[`World.qml`](../apps/tv-shell/qml/World.qml).
