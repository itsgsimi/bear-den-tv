# Bear Den TV validation report

What has actually been checked, where and with what result. Allowed outcomes are PASS, FAIL, BLOCKED, NOT RUN and NOT APPLICABLE. A skipped check is never a pass. The requirement-by-requirement view is in [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

Updated 2026-09-29 (the build under test and the checks on the TV are still those of 2026-09-23).

## Build under test

- Branch `coordinator-shell-remote`, commit `552aefd`.
- Toolchain (user-space, `scripts/bootstrap-toolchain.sh`): Go 1.24, Qt 6.8.4, CMake, Node 22.
- The TV: the reference box, Linux Mint 21.3 Xfce on X11, Haswell graphics.

## Automated checks

These run on the workstation with `make test` unless noted.

| Area | Command | Where the tests are |
|---|---|---|
| Go (with `-race`) | `make test` | `*_test.go` beside the code; `tests/contract`, `tests/docs` |
| Phone remote (Vitest) | `make test` | `apps/remote-web/tests/unit/`, `apps/remote-web/tests/contract.spec.ts` |
| Web apps' navigation script (Playwright, local fixture pages) | `make test` (`make test-webnav`) | `apps/web-nav/tests/` |
| Phone remote in a browser (Playwright against `bear-den-tv dev --dev-fixtures --no-shell`) | `make test` (`make test-web`) | `apps/remote-web/tests/e2e/` |
| TV shell (offscreen) | `make test` | `apps/tv-shell/tests/tst_shell.cpp` |
| Idle drawing budget | `make perf` | `scripts/perf-sandbox.sh` |
| Live end-to-end (on the TV) | `scripts/e2e-target.sh` | `tests/e2e/target_test.go` |
| The .deb in clean containers (Docker) | `packaging/smoke-deb.sh ubuntu:22.04` / `ubuntu:24.04` | [`packaging/smoke-deb.sh`](../packaging/smoke-deb.sh) |
| Wayland on headless sway (Docker) | `scripts/wayland-container-test.sh [--shell]` | `tests/wayland/` |
| CI on GitHub | push or pull request | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) |

No results are recorded here for today's tree: run the commands above to get them. Container results from 2026-09-28 (the .deb, Wayland, app installs against real Flathub, the CI and release replays) are in [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md). Known gaps (checked 2026-09-29): `internal/remote` has only a route-table test and the app icon route's test, `internal/doctor` has no tests, the phone remote's browser suite is a single flow.

## Checks on the TV

| Date | What | Scenario | Outcome | Notes |
|---|---|---|---|---|
| 2026-09-22 | Target probe | X11/EWMH/XTEST activate, observe, input | PASS | [`tests/compatibility/2026-09-22-reference-box-live-probe.json`](../tests/compatibility/2026-09-22-reference-box-live-probe.json) |
| 2026-09-22 | Fail closed while locked | `app.launch`, `nav.right`, `home` from a paired phone | PASS | All `failed/locked` while the lock observer reported locked |
| 2026-09-22 | Autostart and watchdog | `autostart enable`; `kill -9` the coordinator; `start-session.sh stop` | PASS | Back in about 1 s with one shell; stop leaves no processes. Logout and reboot not tried (the box hosts other home services) |
| 2026-09-22 | End-to-end suite (commit `f087d7b`) | Pair; tap and short hold; per app: launch, verified front and fullscreen, 4 keys delivered, Home to the shell with the same focus, re-activate, close | PASS 5/5 (51 s) | WM_CLASS seen: Plex `plex-bin`, VacuumTube `vacuumtube`, Moonlight `moonlight` |
| 2026-09-22 | Shell CPU, before the heartbeat | Home awake / resting / app in front | Measured | About 27-29% awake, 0.5% resting, 0.0% behind YouTube |
| 2026-09-22 | Shell CPU, after the heartbeat (commit `6c5d9af`) | Home, Den theme, `scripts/measure-target.sh` | Measured | 6.8% awake, 11.2% resting (a bear visit may have fallen in the window), down from 60% |
| 2026-09-23 | Weather fetch (commit `bc0d2a1`) | `bear-den-tv weather search` and forecast on the TV's coordinator | PASS | Real geocoding results and a `ready` forecast. The chip on screen was not seen |

NOT RUN on the TV: playing real content in Plex or VacuumTube from the phone, the pixel-art UI, bears, Advanced playback, the Plex sign-in and rows, TV off/on and sleep, packaging, and everything merged after 2026-09-23 (Now playing, the sleep timer and screen off, guest passes, Den badges, optional apps, app icons, HDMI-CEC, web apps, app installs, the Wayland profile).

## Visual review

- Current look: sandbox renders in [`docs/screenshots/readme/`](screenshots/readme/) (`scripts/sandbox.sh`, `make shots`) and the dated `2026-09-28-*` folders (listed in [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md#screenshots)). Not yet seen on the TV.
- [`docs/screenshots/2026-09-22-tv-shell/`](screenshots/2026-09-22-tv-shell/) and [`docs/screenshots/2026-09-22-remote/`](screenshots/2026-09-22-remote/) show the pre-pixel-art UI.

## Security review

No dedicated security review has been run. The model is in [`security.md`](security.md). The HTTP/WebSocket server (`internal/remote`) has no negative suite yet (only its route table and the app icon route are tested).

## Performance

- Sandbox (`make perf`, two cores): pixel-art campfire awake about 41-42 fps and 21-22% of a core, resting about 8 fps and 1%, screensaver under 1 fps. Weather rain adds about 2 points of CPU.
- On the TV: see the CPU rows above. The pixel-art build has not been measured on the TV.
