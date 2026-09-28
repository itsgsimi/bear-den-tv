# Configuration contract

`config.json` lives at `$XDG_CONFIG_HOME/bear-den-tv/config.json`. The coordinator is its only writer. Structure: [`config.schema.json`](config.schema.json). Portable subset: [`layout.schema.json`](layout.schema.json) (`ui` + `sections`). Code: [`internal/config`](../internal/config/config.go).

## Semantic rules (enforced by `internal/config`, tested with negative cases)

1. `applications[].id` and `sections[].id` are unique within their arrays and never collide with the reserved words `active`, `shell`.
2. Every `sections[].application_ids[]` entry references an existing `applications[].id`.
3. `applications[].launch.args` must be a subset of the adapter's approved argument list: `vacuumtube` ⇒ `--fullscreen`, `--no-window-decorations`; `plex-htpc` and `moonlight` ⇒ none. `launch.app_id` must equal the adapter's known Flatpak id (`rocks.shy.VacuumTube`, `tv.plex.PlexHTPC`, `com.moonlight_stream.Moonlight`).
4. `remote.enabled` requires `onboarding.lan_consent == true` and every `remote.interfaces[]` to be an interface that exists on this host and is not loopback or a virtual interface (names starting `docker`, `br`, `veth`, `tun`, `tap`, `wg`, `virbr`, `vmnet`, `lxc`, `cni`, `flannel`, `tailscale`, `utun`). Non-existent interfaces fail loud at load ("interface wlan9 not present") and the listener stays down; the shell shows the reconfiguration route.
5. `remote.transport == "https"` with `remote.enabled` requires readable `certificate_file` and `private_key_file` that parse as a PEM certificate/key pair.
6. `remote.http_layout_editing` is only honored in `trusted-lan-http`; in `https` layout editing is governed by device permission alone.
7. `plex_content.enabled` requires `connection_ref` and `server_url`; the token is looked up in the secret store, never in this file. A file containing a key named `token`, `x_plex_token`, or `password` anywhere is rejected.
8. `revision` must be greater than the revision of the currently loaded configuration when written through the API (optimistic concurrency); on disk it must be ≥ 1.
9. `schema_version` other than 1 ⇒ the file is refused, the last-known-good copy is loaded, and the shell shows an error banner.
10. `weather.place.latitude` and `weather.place.longitude` carry at most 2 decimals (about 1 km): the coordinator never stores a more precise location. `weather.enabled` requires a `place` (also structural) and `units` is `celsius` or `fahrenheit`.

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
