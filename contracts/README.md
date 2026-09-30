# Bear Den TV contracts (protocol 1)

Every process speaks the same versioned contract. Go (coordinator), C++/QML (TV shell), and TypeScript (phone remote) each validate their own representation against the JSON Schemas and fixtures in this directory. Every change updates all three consumers plus `fixtures/` in one commit; additive changes keep protocol 1, breaking ones bump `protocol` ([ADR 0004](../docs/decisions/0004-contract-versioning.md)). The step-by-step checklist is in [`AGENTS.md`](AGENTS.md).

| File | Purpose |
|---|---|
| `actions.md` + `action.schema.json` | Named actions the phone or shell may submit; result envelope with accepted/delivered/observed/failed |
| `state.schema.json` | Authorized state snapshot pushed to phones and the shell. Each viewer gets its own redacted view (`buildStateFor` in [`internal/session/state.go`](../internal/session/state.go)): for example `pairing`, `playback`, `weather`, `plex`, `onboarding`, `autostart` and `tips` reach only the shell, `now_playing` only phones with `controller` or a guest pass ([`http.md`](http.md#now-playing-statenow_playing)), and `achievements` (Den badges) the shell and `controller` phones, never guests ([`http.md`](http.md#den-badges-stateachievements)) |
| `ipc.md` | Private Unix-socket protocol between coordinator and shell (its `state` payload is `state.schema.json`) |
| `http.md` | HTTP + WebSocket API served by the coordinator |
| `config.schema.json` + `config.md` | Structural schema for `config.json`; semantic rules live in `internal/config` and are listed in `config.md` |
| `layout.schema.json` | The home-screen layout (sections, items, `ui` appearance settings) |
| `theme.schema.json` | A theme package's `theme.json` manifest ([`docs/THEMES.md`](../docs/THEMES.md)) |
| `web-hints.schema.json` | A web app's navigation hints (`apps/web-nav/hints/<adapter>.json`, embedded; never from phones or config; [ADR 0010](../docs/decisions/0010-web-apps-over-cdp-pipe.md)) |
| `fixtures/` | Canonical example messages used by contract tests in all three languages |

## What the contract carries

A map for finding things; the files above are the authority.

- **Actions** ([`actions.md`](actions.md)): navigation `nav.up|down|left|right`, `select`, `back`, `home`; apps `app.launch`, `app.close`, and owner-only `app.install`, `app.install_cancel`, `app.uninstall`; media `media.play`, `media.pause`, `media.seek_relative`; `audio.volume_delta`, `audio.mute`; `text.submit`; owner-only `shell.restart`; power `power.sleep_timer`, `display.off`, `tv.power`; the web apps' touchpad `pointer.move`, `pointer.click`, `pointer.scroll`. Holds (`hold.start|renew|stop`) go over the WebSocket for `nav.*` only. Guest passes may send only the list in `contract.GuestActions`.
- **State snapshot** (`state.schema.json`): always `protocol`, `context_epoch`, `generated_at_ms`, `device_name`, `dev_mode`, `config_revision`, `session`, `target`, `capabilities`, `shell`, `applications` (with `hidden`, `enabled` for web apps, `notes` for everyone, and `install` for the shell and owner phones), `remote`, `notifications`. Optional, per view and permission (`buildStateFor`): `me`, `appearance` (theme, art style, app icons), `devices`, `layout`, `layout_pending`, `now_playing`, `achievements`, `power`, `cec`, `apps`, `content` (the Plex rows); for the shell only: `pairing`, `playback`, `weather`, `plex`, `onboarding` (config `onboarding.completed`), `autostart` (the user's autostart entry: `enabled`, `available`, `reason`), `tips` (the bear tips: `enabled`, `done`, `stopped`, `last_day`).
- **Config** (`config.schema.json`, [`config.md`](config.md)): `device`, `ui` and `sections` (the layout), `applications` (incl. `hide_when_missing`, `enabled`, `web.url`), `plex_content`, `remote` (incl. `now_playing`), `cache`, `startup`, `privacy`, `onboarding`, `apps` (`auto_update`, `browser`, `streaming_browser`), `achievements`, `tips`, `weather`, `cec`, `playback`.
- **IPC** ([`ipc.md`](ipc.md)): coordinator → shell `state`, `input`, `home`, layout previews, `confirm_request`, `notify`, `weather_places`, `shutdown`; shell or CLI → coordinator `focus`, `input_result`, `request`, `power.activity`, `settings.update`, `pair.*`, `devices.*`, `remote.*`, `cec.configure`, `app.enable`, `playback.set`, `weather.*`, `plex.*`, `achievements.*`, `tips.*`, `app.install*`, `apps.configure`, `apps.browser`, `onboarding.complete`, `autostart.configure`, `shell.exit`.
- **HTTP** ([`http.md`](http.md)): `/api/v1/info`, `pair/claim`, `session`, `logout`, `state`, `capabilities`, `actions`, `events` (WebSocket), `layout` and its preview/confirm/undo/reset, `devices`, `diagnostics`, `apps/{adapter}/icon`; plus the static phone remote and theme art under `/themes/`.

Rules that hold everywhere:

- The browser sends **named intentions** only. No shell strings, keycodes, executable paths, URLs to fetch, or scripts. Unknown action names are rejected before any routing.
- Every state-changing message carries `protocol`, `request_id`, and (for target-directed actions) `context_epoch`.
- Only the explicit escapes `home`, `app.launch`, and `shell.restart`, the power actions `power.sleep_timer`, `display.off` and `tv.power`, and `app.install`, `app.install_cancel` and `app.uninstall` (they never touch the window in front), may ignore a stale `context_epoch` (`IgnoresStaleEpoch` in [`internal/contract/contract.go`](../internal/contract/contract.go)); they still require authorization and are refused while the session is locked.
- Results are honest: `delivered` means an input was injected or an IPC message was sent; `observed` means a state change was confirmed (shell focus report, window activation, MPRIS status). Never upgrade a result without evidence.
- Unknown foreground window, locked session, or unverifiable focus ⇒ `failed` with a machine-readable `code` and a user-facing `message`.
