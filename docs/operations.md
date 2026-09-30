# Operations

Every command here exists in the current tree. What has been seen working on a real TV is recorded in [`docs/IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

## Owner tasks

Everything an owner does, and where it is described. On the TV most of it
is in **Settings**; from a terminal on the TV the same commands are
`bear-den-tv ...` (package) or `build/bin/bear-den-tv ...` (checkout), and
`bear-den-tv help` lists them. Nothing here has been seen on the TV unless
[`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md) says so.

| Task | On the TV | Command | Section |
|---|---|---|---|
| Install or remove Bear Den | — | `sudo apt install ./bear-den-tv_*.deb`, `sudo apt remove --purge bear-den-tv` | [Packaging](#packaging) |
| Start, stop, start at login | Settings → Exit Bear Den TV | `start-session.sh --watch\|stop`, `bear-den-tv autostart enable` | [Starting, stopping, autostart](#starting-stopping-autostart) |
| Turn on the phone remote | Settings → Phone remote | `bear-den-tv remote enable --interface IF --accept-lan-exposure` | [Phone remote](#phone-remote) |
| Pair a phone, or give a guest pass | Settings → Pair a phone | `bear-den-tv pair [--guest tonight\|24h\|7d]` | [Phone remote](#phone-remote), [Guest passes](#guest-passes) |
| Remove a phone | Settings → Paired phones | `bear-den-tv devices revoke <id\|*>` | [Phone remote](#phone-remote) |
| Install, update or remove apps | a "Not installed" tile, Settings → Add apps, Keep apps up to date | `bear-den-tv apps install <app-id>`, `flatpak uninstall --user <id>` | [App installs](#app-installs) |
| Turn on Netflix, Disney+ or Hulu | Settings → Streaming sites | — | [Streaming sites and the Browser](#streaming-sites-and-the-browser) |
| Sign the TV in to Plex | Settings → Plex | `bear-den-tv plex sign-in` | [Plex](#plex) |
| Playback settings per app | Settings → Playback, Advanced playback | `bear-den-tv apps detect [--apply]` | [App playback settings](#app-playback-settings) |
| Sleep timer, screen off | Settings → Sleep timer, Turn the screen off | — (the phone's Sleep section) | [Sleep timer and screen off](#sleep-timer-and-screen-off) |
| TV power and volume over HDMI-CEC | Settings → TV control over HDMI (CEC) | — | [TV control over HDMI-CEC](#tv-control-over-hdmi-cec) |
| Now playing on phones | Settings → Now playing on phones | — | [Phone remote](#phone-remote) |
| Den badges | Settings → Badges | `bear-den-tv badges status\|on\|off\|reset` | [Den badges](#den-badges) |
| Local weather | Settings → Weather | `bear-den-tv weather ...` | [Local weather](#local-weather) |
| Theme, style, art style, app icons | Settings → Theme, Style, Art style, App icons | `bear-den-tv themes list` | [Themes](#themes), [App icons](#app-icons) |
| Run on Wayland | — | `bear-den-tv doctor --probe` | [Wayland](#wayland) |
| See what's wrong | Settings → Diagnostics | `bear-den-tv doctor` | [Diagnostics](#diagnostics) |

## Development toolchain (no root)

```sh
scripts/bootstrap-toolchain.sh   # micromamba env in ~/.bdtv-toolchain: Go 1.24, Qt 6.8, CMake, Ninja, GCC, Node 22 (toolchain/environment.yml)
. scripts/env.sh                 # put it on PATH for this shell
make help                        # build/test targets
```

The script pins micromamba (override with `BDTV_MICROMAMBA_VERSION`) and Qt's
exact patch release. `BDTV_SKIP_SYSROOT=1` skips the glibc 2.28 sysroot that only
`make shell-target` needs; CI (`.github/workflows/ci.yml`) sets it.

## Point the scripts at your TV

[`scripts/target.sh`](../scripts/target.sh), [`deploy-target.sh`](../scripts/deploy-target.sh),
[`e2e-target.sh`](../scripts/e2e-target.sh) and [`measure-target.sh`](../scripts/measure-target.sh)
read the TV machine's address from `target.env` at the repo root. That file is
untracked, so your details never enter git.

1. Copy the example: `cp target.env.example target.env`
   ([`target.env.example`](../target.env.example)).
2. Set `BDTV_TARGET` to the ssh destination of your TV (for example
   `you@tv.local`). Login must work with a key (`ssh -o BatchMode=yes`).
3. Optional: set `BDTV_TARGET_DIR` (checkout on the TV, relative to its home
   directory; default `bear-den-tv`).
4. Optional: set `BDTV_TARGET_HEALTH_CMD`, a command run on the TV after a
   deploy to check other services it hosts. Empty skips it.

Environment variables of the same name override the file. With no
`BDTV_TARGET` the scripts stop and say so ([`scripts/target-env.sh`](../scripts/target-env.sh)).

Done when: `scripts/target.sh ssh 'true'` exits 0 without asking for a password.

## Working against the TV machine

```sh
scripts/target.sh sync                    # rsync the checkout to $BDTV_TARGET:$BDTV_TARGET_DIR
scripts/target.sh run 'make go'           # sync, then run inside the toolchain env on the target
scripts/target.sh ssh 'bear-den-tv doctor' # run without syncing
```

Graphical checks on the target need `DISPLAY=:0 XAUTHORITY=$HOME/.Xauthority DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$(id -u)/bus` and an **active** user session on the TV (the window manager does not map new windows while the seat is on the login greeter). The bundled Qt needs `QT_XCB_GL_INTEGRATION=xcb_egl` ([ADR 0001](decisions/0001-standalone-project-and-user-space-toolchain.md)); [`scripts/start-session.sh`](../scripts/start-session.sh) sets it for you.

## Runtime layout (XDG)

| Purpose | Path |
|---|---|
| Configuration | `$XDG_CONFIG_HOME/bear-den-tv/config.json` (+ `config.last-known-good.json`, `config.history/`) |
| State (devices, sessions, focus memory, Den badge counters) | `$XDG_DATA_HOME/bear-den-tv/state.db` |
| Artwork cache | `$XDG_CACHE_HOME/bear-den-tv/artwork/` (the DEMO pictures of `dev --dev-fixtures`), `$XDG_CACHE_HOME/bear-den-tv/plex-artwork/` (Plex posters, deleted on sign-out) |
| Plex client id | `$XDG_DATA_HOME/bear-den-tv/plex-client-id` (32 hex characters; not a secret, but Plex ties the sign-in to it) |
| Web app profiles (one per app and browser: cookies, sign-ins, Widevine) | `$XDG_DATA_HOME/bear-den-tv/web/<app-id>/` (Chromium), `$XDG_DATA_HOME/bear-den-tv/web-brave/<app-id>/` (Brave) ([Streaming sites and the Browser](#streaming-sites-and-the-browser)) |
| IPC socket, instance lock | `$XDG_RUNTIME_DIR/bear-den-tv/` |
| Connector tokens | Desktop Secret Service (never files) |

## Deploying to the TV

```sh
scripts/deploy-target.sh             # build here, install on the TV, restart unless an app is on screen, verify
scripts/deploy-target.sh --now       # restart even if an app is on screen
scripts/deploy-target.sh --no-restart
scripts/deploy-target.sh --dry-run   # print the steps
```

- **Builds:** the Go binaries (with the phone remote embedded), and the TV shell
  via `make shell-target`. The shell is compiled here against a glibc 2.28 sysroot
  so it runs on the TV's older glibc ([ADR 0003](decisions/0003-build-tv-shell-off-target.md)); nothing compiles on the TV.
- **Installs** by copy + rename; a running executable is never overwritten.
- **Verifies** the running binaries' hashes, the shell connection
  (`bear-den-tv doctor`), and `BDTV_TARGET_HEALTH_CMD` if set.

## Upgrading

A new version keeps your `config.json`: your apps, their order, what is on
or off, the layout and every setting. When the coordinator starts it adds
the apps this version knows that your file does not have yet (for a box
that started with Plex, YouTube and Moonlight: Spotify, Jellyfin,
RetroArch, Netflix, Disney+, Hulu and the Browser), exactly as a fresh
install has them: the optional apps and the Browser show no tile until
they are installed, and the streaming sites stay off until you turn them on
in Settings → Streaming sites. Their ids go at the end of your "Your Apps"
section (the section with id `favorites`), if you still have one. The
change is written like any other (a new revision, a copy in
`config.history/`, the last-known-good file) and logged as `config: added
the apps this version knows`; starting again adds nothing. Nothing is
added while Bear Den runs on the last-known-good copy or the defaults
because `config.json` did not load.

`config.json` has no way to say "I removed this app", so an app whose row
you deleted by hand comes back (hidden or off where a fresh install has it
so). To keep a streaming site away, leave it off in Settings → Streaming
sites.

## Starting, stopping, autostart

On the TV, in the checkout (`$BDTV_TARGET_DIR`):

```sh
scripts/start-session.sh --watch   # (re)start Bear Den with the crash watchdog
scripts/start-session.sh stop      # stop it (and the watchdog)
build/bin/bear-den-tv autostart enable|disable|status   # start at every desktop login
build/bin/bear-den-tv shortcut enable|disable|status    # "Bear Den TV" icon on the desktop and in the app menu
```

From the .deb, the same commands are `bear-den-tv autostart|shortcut ...` and
`/usr/lib/bear-den-tv/start-session.sh --watch|stop` ([Packaging](#packaging)).

- **Settings → Exit Bear Den TV** closes the home screen until the next start.
  The coordinator keeps running for the phone.
- The **desktop icon** runs `start-session.sh --watch`.
- `autostart enable` and `shortcut enable` find the start script next to the
  binary: `<repo>/scripts/start-session.sh` for `<repo>/build/bin/bear-den-tv`,
  `/usr/lib/bear-den-tv/start-session.sh` for `/usr/bin/bear-den-tv`.
- **Log:** `${XDG_STATE_HOME:-~/.local/state}/bear-den-tv/session.log`, rotated at 5 MB (one old copy, `session.log.1`).
- **Detached:** what `start-session.sh` starts runs in a session of its own
  (`setsid`) with no controlling terminal, stdin from `/dev/null` and its
  output in the log. The script waits until that is true before it returns,
  so closing the terminal or the ssh connection right after it cannot hang
  the start up, and a terminal's job-control signals (Ctrl+Z, background
  reads and writes) never reach Bear Den
  ([`tests/packaging/watchdog_test.go`](../tests/packaging/watchdog_test.go)).
  `bear-den-tv session` also ignores SIGTSTP, SIGTTIN and SIGTTOU itself
  ([`daemon.go`](../cmd/bear-den-tv/daemon.go)).

## Packaging

`make package` builds `build/dist/bear-den-tv_<version>_amd64.deb` for Ubuntu
22.04 / Linux Mint 21 and newer (X11 desktops). It needs only the user-space
toolchain (`scripts/bootstrap-toolchain.sh` also builds [nfpm](https://nfpm.goreleaser.com/)
v2.43.1 into it).

```sh
make package                                   # → build/dist/bear-den-tv_<version>_amd64.deb
packaging/smoke-deb.sh ubuntu:22.04            # install/run/remove in a clean container (Docker)
packaging/smoke-deb.sh ubuntu:24.04
```

On the TV box:

```sh
sudo apt install ./bear-den-tv_<version>_amd64.deb   # pulls X11/EGL/fontconfig from the distro
/usr/lib/bear-den-tv/start-session.sh --watch        # start now (or "Bear Den TV" in the app menu)
bear-den-tv autostart enable                         # optional: start at every desktop login
bear-den-tv autostart disable                        # before removing
sudo apt remove --purge bear-den-tv
```

What [`packaging/build-deb.sh`](../packaging/build-deb.sh) does:

- **Version:** [`packaging/version.sh`](../packaging/version.sh) maps
  `git describe --tags --always --dirty` to a Debian version (`v1.2.0` → `1.2.0`,
  `v1.2.0-3-gabc1234` → `1.2.0+git3.gabc1234`, no tag yet → `0.1.0~git<N>.<sha>` with N the commit count, so every later build sorts higher; a build from before this scheme (`0.1.0~git.<sha>`) must be removed before installing a newer one).
  `VERSION=...` overrides it. The coordinator prints it with `bear-den-tv version`.
- **Coordinator:** static (`CGO_ENABLED=0`; every Go dependency, SQLite
  included, is pure Go).
- **Shell:** built against the glibc 2.28 sysroot like `make shell-target`, but
  with **no rpath**: conda's GCC adds `-rpath <toolchain>/env/lib` to every link
  through its specs, so the build passes a specs file without it. A `DT_RPATH`
  into the toolchain would override the wrapper's `LD_LIBRARY_PATH` and load a
  developer's Qt.
- **Qt runtime:** [`packaging/bundle-qt.sh`](../packaging/bundle-qt.sh) copies the
  toolchain libraries the shell needs, the xcb/offscreen platform plugins, the
  image formats and exactly the QML modules `qmlimportscanner` reports, plus a
  `qt.conf`. It leaves out what must come from the system: glibc, the GL/EGL/DRM
  stack (libglvnd has to find the distro's Mesa), libX11/libxcb (shared with
  Mesa), xkbcommon (reads the system's XKB data) and D-Bus. Those are the
  package's `depends`.
- **libstdc++:** the shell is built with GCC 15. The bundle carries its
  libstdc++/libgcc_s in `lib/compat/`, and the wrapper
  ([`packaging/bear-den-tv-shell.sh`](../packaging/bear-den-tv-shell.sh)) uses
  them only when the system's libstdc++ lacks the needed `GLIBCXX` version, so a
  newer distro's Mesa never gets an older libstdc++ than it was built with.
- **Checks that fail the build:** any shipped ELF with an rpath/runpath into the
  toolchain, or needing a glibc newer than 2.28.
- **Reported, not fatal:** a few conda libraries (fontconfig, glib, libuuid,
  libcrypto, xcb-cursor) keep the build machine's toolchain path as a compiled-in
  default. The wrapper's `FONTCONFIG_FILE` and `qt.conf` override the ones that
  matter. `make package` lists them. On a workstation that path contains your
  user name, which is why published packages are built only by the release
  workflow ([Cutting a release](#cutting-a-release)).

Installed layout (from [`packaging/nfpm.yaml`](../packaging/nfpm.yaml)):

| Path | What |
|---|---|
| `/usr/bin/bear-den-tv` | coordinator and CLI |
| `/opt/bear-den-tv/shell/` | the shell, its wrapper and the bundled Qt |
| `/usr/bin/bear-den-tv-shell` | symlink to the wrapper; the coordinator finds the shell next to itself |
| `/usr/lib/bear-den-tv/start-session.sh` | (re)start with the watchdog; what autostart and the launchers run |
| `/usr/share/applications/bear-den-tv.desktop` | app-menu entry |
| `/usr/share/bear-den-tv/autostart/bear-den-tv.desktop` | a login entry that does nothing until copied; the package never enables autostart |

Removing the package leaves your settings, paired phones and caches in the
XDG folders ([Runtime layout](#runtime-layout-xdg)) and any
`~/.config/autostart` entry you enabled; delete them to forget everything.

### Cutting a release

Push a version tag. The workflow does the rest:

```sh
git tag v0.x.y && git push origin v0.x.y
```

[`.github/workflows/release.yml`](../.github/workflows/release.yml) then runs
on GitHub's `ubuntu-24.04` runner:

1. restores or bootstraps the toolchain **with** the glibc 2.28 sysroot (cache
   key: CI's plus `-sysroot`);
2. `make test`, then `make package`, and checks the package version equals the
   tag (`v0.2.0` → `0.2.0`, `v0.2.0-rc1` → `0.2.0~rc1`);
3. `packaging/smoke-deb.sh ubuntu:22.04` and `ubuntu:24.04`;
4. [`packaging/check-home-paths.sh`](../packaging/check-home-paths.sh): fails if
   any file in the .deb mentions a home directory other than `/home/runner`
   (the runner's toolchain path) or `/home/conda` (conda-forge's own build
   path);
5. creates the GitHub Release for the tag with auto-generated notes, the .deb
   and `SHA256SUMS` (a tag with a `-`, like `v0.2.0-rc1`, is marked
   pre-release). Re-running the workflow replaces the two files.

**Never upload a .deb built locally.** It carries your toolchain path, and
with it your user name, in five bundled libraries; `packaging/check-home-paths.sh`
on a local build shows them. To try the release build without publishing,
run the workflow by hand (Actions → Release → Run workflow): it builds and
checks the same way and keeps the .deb as a workflow artifact for 14 days.

## Sandbox (prototype without the TV)

```sh
make shell                                                   # rebuild after a QML change (seconds)
scripts/sandbox.sh shot --screen home --theme campfire       # one screenshot → build/shots/…png
scripts/sandbox.sh shot --plain --bears hop --size 3840x2160 # Plain style, a bear visit, at 4K
make shots                                                   # every theme × main screens → build/shots/gallery/
make perf                                                    # frames and CPU per phase of Home; fails over budget
```

The sandbox runs the real shell offscreen on the demo fixture. Layout, colour
and decorations match the TV; smoothness and absolute CPU are judged on the TV.

## Diagnostics

```sh
build/bin/bear-den-tv doctor          # JSON: coordinator state, what's in front, capabilities, config
scripts/measure-target.sh 20          # (workstation) CPU % and RSS of the Bear Den processes on the TV
make perf                             # (workstation) sandbox: frames and CPU per phase of Home, offscreen on 2 cores; fails over budget
```

Environment variables (set them in the coordinator's environment; the ones
marked *shell* are passed on to the home screen):

| Variable | Effect |
|---|---|
| `BDTV_FPS_LOG=1` | *shell*: print the shell's frames per second every 5 s |
| `BDTV_SCREENSAVER_SECONDS=N` | *shell*: screensaver after N idle seconds (default 300; 0 = off) |
| `BDTV_BEARS_SECONDS=N` | *shell*: a bear visit every N seconds |
| `BDTV_BEARS_ACT=walk\|peek\|hop\|parade\|chase` | *shell*: force one bear act |
| `BDTV_REST_SECONDS=N` | *shell*: rest after N seconds without input (default 45) |
| `BDTV_MONTH=1..12` | *shell*: pretend it is that month, to preview seasonal touches (October pumpkins, December snow) |
| `BDTV_THEMES_DIR=/path` | look for your own themes there instead of `$XDG_DATA_HOME/bear-den-tv/themes` (coordinator and shell) |
| `BDTV_TIER=entry\|standard\|high` | override the box's tier for playback tuning ([`docs/APP_PERFORMANCE.md`](APP_PERFORMANCE.md)) |

## Wayland

The TV is expected to run X11. On Wayland, `bear-den-tv doctor --probe` says
which compositor family it found (`probe.wayland.family`), lists windows by
`app_id` where it can, and gives the reason for every unavailable capability
([ADR 0007](decisions/0007-wayland-profile.md)). The shell runs through
XWayland: its launcher sets `QT_QPA_PLATFORM=xcb`, so the session needs
XWayland.

Lock observation reads logind `LockedHint` and `org.freedesktop.ScreenSaver`.
GNOME and KDE set `LockedHint` when they lock (not tested here); swaylock on
its own sets neither, so on sway a locked screen may not be seen as locked.

Test it without a Wayland desktop (Docker; nothing runs on your display):

```sh
. scripts/env.sh
scripts/wayland-container-test.sh           # headless sway in ubuntu:24.04: adapter test + doctor --probe
make shell && scripts/wayland-container-test.sh --shell   # also the coordinator with the real shell on XWayland
```

It builds a small image (`tests/wayland/Dockerfile`: sway, foot, XWayland),
opens two `foot` windows with their own app_ids, runs
[`tests/wayland/live_test.go`](../tests/wayland/live_test.go) (list, observe,
activate, fullscreen and close, each checked against `swaymsg`) and
`bear-den-tv doctor --probe`, then checks the capability report. `--shell`
also starts `bear-den-tv session` with the shell, checks the shell is a
fullscreen XWayland window in front, then puts another window in front and
checks the target turns unknown while Home stays available. Output lands in
`build/wayland-test/out/`. It is not part of `make test`.

## App playback settings

The coordinator runs the playback detection test 20 s after it starts and
applies the best settings to closed apps; a running app is tuned when it closes.
Results: **Settings → Playback** on the TV. Turn it off with
`"startup": {"tune_apps": false}` in `config.json`.

```sh
build/bin/bear-den-tv apps detect                 # the box's tier, the display, per app: hardware decoding, what to expect, caveats, the plan (dry run)
build/bin/bear-den-tv apps detect --apply         # write the plans of closed apps (files are backed up)
build/bin/bear-den-tv apps detect --tier standard # preview what another class of box would get
build/bin/bear-den-tv apps detect --json          # everything, machine-readable
build/bin/bear-den-tv apps probe                  # only: codecs each app's runtime decodes in hardware here
```

`apps tune` is the same as `apps detect`. See [`docs/APP_PERFORMANCE.md`](APP_PERFORMANCE.md) for
every rule, the tiers, the probe and how to undo.

## Themes

```sh
build/bin/bear-den-tv themes list             # installed themes (built-in and yours), and skipped packages with the reason
build/bin/bear-den-tv themes validate <dir>   # check a theme folder you are designing
build/bin/bear-den-tv themes path             # where your own themes go
```

A new or changed theme shows up when Settings opens on the TV, and within 10 s
on phones. [`docs/THEMES.md`](THEMES.md) is the theme designer's guide.

## App icons

**Settings → App icons** (or App icons in the phone's Layout editor) chooses
what each tile shows: **App's own** (the default) is the icon the installed
Flatpak exports; **Bear Den style** is Bear Den's own drawing. An app that is
not installed, and the streaming sites (they run in Chromium), show Bear
Den's either way. An icon you put in
`~/.local/share/bear-den-tv/brand/<adapter>/icon.{png,svg,jpg,webp}` comes
before both (phones skip SVG). Details:
[`docs/THEMES.md` → App icons](THEMES.md#app-icons),
[ADR 0012](decisions/0012-app-icons-apps-own-by-default.md).

## Local weather

Off by default. Turn it on in TV **Settings → Weather**, or from the CLI
against the running coordinator ([`cmd/bear-den-tv/weather.go`](../cmd/bear-den-tv/weather.go)).

1. Find your place (asks Open-Meteo; up to 8 numbered results):
   `build/bin/bear-den-tv weather search <place>`
2. Turn weather on for result number INDEX (default 1). Flags go before the
   place: `build/bin/bear-den-tv weather set [--units fahrenheit] [--no-scene] <place> [INDEX]`
   (`--no-scene` keeps it to the header chip, with no rain or snow on Home).
3. Check it: `build/bin/bear-den-tv weather status`

Done when: `weather status` prints your place and a `now:` line with the
temperature.

To stop every weather request: `build/bin/bear-den-tv weather off`.

The CLI never edits `config.json` itself; the coordinator stores the choice
(`weather` in [`contracts/config.md`](../contracts/config.md)). What leaves the
box is described in [`docs/security.md`](security.md).

## Plex

Home can show **Continue Watching** and **Recently Added** from your Plex
server. Selecting an item opens Plex HTPC (not that exact item: the handoff is
not verified yet). Nothing is sent to Plex until you sign in; what is sent
then is in [`docs/security.md`](security.md).

Sign in on the TV: **Settings → Plex → Sign in**.

1. The TV shows a 4-character code, `plex.tv/link` and a QR code of that
   address. On a phone or computer, open it, sign in to Plex and type the code.
2. The TV moves on by itself: with one server it is chosen for you, otherwise
   pick one.
3. Tick the libraries Home should use (movies and shows are proposed) and
   choose **Done**.

Done when: Settings → Plex says "Signed in · <your server>" and Home shows the
two rows after you go back to it.

The same over SSH, against the running coordinator
([`cmd/bear-den-tv/plex.go`](../cmd/bear-den-tv/plex.go)):

```sh
build/bin/bear-den-tv plex sign-in          # prints the code; type it at plex.tv/link
build/bin/bear-den-tv plex status           # servers / libraries with their ids once linked
build/bin/bear-den-tv plex server ID        # only when you have several servers
build/bin/bear-den-tv plex libraries 1 2    # finish with these library ids
build/bin/bear-den-tv plex sign-out         # token out of the keyring, rows and posters gone
```

`bear-den-tv plex cancel` abandons a sign-in in progress.

| You see | Do |
|---|---|
| "Plex sign-in needs a keyring; install or enable gnome-keyring" | The desktop session has no Secret Service (or it is locked). Install/enable gnome-keyring (or another Secret Service) for the TV user, log in again, retry. Bear Den never stores the token in a file. |
| "The code expired" | Choose **Try again** for a new code. |
| "Can't reach plex.tv" | The TV has no internet; check it and retry. |
| "can't reach <server> from this TV" | None of that server's addresses answered as that server. Is it on and on the same network? |
| Rows say "Can't reach your Plex server" | The server is off or unreachable. Rows retry after 30 s, 1, 2, 5, then every 10 minutes while Home is in front. |
| Rows say "Plex no longer accepts this TV's sign-in" | The device was removed on plex.tv. Sign out, then sign in again. |
| Rows say "Plex account is not linked" | The keyring lost the token (for example a new keyring). Sign in again. |

For development, `bear-den-tv dev --dev-plex-fake` runs the whole flow against
a local fake plex.tv and server with DEMO titles and generated DEMO posters
(the code links on the third poll; the keyring is in memory). Plain `dev`
has no Plex connector and never contacts plex.tv.

## Sleep timer and screen off

Set it on the phone (Remote → Sleep: 15, 30, 45, 60, 90 or 120 minutes,
Cancel, Screen off) or on the TV (**Settings → Sleep timer** with ◀ ▶, and
**Settings → Turn the screen off**). What happens and why is in
[`contracts/actions.md`](../contracts/actions.md#sleep-screen-off-and-wake):

- A minute before the timer runs out the TV shows "Going to sleep in 1
  minute" (when Bear Den is in front; over an app only the phone shows it).
  Any key on the TV or any phone button cancels it.
- When it runs out: the app in front is paused only if its own MPRIS player
  is verified (never a guessed key), Bear Den Home comes to the front, then
  the display turns off. While the desktop is locked only the display turns
  off.
- Any phone button or TV key turns the display on again. That first press
  does nothing else. A TV key sent to an app in front (after Screen off from
  the phone) does reach the app: the X server wakes the display and the
  coordinator notices within 2 s.
- The timer lives in the coordinator's memory: restarting Bear Den cancels
  it and turns the display back on.

The display is turned off with X11 DPMS ([`internal/platform/x11/dpms.go`](../internal/platform/x11/dpms.go)).
If your desktop has DPMS disabled (the reference TV has it disabled with
600 s timeouts), Bear Den enables it only while the display is off, with the
standby, suspend and off timeouts at 0, and puts back the exact previous
state (enabled flag and timeouts) when the display comes on and when the
coordinator stops. Check the state on the TV (read-only):

```sh
DISPLAY=:0 xset q | sed -n '/DPMS/,$p'   # "DPMS is Disabled" and 600 s timeouts on the reference TV
```

If the coordinator was killed while the display was off, DPMS stays enabled
with no timeouts: nothing blanks later, but `xset q` shows "DPMS is Enabled".
Put your own settings back by hand, for example `DISPLAY=:0 xset dpms 600 600 600 && DISPLAY=:0 xset -dpms`.

Suspend is never offered: the coordinator only asks logind `CanSuspend` and
reports the answer as `state.power.suspend` (on the reference TV
"challenge": the system asks for a password, which Bear Den never handles).
The coordinator logs it at start (`msg=suspend`).

To try it against a DPMS-capable display you choose (nothing playing, the
display goes dark for a moment and its settings are put back):
`BDTV_DPMS_LIVE_DISPLAY=:0 go test -run DPMSLive -v ./internal/platform/x11/`.
Without it the live test starts a throwaway Xvfb, which has no DPMS extension,
and skips.

## TV control over HDMI-CEC

Optional and off by default ([ADR 0008](decisions/0008-hdmi-cec.md)). HDMI-CEC
is a slow control bus inside the HDMI cable: the box can turn the TV on and
off, make the TV switch to its input, and press the TV's volume keys, with the
TV still never on the network.

**Hardware.** Most mini PCs cannot do CEC on their HDMI port (the reference TV
box cannot). You need one of:

- a USB CEC adapter placed between the box and the TV, such as Pulse-Eight's
  USB-CEC Adapter (the kernel's `pulse8-cec` driver). On most distributions
  it only appears as `/dev/cecN` after `inputattach --pulse8-cec /dev/ttyACM0`
  (package `inputattach` or `linuxconsoletools`); that is a system change for
  the owner to make, Bear Den never runs it;
- a board whose HDMI port has CEC wired up and a kernel driver for it (some
  NUC boards with an on-board Pulse-Eight chip, Raspberry Pi and many ARM
  boards).

libcec-only setups (no `/dev/cec*`) are not supported. None of this has been
seen on hardware yet: it is tested against a fake device only.

**Check** (read-only, on the TV):

```sh
ls -l /dev/cec*          # nothing: no CEC device, Settings says so
id -nG                   # the session user needs read/write access to /dev/cecN
```

The device usually belongs to the `video` group (`crw-rw---- root video`). If
the session user is not in it, TV Settings shows "cannot open /dev/cec0:
permission denied". Adding the user to `video` (`sudo usermod -aG video
<user>`, then log out and in) is the owner's decision; Bear Den never changes
groups or udev rules. `cec-ctl` from v4l-utils, if installed, is a handy
second opinion (`cec-ctl -d0 --playback -S` lists what is on the bus) but
Bear Den does not need it.

**Turn it on** in TV Settings → **TV control over HDMI (CEC)**. Without an
adapter the row says why ("No HDMI-CEC device (/dev/cec*) — most PCs need a
USB CEC adapter"); it can still be turned on and applies once an adapter
appears (checked once a minute). While it is on
([`contracts/actions.md`](../contracts/actions.md#tv-control-over-hdmi-cec)):

- the sleep timer and Screen off put the TV in standby after the display
  goes off;
- the press that wakes the display, a TV key while it was off, any press
  after Bear Den put the TV in standby, and Home turn the TV on and switch
  it to Bear Den's input;
- the phone shows a **TV** section (TV on, TV standby, what the TV last
  reported), and **Phone volume buttons** in TV Settings chooses whether
  the phone's volume buttons change the PC's volume (the default) or press
  the TV's (the heading then reads "TV volume"). Mute uses the explicit
  Mute Function / Restore Volume Function keys; some TVs ignore them.

The coordinator logs each HDMI-CEC failure (`msg="session: ... HDMI-CEC"`).
Turning the setting off gives the bus address back. None of this has been
tried on a real adapter or TV.

## Phone remote

```sh
build/bin/bear-den-tv remote enable --interface <interface> --accept-lan-exposure   # or Settings → Phone remote on the TV
build/bin/bear-den-tv pair            # show a pairing code/URL
build/bin/bear-den-tv pair --guest tonight   # a guest pass: until 04:00 tomorrow morning (also 24h, 7d)
build/bin/bear-den-tv devices [revoke <id|*>]
```

**Settings → Now playing on phones** (on by default) decides whether paired
phones see the title and progress of what is playing, also while an app
keeps playing behind Home ([`docs/security.md`](security.md)). For Plex
HTPC the reading comes from your Plex server once Plex is signed in on the
TV (Settings → Plex); if the card stays empty while Plex plays, the owner's
diagnostics (`plex_now_playing`) say why.

### Guest passes

A guest pass lets a visitor use their phone as a remote for a limited time
without becoming a family phone. Issue one on the TV (Settings → Pair a phone,
then ◀ ▶ to choose *Guest pass · Tonight*, *24 hours* or *7 days*) or with
`bear-den-tv pair --guest tonight|24h|7d`; the visitor scans the code as usual.
*Tonight* ends at 04:00 the next morning in the TV's local time zone (a pass
issued between midnight and 04:00 ends that same morning).

A guest can move around, select, go back and Home, open apps, play, pause and
seek, change the volume and type into a text field, and sees what is playing.
A guest cannot close apps, restart the shell, turn anything off, or see or
change settings, the layout or the list of phones. When the pass ends the phone
is removed automatically (it shows "Your guest pass has ended"), also if Bear
Den was restarted in between. Settings → Paired phones and `bear-den-tv devices`
show each guest with the time it ends; remove one early like any phone.
A guest pass cannot be turned into a family phone: pair that phone again as a
family phone instead. Details: [`contracts/http.md`](../contracts/http.md#guest-passes).

## Den badges

Bear Den awards playful badges ("Den badges") from a few local counters: apps
opened, days Home was shown, rainy days, guest passes, sleep timers and the
like. Everything stays in `state.db` on the TV and only ids, counts and days
are kept, never titles or times ([`docs/security.md`](security.md#den-badges)).
Controller phones can look at the shelf; guests cannot.

On the TV: **Settings → Badges** shows the shelf (earned medals with the day,
the rest as silhouettes with a hint and progress). **Counting** turns counting
off or on; **Reset badges** asks, then deletes everything. A badge earned
while an app is in front is celebrated the next time Home appears.

```sh
build/bin/bear-den-tv badges status   # counting on/off, earned badges with their day, progress
build/bin/bear-den-tv badges off      # stop counting at once (earned badges stay)
build/bin/bear-den-tv badges on
build/bin/bear-den-tv badges reset    # delete every counter and earned badge
```

Off is stored as `achievements.enabled: false` in `config.json`
([`contracts/config.md`](../contracts/config.md)); nothing is counted while it
is off. Reset cannot be undone.

## Streaming sites and the Browser

Netflix, Disney+ and Hulu have no Linux apps; Bear Den opens their websites
full screen in Chromium and drives them with the remote. They play at up to
about 720p in a Linux browser (the services cap it). A Browser tile opens
ordinary Chromium for keyboard and mouse. Design:
[ADR 0010](decisions/0010-web-apps-over-cdp-pipe.md); what is and is not
verified: [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

1. **Chromium from Flathub:** turning a site on while Chromium is missing
   opens the install card for it (or Settings → Add apps → Chromium; one
   press, per user, see [App installs](#app-installs)). By hand:
   `flatpak install --user flathub org.chromium.Chromium`. Until it is
   installed the four tiles stay hidden. Bear Den uses only this Chromium,
   never Google Chrome.
2. **Turn a site on:** TV Settings → Streaming sites, OK on Netflix, Disney+
   or Hulu (they are off by default; the Browser is on). The tile appears on
   Home with "Up to 720p" until it is first opened. Turning a site off hides
   its tile; its profile and sign-in stay.
3. **Sign in once** inside each site: plug in a USB keyboard, or use the
   phone: the D-pad moves between fields and buttons, OK focuses a field, the
   phone's text box types into it (Send presses Enter), and the **Touchpad**
   (shown on the phone while a web app is in front: drag to move, tap to
   click, two fingers to scroll) reaches anything the D-pad cannot.
4. **Widevine:** the services need Chromium's Widevine module. Flathub's
   Chromium fetches it itself into each profile (Bear Den never downloads
   it). After Chromium is installed, and when you turn a site on, Bear Den
   starts that site's profile once, headless and out of sight, until
   `~/.local/share/bear-den-tv/web/<app-id>/WidevineCdm/*/manifest.json`
   appears (about a minute in a container; up to 5 minutes), and the install
   card says "Ready" or "Still setting up playback support". Opening the
   site also lets Chromium fetch it. To check by hand, open the Browser tile
   with a keyboard, go to `chrome://components` and look for "Widevine
   Content Decryption Module" with a version other than 0.0.0.0. Without it
   the sites open but refuse to play.

**Where things live:** each app's profile is
`~/.local/share/bear-den-tv/web/<app-id>/` (`netflix`, `disney-plus`, `hulu`,
`browser`), mode 0700. Deleting a folder signs that app out and forgets its
Widevine copy. The page each tile opens is `applications[].web.url` in
`config.json` (https on the service's own domain; for the Browser any https
start page, or none for a blank page; [`contracts/config.md`](../contracts/config.md) rule 11).
A `config.json` from before the web apps gains their rows when Bear Den
starts ([Upgrading](#upgrading)).

5. **Brave instead of Chromium** (optional,
   [ADR 0013](decisions/0013-brave-as-a-browser-choice.md)): TV Settings →
   Streaming sites, "Browser tile uses" and "Streaming sites use" (◀ ▶).
   Both start on Chromium. Brave comes from Flathub (`com.brave.Browser`,
   published by Brave Software) with the same one-press install. Brave
   itself recommends its native packages over the Flatpak, whose sandbox it
   has not vetted; those need root, so Bear Den does not install them. For
   the streaming sites Brave is **unverified** and the row says so: its
   Flatpak may not load Widevine from Bear Den's profile. Each browser has
   its own profiles (`web/` and `web-brave/`), so switching starts signed
   out and switching back finds the old sign-ins. Before each start Bear Den
   writes Brave's Widevine opt-in (`brave.widevine_opted_in`) and turns its
   welcome page, full-screen reminder, VPN, Wallet, Leo, Rewards and News
   buttons off, in its own profiles only. In config: `apps.browser` and
   `apps.streaming_browser` ([`contracts/config.md`](../contracts/config.md)).

**How Bear Den controls the browser:** it starts
`flatpak run org.chromium.Chromium --user-data-dir=… --remote-debugging-pipe
--no-first-run --no-default-browser-check --class=BearDenWeb-<adapter>
--start-fullscreen --app=<url>` (the Browser: `--start-maximized <url>`) and
talks to it over that private pipe only; nothing listens on the network.
Brave gets the same arguments, as `flatpak run
--filesystem=$XDG_DATA_HOME/bear-den-tv/web-brave com.brave.Browser …`
(its Flatpak cannot see your data folder otherwise; the grant is for that
folder and that run only).
Home pauses a playing video with the site's own pause key first. If Bear Den
was restarted while a web app was open, the phone says it is not connected:
close the app and open it again.

**Try it without the TV:** `bear-den-tv dev --dev-fixtures` shows the tiles
with a pretend page (the phone's Touchpad appears when a web app is in
front); `bear-den-tv dev --dev-browser ~/.cache/ms-playwright/chromium-*/chrome-linux64/chrome`
runs web apps in a real Chromium binary. `make test-webnav` runs the
navigation script against local fixture pages.

## App installs

Bear Den installs the apps it knows from Flathub, for the TV's user, with one
press ([ADR 0011](decisions/0011-per-user-flathub-installs.md)): no sudo, no
password, nothing system-wide.

- **On the TV:** OK on a "Not installed" tile opens the install card (size,
  "From Flathub", Install / Not now). Settings → Add apps lists every app
  Bear Den knows that is not installed, including Chromium ("Browser for
  Netflix, Disney+, Hulu"); turning a streaming site on while Chromium is
  missing offers Chromium the same way. Back hides the card; the install
  carries on and the tile shows its progress.
- **From the owner's phone:** the Add apps section (owner phones only).
- **From a terminal on the TV:** `bear-den-tv apps install moonlight` (asks the
  running Bear Den; `--here` installs without it), `bear-den-tv apps
  install-cancel moonlight`.
- **Where they go:** `~/.local/share/flatpak` (per user), from the per-user
  `flathub` remote Bear Den adds if it is missing
  (`flatpak remotes --user` shows it). Free space is checked first: the app,
  twice its runtime when that is new (GL drivers and codecs come with it),
  and 512 MiB more.
- **What it runs:** `flatpak remote-add --user --if-not-exists flathub
  https://dl.flathub.org/repo/flathub.flatpakrepo`, `flatpak remote-info
  --user flathub <id>`, `flatpak install --user --noninteractive -y flathub
  <id>`, `flatpak info --user <id>`.
- **Updates:** Settings → Keep apps up to date (on by default): once a day,
  while Bear Den is in front with no app running, `flatpak update --user
  --noninteractive -y` for the apps this user installed. Opening an app stops
  it. Apps installed system-wide are updated by the system.
- **Remove an app:** `flatpak uninstall --user <id>` (for example
  `flatpak uninstall --user com.moonlight_stream.Moonlight`), then
  `flatpak uninstall --user --unused` to drop runtimes nothing needs any
  more. Its tile goes back to "Not installed" (or away, for optional apps)
  the next time Bear Den starts.
- **Flatpak missing:** the card says "Flatpak isn't installed on this box".
  Bear Den never runs the system package manager; install it once yourself:
  `sudo apt install flatpak` (Debian, Ubuntu), then restart Bear Den.

**Tried in a container, not on the TV.** In `ubuntu:24.04` with `apt install
flatpak dbus dbus-user-session`, as an unprivileged user,
`bear-den-tv apps install moonlight --here` installed Moonlight and its KDE
runtime (418 MB to download, 2.6 GB on disk) in 16 s on a fast line, then
`flatpak uninstall --user` removed it; flatpak's real output is kept in
[`internal/applications/install/testdata`](../internal/applications/install/testdata/README.md).
flatpak needs a system D-Bus there (it asks malcontent before deploying an
app), as on any desktop. Try the TV screens without Flathub:
`bear-den-tv dev --dev-installs` (Moonlight, RetroArch, Jellyfin and Chromium
start missing; a pretend DEMO install takes 20 s).

## Further reading

- [`AGENTS.md`](../AGENTS.md): how the repository is organised and how to work in it.
- [`docs/HOW_IT_WORKS.md`](HOW_IT_WORKS.md): architecture, app control, input routing, safety,
  performance, weather.
- [`docs/THEMES.md`](THEMES.md): theme packages, decorations, bears, their performance
  rules, and how to design a theme.
- [`docs/APP_PERFORMANCE.md`](APP_PERFORMANCE.md): per-app playback settings, box tiers, the
  detection test.
- [`docs/security.md`](security.md): what is protected, and how.
