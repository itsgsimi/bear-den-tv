# internal/ and cmd/: the Go coordinator

One binary, `bear-den-tv` ([`cmd/bear-den-tv`](../cmd/bear-den-tv/main.go)), is
both the session coordinator and the local admin CLI
([ADR 0002](../docs/decisions/0002-one-cli-binary-embedded-remote.md)). It owns
the session: what is in front, launching and closing apps, routing named
actions, pairing and permissions, `config.json`, the LAN service, playback
tuning and local weather. How the pieces fit: [`docs/HOW_IT_WORKS.md`](../docs/HOW_IT_WORKS.md).
Wire shapes come from [`contracts/`](../contracts/AGENTS.md) and nowhere else.

## Package map (key files)

| Package | Owns | Key files |
|---|---|---|
| [`session`](session/coordinator.go) | the core: epoch, target, state snapshot, action routing; implements `remote.Backend` (phones) and `shellipc.Handler` (shell) | `coordinator.go` (Options, `retargetLocked`, `publish`), `route.go` (`route`, `doLaunch`, `doHome`, `doClose`), `state.go` (`buildStateFor`, `capabilitiesLocked`), `backend.go`, `ipc.go`, `tuning.go`, `nowplaying.go` (`state.now_playing`: the foreground player's reading, memory only), `power.go` (sleep timer, display off and wake: `state.power`) |
| [`contract`](contract/contract.go) | Go types for protocol 1 and JSON Schema validation of the embedded `contracts/*.json` | `contract.go`, `validate.go` |
| [`actions`](actions/dedup.go) | request de-duplication and server-side hold leases, on an injected clock | `dedup.go`, `holds.go` |
| [`applications`](applications/applications.go) | `Adapter` and `Launcher` interfaces | `applications.go` |
| [`applications/adapters`](applications/adapters/adapters.go) | per-app knowledge: Flatpak id, approved args, WM_CLASS or Wayland app_id match, action → key | `adapters.go` |
| [`applications/flatpak`](applications/flatpak/flatpak.go) | the Flatpak launcher (fixed argv `flatpak info\|run\|ps\|kill`) | `flatpak.go`, `runner.go` |
| [`applications/tuning`](applications/tuning/tuning.go) | playback detection and per-app settings ([`docs/APP_PERFORMANCE.md`](../docs/APP_PERFORMANCE.md)) | `detect.go` (`Apps` table), `plans.go` (`Plan<App>`) |
| [`config`](config/config.go) | `config.json`: defaults, validation, atomic writes, last-known-good, layout workflow | `config.go`, `validate.go`, `store.go`, `layout.go` |
| [`pairing`](pairing/pairing.go) | invitations, redemption, device/session records (`remote.Devices`) | `pairing.go`, `qr.go` |
| [`remote`](remote/server.go) | the LAN HTTP/WebSocket server: static assets, cookies, CSRF, Host/Origin, rate limits, revocation | `server.go`, `routes.go`, `ws.go`, `auth.go`, `options.go`, `backend.go` (seams) |
| [`remote/mdns`](remote/mdns/mdns.go) | Avahi advertisement, best effort; not wired into `session` yet | `mdns.go` |
| [`shellipc`](shellipc/server.go) | [`contracts/ipc.md`](../contracts/ipc.md): Unix socket to the shell and CLI; shell supervisor | `messages.go`, `server.go`, `dial.go`, `supervisor.go` |
| [`platform`](platform/platform.go) | the desktop seam (`DesktopAdapter`, lock, media, audio, `DisplayPower`) | `platform.go` |
| `platform/{x11,wayland,detect,lock,mpris,audio,dbusx,probe,suspend,fake}` | X11 EWMH+XTEST adapter and DPMS display power (`dpms.go`: captures and restores the exact DPMS state); Wayland (wlr-foreign-toplevel on wlroots, honest reasons elsewhere; ADR 0007); session detection; lock observation; MPRIS; `pactl`; narrow D-Bus; probe report; logind `CanSuspend` (asks only); in-memory desktop | one file each (x11: `adapter.go`, `keys.go`, `props.go`, `dpms.go`; wayland: `wayland.go`, `client.go`, `wire.go`) |
| [`providers`](providers/providers.go) | optional home content (`ContentProvider`, `Feed`); `plex/` connector (driven by `plexlink`), `plex/plexfake` loopback fake of plex.tv and a server (tests, `dev --dev-plex-fake`), `fixtures/` DEMO items | `feed.go`, `plex/provider.go`, `plex/account.go`, `fixtures/fixtures.go` |
| [`plexlink`](plexlink/plexlink.go) | Plex on the TV: the sign-in flow (`state.plex`, IPC `plex.*`: keyring check, link code, servers, libraries, `plex_content` written), the Plex rows (`state.content`) and their cadence on `clock.Clock` (on Home show unless refreshed within 2 min, every 10 min while Home is in front, never behind an app; backoff 30 s, 1, 2, 5, 10 min) | `plexlink.go` |
| [`secrets`](secrets/secrets.go) | connector tokens outside `config.json` (Secret Service, or memory) | `secrets.go`, `dbus.go`, `memory.go` |
| [`storage`](storage/storage.go) | SQLite: devices, session hashes, invitations, focus memory, launch state | `storage.go`, `secrets.go` |
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
| `fake.Desktop`, `fake.Launcher` | [`platform/fake`](platform/fake/fake.go) | in-memory windows (`AddWindow`, `SetActive`, `RemoveWindow`), delivered keys (`Keys()`); also backs `bear-den-tv dev` |
| `fake.Display` | [`platform/fake/display.go`](platform/fake/display.go) | display power: `Off`/`On` calls (`Calls()`), `Changed()` while settings are not restored, `Wake()` plays a TV key, `SetIdle`, an `OnOff` hook; also backs `bear-den-tv dev` |
| `fake.Media`, `fake.Player` | [`platform/fake/media.go`](platform/fake/media.go) | an MPRIS-style locator and player on an injected clock (position advances while playing; `Set`, `Signal`, `Fail`, `Reads()`, `Calls()`); `DemoMedia` backs `dev --dev-fixtures` with DEMO titles |
| `dbusx.Fake` | [`platform/dbusx/fake.go`](platform/dbusx/fake.go) | scripted `Call`/`Property`/`Names`, `Emit` signals; used by the lock and mpris tests |
| `fakeCompositor` | [`platform/wayland/wayland_test.go`](platform/wayland/wayland_test.go) | the server side of `wl_registry`/`wl_callback`/wlr foreign-toplevel over `net.Pipe`: announce globals and toplevels (`add`), send events or one flush (`w.sendBatch`), `waitRequest` for what the adapter sent. Live twin: `scripts/wayland-container-test.sh` (headless sway in Docker) |
| private D-Bus | [`platform/mpris/bus_test.go`](platform/mpris/bus_test.go) | starts a `dbus-daemon` for one test, exports a fake MPRIS player with godbus and reads it through the real `dbusx`/`mpris` code; skips with the reason when `dbus-daemon` is missing |
| `fakeRunner`, `fakeProc` | [`applications/flatpak/flatpak_test.go`](applications/flatpak/flatpak_test.go) | records every `flatpak` argv (`argvs()`), scripted output and processes |
| `testutil` | [`remote/testutil`](remote/testutil/backend.go) | `FakeBackend`, in-memory `Devices`, `FakeClock` for server tests |

Note: the session harness runs on `clock.Real` with short timeouts
(`AppsRefresh: 30ms`, `eventually` polls up to 3 s); use `clock.Fake` for pure
timing logic (as the `actions`, `pairing`, `config` and `providers` tests do).
`internal/remote` and `internal/doctor` have no tests yet, even though `remote/testutil` exists. The Plex connector is tested against [`providers/plex/plexfake`](providers/plex/plexfake/plexfake.go), a loopback stand-in for plex.tv and one Plex Media Server (PIN linking, resources, libraries, hubs, onDeck, DEMO posters; `SetDown`, `SetPhotoHandler`, `Requests()` to assert the token only ever travels in the header). Weather has its own tests: [`weather/weather_test.go`](weather/weather_test.go) (fake clock) and [`session/weather_test.go`](session/weather_test.go) (IPC search/configure and the snapshot).

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
   submit actions): `controller` for everything; `owner` for `shell.restart`
   and `app.close` with `force`. Otherwise `forbidden`.
3. **Lock**: a locked session refuses every action, `home` included (`locked`).
   Just before it, `powerGate` ([`session/power.go`](session/power.go)) wakes
   a display Bear Den turned off and cancels a sleep warning; an unlocked
   press that woke the display is swallowed (`display_off`) unless it is a
   power action.
4. **Stale epoch** (phones only; shell requests are stamped with the current
   epoch): `req.ContextEpoch != epoch` is `stale_epoch` unless `contract.IgnoresStaleEpoch` (`home`, `app.launch`, `shell.restart`).
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
its cookie from then on. A revoke from the TV (`devices.revoke` over IPC,
[`session/ipc.go`](session/ipc.go)) also cancels its holds and forgets its
de-dup entries.

## Add an app

An app is data in a few closed tables; no engine code branches on it.

1. [`applications/adapters/adapters.go`](applications/adapters/adapters.go):
   add the name and Flatpak id constants, the WM_CLASS fragments, a
   constructor and an entry in `NewRegistry`. Mark the WM_CLASS UNVERIFIED
   until a live probe records it in [`tests/compatibility/`](../tests/compatibility/).
   Map only keys you can justify; `media.*` stays unmapped until verified.
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
     Defaults only seed a fresh install: an existing `config.json` keeps its
     own `applications` list.
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
| `session`, `dev` | `session.go` (wires every package; `dev` uses `fake.Desktop`, loopback, DEMO weather; `--dev-fixtures` adds DEMO content) |
| `doctor`, `pair`, `devices`, `remote` | `cli.go` |
| `artwork fetch` | `artwork.go` |
| `autostart`, `shortcut` `enable\|disable\|status` | `autostart.go`, `shortcut.go` |
| `apps detect\|tune\|probe` | `apps.go` |
| `themes list\|validate DIR\|path` | `themes.go` |
| `weather status\|search Q\|set Q [INDEX]\|off` | `weather.go` |
| `plex status\|sign-in\|server ID\|libraries ID...\|cancel\|sign-out` | `plex.go` (also `newPlexLink`, the session wiring and `--dev-plex-fake`) |
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
go vet ./...
go test -race ./...            # or: make test-go
go test ./tests/contract ./tests/docs
```

`tests/e2e` needs `-tags e2e` and a live TV session
([`docs/operations.md`](../docs/operations.md#working-against-the-tv-machine)).
`tests/wayland` needs `-tags wayland_live` and runs inside the container of
`scripts/wayland-container-test.sh` ([`docs/operations.md`](../docs/operations.md#wayland)).
