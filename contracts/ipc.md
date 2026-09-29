# Coordinator ⇄ shell IPC (protocol 1)

Transport: Unix-domain stream socket at `$XDG_RUNTIME_DIR/bear-den-tv/shell.sock` (directory mode 0700, socket mode 0600). The coordinator listens; the shell connects. Both sides check the peer uid with `SO_PEERCRED` and drop mismatches. Framing: one JSON object per line (`\n`), UTF-8, max 262144 bytes per message; larger frames close the connection. Idle keepalive: `ping`/`pong` every 5 s, disconnect after 15 s of silence.

The development override `BDTV_SHELL_SOCKET=/path` exists for development only: `bear-den-tv dev` exports it (its own socket under a temporary directory) and starts the shell with `--dev`; the shell ignores the variable without `--dev`.

## Handshake

```json
→ {"type":"hello","protocol":1,"client":"shell","version":"0.1.0","pid":4242}
← {"type":"welcome","protocol":1,"session_id":"…","coordinator_version":"0.1.0"}
← {"type":"state","state":{…full state.schema.json snapshot…}}
```

`client` is `shell` or `cli`. A `cli` client (the `bear-den-tv` command run by the same user) is a trusted local peer: it receives `state` and may send `pair.issue`, `pair.cancel`, `devices.revoke`, `devices.grant`, `remote.configure`, `remote.now_playing`, `settings.update`, `playback.set`, `weather.search`, `weather.configure` and the `plex.*` messages, but never `focus`/`input_result`. Only one `shell` client is accepted at a time; a second `shell` hello is rejected with `reason: "shell_already_connected"`.

`hello` with a different `protocol` ⇒ `{"type":"reject","reason":"unsupported_protocol","supported":[1]}` and close. The shell does not fall back.

## Coordinator → shell

| `type` | Payload | Shell obligation |
|---|---|---|
| `state` | `{"state": snapshot}` | Replace its model. Full snapshots only; no diffs in protocol 1. |
| `input` | `{"request_id","action","args","context_epoch"}` | Apply the navigation/select/back/text action to the current focus graph and reply with `input_result`. Applies only if the shell's known epoch matches; otherwise reply `failed/stale_epoch`. |
| `home` | `{"request_id","restore_focus":true}` | Return to the home screen, close open dialogs, restore focus from memory, request window activation, reply `input_result`. |
| `layout_preview` | `{"layout": layout.schema.json, "expires_in_s": 60}` | Render the draft without persisting; do not report it as applied. |
| `layout_preview_end` | `{}` | Return to the persisted layout. |
| `confirm_request` | `{"confirm_id","kind":"layout","summary","expires_in_s"}` | Show a timed confirmation dialog with Keep / Revert; Back also reverts. Reply with `confirm_result`. |
| `notify` | `{"id","kind","text"}` | Show a transient notification. |
| `weather_places` | `{"request_id","ok","places":[{"name","region","country","latitude","longitude"}],"error"}` | Reply to `weather.search` (to the client that asked): show the places as a list; `error` is user-facing. |
| `shutdown` | `{"reason":"maintenance"|"restart"}` | Exit cleanly with code 0. |
| `ping` / `pong` | `{}` | Reply `pong` to `ping`. |

## Shell → coordinator

| `type` | Payload | Notes |
|---|---|---|
| `hello` | see above | First message. |
| `focus` | `{"screen","section_id","item_id","scroll_x","text_field"}` | Sent on every focus change and after restoring focus; the coordinator stores it as focus memory (by stable ids). Optional `text_field` (boolean, default false) is true while a text field in the shell has keyboard focus: only then, with the shell in front and connected, is `text.submit` available (backend `shell`); otherwise it is unavailable with "No text field is focused.". An accepted `text.submit` reaches the shell as `input`. |
| `input_result` | `{"request_id","outcome":"observed"|"failed","code","detail"}` | Terminal reply to `input`/`home`. |
| `confirm_result` | `{"confirm_id","accepted":true|false}` | User pressed Keep (true) or Revert/Back/timeout (false). |
| `request` | `{"request_id","action","args"}` | Shell-originated action (`app.launch`, `app.close`, `home`, `audio.*`, `power.sleep_timer`, `display.off`). Same action contract; the coordinator answers with `action_result`. The shell is a trusted local sender: no epoch check, always authorized. |
| `power.activity` | `{}` | A key was pressed on the TV while `state.power.display` was `off` or `state.power.warning` was true; the shell swallowed it (it must not also move focus or select). The coordinator turns the display on and cancels the sleep timer, then pushes a new `state`. No reply; `shell` clients only (ignored from `cli`). Input the coordinator itself forwards as `input` never needs this: the coordinator woke the display or cancelled the timer before forwarding it, and the `state` saying so is written to the socket before that `input`. |
| `settings.update` | `{"request_id","base_revision","layout": layout.schema.json}` | TV settings change; reply `settings_result {request_id, ok, revision, error}`. |
| `pair.issue` | `{"request_id","pass"?}` | Ask for a fresh invitation; the next `state` carries `pairing`. Without `pass` (or `""`) it is a family phone (`controller`). `pass` `"tonight"` (until 04:00 the next morning in the coordinator's local time zone; issued between 00:00 and 03:59 it ends at 04:00 the same morning), `"24h"` or `"7d"` issues a **guest pass**: the phone that redeems it gets only `guest` and is revoked automatically when the pass ends (`state.pairing.guest`, `pass_expires_at_ms`; [`http.md`](http.md#guest-passes)). Any other `pass` value fails closed. Reply `result {request_id, ok, error, data: {code, url, expires_at_ms, pass_expires_at_ms?}}`. Trusted local senders only: phones can never issue invitations. |
| `pair.cancel` | `{"request_id"}` | Cancel the displayed invitation. |
| `devices.revoke` | `{"request_id","device_id":"…"|"*"}` | Revoke one device or all. |
| `devices.grant` | `{"request_id","device_id","permissions":["controller","layout_editor"]}` | Change a device's permissions; the only path that can grant `layout_editor`/`owner` (trusted local). `guest` cannot be granted, and a guest pass cannot be upgraded (pair the phone again as a family phone). |
| `remote.configure` | `{"request_id","enabled","transport","interface","port","http_layout_editing","lan_consent":true}` | Local trusted onboarding step; the only path that can enable LAN exposure. |
| `remote.now_playing` | `{"request_id","enabled"}` | Settings → Now playing on phones: whether paired phones with `controller` see what the app in front is playing (`state.now_playing`, [`http.md`](http.md#now-playing-statenow_playing)). Stored as `config.json` `remote.now_playing` (revision bump); a new `state` follows with `state.remote.now_playing` updated, and phones lose or regain `now_playing` at once. Reply `result {request_id, ok, error}`. Trusted local senders only (`shell`, `cli`). |
| `playback.set` | `{"request_id","adapter","setting","value"}` | Settings → Advanced playback: one app's playback setting chosen by hand (`value: ""` returns it to automatic). The coordinator accepts only an adapter it detected, a setting in that app's `state.playback.apps[].settings`, and a value among that setting's current `options`; anything else fails closed with a reason. Accepted values are stored in `config.json` (`playback.overrides`, `config.md`), the app is re-planned, its settings file is written at once when the app is closed or when it next closes (with `startup.tune_apps: false` the choice is stored but nothing is written), and a new `state` follows. Reply `result {request_id, ok, error}`. Trusted local senders only (`shell`, `cli`). |
| `weather.search` | `{"request_id","query"}` | Settings → Weather: find a place by name. `query` is 1..80 characters without control characters. The coordinator asks Open-Meteo geocoding (8 results, English, 10 s timeout) and replies `weather_places {"request_id","ok","places":[{"name","region","country","latitude","longitude"}],"error"}` with coordinates already rounded to 2 decimals; on failure `ok: false`, `places: []` and a user-safe `error` (never a URL or coordinates). Trusted local senders only (`shell`, `cli`). |
| `weather.configure` | `{"request_id","enabled","place":{"name","region","country","latitude","longitude"}\|null,"units":"celsius"\|"fahrenheit","scene"}` | Settings → Weather: turn weather on or off, choose the place, units and whether it drives the scene. `place: null` keeps the stored place (the shell and the CLI only see its name, `state.weather.place`, so toggles and unit changes send `null`); a place object replaces it. Validated like `config.json` (`enabled` needs a place, sent or stored; `config.md` rule 10); the coordinator rounds the coordinates to 2 decimals, stores the block in `config.json` (revision bump), refreshes at once and a new `state` follows (`state.weather`). Reply `result {request_id, ok, error}`. Trusted local senders only. |
| `plex.sign_in` | `{"request_id"}` | Settings → Plex: start (or restart) linking this TV to a Plex account. The coordinator first checks that a keyring (Secret Service) is usable; without one it fails closed (`state.plex.status: "error"`, "Plex sign-in needs a keyring; install or enable gnome-keyring") and nothing is sent anywhere. Otherwise it asks plex.tv for a link code (`POST /api/v2/pins`) and `state.plex` becomes `linking` with `code`, `link_url` (`https://plex.tv/link`) and `qr_modules` (the link as a QR code, encoded by the coordinator like `pairing.qr_modules`); the shell shows all three. The coordinator polls the code every 2 s until it is linked or expires (`error`, "The code expired"). Once linked, the account token goes into the keyring under `plex_content.connection_ref` (default `plex`), never `config.json`, and the account's servers are listed (`GET /api/v2/resources`): no server is an `error`, exactly one is chosen automatically, several make `choose_server`. Reply `result {request_id, ok, error}` as soon as the code is shown. Trusted local senders only. |
| `plex.cancel` | `{"request_id"}` | Abandon linking or choosing: polling stops, nothing is stored, `state.plex` returns to `signed_out` (or `connected` when an earlier sign-in is still configured). |
| `plex.choose_server` | `{"request_id","server_id"}` | Pick one of `state.plex.servers` (by `id`). The coordinator tries that server's connections in the order local https, local http, remote https, remote http (relays are never used), keeps the first whose `/identity` names the same server, and lists its libraries: `state.plex` becomes `choose_libraries` with every movie and show library `selected`. An unknown id or an unreachable server fails with a reason. |
| `plex.choose_libraries` | `{"request_id","library_ids":["1","2"]}` | Finish sign-in with at least one of `state.plex.libraries`. The coordinator writes `plex_content` (`enabled: true`, `server_url`, `library_ids`, `connection_ref`) to `config.json` (revision bump), enables the `plex-continue-watching` and `plex-recently-added` sections, refreshes the rows and `state.plex` becomes `connected`. |
| `plex.sign_out` | `{"request_id"}` | Delete the token from the keyring, turn `plex_content` off (`enabled: false`, no `server_url` or `library_ids`), disable the Plex sections, drop the rows and the cached artwork; `state.plex` becomes `signed_out`. When the keyring cannot delete the token the rest still happens and the reply says why. |
| `applications.install_request` | `{"request_id","app_id"}` | Reserved for a guided Flatpak install. Not implemented: the coordinator always replies `result {ok: false}` with "guided installation is not available yet". |
| `shell.exit` | `{"reason":"maintenance"}` | Intentional exit: the coordinator must not restart the shell. |
| `ping` / `pong` | `{}` | |

Every `request_id`-bearing message from the coordinator gets exactly one terminal reply; the shell answers within 2 s or the coordinator records `failed/timeout`.

## Supervisor semantics

The coordinator starts the shell binary (`bear-den-tv-shell`) unless `--no-shell` is set. Exit code 0 after `shutdown`/`shell.exit` = intentional, no restart. Any other exit or a lost socket without `shell.exit` = crash: restart with backoff 1 s, 2 s, 4 s, 8 s, 16 s (max); five crashes within 5 minutes open the circuit (`shell_state = circuit_open`) and the remote offers `shell.restart` (owner) instead of looping. External applications are never touched by shell restarts.
