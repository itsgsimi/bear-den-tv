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
| `context_epoch` | Integer from the last `state` the sender saw. Must equal the coordinator's current epoch for every action except `home`, `app.launch`, `shell.restart`, `power.sleep_timer`, `display.off`, `tv.power`, `app.install` and `app.install_cancel` (these never touch the window in front); otherwise `failed/stale_epoch`. |
| `target` | `"active"` (whatever the coordinator currently observes in the foreground), `"shell"`, or a registered application id. Actions on `"active"` are refused when the observed target is `unknown` or `locked`. |
| `action` | One of the names below. |
| `args` | Object validated per action; extra keys are rejected. |

## Actions

| Action | Args | Who may send | Routing notes |
|---|---|---|---|
| `nav.up` `nav.down` `nav.left` `nav.right` | `{}` | controller | Shell: IPC `input` and the shell reports the resulting focus (`observed`). App: XTEST key tap on the verified foreground window (`delivered`). Holdable over WebSocket. |
| `select` | `{}` | controller | Same as nav. Never holdable. |
| `back` | `{}` | controller | Shell: one navigation level; at root it is a no-op (`observed`, `detail: "at_root"`), never exits the shell. App: mapped Back/Escape key. |
| `home` | `{}` | controller | Explicit escape: pauses the foreground app when its `home_policy` is `pause-if-supported` and the pause can be verified (a native app: its own MPRIS player reports Playing, gets Pause and then reports Paused, `detail.paused` true only then; never a key, so RetroArch keeps running; a web app: see below), activates the shell window, restores the remembered focus. Ignores stale epochs. Refused while locked (`failed/locked`). |
| `app.launch` | `{"app_id": "plex-htpc"}` | controller | Launch-or-activate a registered application. `accepted` immediately; a later `action_result` carries `delivered` (process started / window activated) then `observed` (window mapped and active) or `failed`. Ignores stale epochs. |
| `app.close` | `{"app_id": "plex-htpc", "force": false}` | controller (`force` requires owner) | Normal close = WM_DELETE to each mapped window of the app; windows the app opens in reply within 4 s are closed too (Moonlight answers a stream-window close by showing its host list), unless the app is relaunched. Available while the app is in front or left running behind Home. `force` = kill the tracked flatpak instance only, never by process name. |
| `media.play` `media.pause` | `{}` | controller | Only when `capabilities[action].available` is true. Delivered through MPRIS (`observed` when `PlaybackStatus` changes within 2 s) or mapped key (`delivered`). With the shell in front they reach the app playing behind Home (`state.now_playing.foreground: false`) only when `target` names that app; `"active"` (the shell) or another app is `failed/target_unfocused`. A web app behind Home is not controlled (its page is not in front): `failed/unsupported`. |
| `media.seek_relative` | `{"seconds": -30}` (−600..600, non-zero) | controller | Only with a verified seek capability; behind Home, the same target rule as play and pause. |
| `audio.volume_delta` | `{"delta": 5}` (−100..100, non-zero) | controller | PC/PulseAudio default sink; labeled "PC volume". With HDMI-CEC enabled and `cec.volume_target: "tv"`: the TV's volume instead (see [TV control over HDMI-CEC](#tv-control-over-hdmi-cec)). |
| `audio.mute` | `{"muted": true}` | controller | PC/PulseAudio default sink; the TV with `cec.volume_target: "tv"`. |
| `text.submit` | `{"text": "..."}` (1..256 chars, no control chars) | controller | Only into a target with a validated text adapter: a focused shell text field, or a focused field on a web app's page (the page reports it; the text replaces the field's value, then Enter). Off for other external apps. |
| `shell.restart` | `{}` | owner | Recovery only: restart a crashed/stuck shell through the coordinator supervisor. Never kills external players. |
| `power.sleep_timer` | `{"minutes": 45}` (0 cancels; else 15, 30, 45, 60, 90 or 120) | controller | Sets, replaces or cancels the sleep timer on the coordinator clock (`state.power.sleep_at_ms`). `observed` with `detail.sleep_at_ms` (null after a cancel). 60 s before it fires `state.power.warning` turns true, the TV shows "Going to sleep in 1 minute" and any input cancels it. When it fires: pause through MPRIS only if the foreground app's own player is verified (never a guessed key), then the `home` path, then the display off; while locked only the display goes off. Ignores stale epochs. |
| `display.off` | `{}` | controller | Turns the display off now (X11 DPMS; `state.power.display = "off"`). `observed` when the display reports off, `delivered` when that could not be read back, `observed` with `detail.already_off` when it was off. Any input turns it on again. Ignores stale epochs. |
| `pointer.move` | `{"dx": 12, "dy": -7}` (each −400..400, CSS pixels) | controller | Touchpad: moves the pointer on a web app's page. See [Web apps](#web-apps). `delivered`. |
| `pointer.click` | `{"button": "left"}` (`left` or `right`) | controller | Touchpad: clicks where the pointer is. `delivered`. |
| `pointer.scroll` | `{"dy": 240}` (−2000..2000, non-zero) | controller | Touchpad: scrolls the page under the pointer. `delivered`. |
| `app.install` | `{"app_id": "moonlight"}` | owner | Install the app's Flatpak for this user from Flathub. See [App installs](#app-installs). `delivered` once the install has started (progress in `state.applications[].install`), `observed` with `detail.already_installed` when it is installed. Ignores stale epochs. |
| `app.install_cancel` | `{"app_id": "moonlight"}` | owner | Stop that app's running install. `delivered`. Ignores stale epochs. |
| `tv.power` | `{"power": "on"}` or `{"power": "standby"}` | controller | HDMI-CEC, only while `capabilities["tv.power"]` is available (an adapter is present and the owner turned on `cec.enabled`). `on`: Image View On, then Active Source, so the TV wakes and switches to Bear Den's input. `standby`: Standby to the TV. `delivered` when the TV acknowledged the frames, `observed` (`detail.tv_power`) when it then reports that power state. Ignores stale epochs. |

**Guest passes** ([`http.md`](http.md#guest-passes)) are not `controller`: a device with `guest` may send only `nav.*`, `select`, `back`, `home`, `app.launch`, `media.play`, `media.pause`, `media.seek_relative`, `audio.volume_delta`, `audio.mute` and `text.submit` (`contract.GuestActions`). Every other action, including `app.close` in any mode, `shell.restart`, `app.install`, `app.install_cancel`, the `pointer.*` touchpad and any action added later, is refused to guests with `failed/forbidden`. A new action joins the guest list only on purpose.

**Owner only:** `shell.restart`, `app.install`, `app.install_cancel` and `app.close` with `force` need `owner` (`contract.OwnerActions`); `controller` and `layout_editor` phones get `failed/forbidden` ("Only the owner's phone can do that.").

Unknown or disabled actions ⇒ `failed/unsupported` with a `message` explaining why (for example "VacuumTube pause has not been verified on this installation").

### Sleep, screen off and wake

- **Every action wakes the display.** While `state.power.display` is `off`, an authorized action turns the display on first. The press that woke it is swallowed, as a TV does, so it never reaches the shell or an app: the result is `failed/display_off` ("The screen was off. This press turned it on; press again."), except `power.sleep_timer`, which also applies, and `display.off`, which leaves the display off (`observed`, `detail.already_off`).
- **The sleep warning.** While `state.power.warning` is true, any authorized action cancels the timer and then does its normal job; a key on the TV cancels it too (the shell swallows that key and sends IPC `power.activity`; with an app in front the coordinator notices TV input through the X idle counter).
- **Locked.** A locked session refuses every phone action (`failed/locked`), `power.sleep_timer` and `display.off` included, like every other action; the press still wakes a display that is off, since the TV then shows only the lock screen. A timer set before the lock still fires, but only turns the display off: nothing is paused and Home is not pressed behind the lock screen.

### TV control over HDMI-CEC

Optional, off unless an HDMI-CEC adapter is present and the owner turned on `cec.enabled` ([`config.md`](config.md#tv-control-over-hdmi-cec-optional-field)); nothing is sent on the bus otherwise. `state.cec` reports it ([`http.md`](http.md#tv-control-over-hdmi-cec-statecec)).

- **Sleep and screen off.** When the sleep timer fires and on `display.off`, the TV is sent Standby after the display step.
- **Wake.** The press that wakes a display Bear Den turned off, any action after Bear Den put the TV in standby, a TV key while the display was off, and every `home` send Image View On and Active Source, so the TV comes on and shows Bear Den's input. These run in the background: an action never waits on the HDMI-CEC bus.
- **Volume.** With `cec.volume_target: "tv"`, `audio.volume_delta` sends one Volume Up or Volume Down key press (User Control Pressed, then Released) per started 5 % of `delta` (at most 5), and `audio.mute` sends Mute Function (`muted: true`) or Restore Volume Function (`false`), to the audio system when one answers on the bus, else to the TV. `delivered` only: the TV does not report its volume. When the TV is chosen but HDMI-CEC is not working, both are unavailable with the reason; they never fall back to the PC silently.
- **Timeouts.** Every HDMI-CEC call is bounded (3 s); a TV that does not answer leaves `state.cec.tv_power` `unknown`.

## Web apps

The web adapters (`netflix`, `disney-plus`, `hulu`, `browser`; [`config.md`](config.md) rule 11) run in Flathub Chromium, driven over the DevTools pipe ([ADR 0010](../docs/decisions/0010-web-apps-over-cdp-pipe.md)). With one in front:

| Action | What happens | Outcome |
|---|---|---|
| `nav.*` | The navigation script moves its focus ring to the nearest focusable element in that direction (links, buttons, `[role=button]`, `[tabindex]`, `cursor: pointer` cards; inside an open overlay only) and reports where it landed. At a row's end, up/down scroll the page. | `observed`, `detail`: `page` (`moved`, `edge`, `scrolled`), `focus_role`, `focus_index`, `text_field`. Never a label (titles are private). |
| `select` | A trusted click at the focused element's centre; on a text field, focus only. | `delivered` (`page: click`), or `observed` (`page: text_field`). |
| `back` | First that applies: leave a text field, leave full screen, close an overlay (its Close button, else Escape), history back. At the start page nothing happens. | `observed` (`left_field`, `at_root`) or `delivered` (`exited_fullscreen`, `closing_overlay`, `history_back`). |
| `media.play`, `media.pause`, `media.seek_relative` | Only while the page reports a `<video>`. The site's documented keys (space/k, arrows; per-site hints), never setting `currentTime`. | Play/pause `observed` when the page then reports the video playing/paused (`detail.video`), else `delivered`; seek `delivered`. |
| `text.submit` | Only while the page reports a focused text field. | `delivered`. |
| `pointer.*` | The touchpad: a cursor Bear Den keeps inside the page, trusted mouse events. | `delivered`. |
| `home` | If the app's `home_policy` is `pause-if-supported` and the page reports a playing video: its pause key first. | Home's usual result, `detail.paused` true or false. |

Rules: input is sent only after the web app's own window is re-read as the active window (`target_unfocused` otherwise); XTEST keys never reach a web app. Capabilities (backend `web-cdp`) are unavailable with a reason when the session has no web manager, the desktop cannot confirm the foreground, or Bear Den holds no DevTools connection to the app ("Close it and open it again from Home"). `pointer.*` are available only with a web app in front ("The touchpad works only in web apps." otherwise), are never holdable, and are rate limited per sender: 60 `pointer.move`, 30 `pointer.scroll`, 5 `pointer.click` a second (`contract.PointerRates`); more is `failed/rate_limited`. A refusal from the page (no video, no focusable element, an effect outside the closed set) is `failed/unsupported` with its reason.

## App installs

Bear Den can install the apps it knows (the rows of the adapter table) from Flathub, for the TV's user, without root ([ADR 0011](../docs/decisions/0011-per-user-flathub-installs.md)). One owner press is the consent: nothing installs on its own.

- **What is installed.** `app_id` names a config application; the Flatpak is that application's adapter's Flatpak id (for the web apps `org.chromium.Chromium`, shared by all four). Nothing else can be named: no refs, remotes, branches or URLs cross the network. Only the per-user `flathub` remote with the URL compiled into the binary is used, only `x86_64`, with Flatpak's own signature checks.
- **What is not touched.** An app installed system-wide counts as installed and is never changed; `observed` with `detail.already_installed`.
- **Capability.** `app.install` and `app.install_cancel` are unavailable with the reason when the session has no installer or Flatpak is missing ("Flatpak isn't installed on this box"), and while locked.
- **Refusals.** An unknown app (`failed/unsupported`), another install already running (`failed/busy`), an app already installing (`delivered`, `detail.already_running`). Cancel with nothing running is `failed/unsupported`.
- **Progress** is in `state.applications[].install` ([`http.md`](http.md#app-installs-stateapplicationsinstall)), not in results.

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

Failure codes: `invalid`, `unsupported_protocol`, `unauthorized`, `forbidden`, `rate_limited`, `duplicate_mismatch`, `stale_epoch`, `no_target`, `unknown_foreground`, `locked`, `unsupported`, `target_unfocused`, `busy`, `launch_failed`, `timeout`, `internal`, `display_off` (the display was off; this press only turned it on and was not applied).

Over HTTP, `POST /api/v1/actions` returns the first terminal or `accepted` result; later results for the same `request_id` arrive on the WebSocket as `action_result` events.

## Holds (WebSocket only)

```json
{"type": "hold.start", "hold_id": "…uuid…", "action": "nav.right", "context_epoch": 47}
{"type": "hold.renew", "hold_id": "…uuid…"}
{"type": "hold.stop",  "hold_id": "…uuid…"}
```

Server-side lease per device session. The initial press is the client's own `action` (sent alongside `hold.start`, so it gets a normal result); the lease produces only the repeats, which begin `repeat_delay_ms` (350) after `hold.start` at `repeat_hz` (6). A press released before the delay therefore moves exactly once. The lease expires `hold_expiry_ms` (600) after the last renew (client renews every 200 ms). The lease is cancelled on: `hold.stop`, socket close, `visibility.hidden` message, revocation, session lock, target change (epoch bump), or a second device holding (`busy` to the second). Reconnecting never resumes a hold. Only `nav.*` actions are holdable. The server reports the lease state as `hold` events with `{hold_id, state: "active"|"expired"|"cancelled"|"busy", reason}`.
