# apps/remote-web/: the phone remote (TypeScript, Preact)

A static single-page app that paired phones open in the browser. The
coordinator serves it from the binary: `dist/` is committed and embedded by
Go (`//go:embed all:apps/remote-web/dist` in [`embed.go`](../../embed.go)),
so a change here ships only after `make web` rebuilds `dist/` and the Go binary
is rebuilt. The API it speaks: [`contracts/http.md`](../../contracts/http.md)
and [`contracts/actions.md`](../../contracts/actions.md).

## Key files

| File | Role |
|---|---|
| [`src/main.tsx`](src/main.tsx) | entry: mounts `Root`, starts the controller, mirrors `state.appearance` into CSS variables; the only module that reads browser globals |
| [`src/app.ts`](src/app.ts) | the controller: every side effect (API calls, socket, holds, toasts); boot order info → invite fragment → session |
| [`src/api.ts`](src/api.ts) | typed HTTP client (`ApiClient`, CSRF header on writes) and `EventsSocket` (`/api/v1/events`, backoff, ping, visibility) |
| [`src/state.ts`](src/state.ts) | pure reducer and store; holds the last server snapshot verbatim |
| [`src/contract.ts`](src/contract.ts) | TypeScript mirror of the schemas (`ActionName`, `ActionArgs`, `StateSnapshot`, `ServerMessage`, ...) |
| [`src/hold.ts`](src/hold.ts) | `HoldController`: `hold.start`/`renew`/`stop` leases; the server does the repeating |
| [`src/i18n.ts`](src/i18n.ts) | every user-facing string (`t`, `ACTION_NAMES`) |
| [`src/touchpad.ts`](src/touchpad.ts) | touchpad gestures → `pointer.move`/`click`/`scroll`: drag, tap, two-finger tap, two-finger scroll; coalesced, one request in flight, within the contract's limits |
| [`src/uuid.ts`](src/uuid.ts) | UUID v4 from `crypto` only (works on plain-HTTP LAN pages) |
| `src/device-name.ts`, `src/press-fx.ts`, `src/icons.tsx`, `src/vines.tsx` | default device name, press ripple, inline SVG icons, `AppArt` (the app tiles' icons: the app's own icon from the TV, `GET /api/v1/apps/{adapter}/icon` via `appIconUrl` with `?icons=<appearance.app_icons>&installed=0|1`, shown only once it has loaded (a `data-own` attribute set by its load/error handlers); otherwise Bear Den's own `app-<adapter>` art from `tools/pixelart`/`tools/classicart` appicons.py, never bundled official logos) and `Art` (Pixel: `static/art/pixel/*.png` at whole-number scales; Classic: `static/art/*.svg`), theme-styled tile vines drawn on a pixel grid or, in Classic, as smooth Bézier vines (`tests/unit/vines.spec.ts`, `tests/unit/art-style.spec.ts`) |
| [`src/views/`](src/views/shell.tsx) | `shell.tsx` (frame, tabs), `remote.tsx` (D-pad, apps, playback, volume, text; an app tile's status line from `tileStatus` in `state.ts`: "Installing 42%" from the snapshot while an install runs, "Ready" for `INSTALL_READY_MS` after one this phone saw finish (`installReady`, ended by a controller timer), "Not installed", launching, in front, running), `nowplaying.tsx` (the Now playing card: `state.now_playing`, position extrapolated from `position_at` + `rate` with a 1 s tick only while visible and playing; `snapshotAt` and `hidden` in the store), `sleep.tsx` (Sleep: timer chips, time left counted from `state.power.sleep_at_ms` with a 1 s tick only while a timer runs and the page is visible, Cancel, Screen off), `tv.tsx` (TV on / standby over HDMI-CEC, drawn only while `tv.power` is available; the volume group's heading, "TV volume" while `state.cec` sends the volume buttons to the TV), `install.tsx` (Add apps: owner phones only, never a guest pass or a family phone (`mayInstall`), while `app.install` is listed; one row per missing Flatpak, the web apps as one Chromium row; Install, progress (`<progress>`, from snapshots only) and Cancel through `app.install`/`app.install_cancel`), `touchpad.tsx` (the Touchpad for web apps: drawn only while `pointer.move` is available and never on a guest pass; gestures from `src/touchpad.ts`, sent quietly with `app.pointer`), `pair.tsx`, `editor.tsx` (layout; includes Art style and App icons: App's own / Bear Den style, `ui.app_icons`), `devices.tsx` (owner), `badges.tsx` (the Badges tab: the TV's Den badges, read-only, only when `state.achievements` is present, which the coordinator sends to controller phones and never to guest passes; names and hints in `i18n.ts` `t.badges`, medals from `static/art/{pixel/,}badge-<id>[-locked].*`), `about.tsx` |
| `src/app.css`, `static/` | styles (Pixel by default: square corners, square particles, stepped motion, `html[data-pixel]` draws the backdrop unsmoothed; the Classic overrides at the end of the file, keyed on `html[data-art='classic']`, restore the rounded, smooth look); `index.html`, manifest, icons and art copied verbatim into `dist/` |
| [`scripts/pixel-icons.mjs`](scripts/pixel-icons.mjs) | makes the PWA icons from `static/art/pixel/bear-mark.png` (nearest-neighbour); the pixel art itself comes from `tools/pixelart` |
| [`scripts/build.mjs`](scripts/build.mjs) | esbuild bundle into `dist/`, deterministic (no hashes or timestamps) |
| [`scripts/serve-dist.mjs`](scripts/serve-dist.mjs) | dev-only static server for Playwright with the production headers |

## How it talks to the coordinator

- **State in, over the WebSocket.** `EventsSocket` receives `state`,
  `action_result`, `hold`, `revoked` and `pong` (anything else is dropped), and
  refetches `/api/v1/state` on every (re)open. The server is the only
  authority on target, epoch and capabilities; the store keeps its snapshot
  as sent.
- **Actions out, as POSTs.** `tap()` in `app.ts` builds `{protocol,
  request_id: uuidV4(), context_epoch, target, action, args}` and posts it to
  `/api/v1/actions`. `context_epoch` is the epoch of the snapshot the button
  was drawn from; a `stale_epoch` result refetches state and shows a toast.
  `target` is `shell` for the actions in `SHELL_TARGETED` (`home`,
  `app.launch`, `app.close`, `shell.restart`, `power.sleep_timer`,
  `display.off`, `tv.power`, `app.install`, `app.install_cancel`), otherwise `active`. A `failed/display_off` result (the
  press only woke the TV's screen) is shown as an info toast.
- **Holds** go over the socket (`hold.*`), only for `nav.*`.
- **Capabilities gate every control.** A listed but unavailable capability
  renders disabled with the server's reason; an unlisted one is not rendered.
  Result `message`s from the server are shown verbatim.

## Appearance

The phone mirrors the TV's theme from `state.appearance` (resolved by
`internal/themes`). `applyAppearance` in `main.tsx` sets CSS custom
properties on `<html>` through the CSSOM: `--sage`, `--sage-rgb`,
`--sage-deep`, `--sage-deep-rgb`, `--sage-light`, `--veil-rgb`, `--backdrop`,
plus four attributes that `app.css` keys on:

- `data-particles`: the particle set (`fireflies` by default; `embers`,
  `leaves`, `snow`, `stars`, `none`);
- `data-style`: the theme id (`den-dark` by default; `app.css` has rules for
  `plain-dark` and `performance`);
- `data-pixel`: `on` draws the backdrop unsmoothed (pixel art);
- `data-art`: the art style, `pixel` (default, also before pairing) or
  `classic` (`artStyleOf` in `icons.tsx`). Classic brings back radii (the
  `--r*` variables, unset so 0 under Pixel), SVG art, round particles, the round
  D-pad backdrop and smooth motion with rotation and scale. Markup that differs
  switches in TSX from the same value: `Art` picks the SVG or the PNG, `Vines`
  the Bézier renderer (`ClassicCorner`) or the pixel grid one.

A phone on a guest pass (`me.permissions` `["guest"]`, `me.expires_at_ms`;
[`contracts/http.md`](../../contracts/http.md#guest-passes)) sees only what it
may use: `mayUse` in `state.ts` mirrors `GUEST_ACTIONS` from `contract.ts`
(the server enforces it), so Close app and restart are not drawn, and
`GuestChip` in `shell.tsx` shows "Guest · ends 04:00". When a guest's session
ends (`revoked` or a 401) the pair screen says "Your guest pass has ended"
(`pass_ended`).

Theme art is fetched same-origin from `/themes/...`. Controller phones may receive
`state.now_playing` (the Now playing card) and `state.achievements` (the
Badges tab); phones never receive `state.weather`, so the phone shows no
weather.

## Content-Security-Policy

```
default-src 'self'; img-src 'self' data:; connect-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'
```

It is defined twice and **the two must stay identical**: `cspValue` in
[`internal/remote/server.go`](../../internal/remote/server.go) (production) and
`CSP` in [`scripts/serve-dist.mjs`](scripts/serve-dist.mjs) (Playwright). No
inline `<script>`, no `<style>` blocks, no `style="..."` attributes, no
external origins. Dynamic styling goes through classes, `data-*` attributes or
`element.style.setProperty` (CSSOM, allowed).

## Rules

- **No string literals in views.** Copy lives in [`src/i18n.ts`](src/i18n.ts);
  views use `t.*`. Server messages are not translated.
- **Views are pure** functions of `AppState` plus the controller; side effects
  only in `app.ts`, and the reducer in `state.ts` stays pure.
- **Types follow the contract** and never widen it. A contract change updates
  `contract.ts` in the same commit
  ([`contracts/AGENTS.md`](../../contracts/AGENTS.md#change-a-contract)).
- **Named actions only.** Nothing here builds URLs to fetch, keycodes or
  commands for the TV.
- **Rebuild `dist/`** with `make web` (`npm ci` + `npm run build`) and commit
  it with the source change. `make check-web-dist`
  ([`scripts/check-web-dist.sh`](../../scripts/check-web-dist.sh), run by CI)
  fails when the committed `dist/` differs from a fresh build.

## Add a control

1. Check the action exists in [`src/contract.ts`](src/contract.ts)
   (`ActionName`). A new action starts in
   [`contracts/AGENTS.md` → Add an action](../../contracts/AGENTS.md#add-an-action).
2. Add its copy to [`src/i18n.ts`](src/i18n.ts) (`t.*`, and its label in
   `ACTION_NAMES`).
3. Draw it in the right view under [`src/views/`](src/views/remote.tsx),
   gated by `snapshot.capabilities[action]`: disabled with the server's
   `reason` when unavailable, absent when unlisted. Send it through the
   controller (`tap()` in [`src/app.ts`](src/app.ts)); add the action to
   `SHELL_TARGETED` if it targets the shell.
4. A unit test in [`tests/unit/`](tests/unit/state.spec.ts) for any new
   reducer or controller logic.
5. Rebuild: `make web`, then `make go` to embed the new `dist/`.

Done when: the checks below pass, `git status` shows the rebuilt `dist/`, and
the control works against `make dev` (open `http://127.0.0.1:8787`, pair with the
code the shell shows on its Pair phone screen).

## Change a snapshot field the phone shows

1. Change the contract first:
   [`contracts/AGENTS.md` → Change a contract](../../contracts/AGENTS.md#change-a-contract).
2. Mirror it in [`src/contract.ts`](src/contract.ts), field for field.
3. Read it in a view; keep optional fields optional (a locked session and
   lower permissions omit things).
4. `make web`.

Done when: `tests/contract.spec.ts` passes and the view renders both with and
without the field.

## Tests

| Suite | Where | Runs |
|---|---|---|
| Unit | [`tests/unit/`](tests/unit/state.spec.ts) (`state.spec.ts`, `hold.spec.ts`, `vines.spec.ts`, `art-style.spec.ts`, `nowplaying.spec.ts`, `sleep.spec.ts`, `guest.spec.ts`, `invitation.spec.ts`, `badges.spec.ts`, `touchpad.spec.ts`, `tv.spec.ts`, `install.spec.ts`) | vitest under Node, fake timers, injected fakes |
| Contract | [`tests/contract.spec.ts`](tests/contract.spec.ts) | Ajv 2020 loads the layout, action, state and config schemas; every `contracts/fixtures` file must be in `SCHEMA_FOR` or `SEMANTIC_ONLY`; client-built requests, holds and edited layouts must validate |
| Browser | `tests/e2e/` (Playwright, `playwright.config.ts`, `serve-dist.mjs`) | **none yet** (the folder does not exist); `npm test` passes `--pass-with-no-tests` so the empty suite does not fail the run. The phone screenshots under `docs/screenshots/2026-09-28-*` were captured ad hoc with Playwright's Chromium, not by a test suite |

## Checks

```sh
. scripts/env.sh
cd apps/remote-web
npm run lint        # tsc --noEmit + eslint
npm run test:unit   # vitest: unit + contract
npm test            # vitest, then playwright (or `make test-web` from the root)
npm run build       # or `make web` from the root (npm ci + build)
```
