# Bear Den TV validation report

What has actually been checked, where and with what result. Allowed outcomes are PASS, FAIL, BLOCKED, NOT RUN and NOT APPLICABLE ([acceptance matrix](../bear-den-tv-materials/ACCEPTANCE_MATRIX.md)). A skipped check is never a pass. The requirement-by-requirement view is in [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

Updated 2026-09-23.

## Build under test

- Branch `coordinator-shell-remote`, commit `552aefd`.
- Toolchain (user-space, `scripts/bootstrap-toolchain.sh`): Go 1.24, Qt 6.8.4, CMake, Node 22.
- The TV: the reference box, Linux Mint 21.3 Xfce on X11, Haswell graphics.

## Automated checks

These run on the workstation with `make test` unless noted.

| Area | Command | Where the tests are |
|---|---|---|
| Go (with `-race`) | `make test` | `*_test.go` beside the code; `tests/contract`, `tests/docs` |
| Phone remote (Vitest, unit only) | `make test` | `apps/remote-web/tests/unit/` |
| TV shell (offscreen) | `make test` | `apps/tv-shell/tests/tst_shell.cpp` |
| Idle drawing budget | `make perf` | `scripts/perf-sandbox.sh` |
| Live end-to-end (on the TV) | `scripts/e2e-target.sh` | `tests/e2e/target_test.go` |

No results are recorded here for today's tree: run the commands above to get them. Known gaps: `internal/remote`, `internal/doctor` and `internal/providers/plex` have no tests.

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

NOT RUN on the TV: playing real content in Plex or VacuumTube from the phone, the pixel-art UI, bears, Advanced playback, the Plex provider, TV off/on and sleep, packaging.

## Visual review

- Current look: sandbox renders in [`docs/screenshots/readme/`](screenshots/readme/) (`scripts/sandbox.sh`, `make shots`). Not yet seen on the TV.
- [`docs/screenshots/2026-09-22-tv-shell/`](screenshots/2026-09-22-tv-shell/) and [`docs/screenshots/2026-09-22-remote/`](screenshots/2026-09-22-remote/) show the pre-pixel-art UI.

## Security review

No dedicated security review has been run. The model is in [`security.md`](security.md). The HTTP/WebSocket server (`internal/remote`) has no negative tests yet.

## Performance

- Sandbox (`make perf`, two cores): pixel-art campfire awake about 41-42 fps and 21-22% of a core, resting about 8 fps and 1%, screensaver under 1 fps. Weather rain adds about 2 points of CPU.
- On the TV: see the CPU rows above. The pixel-art build has not been measured on the TV.
