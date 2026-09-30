# tools/pixelart: the pixel-art generator

All of Bear Den's built-in pixel art is drawn by Python code in this folder:
the worlds (wallpapers), the bears, the ornaments, the corner scenes and their
weather props, the featured-panel rooms, the weather icons, Bear Den's own app
icons and the Den badge medals. It uses only the Python standard
library (no Pillow, no image editor). Why: [ADR 0005](../../docs/decisions/0005-pixel-art.md).

- The output is **deterministic**: running it twice changes nothing.
- The output PNGs are **committed**. Edit the code, never the PNGs.
- Guide for themes and art rules: [`docs/THEMES.md` → Pixel art](../../docs/THEMES.md#pixel-art).
- Words like "art pixel" and "ornament": [`docs/THEMES.md`](../../docs/THEMES.md#words-used-here)
  and [`docs/GLOSSARY.md`](../../docs/GLOSSARY.md).

## Files

| File | Draws | Writes |
|---|---|---|
| [`build.py`](build.py) | the entry point; runs the others | (see below) |
| [`px.py`](px.py) | the drawing library: `Img` (put, rect, line, disc, ellipse, poly, glow, blit, outline, scaled, save), `from_ascii`, `sheet`, noise (`fbm1`, `fbm2`), `dither`, `Rng` | nothing |
| [`common.py`](common.py) | shared scenery: ridges, pines, clouds, rain, flames, smoke; the world size `W, H = 480, 270` | nothing |
| [`worlds/<id>.py`](worlds/) | one theme's world: `den`, `forest`, `midnight`, `campfire`, `winter` | `themes/<id>/wallpaper.png`, `backdrop.png` (phone), sprite sheets, and the `wallpaper` block (and `phone.backdrop`) of `themes/<id>/theme.json` |
| [`bears.py`](bears.py) | the family (dad, mama, cub) and the logo | `apps/tv-shell/assets/pixel/head-<kind>.png`, `bear-<kind>.png`, `bear-mark.png`; `apps/tv-shell/qml/BearRig.js`; phone copies in `apps/remote-web/static/art/pixel/` (`bear-mark.png`, `head-<kind>[-blink|-sleep].png`) |
| [`ornaments.py`](ornaments.py) | the built-in ornaments as letter grids (`SPRITES`), plus Winter's own snowflake (`EXTRA`) | `apps/tv-shell/assets/ornaments/<name>.png`, `themes/winter/snowflake.png` |
| [`scenes.py`](scenes.py) | the props of Home's corner scenes (bears are placed by QML) | `apps/tv-shell/assets/pixel/scene-*.png` |
| [`hero.py`](hero.py) | the featured panel's rooms: cinema, cabin, arcade, nook (Spotify), theatre (Jellyfin), retro (RetroArch) | `apps/tv-shell/assets/pixel/hero-<scene>.png`, `apps/tv-shell/qml/HeroRig.js` |
| [`weather.py`](weather.py) | ten weather icons | `apps/tv-shell/assets/pixel/weather-<name>.png` |
| [`appicons.py`](appicons.py) | Bear Den's own app icons: one 32×32 badge per adapter (bear ears, brand colours, a motif; never an official logo; shown with Settings → App icons → Bear Den style, or when an app's own icon is not available; [`docs/THEMES.md` → App icons](../../docs/THEMES.md#app-icons)) | `apps/tv-shell/assets/pixel/app-<adapter>.png`; phone copies in `apps/remote-web/static/art/pixel/` |
| [`uiicons.py`](uiicons.py) | Bear Den's UI icons: one 32×32 badge per shell pill, settings row or setup card (`gear`, `apps`, `themes`, `phone`, `display`, `home`, `play`, `power`, `about`, `plus`, `globe`, `refresh`, `medal`, `wave`), the app icons' bear-ear badge in its own den colour with a bold cream motif ([`docs/THEMES.md` → UI icons](../../docs/THEMES.md#ui-icons)) | `apps/tv-shell/assets/pixel/icon-<name>.png`; phone copies of the icons in `PHONE` (today `plus`) in `apps/remote-web/static/art/pixel/` |
| [`badges.py`](badges.py) | the Den badge medals and their silhouettes ([`docs/THEMES.md` → Den badges](../../docs/THEMES.md#den-badges)) | `apps/tv-shell/assets/pixel/badge-<id>[-locked].png`; phone copies in `apps/remote-web/static/art/pixel/` |
| [`weatherprops.py`](weatherprops.py) | the props of the weather in the corner scene: a tarp on poles, the startled "!", a leaf umbrella ([`docs/THEMES.md` → Weather in the corner scene](../../docs/THEMES.md#weather-in-the-corner-scene)) | `apps/tv-shell/assets/pixel/scene-wx-{tarp,startle,umbrella}.png` |

`BearRig.js` and `HeroRig.js` are generated. Do not edit them by hand.

## Run it

From the repository root:

```sh
python3 -B tools/pixelart/build.py                      # everything
python3 -B tools/pixelart/build.py ornaments bears      # only these parts
python3 -B tools/pixelart/build.py --preview winter     # also write enlarged previews
```

- Part names: `den`, `forest`, `midnight`, `campfire`, `winter`, `bears`,
  `ornaments`, `scenes`, `hero`, `weather`, `weatherprops`, `appicons`, `badges`, `uiicons`.
- `--preview` writes to `build/pixel-preview/` (for example `winter-f0.png`,
  `bears.png`, `ornaments.png`, `scenes.png`, `hero.png`, `weather.png`,
  `weatherprops.png`, `appicons.png`, `badges.png`, `uiicons.png` and
  `uiicons-small.png`).
- `-B` stops Python writing `__pycache__` files into the tree.

## Recipes

### Redraw an ornament

1. Open [`ornaments.py`](ornaments.py) and find the name in `SPRITES`.
2. Edit its rows. One letter is one art pixel, coloured from `PAL`; `.` is
   transparent. The second value (`True`/`False`) adds a dark outline.
3. Run `python3 -B tools/pixelart/build.py --preview ornaments` and open
   `build/pixel-preview/ornaments.png`.
4. Run `make shell`, then `scripts/sandbox.sh shot --theme <a theme that uses it>` and look.

Done when: only that ornament's PNG changed in `git status`, and the
screenshot looks right.

### Add an ornament

1. Add `'<name>': ([rows…], True),` to `SPRITES` in
   [`ornaments.py`](ornaments.py). The name must match `^[a-z][a-z0-9-]{0,31}$`.
   Keep it 7–16 art pixels.
2. Run `python3 -B tools/pixelart/build.py ornaments`. It writes
   `apps/tv-shell/assets/ornaments/<name>.png`; assets are globbed into the
   shell build, so there is no list to edit.
3. Add the name to the list in
   [`docs/THEMES.md` → Built-in ornaments](../../docs/THEMES.md#built-in-ornaments).
4. Use it from a theme (`focus.tip`, `focus.extras`, `heading`, `bears.hat`, …).
5. `make shell`, then validate and screenshot the theme.

An ornament for **one theme only** doesn't belong here: put `<name>.png` in
that theme's folder.

Done when: `build/bin/bear-den-tv themes validate themes/<id>` passes for the
theme that uses it, and you looked at the screenshot.

### Change a world (wallpaper)

1. Edit [`worlds/<id>.py`](worlds/). `build()` returns the 480×270 image and
   a list of sprites `(file, sheet, fps, x, y)`; `phone(img)` returns the
   phone backdrop (a 168×270 crop in the built-ins).
2. Run `python3 -B tools/pixelart/build.py --preview <id>`. It rewrites the
   PNGs and the `wallpaper` block of `themes/<id>/theme.json`, so sprite
   positions always match the art.
3. Open `build/pixel-preview/<id>-f0.png` and `<id>-f1.png` (two animation frames).
4. `make shell`, then `scripts/sandbox.sh shot --theme <id>` and look.

Done when: `build/bin/bear-den-tv themes validate themes/<id>` passes and
the screenshot looks right.

### Add a world for a new built-in theme

1. Make the theme first ([`docs/THEMES.md` → Make a theme in five minutes](../../docs/THEMES.md#make-a-theme-in-five-minutes),
   step 7): `themes/<id>/theme.json` must exist, because the build updates it.
2. Copy [`worlds/winter.py`](worlds/winter.py) to `worlds/<id>.py`, set
   `ID = '<id>'` and draw.
3. Add `'<id>'` to `WORLDS` in [`build.py`](build.py).
4. Run `python3 -B tools/pixelart/build.py --preview <id>`, then continue as
   in "Change a world".

Done when: the theme validates, `go test ./internal/themes` passes, and the
screenshot looks right.

### Redraw the bears

See [`docs/THEMES.md` → Change the bears](../../docs/THEMES.md#change-the-bears):
what must stay the same (kind names, pose order, frame rows) and every
screen to check.

## Checks

```sh
python3 -B tools/pixelart/build.py && git status --short   # a second run changes nothing
make test-shell                                            # the shell loads the new art
go test ./internal/themes                                  # every built-in theme validates
```
