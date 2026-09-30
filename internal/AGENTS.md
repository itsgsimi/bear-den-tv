# internal/ and cmd/: the Go coordinator

One binary, `bear-den-tv` ([`cmd/bear-den-tv`](../cmd/bear-den-tv/main.go)), is
both the session coordinator and the local admin CLI
([ADR 0002](../docs/decisions/0002-one-cli-binary-embedded-remote.md)). It owns
the session: what is in front, launching, closing and installing apps,
routing named actions, pairing and permissions, `config.json`, the LAN
service, playback tuning, and the optional parts (local weather, Plex rows,
Now playing, the sleep timer, HDMI-CEC, Den badges, web apps). How the pieces fit: [`docs/HOW_IT_WORKS.md`](../docs/HOW_IT_WORKS.md).
Wire shapes come from [`contracts/`](../contracts/AGENTS.md) and nowhere else.

## Package map (key files)

| Package | Owns | Key files |
|---|---|---|
| [`session`](session/coordinator.go) | the core: epoch, target, state snapshot, action routing; implements `remote.Backend` (phones) and `shellipc.Handler` (shell) | `coordinator.go` (Options, `retargetLocked`, `publish`), `route.go` (`route`, `doLaunch`, `doHome`, `doClose`), `homepause.go` (Home's verified MPRIS pause of a native app), `state.go` (`buildStateFor`, `capabilitiesLocked`), `web.go` (web apps: `WebApps` seam, routing to the page after a verified foreground, pointer capabilities and rate limits, Home's page pause, IPC `app.enable`), `backend.go` (phones), `ipc.go` (the shell and CLI), `tuning.go`, `plex.go` (`state.plex`, the Plex rows as `state.content`, IPC `plex.*`, telling `plexlink` what is in front), `appicons.go` (the phone icon route's side: adapter check, `ui.app_icons`, installed only), `startpage.go` (the Browser's start page: its cards, and a card opening that site as its own app), `plexplaying.go` (Now playing for Plex HTPC from the Plex server: `PlexPlaying` seam, asked every 5 s only while wanted, read-only, `plex_now_playing` in the owners' diagnostics), `nowplaying.go` (`state.now_playing`: the foreground player's reading, or the player of the app behind Home (`c.behind`, `foreground: false`; media actions must name it, `doMedia`), memory only), `power.go` (sleep timer, display off and wake: `state.power`), `cec.go` (TV control over HDMI-CEC: `state.cec`, `tv.power`, standby/wake hooks, TV volume), `achievements.go` (Den badges: the event points, `achievements.*` IPC, `state.achievements`), `widevine.go` (the streaming sites' playback support after their browser is installed: `install.drm`), `browsers.go` (IPC `apps.browser`: config `apps.browser`/`apps.streaming_browser` and every web row's `launch.app_id` in one write, ADR 0013), `setup.go` (IPC `onboarding.complete` and `autostart.configure`, `state.onboarding` and `state.autostart`; `Options.Autostart`, nil in `dev`), `install.go` (app installs: `app.install`/`app.install_cancel` for owner phones, IPC `app.install*` (with `enable`: the app whose own card was pressed is turned on when its install is done, `enableAfterInstall`) and `apps.configure`, `state.applications[].install` and `state.apps`, rediscovery after an install, the idle daily update; `app.uninstall` (owner phones, IPC twin): the adapter's Flatpak for this user only, refusing system-wide installs and running apps, logged, then rediscovered) |
| [`appicons`](appicons/appicons.go) | the apps' own icons for phone tiles (`GET /api/v1/apps/{adapter}/icon`, [`contracts/http.md`](../contracts/http.md#app-icons)): the owner's brand PNG/JPEG, then with `ui.app_icons` `app` the installed Flatpak's exported PNG when the Flatpak is the app itself; never SVG, 1 MiB and 1024 px caps, decoded and re-encoded as PNG; a one-minute cache. The coordinator side is `session/appicons.go` | `appicons.go` |
| [`contract`](contract/contract.go) | Go types for protocol 1 and JSON Schema validation of the embedded `contracts/*.json` | `contract.go`, `validate.go` |
| [`achievements`](achievements/achievements.go) | Den badges: the badge catalogue (`Badges`, data), events that move named counters (`Launched`, `HomeShown`, `Paired`, `PassIssued`, `SleepTimerSet`, `Parade`) on local calendar days of the injected clock, awards once, `Snapshot`/`Phone` for `state.achievements`, `Reset`; nothing counted while config `achievements.enabled` is false | `achievements.go` |
| [`actions`](actions/dedup.go) | request de-duplication and server-side hold leases, on an injected clock | `dedup.go`, `holds.go` |
| [`applications`](applications/applications.go) | `Adapter` and `Launcher` interfaces | `applications.go` |
| [`applications/adapters`](applications/adapters/adapters.go) | per-app knowledge: Flatpak id, approved args, WM_CLASS or Wayland app_id match, action → key; the browser table for web apps (`Browsers`, `RunsIn`, `InstallableFlatpakIDs`) | `adapters.go` |
| [`applications/flatpak`](applications/flatpak/flatpak.go) | the Flatpak launcher (fixed argv `flatpak info\|run\|ps\|kill`) | `flatpak.go`, `runner.go` |
| [`applications/install`](applications/install/install.go) | one-press app installs from Flathub, per user, no root ([ADR 0011](../docs/decisions/0011-per-user-flathub-installs.md)): fixed argv `flatpak remote-add\|remote-info\|info\|install\|update\|uninstall --user` (uninstall: the owner's Remove, `--delete-data` on request, then `--unused` for the user's shared parts nothing else needs; never `--system`), the one remote URL a constant, ids only from the adapter table, x86_64 only; sizes and the free-space check, phases from flatpak's lines, percent from the disk filling, cancel (SIGTERM then SIGKILL to the process group), plain-words failures, updates | `install.go`, `runner.go`, `testdata/` (real flatpak output from a container) |
| [`applications/web`](applications/web/web.go) | web apps (streaming sites, the Browser tile): Google Chrome (the streaming sites) and Brave (the Browser tile) from Flathub, either by the owner's choice (the adapter table's browsers, ADR 0013, ADR 0014; Chromium retired), per app with its own profile per browser and its own window class (`prefs.go` writes the browser row's prefs, only into Bear Den's own profiles), the DevTools pipe (`--remote-debugging-pipe`, no port), the navigation script in the `bearden` isolated world, the closed set of trusted input it performs ([ADR 0010](../docs/decisions/0010-web-apps-over-cdp-pipe.md)) | `web.go` (profile dir, argv, `AllowedKeys`), `cdp.go` (the pipe client), `browser.go` (`Manager`: Launch, Apply, Pointer, PauseIfPlaying, Close), `script.go` (embedded `nav.js` + hints), `start.go` + `start.html` (the Browser tile's start page: written into its profile root, cards that ask the coordinator to open a site, `OpenRequest`), `widevine.go` (`Widevine`: for Chrome, is the bundled `files/extra/WidevineCdm/manifest.json` in the installed Flatpak; for Brave, is `<profile>/WidevineCdm/*/manifest.json` there, and the quiet headless first run that lets Brave fetch it) |
| [`applications/tuning`](applications/tuning/tuning.go) | playback detection and per-app settings ([`docs/APP_PERFORMANCE.md`](../docs/APP_PERFORMANCE.md)) | `detect.go` (`Apps` table), `plans.go` (`Plan<App>`) |
| [`config`](config/config.go) | `config.json`: defaults, validation, atomic writes, last-known-good, layout workflow, the upgrade that adds apps a newer version knows and moves a Chromium-era file to Chrome and Brave (`UpgradeBrowsers`, ADR 0014) | `config.go`, `validate.go`, `store.go`, `layout.go`, `upgrade.go` |
| [`pairing`](pairing/pairing.go) | invitations, redemption, device/session records (`remote.Devices`), guest passes (`IssuePass`, `PassEnd`, the expiry sweep on the injected clock) | `pairing.go`, `guest.go`, `qr.go` |
| [`remote`](remote/server.go) | the LAN HTTP/WebSocket server: static assets, cookies, CSRF, Host/Origin, rate limits, revocation, the app icon route (`handleAppIcon`, seam `AppIconSource`) | `server.go`, `routes.go`, `ws.go`, `auth.go`, `options.go`, `backend.go` (seams) |
| [`remote/mdns`](remote/mdns/mdns.go) | Avahi advertisement, best effort: the remote host in `cmd/bear-den-tv/session.go` (`mdnsPlan`) publishes it on the selected interfaces while the consented LAN listener is up (`remote.mdns`), never in dev, and withdraws it with the listener | `mdns.go`, `advertiser.go` |
| [`shellipc`](shellipc/server.go) | [`contracts/ipc.md`](../contracts/ipc.md): Unix socket to the shell and CLI; shell supervisor | `messages.go`, `server.go`, `dial.go`, `supervisor.go` |
| [`platform`](platform/platform.go) | the desktop seam (`DesktopAdapter`, lock, media, audio, `DisplayPower`, `TVControl`) | `platform.go` |
| [`platform/cec`](platform/cec/cec.go) | HDMI-CEC through the kernel CEC API: ioctl on `/dev/cecN`, no cgo ([ADR 0008](../docs/decisions/0008-hdmi-cec.md)); not seen on hardware | `cec.go`, `kernel.go` (structs, ioctl numbers), `msg.go` (frames) |
| [`platform/autostart`](platform/autostart/autostart.go) | the user's XDG autostart entry (`File`, `StartScript`/`StartScriptFor` for the checkout and installed layouts, `DesktopEntry`, `Enable`, `Disable`, `Enabled`); used by `bear-den-tv autostart` and the TV's "Start with this PC" (`session/setup.go`) | `autostart.go` |
| `platform/{x11,wayland,detect,lock,mpris,proc,audio,dbusx,probe,suspend,fake}` | X11 EWMH+XTEST adapter and DPMS display power (`dpms.go`: captures and restores the exact DPMS state); Wayland (wlr-foreign-toplevel on wlroots, honest reasons elsewhere; ADR 0007); session detection; lock observation; MPRIS (a player belongs to an app by the process owning its bus name: `Locator.WithProcesses`, `platform.MediaMatch`); a process's Flatpak app id and ancestry from `/proc`, same user only (`proc`); `pactl`; narrow D-Bus; probe report; logind `CanSuspend` (asks only); in-memory desktop | one file each (x11: `adapter.go`, `keys.go`, `props.go`, `dpms.go`; wayland: `wayland.go`, `client.go`, `wire.go`) |
| [`providers`](providers/providers.go) | optional home content (`ContentProvider`, `Feed`); `plex/` connector (driven by `plexlink`; `sessions.go`: `/status/sessions` and `ThisPlayer`), `plex/plexfake` loopback fake of plex.tv and a server (tests, `dev --dev-plex-fake`), `fixtures/` DEMO items | `feed.go`, `plex/provider.go`, `plex/account.go`, `fixtures/fixtures.go` |
| [`plexlink`](plexlink/plexlink.go) | Plex on the TV: `NowPlaying` (`nowplaying.go`: this TV's Plex HTPC session on the chosen server, `LocalAddrs`); the sign-in flow (`state.plex`, IPC `plex.*`: secret store check (keyring, else private file; `stored_in`), link code, servers, libraries, `plex_content` written), the Plex rows (`state.content`) and their cadence on `clock.Clock` (on Home show unless refreshed within 2 min, every 10 min while Home is in front, never behind an app; backoff 30 s, 1, 2, 5, 10 min) | `plexlink.go`, `nowplaying.go` |
| [`secrets`](secrets/secrets.go) | connector tokens outside `config.json`: the Secret Service when unlocked, else a private file (`$XDG_DATA_HOME/bear-den-tv/secrets/plex-<ref>`, 0700/0600, links refused; `Fallback`, `Locator` for `state.plex.stored_in`), or memory in tests | `secrets.go`, `dbus.go`, `file.go`, `fallback.go`, `memory.go` |
| [`storage`](storage/storage.go) | SQLite: devices (with a guest pass end since schema 2), session hashes, invitations, focus memory, launch state, Den badge counters and earned badges (schema 3: ids, counts and days only, enforced by CHECKs) | `storage.go`, `secrets.go`, `achievements.go` |
| [`themes`](themes/themes.go) | theme packages for phones and `themes validate` (Go twin of the shell's `ThemeRegistry`) | `themes.go` |
| [`weather`](weather/openmeteo.go) | local weather: Open-Meteo client (constant endpoints, no key), WMO code mapping, poller on `clock.Clock` (`PollInterval` 30 min, retries 5, 10, 20, 30 min, `MaxAge` 6 h) behind `state.weather` (shell view only), DEMO `Fixture` for `dev` | `openmeteo.go`, `poller.go`, `fixture.go` |
| [`doctor`](doctor/doctor.go) | `bear-den-tv doctor` and the owners' diagnostics | `doctor.go` |
| [`clock`](clock/clock.go) | `Clock` interface; `Real` and `Fake` | `clock.go`, `fake.go` |
| [`cmd/bear-den-tv`](../cmd/bear-den-tv/main.go) | CLI dispatch and `session`/`dev` wiring | `main.go`, `session.go`, `cli.go`, one file per subcommand group |
| [`cmd/bdtv-probe`](../cmd/bdtv-probe/main.go) | prints the probe report as JSON on the TV | `main.go` |

Embeds live in the repository root [`embed.go`](../embed.go): `contracts/`,
`apps/remote-web/dist`, `themes/` and the built-in ornament SVGs.

## Fakes and test helpers

| Fake | Where | Use |
|---|---|---|
| `clock.Fake` | [`clock/fake.go`](clock/fake.go) | `NewFake(t0)`, `Advance(d)` fires timers in order; `Pending()`, `NextDeadline()` |
| `newHarness(t, configure...)` | [`session/session_test.go`](session/session_test.go) | a real `Coordinator` on `fake.Desktop`, `fakeLock`, `fakeLauncher`, an on-disk `config.Store`, in-memory SQLite, a real shell socket. `h.req(action, args)` builds a request at the current epoch, `h.submit(viewer, req)`, `h.eventually(what, cond)`, viewers `h.ctl` (controller) and `h.owner` |
| `fakeShell` | same file | answers `input`/`home` like the real shell (`observed` with focus detail), records inputs in `h.inputs` |
| `fakeLauncher`, `fakeLock`, `fakeTuner` | same file | launching maps a window on the fake desktop; lock pushes through `set`; the tuner returns a canned report and records apps it applied |
| `fakePlexPlaying`, `newPlexNPHarness` | [`session/plexplaying_test.go`](session/plexplaying_test.go) | a scripted Plex server reading on the fake clock; `pass(d)` advances (or kicks) and waits until the loop is idle again (`plexNP.loop.idle`), then returns the asks sent: no sleeps |
| `plexHarnessWith`, `observedPlex` | [`session/plex_test.go`](session/plex_test.go) | the real Plex manager against `plexfake`, with what the coordinator last told it is in front (`front()`) and every loop decision (`plexlink.Options.OnPass`): wait on these, not on `Target()` or a sleep |
| `fakeWeb`, `lyingDesk` | [`session/web_test.go`](session/web_test.go) | a web manager that maps a window with the adapter's class and records what reached the page (its launch can wait at a `gate` or fail); a desktop that names another window when the foreground is re-read. `h.reconciled()` waits until the apps reconcile pass running now has finished (`fakeLauncher` counts its `Instances` calls), `activations` records every window the fake desktop activated |
| `fakeInstaller`, `scopedLauncher` | [`session/install_test.go`](session/install_test.go) | an installer that records starts, cancels and updates (`calls()`) and whose statuses the test sets (`set`, which also publishes); a launcher whose discovery answers per Flatpak id (`setScope`) |
| `fake.Installer` | [`platform/fake/installer.go`](platform/fake/installer.go) | the pretend Flathub behind `bear-den-tv dev --dev-installs`: `fake.DemoMissing` start missing, an install walks its states on a timer and then `SetInstalled` |
| `fake.Web` | [`platform/fake/web.go`](platform/fake/web.go) | the pretend web manager behind `bear-den-tv dev` (a DEMO page, no browser); `dev --dev-browser PATH` runs the real manager on a Chromium binary instead |
| `fakeChromium` | [`applications/web/pipe_test.go`](applications/web/pipe_test.go) | the other end of the DevTools pipe: scripted replies, events, and a record of every command |
| `webtest.Require` | [`applications/web/webtest`](applications/web/webtest/webtest.go) | the real Chromium for the web-app end-to-end tests (`BDTV_TEST_CHROMIUM`, else Playwright's download), started once first: missing means skip, present but unable to start means fail with Chromium's own error |
| `fake.Desktop`, `fake.Launcher` | [`platform/fake`](platform/fake/fake.go) | in-memory windows (`AddWindow`, `SetActive`, `RemoveWindow`), delivered keys (`Keys()`); also backs `bear-den-tv dev` |
| `fake.Display` | [`platform/fake/display.go`](platform/fake/display.go) | display power: `Off`/`On` calls (`Calls()`), `Changed()` while settings are not restored, `Wake()` plays a TV key, `SetIdle`, an `OnOff` hook; also backs `bear-den-tv dev` |
| `fake.TV` | [`platform/fake/tv.go`](platform/fake/tv.go) | HDMI-CEC TV: power state changed by `PowerOn`/`Standby`, `Calls()` in order, `SetCapability` (no adapter), `Fail`, `Hang` (until the caller's deadline, or until `Hang(false)` releases it), `SetAnswers`; also backs `bear-den-tv dev`. Wait for background HDMI-CEC sends with `c.waitCECIdle()`, not a sleep |
| `fakeBus` | [`platform/cec/cec_test.go`](platform/cec/cec_test.go) | a fake ioctl layer under the real `cec.Adapter`: decodes the kernel structs, acknowledges by logical address, answers power status, can hang or be unplugged |
| `fake.Media`, `fake.Player` | [`platform/fake/media.go`](platform/fake/media.go) | an MPRIS-style locator (players added under the owning Flatpak id, or `fake.ProcessKey(pid)` for a web app's browser) and player on an injected clock (position advances while playing; `Set`, `Signal`, `Fail`, `SetStuck` (accepts calls, changes nothing), `Reads()`, `Calls()`); `DemoMedia` backs `dev --dev-fixtures` with DEMO titles |
| `dbusx.Fake` | [`platform/dbusx/fake.go`](platform/dbusx/fake.go) | scripted `Call`/`Property`/`Names`, `Emit` signals; used by the lock and mpris tests |
| `fakeCompositor` | [`platform/wayland/wayland_test.go`](platform/wayland/wayland_test.go) | the server side of `wl_registry`/`wl_callback`/wlr foreign-toplevel over `net.Pipe`: announce globals and toplevels (`add`), send events or one flush (`w.sendBatch`), `waitRequest` for what the adapter sent. Live twin: `scripts/wayland-container-test.sh` (headless sway in Docker) |
| private D-Bus | [`platform/mpris/bus_test.go`](platform/mpris/bus_test.go) | starts a `dbus-daemon` for one test, exports a fake MPRIS player with godbus and reads it through the real `dbusx`/`mpris` code (`TestRealBusObservedVacuumTube`: the owner's pid from the real daemon, only the pid-to-Flatpak lookup faked); skips with the reason when `dbus-daemon` is missing |
| `fakeProcs`, `pidBus` | [`platform/mpris/owner_test.go`](platform/mpris/owner_test.go) | pid → Flatpak id and parent; `dbusx.Fake` answering `GetConnectionUnixProcessID` per bus name. `observedBus`/`ownerProcs` in [`session/owner_media_test.go`](session/owner_media_test.go) put the real `mpris.Locator` under the coordinator |
| fake `/proc` | [`platform/proc/proc_test.go`](platform/proc/proc_test.go) | a temp directory with `<pid>/stat`, `<pid>/root/.flatpak-info` and `<pid>/cgroup` |
| `fakeRunner`, `fakeProc` | [`applications/flatpak/flatpak_test.go`](applications/flatpak/flatpak_test.go) | records every `flatpak` argv (`argvs()`), scripted output and processes |
| `fakeRunner`, `fakeProc`, `disk` | [`applications/install/install_test.go`](applications/install/install_test.go) | the installer's flatpak: answers from a table keyed by argv (real captured output in `testdata/`), started processes the test feeds lines to and ends (`line`, `exit`; SIGTERM ends it unless `ignoreTerm`), a fake free-space reading (`use`); `checkArgvs` rejects any argv without `--user` or with a weakening flag |
| `testutil` | [`remote/testutil`](remote/testutil/backend.go) | `FakeBackend`, in-memory `Devices`, `FakeClock` for server tests |

Note: the session harness runs on `clock.Real` with short timeouts
(`AppsRefresh: 30ms`, `eventually` polls up to 3 s; `eventuallyWithin` is
only for waits on a real Chromium, in `web_e2e_test.go`); use `clock.Fake` for pure
timing logic (as the `actions`, `pairing`, `config` and `providers` tests do).
Known test gaps (checked 2026-09-29): `internal/remote` has only a route-table test ([`remote/routes_test.go`](remote/routes_test.go)) and the app icon route's ([`remote/appicon_test.go`](remote/appicon_test.go): the real server with `testutil` fakes); [`pairing/guest_remote_test.go`](pairing/guest_remote_test.go) drives the real server with `remote/testutil` and a WebSocket to prove a guest pass ends with close 4001; there is no negative suite for cookies, CSRF, Host/Origin or rate limits. `internal/doctor` has no tests yet, and the shell supervisor's restart path has none ([`shellipc/supervisor_test.go`](shellipc/supervisor_test.go) checks the environment only). The Plex connector is tested against [`providers/plex/plexfake`](providers/plex/plexfake/plexfake.go), a loopback stand-in for plex.tv and one Plex Media Server (PIN linking, resources, libraries, hubs, onDeck, DEMO posters, `/status/sessions` in the public API's shape; `SetDown`, `SetPhotoHandler`, `SetSessions`, `SetSessionsStatus`, `Requests()` to assert the token only ever travels in the header). Weather has its own tests: [`weather/weather_test.go`](weather/weather_test.go) (fake clock) and [`session/weather_test.go`](session/weather_test.go) (IPC search/configure and the snapshot).

## How the coordinator fails closed

Every action enters `submit` in [`session/route.go`](session/route.go): the
de-dup cache first (replay returns the stored result, a reused id with a
different body is `duplicate_mismatch`), then `route()`, which checks in order:

1. **Protocol**: anything but 1 is `unsupported_protocol`. Phone requests were
   already validated against `action.schema.json` by the HTTP layer
   (`contract.ValidateActionRequest`); shell requests (IPC `request`,
   [`session/ipc.go`](session/ipc.go)) are not, and an unknown name falls
   through to `unsupported` at the end of `route()`.
2. **Permission** (phones only; the shell is trusted, CLI clients may not
   submit actions), `phoneMay` in `route.go`: a guest pass may send only
   `contract.GuestActions` and nothing once its pass has ended; everyone else
   needs `controller`, and `owner` for `contract.OwnerActions`
   (`shell.restart`, `app.install`, `app.install_cancel`) and `app.close`
   with `force`. Otherwise `forbidden`. A new action is refused to guests until it
   is added to `GuestActions` on purpose.
3. **Lock**: a locked session refuses every action, `home` included (`locked`).
   Just before it, `powerGate` ([`session/power.go`](session/power.go)) wakes
   a display Bear Den turned off and cancels a sleep warning; an unlocked
   press that woke the display is swallowed (`display_off`) unless it is a
   power action.
4. **Stale epoch** (phones only; shell requests are stamped with the current
   epoch): `req.ContextEpoch != epoch` is `stale_epoch` unless `contract.IgnoresStaleEpoch` (`home`, `app.launch`, `shell.restart`, the power actions `power.sleep_timer`, `display.off` and `tv.power`, and `app.install`/`app.install_cancel`).
5. **Capability**: input, media and audio go through `capability()`, which reads
   `capabilitiesLocked()` in [`state.go`](session/state.go), the single source of
   truth phones also see. Unavailable maps to `unknown_foreground`, `no_target`,
   `locked` or `unsupported` with the capability's reason. Keys go only to an
   app window matched by its adapter, and only for mapped actions.

The target comes from `retargetLocked()` in
[`coordinator.go`](session/coordinator.go): locked, else unknown foreground,
else the shell window (by pid, WM_CLASS or app_id), else a configured app whose adapter
matches WM_CLASS or app_id, else `unknown` ("Another window"). A change of target kind/app
or of lock state bumps the epoch and cancels every hold.

**Revocation:** `pairing.Service.Revoke` marks the device revoked in storage
and publishes the id on the channel from `Revocations(ctx)`; `remote.Server.revocationLoop` sends
`revoked` and closes that device's sockets with 4001; `authenticate` fails for
its cookie from then on. The coordinator watches the same channel
(`watchRevocations` in [`session/coordinator.go`](session/coordinator.go)) and
cancels the device's holds and forgets its de-dup entries, whoever revoked it.
A guest pass that ends is revoked through this same path
([`pairing/guest.go`](pairing/guest.go)).

## Add an app

An app is data in a few closed tables; no engine code branches on it.

1. [`applications/adapters/adapters.go`](applications/adapters/adapters.go):
   add the name and Flatpak id constants, the WM_CLASS fragments, a
   constructor and an entry in `NewRegistry`. In the constructor: the key
   map from the app's documented keyboard controls (a new logical key goes in
   `platform.Key` and the X11 keysym table), `media` when its MPRIS name is
   not the Flatpak id, and `home` (`HomePause`: none, mpris or a pause key; Home acts only on mpris, verified before and after, `session/homepause.go`). Mark the WM_CLASS UNVERIFIED
   until a live probe records it in [`tests/compatibility/`](../tests/compatibility/).
   Map only keys you can justify; `media.*` stays unmapped until verified.
   Write its notes (`notes:` in the constructor, a list beside the others):
   one to five short plain sentences the TV and phones show
   (`state.applications[].notes`, [`contracts/http.md`](../contracts/http.md#app-notes-stateapplicationsnotes)):
   what it needs, what the remote reaches, whether Home pauses it. Check
   each fact against the code or docs, and say when Home leaves it running
   (`TestNotes` checks that and the limits).
2. [`applications/adapters/adapters_test.go`](applications/adapters/adapters_test.go):
   add the name to the list `Names()` must return.
3. [`config/validate.go`](config/validate.go) `DefaultAdapters`: the same
   Flatpak id and the approved launch args (config rule 3).
4. Contracts:
   - the `adapter` enum in [`config.schema.json`](../contracts/config.schema.json);
   - rule 3 in [`config.md`](../contracts/config.md);
   - to ship it by default, a row in
     [`config.default.valid.json`](../contracts/fixtures/config.default.valid.json).
     That fixture *is* `config.Defaults()`; `TestDefaultsMatchFixture` in
     [`config/config_test.go`](config/config_test.go) checks the app count.
     Defaults seed a fresh install; an existing `config.json` gains the new
     row when the coordinator starts (`config/upgrade.go`,
     `Store.UpgradeApps`: missing default apps appended as the defaults
     have them, nothing else changed). An optional app (tile only when installed)
     sets `"hide_when_missing": true` in its row; the coordinator then marks
     it `hidden` in the state while it is missing (`appStatesLocked`).
5. Playback settings: [`docs/APP_PERFORMANCE.md` → Adding an app](../docs/APP_PERFORMANCE.md#adding-an-app).
6. The shell: [`apps/tv-shell/AGENTS.md` → App tiles and branding](../apps/tv-shell/AGENTS.md#app-tiles-and-branding).
7. Run the tests:

   ```sh
   go test -race ./internal/... ./tests/contract
   ```

8. Record the real WM_CLASS on your TV (the app must be installed and open):

   ```sh
   scripts/deploy-target.sh --no-restart     # ships build/bin/bdtv-probe too
   scripts/target.sh ssh 'DISPLAY=:0 build/bin/bdtv-probe' > /tmp/probe.json
   ```

A **web app** (a website in Google Chrome or Brave) is also a row: `newWeb(name, mode, hints)` in
`adapters.go`, a `WebRule` (allowed hosts) in `config.DefaultAdapters`, a hints
file in [`apps/web-nav/hints/`](../apps/web-nav/AGENTS.md) (UNVERIFIED until
checked on the real site), and its row in the defaults with `web.url`,
`hide_when_missing` and, for a streaming service, `enabled: false`
([`contracts/config.md`](../contracts/config.md) rule 11).

Done when: the tests pass, the WM_CLASS in `adapters.go` matches the probe
(or is marked UNVERIFIED), and, if it ships by default, a fresh `make dev` shows its tile.

## Add an action

Contract side first: [`contracts/AGENTS.md` → Add an action](../contracts/AGENTS.md#add-an-action).
Then in Go:

1. [`contract/contract.go`](contract/contract.go): the `Action*` constant,
   its place in `AllActions`, and `IgnoresStaleEpoch` (an escape) or `IsNav`
   (holdable) if it is one.
2. [`session/route.go`](session/route.go) `route()`: the permission (default
   `controller`), then a `case` that routes it. Synchronous work returns
   `delivered`/`observed`. Slow work uses `c.async`: it returns `accepted`
   and pushes the final result later. Never report `observed` without
   evidence.
3. [`session/state.go`](session/state.go) `capabilitiesLocked()`: an entry
   with an honest reason whenever it is unavailable (locked is handled for
   all).
4. A test in [`session/session_test.go`](session/session_test.go) with
   `newHarness`: allowed, forbidden, locked, stale epoch, unavailable.
5. Run `go test -race ./internal/session ./internal/contract ./tests/contract`.

Done when: those tests pass and each of the five cases was seen failing
against deliberately broken routing.

## Add a config field

1. [`contracts/config.schema.json`](../contracts/config.schema.json) (objects
   are `additionalProperties: false`), and
   [`config.md`](../contracts/config.md) if it carries a semantic rule.
2. The struct in [`config/config.go`](config/config.go). An optional field is
   a pointer with `omitempty` and a helper for its default (see
   `Startup.TuneApps` / `AutoTune()`, or `Weather` / `WeatherSettings()`), so
   existing files still load. Deep-copy pointers and slices in `Clone()`.
3. Semantic checks in `ValidatePortable` (host-independent) or `ValidateHost`
   in [`config/validate.go`](config/validate.go), with a negative test in
   [`config/config_test.go`](config/config_test.go) and, for a rule phones or
   other tools should know, an `.invalid.json` fixture (see
   `config.weather-precise.invalid.json`).
4. The default in [`contracts/fixtures/config.default.valid.json`](../contracts/fixtures/config.default.valid.json)
   if the field ships with a value.
5. Layout (`ui`/`sections`) fields are shared with phones and the shell:
   follow [`contracts/AGENTS.md` → Change a contract](../contracts/AGENTS.md#change-a-contract).
6. Run `go test -race ./internal/config ./tests/contract`.

Done when: a `config.json` without the field still loads, the negative test
fails when the check is removed, and the fixtures pass in Go and TypeScript.

## Add a state field

The state snapshot is what the shell and phones see
([`state.schema.json`](../contracts/state.schema.json)). `weather` is a recent
example: shell view only, omitted while locked.

1. [`contracts/state.schema.json`](../contracts/state.schema.json) and the
   `state.*.valid.json` fixtures in [`contracts/fixtures/`](../contracts/fixtures/README.md).
2. The type in [`contract/contract.go`](contract/contract.go) (`State` or a
   nested struct; optional ones are pointers with `omitempty`).
3. Fill it in `buildStateFor` ([`session/state.go`](session/state.go)) and
   decide per view: `viewShell`, `viewPhone` (by permission),
   `viewAnonymous`, and what a locked session hides.
4. Shell `SessionModel` and remote-web `contract.ts`
   ([`contracts/AGENTS.md` → Change a contract](../contracts/AGENTS.md#change-a-contract)).
5. A session test that the field appears (and is absent) in the right views;
   `contract.MarshalAndValidateState` proves the result matches the schema.
6. Run `go test -race ./internal/session ./internal/contract ./tests/contract`.

Done when: those tests pass and the phone and shell checks in
[`contracts/AGENTS.md`](../contracts/AGENTS.md#checks) pass.

## CLI subcommands

Dispatch is the `switch` in [`cmd/bear-den-tv/main.go`](../cmd/bear-den-tv/main.go);
keep its header comment and `usage()` text in step with it.

| Command | File |
|---|---|
| `session`, `dev` | `session.go` (wires every package; `daemon.go`: `session` ignores SIGTSTP/SIGTTIN/SIGTTOU; `dev` uses `fake.Desktop`, loopback, DEMO weather, a pretend web app page; `--dev-fixtures` adds DEMO content; `--dev-browser PATH` runs web apps in a real Chromium-engine binary, `webdev.go`; `--dev-installs` a pretend Flathub, `fake.Installer`) |
| `doctor [--probe\|--ping]` (`--ping`: the watchdog's liveness check, `shellipc.CheckAlive`), `pair [--guest tonight\|24h\|7d]`, `devices [revoke ID\|*]`, `remote enable --interface IF --accept-lan-exposure\|disable` | `cli.go` |
| `autostart`, `shortcut` `enable\|disable\|status` | `autostart.go`, `shortcut.go` (the autostart entry and the start script: `internal/platform/autostart`) |
| `apps detect\|tune\|probe` | `apps.go` |
| `apps install APP-ID [--here]\|install-cancel APP-ID` | `appinstall.go` (IPC `app.install` and following `state`; `--here` runs `internal/applications/install` in-process) |
| `themes list\|validate DIR\|path` | `themes.go` |
| `weather status\|search Q\|set Q [INDEX]\|off` | `weather.go` |
| `plex status\|sign-in\|server ID\|libraries ID...\|cancel\|sign-out` | `plex.go` (also `newPlexLink`, the session wiring and `--dev-plex-fake`) |
| `badges status\|on\|off\|reset` | `badges.go` |
| `version` | `main.go` |

Commands that act on the running coordinator send one typed `shellipc`
message over the socket with `call()` in `cli.go` (as a trusted `cli`
client). They never edit its files behind its back. Nothing here installs
system packages, autostart entries or firewall rules without the owner
asking.

### Add a CLI subcommand

1. A new file `cmd/bear-den-tv/<name>.go` with a
   `cmd<Name>(args []string) error` that parses a `flag.FlagSet`.
2. In [`main.go`](../cmd/bear-den-tv/main.go): a `case` in the `switch`, a
   line in `usage()` and a line in the header comment.
3. If it talks to the running coordinator: use `call()` from
   [`cli.go`](../cmd/bear-den-tv/cli.go) with an existing `shellipc` message,
   or add one (the `ipc.md` spec, [`shellipc/messages.go`](shellipc/messages.go)
   and a `case` in `Receive` in [`session/ipc.go`](session/ipc.go)).
4. Run `go build ./cmd/... && go test -race ./cmd/... ./internal/session`.

Done when: `build/bin/bear-den-tv` with no arguments lists it (after
`make go`), and the command works against `make dev`.

## Checks

```sh
. scripts/env.sh
gofmt -l cmd internal          # must print nothing
make lint                      # includes go vet over our packages
make test-go                   # go test -race over our packages
go test ./tests/contract ./tests/docs
```

Prefer the make targets to a bare `go test ./...` or `go vet ./...`: those
also walk into `apps/*/node_modules`, where an npm package may ship Go code
(`flatted/golang`) that is not ours. The Makefile's `GO_PKGS` is `go list
./...` without `node_modules`.

`tests/e2e` needs `-tags e2e` and a live TV session
([`docs/operations.md`](../docs/operations.md#working-against-the-tv-machine)).
`tests/wayland` needs `-tags wayland_live` and runs inside the container of
`scripts/wayland-container-test.sh` ([`docs/operations.md`](../docs/operations.md#wayland)).
