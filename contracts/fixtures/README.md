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
| `action.request.pointer-move.valid.json` | `action.schema.json#/$defs/request` (touchpad move) |
| `action.request.pointer-click.valid.json` | `action.schema.json#/$defs/request` (touchpad left click) |
| `action.request.pointer-scroll.valid.json` | `action.schema.json#/$defs/request` (touchpad scroll) |
| `action.request.pointer-move-too-far.invalid.json` | rejected: `pointer.move` `dx` is at most 400 |
| `action.request.pointer-click-coordinates.invalid.json` | rejected: phones never send coordinates (`pointer.click` takes only `button`) |
| `action.request.tv-power.valid.json` | `action.schema.json#/$defs/request` (`tv.power`, standby) |
| `action.request.tv-power-off.invalid.json` | rejected: `tv.power` `power` must be `on` or `standby` |
| `action.request.app-install.valid.json` | `action.schema.json#/$defs/request` (`app.install` for a config app id) |
| `action.request.app-install-cancel.valid.json` | `action.schema.json#/$defs/request` (`app.install_cancel`) |
| `action.request.app-install-ref.invalid.json` | rejected: `app.install` takes only `app_id` (a phone never names a Flatpak ref) |
| `action.request.app-uninstall.valid.json` | `action.schema.json#/$defs/request` (`app.uninstall` with `delete_data`) |
| `action.request.app-uninstall-system.invalid.json` | rejected: `app.uninstall` takes only `app_id` and `delete_data` (nothing asks for a system-wide removal) |
| `action.result.observed.valid.json` | `action.schema.json#/$defs/result` |
| `action.result.failed-stale.valid.json` | `action.schema.json#/$defs/result` |
| `action.result.display-off-woke.valid.json` | `action.schema.json#/$defs/result` (`failed/display_off`: the press woke the screen and was not applied) |
| `state.shell-home.valid.json` | `state.schema.json` (shell view, pairing shown, Den badges with one to `celebrate`) |
| `state.phone-controller.valid.json` | `state.schema.json` (phone view, redacted; Den badges without `celebrate`) |
| `state.phone-now-playing.valid.json` | `state.schema.json` (phone view with DEMO `now_playing` and `remote.now_playing`) |
| `state.phone-optional-apps.valid.json` | `state.schema.json` (phone view: an installed optional app, and a missing one with `hidden: true`) |
| `state.hidden-not-bool.invalid.json` | rejected: `applications[].hidden` must be a boolean |
| `state.phone-web-app.valid.json` | `state.schema.json` (phone view: the Browser in front with the touchpad and text available, Netflix turned off: `enabled: false`, `hidden: true`) |
| `state.enabled-not-bool.invalid.json` | rejected: `applications[].enabled` must be a boolean |
| `state.now-playing-bad-status.invalid.json` | rejected: `now_playing.status` must be `playing`, `paused` or `stopped` |
| `state.now-playing-no-title.invalid.json` | rejected: `now_playing.title` must not be empty (no title means no `now_playing`) |
| `state.phone-now-playing-behind-home.valid.json` | `state.schema.json` (phone view: the shell in front, YouTube paused behind Home: `now_playing.foreground: false`, media controls available for it) |
| `state.now-playing-behind-home-stopped.invalid.json` | rejected: with `foreground: false` the status must be `playing` or `paused` |
| `state.now-playing-foreground-not-bool.invalid.json` | rejected: `now_playing.foreground` must be a boolean |
| `state.phone-now-playing-plex-server.valid.json` | `state.schema.json` (phone view: Plex HTPC playing behind Home, read from the Plex server: `source: "plex_server"`, media actions unavailable with the read-only reason) |
| `state.now-playing-bad-source.invalid.json` | rejected: `now_playing.source` must be `mpris` or `plex_server` |
| `state.phone-sleep-warning.valid.json` | `state.schema.json` (phone view: a 45-minute sleep timer in its last minute, `power.suspend` unavailable) |
| `state.power-bad-display.invalid.json` | rejected: `power.display` must be `on` or `off` |
| `state.phone-cec.valid.json` | `state.schema.json` (phone view: HDMI-CEC enabled, TV on, volume buttons driving the TV) |
| `state.phone-cec-unavailable.valid.json` | `state.schema.json` (phone view: no HDMI-CEC device, with the reason) |
| `state.cec-bad-tv-power.invalid.json` | rejected: `cec.tv_power` must be `on`, `standby` or `unknown` |
| `state.shell-setup.valid.json` | `state.schema.json` (shell view before first-run setup: `onboarding.completed` false, `autostart` off but available) |
| `state.onboarding-not-bool.invalid.json` | rejected: `onboarding.completed` must be a boolean |
| `state.autostart-no-available.invalid.json` | rejected: `autostart` must say whether it is `available` |
| `state.phone-autostart.invalid.json` | rejected: a phone view (with `me`) never carries `onboarding` or `autostart` |
| `state.shell-tips.valid.json` | `state.schema.json` (shell view after setup: bear tips on, two done, one shown today) |
| `state.tips-unknown-id.invalid.json` | rejected: `tips.done` holds only the seven tip ids |
| `state.phone-tips.invalid.json` | rejected: a phone view (with `me`) never carries `tips` |
| `config.tips.valid.json` | `config.schema.json` (bear tips: two done, one "Not now", last shown the day before) |
| `config.tips-streak-too-high.invalid.json` | rejected structurally: `tips.not_now_streak` is at most 3 |
| `state.phone-app-notes.valid.json` | `state.schema.json` (phone view: Plex HTPC and YouTube with their `applications[].notes`) |
| `state.app-notes-too-many.invalid.json` | rejected: `applications[].notes` holds at most 6 notes |
| `state.app-note-url.invalid.json` | rejected: a note never carries a URL |
| `state.app-note-too-long.invalid.json` | rejected: a note is at most 160 characters |
| `state.shell-plex-linking.valid.json` | `state.schema.json` (shell view in Settings → Plex, a link code shown) |
| `state.shell-plex-libraries.valid.json` | `state.schema.json` (shell view choosing DEMO libraries; the sign-in kept in a private file, `stored_in: "file"`) |
| `state.plex-bad-status.invalid.json` | rejected: `plex.status` must be one of the six sign-in states |
| `state.plex-stored-in-bad.invalid.json` | rejected: `plex.stored_in` must be `keyring` or `file` |
| `state.plex-token-field.invalid.json` | rejected: `plex` has no room for a token (`additionalProperties: false`) |
| `state.phone-guest.valid.json` | `state.schema.json` (phone view of a guest pass: `me.permissions` `["guest"]` with `me.expires_at_ms`, DEMO `now_playing`) |
| `state.shell-guest-pass.valid.json` | `state.schema.json` (shell view: a live guest-pass invitation, a family phone and a guest in `devices`) |
| `state.guest-with-controller.invalid.json` | rejected: `guest` never comes with another permission |
| `state.guest-no-expiry.invalid.json` | rejected: a guest `me` must carry `expires_at_ms` |
| `state.achievements-time.invalid.json` | rejected: `achievements.earned[].day` is a calendar day (`YYYY-MM-DD`), never a time |
| `state.achievements-title.invalid.json` | rejected: an earned badge carries only `id` and `day` (no `title` or anything else) |
| `state.guest-achievements.invalid.json` | rejected: a guest pass never gets `achievements` |
| `state.locked-achievements.invalid.json` | rejected: a locked session never carries `achievements` |
| `state.phone-audio.valid.json` | a controller phone with `audio` (the PC's real mute state and volume) |
| `state.shell-audio.invalid.json` | rejected: `audio` is for phones only, never the shell |
| `state.locked-audio.invalid.json` | rejected: a locked session never carries `audio` |
| `state.phone-celebrate.invalid.json` | rejected: `achievements.celebrate` is for the shell only (a phone view has `me`) |
| `state.phone-owner-installs.valid.json` | `state.schema.json` (owner phone: one app downloading a runtime, one available with its size, a system install, the streaming sites' browser done with playback support pending, `apps.auto_update`) |
| `state.phone-owner-browsers.valid.json` | `state.schema.json` (owner phone: `apps.browser` brave, `apps.streaming_browser` chrome, and the two browsers with Brave marked `streaming_unverified` and Chrome's `notes`) |
| `state.browser-no-note.invalid.json` | rejected: every `apps.browsers[]` entry says whether it is unverified for streaming |
| `state.family-install.invalid.json` | rejected: a phone without `owner` never gets `applications[].install` |
| `state.guest-install.invalid.json` | rejected: a guest pass never gets `applications[].install` |
| `state.install-bad-state.invalid.json` | rejected: `install.state` must be one of the eight install states |
| `state.install-progress-range.invalid.json` | rejected: `install.progress` is 0..100 |
| `state.install-installed-bytes.invalid.json` | rejected: `install.installed_bytes` is not negative |
| `state.install-ref.invalid.json` | rejected: `install` has no room for a ref or anything else (`additionalProperties: false`) |
| `config.apps-auto-update-not-bool.invalid.json` | rejected structurally: `apps.auto_update` must be a boolean |
| `config.browsers-swapped.valid.json` | `config.schema.json` (the defaults swapped: the Browser tile in Google Chrome, `apps.browser: chrome`, its row's `launch.app_id` `com.google.Chrome`; the streaming sites in Brave, `apps.streaming_browser: brave`, their rows `com.brave.Browser`) |
| `config.browser-mismatch.invalid.json` | rejected semantically: `apps.browser` is `chrome` but the Browser row still runs `com.brave.Browser` (rule 3) |
| `config.browser-unknown.invalid.json` | rejected structurally: `apps.streaming_browser` must be `chrome` or `brave` |
| `config.default.valid.json` | `config.schema.json` (built-in defaults) |
| `config.web-apps.valid.json` | `config.schema.json` (a Netflix row and a Browser with its own start page) |
| `config.web-http.invalid.json` | rejected structurally: `web.url` must start with `https://` |
| `config.web-no-url.invalid.json` | rejected structurally: a streaming adapter needs `web.url` |
| `config.web-credentials.invalid.json` | rejected semantically: `web.url` carries a user name and password (rule 11) |
| `config.web-wrong-host.invalid.json` | rejected semantically: Netflix's page must be on `netflix.com` (rule 11) |
| `config.web-launch-args.invalid.json` | rejected semantically: web adapters take no launch arguments (rule 3) |
| `config.achievements-not-bool.invalid.json` | rejected structurally: `achievements.enabled` must be a boolean |
| `config.now-playing-not-bool.invalid.json` | rejected structurally: `remote.now_playing` must be a boolean |
| `config.cec-bad-volume-target.invalid.json` | rejected structurally: `cec.volume_target` must be `pc` or `tv` |
| `config.dangling-ref.invalid.json` | rejected semantically: section references unknown app |
| `config.token-leak.invalid.json` | rejected semantically: contains a `token` key |
| `config.weather-no-place.invalid.json` | rejected structurally: `weather.enabled` is true while `weather.place` is null |
| `config.weather-precise.invalid.json` | rejected semantically: weather coordinates carry more than 2 decimals (privacy, `config.md` rule 10) |
| `layout.default.valid.json` | `layout.schema.json` (no `art_style`: optional, means pixel) |
| `layout.art-style-bad.invalid.json` | rejected: `ui.art_style` must be `pixel` or `classic` |
| `layout.app-icons-bear-den.valid.json` | `layout.schema.json` (`ui.app_icons` `bear_den`: Bear Den's own icons) |
| `layout.app-icons-bad.invalid.json` | rejected: `ui.app_icons` must be `app` or `bear_den` |
