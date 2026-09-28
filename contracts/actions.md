# Action contract (protocol 1)

## Request

```json
{
  "protocol": 1,
  "request_id": "b1ed0b5e-e592-4b33-b7a7-9029a434a818",
  "context_epoch": 47,
  "target": "active",
  "action": "nav.left",
  "args": {}
}
```

| Field | Rules |
|---|---|
| `protocol` | Must be `1`. Other values ⇒ `failed/unsupported_protocol`. |
| `request_id` | UUID v4 string, unique per device session. The coordinator remembers the last 256 ids per session for 60 s (monotonic clock). Same id + identical payload ⇒ the original result is returned again. Same id + different payload ⇒ `failed/duplicate_mismatch`. |
| `context_epoch` | Integer from the last `state` the sender saw. Must equal the coordinator's current epoch for every action except `home`, `app.launch`, and `shell.restart`; otherwise `failed/stale_epoch`. |
| `target` | `"active"` (whatever the coordinator currently observes in the foreground), `"shell"`, or a registered application id. Actions on `"active"` are refused when the observed target is `unknown` or `locked`. |
| `action` | One of the names below. |
| `args` | Object validated per action; extra keys are rejected. |

## Actions

| Action | Args | Who may send | Routing notes |
|---|---|---|---|
| `nav.up` `nav.down` `nav.left` `nav.right` | `{}` | controller | Shell: IPC `input` and the shell reports the resulting focus (`observed`). App: XTEST key tap on the verified foreground window (`delivered`). Holdable over WebSocket. |
| `select` | `{}` | controller | Same as nav. Never holdable. |
| `back` | `{}` | controller | Shell: one navigation level; at root it is a no-op (`observed`, `detail: "at_root"`), never exits the shell. App: mapped Back/Escape key. |
| `home` | `{}` | controller | Explicit escape: pauses the foreground app if a verified pause capability exists, activates the shell window, restores the remembered focus. Ignores stale epochs. Refused while locked (`failed/locked`). |
| `app.launch` | `{"app_id": "plex-htpc"}` | controller | Launch-or-activate a registered application. `accepted` immediately; a later `action_result` carries `delivered` (process started / window activated) then `observed` (window mapped and active) or `failed`. Ignores stale epochs. |
| `app.close` | `{"app_id": "plex-htpc", "force": false}` | controller (`force` requires owner) | Normal close = WM_DELETE to each mapped window of the app; windows the app opens in reply within 4 s are closed too (Moonlight answers a stream-window close by showing its host list), unless the app is relaunched. Available while the app is in front or left running behind Home. `force` = kill the tracked flatpak instance only, never by process name. |
| `media.play` `media.pause` | `{}` | controller | Only when `capabilities[action].available` is true. Delivered through MPRIS (`observed` when `PlaybackStatus` changes within 2 s) or mapped key (`delivered`). |
| `media.seek_relative` | `{"seconds": -30}` (−600..600, non-zero) | controller | Only with a verified seek capability. |
| `audio.volume_delta` | `{"delta": 5}` (−100..100, non-zero) | controller | PC/PulseAudio default sink; labeled "PC volume". |
| `audio.mute` | `{"muted": true}` | controller | PC/PulseAudio default sink. |
| `text.submit` | `{"text": "..."}` (1..256 chars, no control chars) | controller | Only into a target with a validated text adapter (shell search/pairing fields). Off for external apps. |
| `shell.restart` | `{}` | owner | Recovery only: restart a crashed/stuck shell through the coordinator supervisor. Never kills external players. |

Unknown or disabled actions ⇒ `failed/unsupported` with a `message` explaining why (for example "VacuumTube pause has not been verified on this installation").

## Result

```json
{
  "protocol": 1,
  "request_id": "b1ed0b5e-e592-4b33-b7a7-9029a434a818",
  "outcome": "observed",
  "code": "ok",
  "message": "",
  "context_epoch": 47,
  "target": {"kind": "shell", "app_id": null, "label": "Bear Den TV"},
  "detail": {"section_id": "favorites", "item_id": "youtube"}
}
```

| `outcome` | Meaning |
|---|---|
| `accepted` | Validated, authorized, and queued. Only sent for asynchronous actions (`app.launch`, `home`) before a terminal result. |
| `delivered` | The input was injected / the IPC message was sent / the process was started. No proof of effect. |
| `observed` | A state change confirming the action was seen (shell focus report, window active, MPRIS status). |
| `failed` | Not applied. `code` is one of the failure codes below. |

Failure codes: `invalid`, `unsupported_protocol`, `unauthorized`, `forbidden`, `rate_limited`, `duplicate_mismatch`, `stale_epoch`, `no_target`, `unknown_foreground`, `locked`, `unsupported`, `target_unfocused`, `busy`, `launch_failed`, `timeout`, `internal`.

Over HTTP, `POST /api/v1/actions` returns the first terminal or `accepted` result; later results for the same `request_id` arrive on the WebSocket as `action_result` events.

## Holds (WebSocket only)

```json
{"type": "hold.start", "hold_id": "…uuid…", "action": "nav.right", "context_epoch": 47}
{"type": "hold.renew", "hold_id": "…uuid…"}
{"type": "hold.stop",  "hold_id": "…uuid…"}
```

Server-side lease per device session. The initial press is the client's own `action` (sent alongside `hold.start`, so it gets a normal result); the lease produces only the repeats, which begin `repeat_delay_ms` (350) after `hold.start` at `repeat_hz` (6). A press released before the delay therefore moves exactly once. The lease expires `hold_expiry_ms` (600) after the last renew (client renews every 200 ms). The lease is cancelled on: `hold.stop`, socket close, `visibility.hidden` message, revocation, session lock, target change (epoch bump), or a second device holding (`busy` to the second). Reconnecting never resumes a hold. Only `nav.*` actions are holdable. The server reports the lease state as `hold` events with `{hold_id, state: "active"|"expired"|"cancelled"|"busy", reason}`.
