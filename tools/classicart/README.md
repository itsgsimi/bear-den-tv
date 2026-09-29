# tools/classicart: the Classic-art generator

Bear Den has two art styles ([ADR 0006](../../docs/decisions/0006-classic-art-style.md)):
**Pixel** (drawn by [`tools/pixelart`](../pixelart/README.md)) and **Classic**
(smooth vector art and pictures). Some features were only ever pixel art. This
folder draws their Classic counterparts as SVG (and the worlds' animated
layers as soft PNG sprite sheets), from Python code that uses only the
standard library.

- The output is **deterministic**: running it twice changes nothing.
- The output SVGs and sheets are **committed**. Edit the code, never the
  outputs.
- The SVGs stay inside what Qt's SVG renderer draws (SVG Tiny 1.2): paths,
  circles, ellipses, rects, linear and radial gradients, fill and stop
  opacity. No filters, masks, clip paths or blur. Soft light and shadows are
  radial gradients that fade out.
- Look: soft gradients, rounded shapes and a warm palette, like the classic
  ornaments and bears in `apps/tv-shell/assets/*.svg` and the classic
  pictures in `themes/*/wallpaper.jpg`.

## Files

| File | Draws | Writes |
|---|---|---|
| [`build.py`](build.py) | the entry point; runs the others | (see below) |
| [`svg.py`](svg.py) | the drawing library: `Svg` (rect, circle, ellipse, path, poly, line, glow, linear/radial gradients, crop, save), `Rng`, `smooth_ridge`, `pine`, `star`, `wave` | nothing |
| [`winter.py`](winter.py) | Winter's world: a snowy cabin under an aurora at night | `themes/winter/classic-wallpaper.svg` (2560×1440), `classic-backdrop.svg` (780×1260 phone crop) |
| [`hero.py`](hero.py) | the featured panel's rooms: cinema, cabin (night, dawn, day, dusk), arcade; each room's light; the cabin TV's intro static | `apps/tv-shell/assets/classic/hero-<scene>[-<time>].svg`, `hero-<scene>-glow.svg`, `hero-static-0.svg`, `hero-static-1.svg` |
| [`weather.py`](weather.py) | ten weather icons | `apps/tv-shell/assets/classic/weather-<name>.svg` |
| [`layers.py`](layers.py) | every world's animated layers in Classic, placed on features of its Classic picture: den (light shafts in the cave mouth, floor mist, a lantern in the dark arch), forest (valley mist), midnight (moon path on the lake, shore mist, twinkling stars), campfire (rain, smoke wisps), winter (aurora rays, chimney smoke, window glow). Its own tiny antialiased RGBA canvas and PNG writer | `themes/<id>/classic-*.png` (frames side by side) and the `classic.wallpaper.sprites` list in each `themes/<id>/theme.json` |
| [`appicons.py`](appicons.py) | Bear Den's own app icons, the smooth twins of `tools/pixelart/appicons.py` (same badge, motifs and palettes, a 32×32 view box) | `apps/tv-shell/assets/classic/app-<adapter>.svg`; phone copies `apps/remote-web/static/art/app-<adapter>.svg` |
| [`extras.py`](extras.py) | the October pumpkin, December's snow on the panel, the "z" over a dozing bear, and the weather props of the corner scenes (a tarp, the startled "!", a leaf umbrella; twins of `tools/pixelart/weatherprops.py`) | `apps/tv-shell/assets/ornaments/pumpkin.svg`, `apps/tv-shell/assets/classic/snowcap.svg`, `sleep-z.svg`, `scene-tarp.svg`, `startle.svg`, `umbrella-leaf.svg` |

Assets under `assets/classic/` and `assets/ornaments/` are globbed into the
shell build, so there is no list to edit.

## How the shell uses them

- **Rooms** use the pixel rooms' rig (`qml/HeroRig.js`, from
  `tools/pixelart/hero.py`). The SVGs have the same 176×92 view box, so the
  icon's `screen` rectangle lines up in both styles. In Classic,
  `HeroScene.qml` pulses the `-glow` layer instead of stepping pixel frames,
  slides continuously for parallax, and alternates the two static pictures
  over the screen during the cabin's intro. If you move a room's screen, move
  it in both `tools/pixelart/hero.py` and [`hero.py`](hero.py).
- **Weather icons** have the pixel icons' names. `Header.qml` and
  `WeatherScreen.qml` show them at the pixel icon's size (16 art pixels).
- **Winter's world** is listed in the `classic` block of
  `themes/winter/theme.json`. It is written by hand: `tools/pixelart` only
  rewrites `wallpaper` and `phone.backdrop`, so the block survives. The
  wallpaper has no chimney smoke of its own (the phone crop keeps static
  puffs): the smoke is an animated layer.
- **Animated layers** (`layers.py`) are `classic.wallpaper.sprites`:
  `Wallpaper.qml` draws them smooth over the Classic picture, in its pixels
  (2560×1440), scaled, cropped and drifted with it, stepped on the heartbeat.
  `layers.py` rewrites only `classic.wallpaper.sprites`, so hand-written keys
  and the pixel blocks survive. Every sheet is a texture on the TV (width ×
  height × 4 bytes): keep regions small, frames few (3–6) and rates low
  (2–8 fps), never a full-screen layer. If you move a world's picture, move
  its layers' `x`/`y` in `layers.py`.
- **The pumpkin** is an ornament. `World.ornament("pumpkin")` picks the SVG in
  Classic. **The snowcap** tiles along `HeroPanel.qml`'s top edge. **The
  "z"** floats over the dozing bear in `HeroLife.qml`.

## Run it

From the repository root:

```sh
python3 -B tools/classicart/build.py                  # everything
python3 -B tools/classicart/build.py hero weather     # only these parts
```

- Part names: `winter`, `hero`, `weather`, `extras`, `layers`, `appicons`.
- `-B` stops Python writing `__pycache__` files into the tree.

To look at a result, run `make shell`, then
`scripts/sandbox.sh shot --classic --theme <id>` (add `--screen weather` for
the weather screen). Set `BDTV_MONTH=10` for the pumpkins or `BDTV_MONTH=12`
for the snow. Open the PNG.

## Checks

```sh
python3 -B tools/classicart/build.py && git status --short   # a second run changes nothing
make test-shell                                              # classicArtAssets: every piece exists, rooms line up, every world has classic layers whose sheets load
go test ./internal/themes                                    # Winter's classic block validates
```
