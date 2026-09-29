# Operations

Every command here exists in the current tree. What has been seen working on a real TV is recorded in [`docs/IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

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
| State (devices, sessions, focus memory) | `$XDG_DATA_HOME/bear-den-tv/state.db` |
| Artwork cache | `$XDG_CACHE_HOME/bear-den-tv/artwork/` |
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
  `v1.2.0-3-gabc1234` → `1.2.0+git3.gabc1234`, no tag yet → `0.1.0~git.<sha>`).
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

## Phone remote

```sh
build/bin/bear-den-tv remote enable --interface <interface> --accept-lan-exposure   # or Settings → Phone remote on the TV
build/bin/bear-den-tv pair            # show a pairing code/URL
build/bin/bear-den-tv pair --guest tonight   # a guest pass: until 04:00 tomorrow morning (also 24h, 7d)
build/bin/bear-den-tv devices [revoke <id|*>]
```

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

## Further reading

- [`AGENTS.md`](../AGENTS.md): how the repository is organised and how to work in it.
- [`docs/HOW_IT_WORKS.md`](HOW_IT_WORKS.md): architecture, app control, input routing, safety,
  performance, weather.
- [`docs/THEMES.md`](THEMES.md): theme packages, decorations, bears, their performance
  rules, and how to design a theme.
- [`docs/APP_PERFORMANCE.md`](APP_PERFORMANCE.md): per-app playback settings, box tiers, the
  detection test.
- [`docs/security.md`](security.md): what is protected, and how.
