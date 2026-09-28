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
| `action.result.observed.valid.json` | `action.schema.json#/$defs/result` |
| `action.result.failed-stale.valid.json` | `action.schema.json#/$defs/result` |
| `state.shell-home.valid.json` | `state.schema.json` (shell view, pairing shown) |
| `state.phone-controller.valid.json` | `state.schema.json` (phone view, redacted) |
| `config.default.valid.json` | `config.schema.json` (built-in defaults) |
| `config.dangling-ref.invalid.json` | rejected semantically: section references unknown app |
| `config.token-leak.invalid.json` | rejected semantically: contains a `token` key |
| `config.weather-no-place.invalid.json` | rejected structurally: `weather.enabled` is true while `weather.place` is null |
| `config.weather-precise.invalid.json` | rejected semantically: weather coordinates carry more than 2 decimals (privacy, `config.md` rule 10) |
| `layout.default.valid.json` | `layout.schema.json` (no `art_style`: optional, means pixel) |
| `layout.art-style-bad.invalid.json` | rejected: `ui.art_style` must be `pixel` or `classic` |
