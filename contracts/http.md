# HTTP and WebSocket API (protocol 1)

Served by the coordinator on the selected interface addresses only (`remote.interfaces`), port `remote.port`. Nothing listens until `onboarding.lan_consent` and `remote.enabled` are both true. The development flag `--dev-listen 127.0.0.1:0` binds loopback for tests without consent and prints the bound address.

## Transport and headers

- Every request must carry a `Host` header equal to one of the bound literal addresses (with port) or an entry of `remote.allowed_hosts`. Otherwise `421 Misdirected Request`. IPv6 literals are bracketed.
- State-changing requests must carry an `Origin` header matching `scheme://host` of an allowed host. Missing or foreign origin ⇒ `403 origin_rejected`. WebSocket upgrades apply the same check.
- Forwarded headers (`X-Forwarded-*`, `Forwarded`) are ignored entirely.
- Static assets are served from the embedded bundle with `Content-Security-Policy: default-src 'self'; img-src 'self' data:; connect-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store` for API responses.
- Request bodies are limited to 64 KiB (`413`). WebSocket messages are limited to 16 KiB.
- Rate limits: `/api/v1/pair/claim` 5 attempts per invitation and 10 per source address per minute; `/api/v1/actions` 30 per second per device; WebSocket 60 messages per second per connection. Excess ⇒ `429` / `failed/rate_limited`.

## Authentication

Session cookie `bdtv_session`: `HttpOnly; SameSite=Strict; Path=/; Secure` (Secure only on HTTPS). Value = random 256-bit token; the server stores only its SHA-256. Every authenticated state-changing request must also send `X-BDTV-CSRF: <csrf_token>` obtained from `/api/v1/pair/claim` or `/api/v1/session`; the CSRF token is bound to the session and rotated on re-pair. Missing/incorrect CSRF ⇒ `403 csrf_rejected`.

Permissions: `controller` ⊂ `layout_editor` ⊂ `owner`. A device can never change its own permissions; grants are made on the TV only. `guest` is separate and below `controller`: a time-limited [guest pass](#guest-passes) that never comes with another permission.

Trusted-LAN HTTP mode additionally refuses: `PUT /api/v1/layout*` unless `remote.http_layout_editing` is true; every `/api/v1/devices*` write; every `/api/v1/admin/*` route (none exist yet; the prefix is refused over HTTP in advance). Responses include `X-BDTV-Transport: trusted-lan-http` and the UI states the limitation.

## Endpoints

| Method + path | Auth | Purpose |
|---|---|---|
| `GET /` and static files | none | Remote app (embedded). |
| `GET /api/v1/info` | none | `{protocol:1, device_name, transport, https, pairing_required:true, dev_mode}`; never private state. |
| `POST /api/v1/pair/claim` | none, rate limited | Body `{invitation:"<fragment-token>"|null, code:"123456"|null, device_name:"Alice's phone"}`. Redeems a TV-issued invitation. The TV's QR code opens `<base URL>#pair=<fragment-token>`; the phone reads the token from the fragment (never sent to the server in the URL), strips it from the address bar and claims it (`#invite=` is accepted as well). Success `200 {device_id, device_name, permissions, csrf_token}` and sets the cookie. Failure `401 invalid_invitation`, `410 invitation_expired`, `429 too_many_attempts`. |
| `GET /api/v1/session` | cookie | `{device_id, device_name, permissions, csrf_token, transport_secure}`. `401` when unauthenticated or revoked. |
| `POST /api/v1/logout` | cookie + CSRF | Ends this session (device record stays until revoked). |
| `GET /api/v1/state` | cookie | `state.schema.json` snapshot redacted for the caller's permission: `devices` owner-only, `layout` for `layout_editor` and up, `now_playing` for `guest`, `controller` and up (see [Now playing](#now-playing-statenow_playing)), `power` for every authenticated phone (see [Sleep timer](#sleep-timer-and-screen-off-statepower)), `achievements` for `controller` and up, never guests (see [Den badges](#den-badges-stateachievements)), `pairing`, `playback`, `weather` and `plex` never (shell only; guests never see them either). A guest's `me` carries `expires_at_ms`. |
| `GET /api/v1/capabilities` | cookie | `{context_epoch, target, capabilities}` for the current target. |
| `POST /api/v1/actions` | cookie + CSRF | Body: action request. Returns the action result (`200` for any outcome including `failed`; HTTP errors only for transport/auth problems). |
| `GET /api/v1/events` | cookie, Origin | WebSocket. Server → client: `{"type":"state", state}`, `{"type":"action_result", result}`, `{"type":"hold", …}`, `{"type":"revoked"}` then close 4001, `{"type":"pong"}`. Client → server: `{"type":"action", request}`, hold messages, `{"type":"visibility","hidden":true|false}`, `{"type":"ping"}`. |
| `GET /api/v1/layout` | layout_editor | `{revision, layout, defaults, pending}`. |
| `PUT /api/v1/layout` | layout_editor + CSRF | Body `{base_revision, layout}` → `200 {revision, pending:bool}`; `409 revision_conflict {current_revision}`; `422 invalid {errors:[…]}`. |
| `POST /api/v1/layout/preview` | layout_editor + CSRF | Body `{layout}` → shell preview for 60 s. |
| `POST /api/v1/layout/preview/end` | layout_editor + CSRF | Ends the preview. |
| `POST /api/v1/layout/confirm` | layout_editor + CSRF | Body `{revision}`; keeps a pending change. |
| `POST /api/v1/layout/cancel` | layout_editor + CSRF | Body `{revision}`; reverts a pending change now. |
| `POST /api/v1/layout/undo` | layout_editor + CSRF | Restores the previous revision. |
| `POST /api/v1/layout/reset` | layout_editor + CSRF | Body `{section_id}` or `{}` for all. |
| `GET /api/v1/devices` | owner | Device list. |
| `DELETE /api/v1/devices/{id}` | owner + CSRF, or self | Revokes: sessions invalidated, WebSockets closed with 4001 within 1 s. |
| `GET /api/v1/diagnostics` | owner, HTTPS or loopback | Redacted `doctor` output. |
| `GET /api/v1/apps/{adapter}/icon` | cookie (guests too) | The app's own icon as PNG, or `404 no_icon` when Bear Den's own drawing applies; see [App icons](#app-icons). |

Error body shape: `{"error":"<code>","message":"<human text>"}`.

There is no HTTP route for playback settings, local weather or Plex sign-in: those are changed on the TV or with the local CLI over IPC ([`ipc.md`](ipc.md) `playback.set`, `weather.search`, `weather.configure`, `plex.*`), and phones never receive `state.weather` or `state.plex` (the Plex link code is shown on the TV only). Phones do receive `state.content`: the Home rows' titles, subtitles and progress, with `artwork` as a path on the TV that phones cannot fetch.

## App icons

`GET /api/v1/apps/{adapter}/icon` serves the icon a phone tile shows when it
is not Bear Den's own drawing (the phone bundles those). `{adapter}` must be a
name in the coordinator's adapter table (`plex-htpc`, `vacuumtube`,
`moonlight`, `spotify`, `jellyfin`, `retroarch`, `netflix`, `disney-plus`,
`hulu`, `browser`); anything else is `404 unknown_app`. No path, URL or file
name comes from the phone, and a query string is ignored (phones add
`?icons=<choice>&installed=0|1` only so the image changes with the setting
and after an install).

- **Who:** every authenticated phone, guest passes included: they see the
  same tiles. Unauthenticated `401`.
- **What:** the first of (1) the owner's brand icon
  (`$XDG_DATA_HOME/bear-den-tv/brand/<adapter>/icon.png|.jpg`), (2) with
  `layout.ui.app_icons` `app` (or missing) and the app installed (as
  `state.applications[].installed` says, so a lingering export does not
  count), its Flatpak's exported PNG (`hicolor/{256x256,128x128,512x512,192x192,96x96,64x64,48x48}/apps/<flatpak-id>.png`
  under the user's then the system's Flatpak exports), only when the Flatpak
  is the app itself (never Google Chrome's icon for the streaming sites; the
  Browser tile may use its browser's). Otherwise `404 no_icon`, and the phone draws Bear
  Den's own icon (then a monogram).
- **Safety:** SVG is never served (the coordinator has no SVG rasteriser, so
  an SVG-only brand icon or export is skipped); a source file over 1 MiB or
  over 1024×1024 pixels is skipped; the PNG or JPEG is decoded and re-encoded
  as PNG (validates it and drops metadata).
- **Headers:** `Content-Type: image/png`, `X-Content-Type-Options: nosniff`,
  `Content-Security-Policy: default-src 'none'`, `Cache-Control: private, max-age=300`.

## Optional apps (`state.applications[].hidden`)

An application whose config has `hide_when_missing: true` ([`config.md`](config.md#optional-apps-optional-field)) carries `hidden: true` while its Flatpak is not installed or not yet discovered. Phones and the shell draw no tile for a hidden app; it stays in the list so layout editors still see it. Absent means `false`.

## App notes (`state.applications[].notes`)

Every application may carry `notes`: 0..6 short plain sentences (each 1..160 characters, never a URL) saying what the owner should know about it: what it needs (a Plex or Jellyfin server, Sunshine on the gaming PC), what the remote reaches, whether Home pauses it, and the streaming sites' picture limit in a Linux browser. The shell and every phone get the same notes; absent means none (and older coordinators never send them). The source is the adapter table (`Notes` in [`internal/applications/adapters`](../internal/applications/adapters/adapters.go)); the coordinator adds notes from live data after them. Today that is one: while the streaming sites run in a browser marked `streaming_unverified` (`state.apps.browsers`), Netflix, Disney+ and Hulu add "Streaming in Brave is unverified: the sites may not play.". Notes are facts for this box and any account, never marketing; the phone and the TV show them as they are.

## App installs (`state.applications[].install`)

The shell and **owner** phones get `install` on every application and `state.apps` (`{"auto_update": true|false, "browser", "streaming_browser", "browsers": [{"id", "label", "flatpak_id", "streaming_unverified", "notes"?}]}`: config `apps.auto_update`, the web apps' browsers, and the browser table; the last three are absent from older coordinators). Family (`controller`) phones, layout editors, guest passes and anonymous viewers never do (the schema rejects it), so they draw no install controls. Apps that share a Flatpak share one install: the web apps show their browser's (Google Chrome's for the streaming sites and Brave's for the Browser tile by default). `notes` are plain words the TV shows beside a browser choice (for Chrome that it shares usage data with Google, the 720p cap, and that the Flatpak is community-packaged).

| `state` | Meaning |
|---|---|
| `none` | Nothing to install: the app is installed (for this user or system-wide; a system-wide install carries `message` "Updated by your system"), or installs are unavailable (`message` says why, for example "Flatpak isn't installed on this box"). |
| `available` | Not installed, and Install would work. `size_bytes`/`disk_bytes` appear once the TV's install card asked Flathub (IPC `app.install_info`). |
| `preparing` | Adding the `flathub` remote, reading sizes, checking free space (`phase: checking`). |
| `downloading` | `flatpak install` is running: `phase` `runtime` (a shared runtime or extension) or `app`; `progress` 0..100. |
| `installing` | flatpak finished; Bear Den is checking the result (`phase: finishing`). |
| `failed` | `message` says why: no network, not enough space, Flathub refused, flatpak failed. Install again to retry. |
| `done` | Installed by this session; the tile is ready. |
| `removing` | The owner pressed Remove (`app.uninstall`) and `flatpak uninstall --user` is running; afterwards the app is not installed. |

An installed app carries `installed_bytes` when known: how much the app itself takes (`flatpak info`'s "Installed"), what Remove frees. A Remove that did not work leaves the app installed with `message` saying why.

- **Progress** comes from flatpak's own output (one line per runtime or app it starts) and from how much the disk under `~/.local/share/flatpak` has filled against Flathub's sizes: `flatpak install --noninteractive` prints no percentages. It never goes backwards and reaches 100 only when the install is verified.
- **Streaming sites** (`netflix`, `disney-plus`, `hulu`) carry `drm` once their browser is installed: `ready` when it can play protected video (Google Chrome: its bundled Widevine is in the installed Flatpak; Brave: the site's profile holds Widevine), `preparing` while Bear Den's quiet first run in Brave fetches it, `pending` otherwise ("Still setting up playback support").

## Now playing (`state.now_playing`)

While an app that exposes an MPRIS player is in front, or plays behind Home, phones with the `controller` permission (or a [guest pass](#guest-passes)) get `state.now_playing`: the app id, `foreground`, the title, an optional subtitle (artist or album), `status` (`playing`, `paused`, `stopped`), optional `length_ms` and `position_ms`, `position_at` and `rate`. This is a deliberate exception to "phones never see external window titles" (`target.window_title` stays redacted): the owner shows what is playing to the devices they paired for control, and can turn it off.

- **Who sees it:** authenticated phones with `controller` or higher, and guest passes (they watch the same screen). Never anonymous viewers, and never the shell or `cli` peers (so never `bear-den-tv doctor` or `/api/v1/diagnostics`).
- **Whose player:** an app's own player, decided by the process that owns its bus name (the app's Flatpak; a web app's own browser process), never by the names the player reports ([`docs/security.md`](../docs/security.md)). `source` is `mpris` for these.
- **Plex HTPC (`source: "plex_server"`):** Plex HTPC publishes no MPRIS player. While it is in front or behind Home and a `controller` phone (or guest pass) is connected, the coordinator asks the owner's chosen Plex server for its sessions (`GET /status/sessions`, every 5 s, backing off 10, 20, 40, 60 s after errors) and keeps the one that is this TV's: product `Plex HTPC` (UNVERIFIED on the TV) from one of this machine's own addresses. Its `title`, `grandparentTitle` (the show; the subtitle), `viewOffset` (`position_ms`), `duration` (`length_ms`) and state (`playing`, `paused`, or `buffering`, sent as `playing` at `rate` 0) become `now_playing`. It is read-only: `media.*` stay unavailable with the reason ("Plex is shown from your Plex server, read-only: control it on the TV."), and the phone says "From your Plex server · read-only". When no session can be tied to this TV (Plex HTPC playing only from other addresses, or more than one from this TV's) nothing is shown and the `media.*` reason says why; the owners' `GET /api/v1/diagnostics` has `plex_now_playing` (`status`, `reason`, never a title). Without Plex signed in on the TV nothing is asked.
- **Behind Home (`foreground: false`):** when Bear Den's shell comes to the front, the app that was in front stays "behind Home". While its own player reports `playing` or `paused`, phones keep the card with `foreground: false` (the phone says "Playing in YouTube · behind Home"); a stopped player, another app coming to the front, or the app exiting ends it. `media.play`, `media.pause` and `media.seek_relative` are then available when that player allows control, and must name the app as their `target` ([`actions.md`](actions.md)); a web app behind Home is shown but not controlled ("Open Netflix to control it."). With the app in front `foreground` is `true`.
- **When it is omitted:** the session is locked; config `remote.now_playing` is `false` (TV Settings → Now playing on phones; `state.remote.now_playing` mirrors the setting); the foreground is neither a configured app nor the shell with an app playing behind it; that app's own player is missing or reports no title. Another app's player is never used. `null` means the same as absent.
- **Position without a stream of pushes:** the coordinator re-reads the player when it signals a change (`PropertiesChanged`, `Seeked`), after a play, pause or seek, and every 5 s while a phone is connected and something is playing. Between snapshots the phone extrapolates `position_ms + rate × (generated_at_ms − position_at + time since the snapshot arrived)` while `status` is `playing`, capped at `length_ms`.
- **Never stored or logged:** the title and subtitle live only in the coordinator's memory and in phone snapshots; they are not written to `config.json`, SQLite, logs, diagnostics or exports ([`docs/security.md`](../docs/security.md)).

## Den badges (`state.achievements`)

Den badges are playful badges earned from local counters on the TV ([`docs/security.md`](../docs/security.md#den-badges)). Phones with the `controller` permission get `state.achievements`, read-only: `enabled` (config `achievements.enabled`), `earned` (`[{id, day}]`, oldest first; `day` is the local calendar day it was earned, `YYYY-MM-DD`, never a time) and `progress` (`[{id, count, goal}]`, every badge in catalogue order, `count` capped at `goal`).

- **Only ids, counts and days.** No titles, no app media, no times of day, no per-day history: nothing a phone could use to reconstruct what was watched. Badge names, hints and art are the phone's own copy, keyed by `id`; an id the phone does not know is drawn as a plain badge.
- **Who does not get it:** guest passes (the schema rejects it), anonymous viewers, and every viewer while the session is locked. `celebrate` (badges the TV has not celebrated yet) is shell-only.
- **Phones cannot change it.** Turning badges off and resetting them are TV-only (IPC `achievements.configure`, `achievements.reset`).

## Sleep timer and screen off (`state.power`)

`state.power` is sent to the shell and to every authenticated phone (never to anonymous viewers), also while the session is locked: it names no media and the shell needs `display` to swallow the key that wakes the screen.

- `sleep_at_ms`: when the sleep timer fires, in the same coordinator monotonic milliseconds as `generated_at_ms` (not wall-clock); `null` with no timer. Phones count down `sleep_at_ms − generated_at_ms − time since the snapshot arrived`. `sleep_minutes` is the choice that set it.
- `warning`: true during the last 60 s. The TV shows "Going to sleep in 1 minute — press any key to stay awake"; any action or TV key cancels the timer.
- `display`: `off` while Bear Den has turned the display off (`display.off`, or the timer); any action or TV key turns it on again ([`actions.md`](actions.md#sleep-screen-off-and-wake)).
- `suspend`: whether the box could suspend. Bear Den never suspends today, and says why (for example "The system asks for a password to suspend.": logind `CanSuspend` answered `challenge`; Bear Den never handles passwords).

Set it with the actions `power.sleep_timer` (`{"minutes": 0|15|30|45|60|90|120}`) and `display.off` ([`actions.md`](actions.md)). The timer lives in the coordinator's memory only: a coordinator restart forgets it and turns the display back on.

## TV control over HDMI-CEC (`state.cec`)

`state.cec` is sent to the shell and to every authenticated phone (never to anonymous viewers), also while locked: it names nothing private.

- `available`: an HDMI-CEC adapter (`/dev/cecN`) was found and opened; otherwise `reason` says why (for example "No HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter").
- `enabled`: the owner's `config.json` `cec.enabled` (default false). While false Bear Den sends nothing on the HDMI-CEC bus.
- `volume_target`: `pc` or `tv`, which volume the phone's volume buttons change while CEC is enabled. Phones label the volume group "TV volume" when it is `tv` and CEC is enabled, else "PC volume".
- `tv_power`: `on`, `standby` or `unknown`, from the TV's answer to Give Device Power Status (read after every HDMI-CEC command and once a minute while enabled).

Phones show the TV power buttons (`tv.power`, [`actions.md`](actions.md#tv-control-over-hdmi-cec)) only while `capabilities["tv.power"]` is available.
## Guest passes

The owner can let a visitor use their phone as a remote for a while without making it a family phone. Only the TV (Pair a phone → Guest pass) and the local CLI (`bear-den-tv pair --guest tonight|24h|7d`) issue guest invitations (IPC `pair.issue` with `pass`, [`ipc.md`](ipc.md)); no HTTP route issues invitations of any kind.

- **Permission:** the redeemed device gets exactly `["guest"]`, never `controller`. Guests may send only the allow-list in [`actions.md`](actions.md) (navigation, `select`, `back`, `home`, `app.launch`, media play/pause/seek, volume and mute, `text.submit`); everything else, including any action added later, fails with `forbidden` ("Guest passes can't do that."). Holds work for `nav.*`. Every `perm`-gated route (`layout*`, `devices` list, `diagnostics`) refuses guests; `DELETE /api/v1/devices/{self}` still lets a guest leave.
- **What guests see:** `me` with `permissions: ["guest"]` and `expires_at_ms`, `capabilities`, `applications`, `now_playing` (they watch the same screen) and `content`; never `devices`, `layout`, `pairing`, `playback`, `weather`, `achievements` or diagnostics.
- **Expiry:** the pass ends at `expires_at_ms` (Unix epoch ms on the coordinator's wall clock; stored in SQLite, so it survives restarts). The coordinator checks it on every authenticated request and runs a timer that re-checks at least every 5 minutes; an expired pass is revoked exactly like a revocation from the TV (below): sessions deleted, `revoked` then close 4001, holds cancelled. The phone then shows "Your guest pass has ended". The TV lists guests with a badge and the time left and can remove them early.

## Revocation and lock

On revocation the device's sessions are deleted, every WebSocket for that device receives `revoked` and is closed, and any hold lease it owned is cancelled. On session lock the coordinator bumps the epoch, sets `target.kind = locked`, cancels holds, and refuses every action with `failed/locked` until unlock; state snapshots while locked contain no `devices`, `layout`, `layout_pending`, `content`, `playback`, `weather`, `now_playing` or `achievements` (they are omitted) and an empty `shell.focus`.
