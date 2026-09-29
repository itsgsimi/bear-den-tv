# Configuration contract

`config.json` lives at `$XDG_CONFIG_HOME/bear-den-tv/config.json`. The coordinator is its only writer. Structure: [`config.schema.json`](config.schema.json). Portable subset: [`layout.schema.json`](layout.schema.json) (`ui` + `sections`). Code: [`internal/config`](../internal/config/config.go).

## Semantic rules (enforced by `internal/config`, tested with negative cases)

1. `applications[].id` and `sections[].id` are unique within their arrays and never collide with the reserved words `active`, `shell`.
2. Every `sections[].application_ids[]` entry references an existing `applications[].id`.
3. `applications[].launch.args` must be a subset of the adapter's approved argument list: `vacuumtube` ⇒ `--fullscreen`, `--no-window-decorations`; `jellyfin` ⇒ `--fullscreen`, `--tv`; `retroarch` ⇒ `--fullscreen`; `plex-htpc`, `moonlight` and `spotify` ⇒ none; the web adapters `netflix`, `disney-plus`, `hulu`, `browser` ⇒ none (the coordinator builds Chromium's arguments itself, rule 11). `launch.app_id` must equal the adapter's known Flatpak id (`rocks.shy.VacuumTube`, `tv.plex.PlexHTPC`, `com.moonlight_stream.Moonlight`, `com.spotify.Client`, `org.jellyfin.JellyfinDesktop`, `org.libretro.RetroArch`, and `org.chromium.Chromium` for every web adapter).
4. `remote.enabled` requires `onboarding.lan_consent == true` and every `remote.interfaces[]` to be an interface that exists on this host and is not loopback or a virtual interface (names starting `docker`, `br`, `veth`, `tun`, `tap`, `wg`, `virbr`, `vmnet`, `lxc`, `cni`, `flannel`, `tailscale`, `utun`). Non-existent interfaces fail loud at load ("interface wlan9 not present") and the listener stays down; the shell shows the reconfiguration route.
5. `remote.transport == "https"` with `remote.enabled` requires readable `certificate_file` and `private_key_file` that parse as a PEM certificate/key pair.
6. `remote.http_layout_editing` is only honored in `trusted-lan-http`; in `https` layout editing is governed by device permission alone.
7. `plex_content.enabled` requires `connection_ref` and `server_url`; the token is looked up in the secret store, never in this file. A file containing a key named `token`, `x_plex_token`, or `password` anywhere is rejected. TV Settings → Plex writes the block when sign-in finishes (IPC `plex.choose_libraries`, [`ipc.md`](ipc.md)): `enabled: true`, `connection_ref` (kept, or `"plex"` when null), the chosen server's `server_url`, and `library_ids` (the ticked libraries; an empty list means every movie and show library); it also turns the `plex-continue-watching` and `plex-recently-added` sections on. Sign-out sets `enabled: false`, drops `server_url` and `library_ids`, and turns those sections off.
8. `revision` must be greater than the revision of the currently loaded configuration when written through the API (optimistic concurrency); on disk it must be ≥ 1.
9. `schema_version` other than 1 ⇒ the file is refused, the last-known-good copy is loaded, and the shell shows an error banner.
10. `weather.place.latitude` and `weather.place.longitude` carry at most 2 decimals (about 1 km): the coordinator never stores a more precise location. `weather.enabled` requires a `place` (also structural) and `units` is `celsius` or `fahrenheit`.

11. **Web adapters** (`netflix`, `disney-plus`, `hulu`, `browser`): `web.url` is the page Chromium opens. It must be `https`, name a host (not an IP address), carry no user name or password, no port and no fragment, no spaces or control characters, at most 512 characters; for `netflix`, `disney-plus` and `hulu` the host must be `netflix.com`, `disneyplus.com`, `hulu.com` or a subdomain (and `web.url` is required); the `browser` takes any such host, and without `web` it opens `about:blank` (no search engine). Only web adapters may carry `web` or `enabled`, and each web adapter is used by at most one application (its window class and profile are its own). Errors never repeat the URL.

## Persistence

- Atomic write: temp file in the same directory, fsync, rename over `config.json`, fsync directory.
- `config.last-known-good.json` is replaced only after a successful load + semantic validation of the new file.
- `config.history/<revision>.json` keeps the last 20 revisions for undo/rollback.
- A corrupt, unreadable, schema-invalid, or semantically invalid `config.json` never replaces the last-known-good copy. Recovery order: `config.json` → `config.last-known-good.json` → built-in defaults (with a persistent error notification).

## Layout changes (web editor / TV settings)

`PUT /api/v1/layout` and the shell's `settings.update` both go through the same `config.ApplyLayout(base_revision, layout, source)` path:

1. Validate structurally and semantically.
2. Reject if `base_revision != current revision` (`409 revision_conflict`).
3. Write the new revision; remember the previous revision as the undo point.
4. If the change is *risky* (any section disabled that leaves zero enabled application sections, `text_scale` > 1.6, or `safe_margin_percent` > 6), it is applied as **pending**: the shell shows a timed confirmation (30 s). `POST /api/v1/layout/confirm` or a TV confirmation keeps it; timeout or `cancel` rolls back to the previous revision automatically.
5. `POST /api/v1/layout/undo` restores the previous revision (one level). `POST /api/v1/layout/reset` restores built-in defaults for one section or the whole layout.

Preview (`POST /api/v1/layout/preview`) sends a draft to the shell over IPC without writing it; the draft ends on `preview_end`, on apply, or after 60 s.

## Optional apps (optional field)

- `applications[].hide_when_missing` (boolean, absent = `false`): an optional app. While its Flatpak is not installed (or not yet discovered) the coordinator marks it `hidden` in `state.applications[]` and neither the shell nor phones draw a tile for it; the core apps leave it out and show "Not installed" instead. The built-in defaults ship Spotify, Jellyfin and RetroArch this way, after the three core apps, in the "Your Apps" rail.
- Defaults only seed a fresh install: an existing `config.json` keeps its own `applications` list, so an existing box gains the optional apps only by adding their rows (copy them from [`fixtures/config.default.valid.json`](fixtures/config.default.valid.json)).
- Additive: `schema_version` stays 1.

## Web apps (optional fields)

- `applications[].web` (`{"url"}`) for the web adapters (rule 11). Owner-edited on the TV; phones never send page addresses. The built-in defaults open `https://www.netflix.com/`, `https://www.disneyplus.com/`, `https://www.hulu.com/` and, for the browser, nothing (`about:blank`).
- `applications[].enabled` (boolean, absent = `true`): `false` means the owner turned the app off; the coordinator marks it `hidden` and refuses to launch it. Written by the trusted local `app.enable` ([`ipc.md`](ipc.md): TV Settings → Streaming sites). The defaults ship Netflix, Disney+ and Hulu with `enabled: false` and all four web apps with `hide_when_missing: true` (no tiles while Chromium is not installed).
- Each web app runs with its own Chromium profile in `$XDG_DATA_HOME/bear-den-tv/web/<app-id>` (sign-ins live there, never in this file).
- Additive: `schema_version` stays 1.

## Playback tuning (optional fields)

- `startup.tune_apps` (boolean, default true): apply the best playback settings for this box automatically (`docs/APP_PERFORMANCE.md`).
- `playback.overrides` (object, absent by default): settings chosen by hand in TV Settings → Advanced playback, as `{"<adapter>": {"<setting id>": "<value>"}}`, for example `{"moonlight": {"fps": "60"}, "vacuumtube": {"codecs": "vp9"}}`. Adapter keys match `^[a-z][a-z0-9-]{1,31}$`, setting ids `^[a-z][a-z0-9_]{1,31}$`, values `^[a-z0-9][a-z0-9_.-]{0,31}$`. A missing setting means automatic; the coordinator removes empty objects, so a box without overrides has no `playback` key.
- Written only by the shell's `playback.set` (`ipc.md`), which accepts a value only when it is one of the options this box currently offers. A value that is no longer offered (hardware or tier changed, hand-edited file) is not an error: tuning falls back to automatic and the Playback screen says so.
- Both fields are additive: older files without them stay valid and `schema_version` stays 1.

## Local weather (optional field)

- `weather` (object, absent = off): `{"enabled", "place", "units", "scene"}`. `place` is `{"name", "region", "country", "latitude", "longitude"}` or `null`; `units` is `celsius` or `fahrenheit`; `scene: true` lets the weather drive the Home backdrop. Built-in default: `{"enabled": false, "place": null, "units": "celsius", "scene": true}`. A file without the block keeps loading and means off.
- Written only by the trusted local `weather.configure` (`ipc.md`: TV Settings → Weather or `bear-den-tv weather set`), which rounds the coordinates to 2 decimals before storing (rule 10).
- Never holds a URL, key or token: the Open-Meteo endpoints are constants in `internal/weather`, and Open-Meteo needs no key. The coordinator contacts it only while `enabled` is true ([`docs/security.md`](../docs/security.md)).
- Additive: `schema_version` stays 1.

## Den badges (optional field)

- `achievements` (object, absent = on): `{"enabled": true|false}`. The built-in default file writes `{"enabled": true}`. While `true` the coordinator keeps local counters and awards Den badges (`state.achievements`, [`http.md`](http.md#den-badges-stateachievements)); `false` counts nothing at all. Badges already earned stay until they are reset (IPC `achievements.reset`).
- Written by the trusted local `achievements.configure` (`ipc.md`: TV Settings → Badges, or `bear-den-tv badges on|off`). The counters themselves live in the state database, never in `config.json` ([`docs/security.md`](../docs/security.md#den-badges)).
- Additive: `schema_version` stays 1.

## Now playing on phones (optional field)

- `remote.now_playing` (boolean, default `true`; the built-in default file writes `true`, and a file without the key means `true`): paired phones with the `controller` permission see what the app in front reports it is playing (`state.now_playing`, [`http.md`](http.md#now-playing-statenow_playing)). `false` removes `now_playing` from every snapshot.
- Written by the trusted local `remote.now_playing` (`ipc.md`: TV Settings → Now playing on phones). The titles themselves are never stored here or anywhere else.
- Additive: `schema_version` stays 1.

## TV control over HDMI-CEC (optional field)

- `cec` (object, absent = off): `{"enabled": false, "volume_target": "pc"}`. `enabled: true` lets Bear Den send HDMI-CEC messages to the TV over the HDMI cable (standby with the sleep timer and screen off, power on and switch to Bear Den's input on wake and Home, `tv.power`). `volume_target` is `pc` (the phone's volume buttons change the PC's volume, the default) or `tv` (they send the TV's volume keys); it applies only while `enabled` is true. A file without the block means off.
- Written by the trusted local `cec.configure` (`ipc.md`: TV Settings → TV control over HDMI (CEC)).
- Needs an HDMI-CEC adapter the kernel exposes as `/dev/cecN`; without one the setting is stored but nothing happens, and `state.cec.reason` says why.
- Additive: `schema_version` stays 1.
