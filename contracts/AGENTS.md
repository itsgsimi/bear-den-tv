# contracts/: the shapes every process agrees on

JSON Schemas (draft 2020-12), the specs that explain them, and canonical
fixtures. The coordinator (Go), the TV shell (C++/QML) and the phone remote
(TypeScript) each validate their own side against these files. Overview and
the rules that hold everywhere: [`README.md`](README.md).

| File | What it defines | Go validator (`internal/contract`) |
|---|---|---|
| [`action.schema.json`](action.schema.json) + [`actions.md`](actions.md) | action request (`$defs/request`), result (`$defs/result`), hold messages (`$defs/holdMessage`), action names and per-action `args` | `ValidateActionRequest`, `ValidateActionResult`, `ValidateHoldMessage` |
| [`state.schema.json`](state.schema.json) | the state snapshot sent to the shell and phones (per-view redaction: `buildStateFor` in [`internal/session/state.go`](../internal/session/state.go); `weather` is shell-only) | `ValidateState`, `MarshalAndValidateState` (tests only) |
| [`layout.schema.json`](layout.schema.json) | `ui` + `sections`, the editable home layout | `ValidateLayout` |
| [`config.schema.json`](config.schema.json) + [`config.md`](config.md) | `config.json` structure; semantic rules in `config.md` are enforced by `internal/config` | `ValidateConfigStructure` |
| [`theme.schema.json`](theme.schema.json) | a theme package's `theme.json` ([`docs/THEMES.md`](../docs/THEMES.md)) | `ValidateTheme` |
| [`ipc.md`](ipc.md) | coordinator ↔ shell Unix socket (framing, handshake, message types) | typed messages in `internal/shellipc/messages.go` |
| [`http.md`](http.md) | HTTP + WebSocket API for phones | `internal/remote` |
| [`fixtures/`](fixtures/README.md) | canonical valid and invalid documents | [`tests/contract/fixtures_test.go`](../tests/contract/fixtures_test.go) |

Every schema's `$id` is under `https://bear-den-tv.local/contracts/`, so
relative `$ref`s (for example `"layout": {"$ref": "layout.schema.json"}` in the
state schema) resolve in Go and Ajv alike. The Go binary embeds
`contracts/*.json` and `contracts/fixtures/*.json` ([`embed.go`](../embed.go)).

## Rules

- **Named intentions only.** Nothing in a phone-facing shape may carry a shell
  string, keycode, executable path, URL to fetch or script.
- **One commit** carries the schema, the fixtures, the three validators and the
  spec. A shape that only one language knows is a bug.
- **Secrets never appear** in any schema here: `config.json` holds an opaque
  `connection_ref`; a `token`/`password` key anywhere is rejected.
- **`config.default.valid.json` is the product default.** `config.Defaults()`
  decodes that fixture, so editing it changes what a fresh install gets.
- **Honest results.** `delivered` and `observed` mean what
  [`actions.md`](actions.md) says; schemas never add an "ok-ish" outcome.

## Versioning

Additive changes (new optional fields, new enum values that every validator
learns in the same build) keep `protocol: 1`. Removing or renaming a field,
making one required, changing a type or meaning, narrowing an enum or pattern,
or changing IPC framing bumps `protocol` and the `hello`/`reject` handling in
[`ipc.md`](ipc.md). Why: [ADR 0004](../docs/decisions/0004-contract-versioning.md).

## Change a contract

1. **Schema.** Edit the `*.schema.json`. Keep `additionalProperties: false`
   where it is; new optional members are simply not `required`.
2. **Fixtures.** Add or update `fixtures/<schema>.<case>.valid.json` or
   `.invalid.json` (for example `state.shell-home.valid.json`; action
   fixtures add the `$defs` name: `action.request.nav-left.valid.json`). An invalid
   fixture proves one rule; say which in [`fixtures/README.md`](fixtures/README.md).
   A new name prefix needs:
   - a `case` in `validate()` in
     [`tests/contract/fixtures_test.go`](../tests/contract/fixtures_test.go)
     (prefix → validator; unknown prefixes fail the test);
   - an entry in `SCHEMA_FOR` (or `SEMANTIC_ONLY` for Go-only semantic
     rejections) in
     [`apps/remote-web/tests/contract.spec.ts`](../apps/remote-web/tests/contract.spec.ts);
     every fixture file must be mapped.
3. **Go types** in [`internal/contract/contract.go`](../internal/contract/contract.go)
   (optional members are pointers or `omitempty`). A **new schema file** must
   be added to both lists in `compile()` in
   [`validate.go`](../internal/contract/validate.go): the file-name list
   (registration) and the location list (compilation), plus a `schema*`
   constant and a `Validate*` function. Ajv in `contract.spec.ts` loads its
   own list of schema files too.
4. **C++**: `SessionModel::validateSnapshot` / `validateLayout` in
   [`apps/tv-shell/src/SessionModel.cpp`](../apps/tv-shell/src/SessionModel.cpp)
   check required keys and enums by hand. New required keys and new enum
   values go there, or the shell rejects the snapshot and keeps the old one.
   Expose new data through a `SessionModel` property.
5. **TypeScript**: [`apps/remote-web/src/contract.ts`](../apps/remote-web/src/contract.ts)
   mirrors the schemas field for field (no widening). If phones render it,
   copy goes in `i18n.ts`, then `make web`.
6. **Spec**: the matching `.md` here (`actions.md`, `http.md`, `ipc.md`,
   `config.md`).
7. Producers: `internal/session/state.go` for state, `internal/config` for
   config and layout (see [`internal/AGENTS.md`](../internal/AGENTS.md#add-a-state-field)).
8. Run every command in [Checks](#checks).

Done when: every check passes, each new `.invalid.json` fixture is rejected
for the reason [`fixtures/README.md`](fixtures/README.md) names, and the
shell still accepts a real snapshot (`make dev`, then look at Home).

## Add an action

1. [`actions.md`](actions.md): a row in the Actions table (args, who may send,
   routing, outcomes), and the stale-epoch rule in the Request table if it is
   an escape.
2. [`action.schema.json`](action.schema.json): the name in
   `$defs/actionName.enum`, and its `args` in the `allOf` of
   `$defs/request` (argument-free actions join the first `if` list, which
   forces `maxProperties: 0`).
3. A fixture: `fixtures/action.request.<name>.valid.json`, and an invalid one
   if the args have rules.
4. Go: the constant, `AllActions`, and `IgnoresStaleEpoch` / `IsNav` in
   [`contract.go`](../internal/contract/contract.go); routing in `route()`
   ([`internal/session/route.go`](../internal/session/route.go)); a capability
   with an honest reason in `capabilitiesLocked()`
   ([`internal/session/state.go`](../internal/session/state.go)). Details:
   [`internal/AGENTS.md` → Add an action](../internal/AGENTS.md#add-an-action).
5. Shell, if it handles it: `Navigator::apply` in
   [`apps/tv-shell/src/Navigator.cpp`](../apps/tv-shell/src/Navigator.cpp).
6. Phone: `ActionName` and `ActionArgs` in
   [`contract.ts`](../apps/remote-web/src/contract.ts), its label in
   `ACTION_NAMES` in [`i18n.ts`](../apps/remote-web/src/i18n.ts), the control
   in a view gated by `snapshot.capabilities[action]`, and `SHELL_TARGETED` in
   [`app.ts`](../apps/remote-web/src/app.ts) if it targets the shell rather
   than the active window. Then `make web` to rebuild `dist/`.
7. Run every command in [Checks](#checks).

Done when: every check passes and the new control appears on the phone only
while its capability is available (`make dev`, then open
`http://127.0.0.1:8787`, the `dev` default of `--dev-listen`).

## Checks

```sh
. scripts/env.sh
go test ./tests/contract ./internal/contract
npm --prefix apps/remote-web exec vitest run tests/contract.spec.ts
make test-shell
```
