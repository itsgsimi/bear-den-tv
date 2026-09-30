# apps/web-nav/: the web apps' navigation script (TypeScript)

The script Bear Den injects into every page of a web app (Netflix, Disney+,
Hulu, the Browser tile) so a TV remote can drive an ordinary website. The
coordinator ([`internal/applications/web`](../../internal/applications/web/web.go))
runs the web app's browser (Google Chrome or Brave from Flathub) with `--remote-debugging-pipe` and puts this script in
an isolated world named `bearden` of each page; the site cannot see it.
Why and the security model: [ADR 0010](../../docs/decisions/0010-web-apps-over-cdp-pipe.md).
`dist/nav.js` is committed and embedded by Go (`WebNav` in [`embed.go`](../../embed.go)),
like the phone remote's `dist/`.

## Files

| File | Role |
|---|---|
| [`src/nav.ts`](src/nav.ts) | everything the page does: focus targets (links, buttons, `[role=button]`, `[tabindex]`, `cursor: pointer` cards, hint `prefer`), spatial moves, the focus ring (closed shadow root), OK, the Back sequence, media keys, text preparation, the touchpad cursor, status reports through `__bdtvReport` |
| [`src/spatial.ts`](src/spatial.ts) | the direction geometry (`score`, `pick`), pure |
| [`src/types.ts`](src/types.ts) | `Hints`, `Reply`, `Effect`, `Status`: mirrored field for field by `Reply`, `Effect`, `Status` in [`browser.go`](../../internal/applications/web/browser.go) |
| [`src/main.ts`](src/main.ts) | entry: `globalThis.__bdtvNav = { install }`; the coordinator appends `__bdtvNav.install(<hints>)` |
| [`hints/<adapter>.json`](hints/) | per-site data ([`contracts/web-hints.schema.json`](../../contracts/web-hints.schema.json)): selectors to prefer, skip, start on, overlays and their close buttons, the player, the Back order, the media keys. All **UNVERIFIED** (`"verified": false`): no real service may be automated |
| [`scripts/build.mjs`](scripts/build.mjs) | esbuild → `dist/nav.js` (IIFE, deterministic) |
| [`tests/nav.spec.ts`](tests/nav.spec.ts), [`tests/driver.ts`](tests/driver.ts), [`tests/fixtures/`](tests/fixtures/) | Playwright (headless Chromium) against local pages that mimic streaming layouts: a poster grid with an overlay, a player with the site's own shortcuts, a search box. `Driver` injects the script exactly like the coordinator (isolated world + binding) and performs effects with trusted input |

## Rules

- **The script decides, the coordinator acts.** It never receives strings
  from phones: `apply(action, n?)` gets a fixed name and at most an integer.
  What needs trusted input comes back as an `Effect` (click at a viewport
  point, keys from the closed set, text), which the coordinator checks
  (`checkEffect`) and performs. A new effect kind or key is a change to
  `AllowedKeys`/`checkEffect` in Go, the schema's `key` enum, and this file.
- **Never set `currentTime`**; media goes through the site's shortcuts.
- **Works without hints.** A hint selector that matches nothing or does not
  parse is ignored.
- **No labels leave the page.** Reports carry a role, an index and flags;
  poster titles are private.
- Rebuild and commit `dist/` with every source change: `make webnav`
  (`scripts/check-web-dist.sh` checks it in CI).

## Checks

```sh
. scripts/env.sh
make webnav                                       # build dist/nav.js
cd apps/web-nav && npx playwright install --with-deps chromium   # once; --with-deps adds Chromium's system libraries (needs sudo)
make test-webnav                                  # Playwright, local fixtures only
cd apps/web-nav && npm run lint                   # tsc + eslint (also in make lint)
```

The Go end-to-end tests (`internal/applications/web/e2e_test.go`,
`internal/session/web_e2e_test.go`) run the real coordinator code against the
same fixtures in the same Playwright Chromium; they skip, saying why, when it
is not installed.
