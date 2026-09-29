# ADR 0012 — App icons: the app's own by default, served safely to phones

Date: 2026-09-29. Status: accepted.

Bear Den bundles its own original icon for every app and never commits a
third-party logo ([`docs/THEMES.md` → App icons](../THEMES.md#app-icons)).
Until now those drawings came before the icon an installed Flatpak exports,
so the TV and the phones always showed Bear Den's art. The owner decided
that an installed app should look like itself by default, with Bear Den's
drawings as a choice.

## Decision

- **A layout setting**, `ui.app_icons`: `app` (the default; missing means
  `app`) or `bear_den`. It sits beside `art_style` in the layout, so TV
  Settings (App icons: App's own / Bear Den style) and the phone's Layout
  editor both change it; phones read it as `state.appearance.app_icons`.
- **One order everywhere** an app is shown (TV tiles, the featured panel
  and its room's screen, the launch overlay, the install card, Add apps,
  phone tiles): the owner's brand folder; with `app`, the icon the
  **installed** Flatpak exports when that Flatpak is the app itself; Bear
  Den's icon; a monogram. "Installed" is what discovery says
  (`applications[].installed`), so an export left behind by an uninstalled
  app never shows. The streaming sites run in Chromium and never show its
  icon; the Browser tile is Chromium and may. This is data (a table of the
  adapters whose Flatpak only hosts them, `adapters.OwnFlatpakIcon` in Go and
  `ownIconFlatpakIdFor` in the shell), not a branch on a name.
- **Nothing third-party is committed.** The exported icons are read from the
  owner's own disk at run time. Committed screenshots use Bear Den style or
  fixtures without real exports.
- **Phones get icons from the coordinator**, `GET /api/v1/apps/{adapter}/icon`
  ([`contracts/http.md`](../../contracts/http.md#app-icons)): the adapter name
  must be in the adapter table (no path, URL or file name from the phone);
  any paired phone may ask, guest passes included, because they see the same
  tiles; a 404 means "draw Bear Den's". The phone keeps Bear Den's icon until
  the TV's has loaded.
- **Never SVG to phones.** Go has no SVG rasteriser in the standard library
  and we add no dependency for one, so SVG brand icons and exports are
  skipped for phones (the TV still shows them). PNG and JPEG are size-capped
  (1 MiB, 1024 px), decoded and re-encoded as PNG (validating and dropping
  metadata), and served with `Content-Type: image/png`, `nosniff` and
  `Content-Security-Policy: default-src 'none'`.

## Consequences

- An owner who prefers the house style picks Bear Den style once.
- The featured panel's rooms stay Bear Den's art; only the room's screen
  shows the chosen icon.
- `bear-den-tv artwork fetch` cached Flathub icons that the shell no longer
  reads: the fetched icons belonged to apps that may not be installed, which
  this order excludes. **Update 2026-09-29:** the command, its download and
  its cache folder (`$XDG_CACHE_HOME/bear-den-tv/brand`) were removed; an
  old cache there is simply never read.
- An app whose only icon is SVG shows Bear Den's icon on phones and its own
  on the TV.
- Not yet seen on the TV: which of the installed apps export a PNG at 128 or
  256 px (Flathub requires one for most apps).
