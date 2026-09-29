# How Bear Den TV works

Bear Den TV turns a small Linux box into a TV: a home screen you drive with a
phone, which opens Plex, YouTube (VacuumTube), Moonlight and, once installed,
Spotify, Jellyfin Desktop and RetroArch, plus Netflix, Disney+, Hulu and a
Browser in Chromium, and always brings you back with Home. It does this
**without wrapping, embedding or modifying the apps**. They are ordinary
Flatpak apps; Bear Den decides which one is on screen, makes it fullscreen,
and routes the remote's buttons to whatever is in front.

This page explains the moving parts. The contracts are in
[`contracts/`](../contracts/README.md); decisions are in
[`docs/decisions/`](decisions/). To change the code, start at
[`AGENTS.md`](../AGENTS.md).

## The pieces

```
             phone (browser)                         TV box (one desktop session)
   ┌───────────────────────────┐         ┌──────────────────────────────────────────────┐
   │ phone remote (TypeScript) │  LAN    │ bear-den-tv session  (Go "coordinator")       │
   │ named actions, state      │◀──────▶│  pairing · permissions · config · app control │
   │ over HTTP + WebSocket     │         │  X11 window control · media · audio           │
   └───────────────────────────┘         │        │ Unix socket (newline JSON)           │
                                         │        ▼                                      │
                                         │ bear-den-tv-shell  (Qt 6 / QML home screen)   │
                                         │                                               │
                                         │ apps (Flatpak) · Chromium for web apps        │
                                         └──────────────────────────────────────────────┘
```

| Part | Where | Owns |
|---|---|---|
| **Coordinator** | `cmd/bear-den-tv`, `internal/` (Go) | the session: which app is in front, launching and closing apps, routing input, pairing and permissions, configuration, the LAN service |
| **Home screen ("shell")** | `apps/tv-shell` (Qt 6 QML/C++) | everything you see on the TV between apps: tiles, focus, settings, pairing, themes, bears |
| **Phone remote** | `apps/remote-web` (TypeScript, Preact) | the remote UI on the phone; served by the coordinator (embedded in its binary) |
| **Navigation script** | `apps/web-nav` (TypeScript) | D-pad control of web pages, injected into Chromium by the coordinator (embedded in its binary) |
| **Contracts** | `contracts/` | the shapes the three talk in (actions, state, IPC, config, layout, theme), validated in Go, C++ and TypeScript |
| **Themes** | `themes/` (built in), `~/.local/share/bear-den-tv/themes/` (yours) | theme packages: a `theme.json` plus art, read by both the shell and the coordinator |

## Starting up

1. At desktop login, `~/.config/autostart/bear-den-tv.desktop` runs
   `scripts/start-session.sh --watch` (installed by `bear-den-tv autostart
   enable`). The **Bear Den TV** desktop icon (`bear-den-tv shortcut enable`)
   runs the same script, which starts Bear Den or restarts it.
2. The script starts the coordinator detached from any terminal. The watchdog
   restarts it if it crashes, with a back-off from 1 s up to 30 s. A clean stop or
   logout ends the watch.
3. The coordinator starts the home screen as a child process and **supervises**
   it: restarts after a crash (with a circuit breaker), never after an
   intentional exit (Settings → Exit Bear Den TV). The shell dies with its
   coordinator (`Pdeathsig`), so a crashed coordinator never leaves a stray
   fullscreen window.
4. The shell connects over the Unix socket, receives the state snapshot, and
   draws the home screen fullscreen with the mouse pointer hidden.

## Opening an app

When you pick a tile (on the TV or the phone):

1. **One launch at a time.** A second press while the app is still starting
   waits for that launch instead of starting another copy.
2. **One app at a time.** Other apps are asked to close normally (a regular
   window close, so they can save their state). If an app answers by opening
   another window, as Moonlight does when its stream window closes, that window
   is closed too for a few seconds afterwards.
3. **Reuse before relaunch.** If the app already has a window, it is simply
   brought to the front.
4. Otherwise it is started with `flatpak run <app id>`, and the coordinator
   waits (up to 30 s) until the app's window is **actually in front**. Windows
   are recognised by their X11 `WM_CLASS`: a lower-case substring match on
   `plex`, `vacuumtube`, `moonlight`, `spotify`, `jellyfin` or `retroarch`,
   and for web apps the class Bear Den gives Chromium (`BearDenWeb-<adapter>`)
   ([`adapters.go`](../internal/applications/adapters/adapters.go)). On
   Wayland the window's `app_id` plays that part (the Flatpak id, or the same
   substrings); see [Wayland](#wayland) below.
5. The window is switched to **fullscreen** (EWMH `_NET_WM_STATE_FULLSCREEN`).
6. While an app is in front, the home screen **stops drawing** (no animations,
   no frames), so the app gets the whole machine.

Optional apps (Spotify, Jellyfin Desktop, RetroArch) have no tile until
their Flatpak is installed (`hide_when_missing` in the config).

### Web apps

Netflix, Disney+ and Hulu have no Linux apps, so Bear Den opens their websites
in Chromium from Flathub, full screen, one profile per app, and a Browser tile
opens ordinary Chromium. The coordinator starts Chromium itself with
`--remote-debugging-pipe` (two private fds, nothing listening), injects its
navigation script (`apps/web-nav`) into an isolated world of every page, and
turns the phone's named actions into the script's moves plus trusted clicks
and keys from a closed list. The phone's Touchpad reaches what the D-pad
cannot. Details: [ADR 0010](decisions/0010-web-apps-over-cdp-pipe.md),
[`contracts/actions.md`](../contracts/actions.md#web-apps).

### An app that isn't installed

A "Not installed" tile opens an install card instead. One press on Install
(the owner's consent; phones need the `owner` permission) has the
coordinator run `flatpak install --user --noninteractive -y flathub <id>`
for that app's Flatpak id from the adapter table (Chromium for the web
apps): per user, no root, from Flathub only. The card and the tile show the
progress from state pushes; when it is done the app opens if you are still
on its card or tile. Settings → Add apps lists everything missing, and
Settings → Keep apps up to date updates this user's installs once a day
while nothing is on screen. After Chromium, each enabled streaming site's
profile runs once, headless, so Chromium fetches Widevine. Details:
[ADR 0011](decisions/0011-per-user-flathub-installs.md),
[`operations.md`](operations.md#app-installs).

## Home, and coming back

- **Home** (phone button or the shell's own key) asks the window manager to
  activate the home screen's window (EWMH `_NET_ACTIVE_WINDOW`); the app keeps
  running behind it. That is why returning to an app is instant. Before
  that, Home pauses a playing app only where the pause can be verified: Plex
  HTPC, VacuumTube and Jellyfin through their own MPRIS player (sent Pause,
  then seen Paused), a web app with the site's pause key. Moonlight, Spotify
  and RetroArch keep running as they are.
- The home screen restores focus to the tile you left.
- Every 3 seconds the coordinator reconciles running apps against the
  window list and Flatpak instances. When an app closes (or crashes), it is
  marked as exited and the home screen is brought back automatically,
  unless something Bear Den knows is in front by then (read again from the
  desktop just before). While another app is starting (opening an app
  closes the others), that waits: the new app comes to the front itself,
  and the home screen comes back only if its launch fails.
- **Close** on the phone closes the app in front, or the one left running
  behind Home.

## The remote: named actions only

The phone never sends key codes, shell commands, paths or URLs. It sends
**named actions**: `nav.left`, `select`, `back`, `home`, `app.launch`,
`app.close`, `media.play`, `audio.volume_delta`, and so on (the list is in
[`contracts/actions.md`](../contracts/actions.md)). The coordinator decides what each one means:

| What is in front | What an action does |
|---|---|
| the home screen | forwarded to the shell over the socket; the shell moves focus and reports what it did (**observed**) |
| a known app | turned into a key press for that app's window (X11 XTEST), **only** after verifying that window is really focused (**delivered**; the app does not confirm). Never on Wayland: refused with the reason |
| a window Bear Den does not recognise, a locked session | **refused**, with a reason; Home still works |

Other rules that keep it predictable:

- **Context epoch:** every snapshot has a number that changes when what's on
  screen changes. A press made against an old picture of the TV is refused
  ("the TV changed since this button was shown") rather than landing somewhere
  unexpected.
- **Holds:** keeping a direction pressed repeats it, starting after 350 ms, six
  times a second. The phone must keep renewing the hold every 200 ms, or it
  expires after 600 ms. The first press is always exactly one step.
- **De-duplication:** every request has an id; a retried request is answered
  once.
- **Media and volume:** play/pause/seek go through the app's MPRIS media
  interface when it has one; volume and mute change the PC's volume through
  PulseAudio or PipeWire.
- **Outcomes are honest:** `accepted` (queued), `delivered` (sent), `observed`
  (confirmed by what's on screen), `failed` (with a reason). `delivered` is
  never shown as `observed`.

## Wayland

The reference TV runs X11; everything above is what it does there. On a
Wayland desktop an ordinary program cannot see or move other programs'
windows unless the compositor offers a protocol for it, so what works depends
on the desktop ([ADR 0007](decisions/0007-wayland-profile.md)):

| Desktop | Sees what's in front, brings a window forward (Home works) | Remote keys into apps | Launch, media (MPRIS), PC volume, lock |
|---|---|---|---|
| sway, labwc, Hyprland and other wlroots compositors | yes (`wlr-foreign-toplevel`; adapter `wayland-wlr`) | no | yes, but a lock is seen only if the locker tells logind ([operations → Wayland](operations.md#wayland)) |
| GNOME, KDE Plasma, others | no; once an app is in front the target is unknown | no | yes; a launch is reported **delivered**, never observed |

The home screen itself runs through XWayland (the bundled Qt has no Wayland
plugin) and the phone drives it over the socket as usual. `bear-den-tv doctor
--probe` shows the compositor family and the reason for every missing
capability. Only a headless sway in a container has been tested
([`scripts/wayland-container-test.sh`](../scripts/wayland-container-test.sh)).

## Pairing and safety

- Nothing listens on the network until you turn on the phone remote in
  Settings, choose the network interface, and consent to LAN exposure.
- Phones pair with a six-digit code or QR shown on the TV, then hold a session
  with permissions (`controller`, `layout_editor`, `owner`). Devices can be
  revoked from the TV or the phone. A visitor can get a guest pass instead
  (permission `guest`, a remote with fewer powers that ends by itself).
- Secrets never go into `config.json`, exports, logs or phone payloads.
  [`docs/security.md`](security.md) has the details.

## Now playing, the sleep timer and the TV itself

- **Now playing.** While the app in front has an MPRIS player, the
  coordinator reads its title, position and state (only that app's player,
  kept in memory) and sends them to controller phones and guest passes,
  never while locked. The owner can turn it off (TV Settings → Now playing
  on phones).
- **Sleep timer and screen off.** The coordinator keeps the timer on its
  clock: a warning a minute before, then a verified pause, Home, and the
  display off through X11 DPMS, putting back the exact previous DPMS
  settings on wake. The press that wakes the screen does nothing else.
  Suspend is never offered (logind is only asked whether it could).
- **HDMI-CEC** (off by default, needs a CEC device). The coordinator talks
  to the TV through the kernel's CEC API: standby after the display went
  off, on and to Bear Den's input on a press or Home, and optionally the
  TV's volume keys. Details: [ADR 0008](decisions/0008-hdmi-cec.md).

## Plex rows

Home can show Continue Watching and Recently Added from the owner's Plex
server. Signing in happens on the TV (Settings → Plex): the coordinator
gets a link code from plex.tv, the owner enters it at plex.tv/link, and the
token goes into the desktop keyring (never a file). The coordinator then
talks only to the chosen server, refreshes the rows when Home shows and
every 10 minutes while it stays in front (never behind an app), and caches
the posters. Selecting a card opens Plex HTPC. Code: `internal/plexlink`,
`internal/providers/plex`; commands in
[`operations.md` → Plex](operations.md#plex).

## Den badges

A few local counters (apps opened, days Home was shown, rainy days, guest
passes, ...) earn playful badges, shown in TV Settings → Badges and on
controller phones. Only ids, counts and days are stored, in `state.db`;
counting can be turned off and reset
([ADR 0009](decisions/0009-den-badges-local-counters.md)).

## App icons

Each tile shows, in order: the owner's brand folder icon, then (by default)
the icon the installed Flatpak exports when that Flatpak is the app itself,
then Bear Den's own drawing, then a monogram. Settings → App icons → Bear
Den style skips the app's own. Phones get the same icon as a sanitised PNG
from the coordinator ([ADR 0012](decisions/0012-app-icons-apps-own-by-default.md)).

## Local weather

Off until the owner turns it on in TV Settings → Weather (or
`bear-den-tv weather set`). The owner searches a place by name, typed on a
keyboard or sent from the phone keyboard as `text.submit` into the focused
search field; the shell asks the coordinator (`weather.search`), which asks
Open-Meteo's geocoding and answers with up to 8 places. Choosing one sends
`weather.configure`; the coordinator rounds the coordinates to 2 decimals and
stores them in `config.json` (`weather`). `internal/weather` then fetches the
current reading every 30 minutes (retrying after 5, 10, 20, 30 minutes when
it fails, dropping a reading older than 6 hours) and keeps it in memory; the
shell's snapshot carries it as `state.weather` for the header chip and, when
`scene` is on, the rain, snow or veil on Home. Phones never see it.
`bear-den-tv dev` uses a DEMO reading and never touches the network. What
leaves the box, and what never does, is in [`docs/security.md`](security.md);
the commands are in [`docs/operations.md`](operations.md#local-weather).

## Themes are packages

A theme is a folder: `theme.json` ([`contracts/theme.schema.json`](../contracts/theme.schema.json)) plus its
wallpaper, phone backdrop and ornaments. It picks from building blocks the
engine already has: focus decorations (vine, fern, stars, embers), ambient
particles, a corner scene, what the bears wear and chase. It never contains
code.

- The shell loads theme packages through `ThemeRegistry` (the QML `Themes`
  singleton). Built-in themes are compiled into the shell.
- The coordinator loads the same packages (`internal/themes`). It tells phones
  the theme's colours and decoration (`state.appearance`) and serves the
  theme's images.
- Owner themes in `~/.local/share/bear-den-tv/themes/<id>/` appear without a
  rebuild.

[`docs/THEMES.md`](THEMES.md) is the designer's guide.

## Built for the floor, scaled up

Bear Den is designed and tested on its weakest target, a 2-core Celeron 2955U
Chromebox, and gets out of the way on anything faster.

**The home screen:**
- **Behind apps:** the home screen draws nothing while an app is in front.
- **Resting:** after 45 s without a button press, background motion
  (fireflies, wallpaper drift, bears blinking) stops, so the screen stops
  redrawing. The focused tile's decoration keeps living at 4 beats a second, and bear
  visits come less often.
- **Screensaver:** after 5 minutes on Home, an OLED-safe screensaver takes over
  (a dim clock and a sleeping bear that moves every 15 s). Everything else
  stops.
- Decorations are drawn with QPainter (Canvas), painted only when something
  changes. Everything that loops moves on one shared heartbeat (20 beats a
  second awake, 4 resting), so the screen redraws at most at that rate, never
  at the TV's 120 Hz. There are no custom GPU shaders. `make perf` checks
  this in a sandbox.
- [`docs/THEMES.md`](THEMES.md) lists every animation and the rule that pauses it.

**The apps.** A detection test runs 20 s after startup:
- It works out the box's **tier** (entry, standard or high) from its cores and
  clocks.
- It reads the TV output's mode, and which codecs each app's runtime decodes in
  hardware.
- It applies each app's best settings for that combination: GPU decoding
  first, never more pixels or frames than the TV shows, and careful settings
  only on entry boxes.
- **Settings → Playback** on the TV shows the results: what to expect and the
  caveats.

[`docs/APP_PERFORMANCE.md`](APP_PERFORMANCE.md) has every rule.

## Installing and deploying

There are two ways onto a TV box. An owner installs the `.deb` from
`make package` (Qt bundled; see [`operations.md` → Packaging](operations.md#packaging));
no release has been published yet. A developer deploys from a checkout:

[`scripts/deploy-target.sh`](../scripts/deploy-target.sh) builds everything on the workstation in seconds,
then:

1. copies the binaries to the TV (copy + rename, never overwriting a running
   file);
2. restarts Bear Den, unless an app is on screen (then it waits for the next
   start; `--now` overrides);
3. verifies the running binaries' hashes, the shell connection, and (if
   `BDTV_TARGET_HEALTH_CMD` is set) other services on the same box.

The home screen is compiled against an old glibc baseline so the same binary
runs on the TV ([ADR 0003](decisions/0003-build-tv-shell-off-target.md)).
[`docs/operations.md`](operations.md) has the everyday commands.

## Where to look when something is off

| Question | Command |
|---|---|
| Is it running, what's in front, what's allowed? | `build/bin/bear-den-tv doctor` |
| What happened? | `~/.local/state/bear-den-tv/session.log` |
| Restart it | `scripts/start-session.sh --watch` (or the desktop icon) |
| Stop it | `scripts/start-session.sh stop` |
| CPU and memory now | `scripts/measure-target.sh 20` (from the workstation) |
| Frame rate | start with `BDTV_FPS_LOG=1` (prints fps every 5 s) |
| What can this box play, and are the apps tuned? | Settings → Playback, or `build/bin/bear-den-tv apps detect` |
| Which themes are installed, and why was mine skipped? | `build/bin/bear-den-tv themes list` |
