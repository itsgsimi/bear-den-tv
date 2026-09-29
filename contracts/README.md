# Bear Den TV contracts (protocol 1)

Every process speaks the same versioned contract. Go (coordinator), C++/QML (TV shell), and TypeScript (phone remote) each validate their own representation against the JSON Schemas and fixtures in this directory. Every change updates all three consumers plus `fixtures/` in one commit; additive changes keep protocol 1, breaking ones bump `protocol` ([ADR 0004](../docs/decisions/0004-contract-versioning.md)). The step-by-step checklist is in [`AGENTS.md`](AGENTS.md).

| File | Purpose |
|---|---|
| `actions.md` + `action.schema.json` | Named actions the phone or shell may submit; result envelope with accepted/delivered/observed/failed |
| `state.schema.json` | Authorized state snapshot pushed to phones and the shell. Each viewer gets its own redacted view (`buildStateFor` in [`internal/session/state.go`](../internal/session/state.go)): for example `pairing`, `playback` and `weather` reach only the shell, and `now_playing` only phones with `controller` ([`http.md`](http.md#now-playing-statenow_playing)) |
| `ipc.md` | Private Unix-socket protocol between coordinator and shell (its `state` payload is `state.schema.json`) |
| `http.md` | HTTP + WebSocket API served by the coordinator |
| `config.schema.json` + `config.md` | Structural schema for `config.json`; semantic rules live in `internal/config` and are listed in `config.md` |
| `layout.schema.json` | The home-screen layout (sections, items, `ui` appearance settings) |
| `theme.schema.json` | A theme package's `theme.json` manifest ([`docs/THEMES.md`](../docs/THEMES.md)) |
| `fixtures/` | Canonical example messages used by contract tests in all three languages |

Rules that hold everywhere:

- The browser sends **named intentions** only. No shell strings, keycodes, executable paths, URLs to fetch, or scripts. Unknown action names are rejected before any routing.
- Every state-changing message carries `protocol`, `request_id`, and (for target-directed actions) `context_epoch`.
- Only the explicit escapes `home`, `app.launch`, and `shell.restart`, and the power actions `power.sleep_timer` and `display.off` (they never touch the window in front), may ignore a stale `context_epoch` (`IgnoresStaleEpoch` in [`internal/contract/contract.go`](../internal/contract/contract.go)); they still require authorization and are refused while the session is locked.
- Results are honest: `delivered` means an input was injected or an IPC message was sent; `observed` means a state change was confirmed (shell focus report, window activation, MPRIS status). Never upgrade a result without evidence.
- Unknown foreground window, locked session, or unverifiable focus ⇒ `failed` with a machine-readable `code` and a user-facing `message`.
