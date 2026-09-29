# Contract fixtures

Canonical messages consumed by contract tests in Go ([`tests/contract/fixtures_test.go`](../../tests/contract/fixtures_test.go)) and TypeScript ([`apps/remote-web/tests/contract.spec.ts`](../../apps/remote-web/tests/contract.spec.ts)). The shell has no fixture test: it validates snapshots at runtime in `SessionModel`, and its tests (`apps/tv-shell/tests/tst_shell.cpp`) load their own DEMO state from `apps/tv-shell/tests/fixtures/`.

- Files named `*.valid.json` must validate; `*.invalid.json` must be rejected for the reason this table gives.
- Go picks the validator from the name prefix (`action.request.`, `action.result.`, `state.`, `layout.`, `config.`); `config.*` files go through `config.Parse`, so semantic rules apply too.
- TypeScript checks the schema named in `SCHEMA_FOR`; files rejected only by Go's semantic rules are listed in `SEMANTIC_ONLY` and skipped there. Every file here must appear in one of the two.

| File | Schema |
|---|---|
| `action.request.nav-left.valid.json` | `action.schema.json#/$defs/request` |
| `action.request.app-launch.valid.json` | `action.schema.json#/$defs/request` |
| `action.request.shell-string.invalid.json` | rejected: unknown action name |
| `action.request.extra-args.invalid.json` | rejected: `args` must be empty for `select` |
| `action.request.power-sleep-timer.valid.json` | `action.schema.json#/$defs/request` (`power.sleep_timer`, 45 minutes) |
| `action.request.sleep-timer-odd-minutes.invalid.json` | rejected: `power.sleep_timer` `minutes` must be 0, 15, 30, 45, 60, 90 or 120 |
| `action.request.display-off.valid.json` | `action.schema.json#/$defs/request` (`display.off`, no args) |
| `action.request.tv-power.valid.json` | `action.schema.json#/$defs/request` (`tv.power`, standby) |
| `action.request.tv-power-off.invalid.json` | rejected: `tv.power` `power` must be `on` or `standby` |
| `action.result.observed.valid.json` | `action.schema.json#/$defs/result` |
| `action.result.failed-stale.valid.json` | `action.schema.json#/$defs/result` |
| `action.result.display-off-woke.valid.json` | `action.schema.json#/$defs/result` (`failed/display_off`: the press woke the screen and was not applied) |
| `state.shell-home.valid.json` | `state.schema.json` (shell view, pairing shown) |
| `state.phone-controller.valid.json` | `state.schema.json` (phone view, redacted) |
| `state.phone-now-playing.valid.json` | `state.schema.json` (phone view with DEMO `now_playing` and `remote.now_playing`) |
| `state.now-playing-bad-status.invalid.json` | rejected: `now_playing.status` must be `playing`, `paused` or `stopped` |
| `state.now-playing-no-title.invalid.json` | rejected: `now_playing.title` must not be empty (no title means no `now_playing`) |
| `state.phone-sleep-warning.valid.json` | `state.schema.json` (phone view: a 45-minute sleep timer in its last minute, `power.suspend` unavailable) |
| `state.power-bad-display.invalid.json` | rejected: `power.display` must be `on` or `off` |
| `state.phone-cec.valid.json` | `state.schema.json` (phone view: HDMI-CEC enabled, TV on, volume buttons driving the TV) |
| `state.phone-cec-unavailable.valid.json` | `state.schema.json` (phone view: no HDMI-CEC device, with the reason) |
| `state.cec-bad-tv-power.invalid.json` | rejected: `cec.tv_power` must be `on`, `standby` or `unknown` |
| `config.default.valid.json` | `config.schema.json` (built-in defaults) |
| `config.now-playing-not-bool.invalid.json` | rejected structurally: `remote.now_playing` must be a boolean |
| `config.cec-bad-volume-target.invalid.json` | rejected structurally: `cec.volume_target` must be `pc` or `tv` |
| `config.dangling-ref.invalid.json` | rejected semantically: section references unknown app |
| `config.token-leak.invalid.json` | rejected semantically: contains a `token` key |
| `config.weather-no-place.invalid.json` | rejected structurally: `weather.enabled` is true while `weather.place` is null |
| `config.weather-precise.invalid.json` | rejected semantically: weather coordinates carry more than 2 decimals (privacy, `config.md` rule 10) |
| `layout.default.valid.json` | `layout.schema.json` (no `art_style`: optional, means pixel) |
| `layout.art-style-bad.invalid.json` | rejected: `ui.art_style` must be `pixel` or `classic` |
