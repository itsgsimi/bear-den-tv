# Themes

A **theme** changes how Bear Den looks everywhere, on the TV and on paired
phones:
- the wallpaper and colours;
- the decoration that grows around the focused card;
- what drifts through the background;
- the scene in Home's corner;
- how the visiting bears dress.

Themes are **packages**: a folder with a `theme.json` manifest and its art. A
manifest *picks* from the engine's building blocks; it contains no code. So
designing a theme means choosing, colouring and drawing, and a new theme needs
no rebuild.

By default Bear Den is **pixel art** everywhere: the worlds, the bears, the
ornaments, the decorations and the boxes on screen are all drawn on one grid
of art pixels (see [Pixel art](#pixel-art)). Text and the apps' own icons stay
smooth. The **Classic** art style (Themes → Art style) draws the same things
smooth instead (see [Classic art](#classic-art)).

- Manifest spec: [`contracts/theme.schema.json`](../contracts/theme.schema.json)
- Built-in themes: [`themes/`](../themes/)
- TV loader: [`apps/tv-shell/src/ThemeRegistry.h`](../apps/tv-shell/src/ThemeRegistry.h) → QML `Themes`, [`World.qml`](../apps/tv-shell/qml/World.qml)
- Coordinator/phone loader: [`internal/themes`](../internal/themes/themes.go)

## Words used here

- **Theme**: one folder with a `theme.json` manifest and its art.
- **Ornament**: a small named icon (a daisy, a flame, a hat) that a manifest
  can use by name.
- **Art pixel**: one pixel of the pixel art. On a 1080p TV it is drawn as a
  4×4 block of screen pixels (`World.px`).
- **Sandbox**: the real TV shell running on your computer, offscreen, taking
  screenshots ([`scripts/sandbox.sh`](../scripts/sandbox.sh)).

More terms: [`docs/GLOSSARY.md`](GLOSSARY.md).

## Where themes live

| Where | What |
|---|---|
| `themes/<id>/` in the repository | Built-in themes: `den`, `forest`, `midnight`, `campfire`, `winter`. They are compiled into the TV shell (`:/themes/<id>/`) and embedded in the coordinator (`bdtv.Themes`). A change here needs a rebuild and a deploy. |
| `~/.local/share/bear-den-tv/themes/<id>/` on the TV (`$XDG_DATA_HOME/bear-den-tv/themes`; `$BDTV_THEMES_DIR` overrides) | Your themes. The TV shell picks them up when Settings opens, phones within 10 s. No rebuild, no restart. A theme with a built-in's id overrides the built-in. `bear-den-tv themes path` prints this folder. |

**Themes → Style → Plain** (`layout.ui.theme: "plain-dark"`) keeps any
theme's wallpaper and colours and turns every decoration off. **Performance**
(`"performance"`) goes further: no bears at all (visits, scenes, the launch
cub, the screensaver bear; the logo stays) and nothing animates, on the TV or
the phone. It is Plain with reduced motion forced on.

**Themes → Art style** (`layout.ui.art_style`) is separate from Style:
**Pixel** (the default; everything is pixel art on one grid, see
[Pixel art](#pixel-art)) or **Classic** (smooth vector art and pictures). In
Classic a theme shows its `classic` wallpaper and phone backdrop, and every
ornament name resolves to its `.svg` before its `.png`; a theme without a
`classic` block keeps its own wallpaper. Why:
[ADR 0006](decisions/0006-classic-art-style.md).

## Make it yours

From easiest to deepest. Each step is a recipe below.

| # | You want to | Needs code? | Recipe |
|---|---|---|---|
| 1 | Pick one of the installed themes | no | [Pick a theme](#pick-a-theme) |
| 2 | Make your own theme: colours, particles, ornaments, wallpaper | no (JSON and pictures) | [Make a theme in five minutes](#make-a-theme-in-five-minutes) |
| 3 | Draw new pixel art: wallpapers, ornaments, scenes | Python drawing code | [Draw new pixel art](#draw-new-pixel-art) |
| 4 | Change the bears, or swap them for another animal | Python drawing code; QML only for new behaviour | [Change the bears](#change-the-bears) |
| 5 | Rename "Bear Den" | copy in QML, TypeScript and Go | [Rename the product](#rename-the-product) |
| 6 | A new decoration style, particle kind or corner scene | QML, C++, TypeScript, schema | [Extending the engine](#extending-the-engine-new-building-blocks) |

## Pick a theme

1. On the TV: open **Settings**, move to **Theme**, press ◀ ▶. The TV
   changes at once and saves it
   ([`SettingsScreen.qml`](../apps/tv-shell/qml/SettingsScreen.qml)). Picking
   a theme also sets its accent colour.
2. Or on a paired phone: open **Layout**, choose **Theme**, then **Apply**
   ([`editor.tsx`](../apps/remote-web/src/views/editor.tsx)). The phone
   needs the layout permission, and applying works only over HTTPS or on
   the TV itself.
3. Optional, **Style**: `Bear Den` (everything), `Plain` (no decorations),
   `Performance` (no bears, no animation).
4. Optional, **Art style**: `Pixel` or `Classic` (smooth art).

The choice is stored as `layout.ui.background` in `config.json` and survives
restarts, so the theme you pick is your default. The old values `den-gradient`
and `charcoal` are aliases for `den` and `campfire`.

To change the out-of-the-box default for a **fresh install**, edit
`ui.background` and `ui.accent` in
[`contracts/fixtures/config.default.valid.json`](../contracts/fixtures/config.default.valid.json)
(the coordinator's built-in defaults), then run `make test`.

Done when:
- the TV shows the theme on Home after you leave Settings;
- after `bear-den-tv` restarts, the same theme is still there.

## Make a theme in five minutes

You need the repository built once (`. scripts/env.sh`, then `make go shell`).
The example makes a theme called `autumn` in a scratch folder and previews it
on your computer before it goes near the TV.

1. Copy the smallest complete theme, Winter, into a scratch themes folder.
   The folder name is the theme id.

   ```sh
   mkdir -p ~/my-themes
   cp -r themes/winter ~/my-themes/autumn
   ```

2. Edit `~/my-themes/autumn/theme.json`:
   - `"id": "autumn"`: must equal the folder name
     (`^[a-z][a-z0-9-]{1,31}$`);
   - `"name"`, `"description"`, `"accent"`;
   - `palette`: the decoration colours;
   - `focus`, `ambient`, `scene`, `bears`, `heading`: pick building blocks
     from the [Manifest reference](#manifest-reference) and
     [Building blocks](#building-blocks);
   - `phone.particles`: usually the same as `ambient.kind`.

   A starting point that only recolours Winter's world:

   ```json
   "accent": "#D98C3F",
   "palette": { "stem": "#6B4A2A", "light": "#E8B04A", "dark": "#B5562B", "bloom": "#D9483F", "glow": "#F4D58D" },
   "focus": { "style": "fern", "tip": "mushroom", "extras": ["berries"], "extras_upright": true },
   "ambient": { "kind": "leaves", "count": 12, "colors": ["#D98C3F", "#B5562B", "#E8B04A"] },
   "scene": "campfire",
   "bears": { "hat": "hat-beanie", "chase": "leaf" },
   "heading": "sprig"
   ```

3. Optional art, all in the theme folder:
   - **Wallpaper**: replace `wallpaper.png`. Pixel art is 480×270 with
     `"pixel": true`; a photo or painting is 16:9, at least 1920×1080, with
     `"pixel": false`.
   - **Sprites**: Winter's `wallpaper.sprites` (the aurora and the chimney
     smoke) are placed for Winter's picture. With your own wallpaper, delete
     the `sprites` list, or delete `aurora.png`/`smoke.png` and list your own
     sheets.
   - **Phone backdrop**: replace `backdrop.png` (pixel art 168×270, or a
     portrait picture of about 780×1260).
   - **Ornaments**: add `<name>.png` to use a new ornament by name, or to
     restyle a built-in one (same name wins). 7–16 art pixels, dark outline.
   - Leave out a file you don't want and remove its key; the gradient
     `wallpaper.top`/`bottom` is the fallback.

4. Validate. It checks the schema, id = folder name, that every file exists
   and that every ornament resolves.

   ```sh
   build/bin/bear-den-tv themes validate ~/my-themes/autumn
   ```

5. Preview on your computer. `--theme` also applies your theme's accent,
   as picking it on the TV does. `--no-weather` drops the demo data's rain,
   which would otherwise replace your particles:

   ```sh
   BDTV_THEMES_DIR=~/my-themes scripts/sandbox.sh shot --theme autumn --no-weather --out /tmp/autumn-home.png
   BDTV_THEMES_DIR=~/my-themes scripts/sandbox.sh shot --theme autumn --no-weather --screen settings --out /tmp/autumn-settings.png
   ```

   Open both PNGs and look at them. The corner scene shows only when Home
   has a single rail; the demo has two, so check it on the TV.

6. Install on the TV: copy the folder into the themes folder on your TV
   (`bear-den-tv themes path` prints it; normally
   `~/.local/share/bear-den-tv/themes/`), for example with
   `scp -r ~/my-themes/autumn <you>@<your-tv>:.local/share/bear-den-tv/themes/`.
   Then [pick it](#pick-a-theme).

7. Optional, make it a built-in: move the folder to `themes/autumn/`, add
   `autumn` to `builtInOrder` in
   [`internal/themes/themes.go`](../internal/themes/themes.go) and
   `kBuiltInOrder` in
   [`ThemeRegistry.cpp`](../apps/tv-shell/src/ThemeRegistry.cpp) (keep the
   two lists identical; without them it sorts last), run `make test`, then
   deploy with `scripts/deploy-target.sh`.

Done when:
- `bear-den-tv themes validate` prints `valid theme "autumn"`;
- `BDTV_THEMES_DIR=~/my-themes build/bin/bear-den-tv themes list` shows
  `autumn … (yours)`;
- the sandbox screenshots show your accent on the focus ring, your
  decoration around the focused tile and your particles, and you looked at
  them;
- The Themes page on the TV offers it.

A minimal manifest is valid and simply has no decorations:

```json
{ "schema": 1, "id": "autumn", "name": "Autumn", "accent": "#D98C3F",
  "wallpaper": { "top": "#3A2416", "bottom": "#120B07" } }
```

A full one: this is `themes/forest/theme.json`.

```json
{
  "schema": 1, "id": "forest", "name": "Forest",
  "description": "Misty pines: ferns unfurling, mushrooms and berries, falling leaves, bears camping.",
  "accent": "#8DBF7F",
  "wallpaper": { "image": "wallpaper.png", "top": "#1B3A2C", "bottom": "#0A100D", "pixel": true,
                 "sprites": [ { "sheet": "mist.png", "frames": 8, "fps": 4, "x": 20, "y": 196 },
                              { "sheet": "glints.png", "frames": 4, "fps": 6, "x": 150, "y": 246 } ] },
  "palette": { "stem": "#46704F", "light": "#A6D38F", "dark": "#5C9161", "bloom": "#D9483F", "glow": "#F4D58D" },
  "focus": { "style": "fern", "extras": ["mushroom"], "extras_bottom": ["berries"], "extras_upright": true },
  "ambient": { "kind": "leaves", "count": 10, "colors": ["#C9A65A", "#7FAE72", "#5E8F5E"] },
  "scene": "camp",
  "bears": { "hat": "hat-beanie", "chase": "leaf" },
  "heading": "sprig",
  "phone": { "backdrop": "backdrop.png", "veil": "#121814", "particles": "leaves" }
}
```

## Manifest reference

Unknown keys are rejected, so typos show up in `themes validate`.

| Key | Type | Default | Effect |
|---|---|---|---|
| `schema` | `1` | required | Manifest format version. |
| `id` | `^[a-z][a-z0-9-]{1,31}$` | required | Must equal the folder name; stored in `layout.ui.background`. |
| `name` | string ≤ 32 | required | Shown on the Themes page and on phones. |
| `description` | string ≤ 200 | "" | For people browsing themes. |
| `aliases` | ids | [] | Older values that mean this theme. |
| `accent` | `#RRGGBB` | required | Focus ring, buttons, highlights (TV and phone). Applied when the theme is picked. |
| `wallpaper.image` | file | none | 16:9, ≥ 1920×1080 (2560×1440 recommended) unless `pixel` is true. Without it, the gradient is the wallpaper. |
| `wallpaper.pixel` | boolean | `false` | The image is pixel art: drawn at a whole-number scale without smoothing (a 480×270 image draws 4× on a 1080p screen). The ≥ 1920×1080 advice does not apply. Phones draw the backdrop unsmoothed too (`state.appearance.pixel`). |
| `wallpaper.sprites` | list (≤ 16) | none | Animated layers drawn over the image at its scale: `{ "sheet": file, "frames": 1–32, "fps": 1–20, "x": 0–3840, "y": 0–3840 }`. The sheet holds the frames side by side, equal width, in one row; `x`/`y` place its top-left in the image's own pixels. All five keys are required. |
| `wallpaper.top` / `.bottom` | `#RRGGBB` | required | Gradient behind (or instead of) the image. `bottom` is also the lock-screen and window colour. |
| `palette.stem` / `light` / `dark` / `bloom` / `glow` | `#RRGGBB` | derived from `accent` | Decoration colours: lines/stems, the two leaf tones, blossoms/cores, glows. |
| `focus.style` | `none` `vine` `fern` `stars` `embers` | `none` | What grows around the focused card (see *Building blocks*). |
| `focus.tip` | ornament | none | Blooms at each end of the decoration. |
| `focus.tip_upright` | bool | false | Keep the tip upright on mirrored corners (flames, mushrooms) and flicker it instead of spinning. |
| `focus.extras` | ornaments ≤ 6 | [] | Pop open along the decoration the longer focus stays (top corners). |
| `focus.extras_bottom` | ornaments ≤ 6 | `extras` | The same for the bottom corners (mirrored vertically). |
| `focus.extras_upright` | bool | false | Extras stand upright on the top edge (mushrooms). |
| `panel` | like `focus` | `focus` | The decoration hugging the featured panel's corners. |
| `ambient.kind` | `none` `fireflies` `leaves` `stars` `embers` `snow` | `none` | Particles drifting through the background. |
| `ambient.count` | 0–40 | 14 | How many. |
| `ambient.colors` | 1–4 × `#RRGGBB` | per kind | Their colours. |
| `scene` | `none` `den` `camp` `moon` `campfire` | `none` | Corner scene on Home. |
| `bears.hat` | ornament | none | What visiting bears wear. |
| `bears.carry` | ornament or `stick` | none | What they hold (`stick` is a toasting stick with a marshmallow). |
| `bears.chase` | `firefly` `leaf` `star` `ember` or an ornament | `firefly` | What the cub runs after. |
| `heading` | ornament | none | Little mark beside rail headings ("Your Apps"). |
| `phone.backdrop` | file | `wallpaper.image` | Portrait picture for phones (~780×1260). |
| `phone.veil` | `#RRGGBB` | `wallpaper.bottom` | The dark veil over the phone backdrop. |
| `phone.particles` | as `ambient.kind` | `ambient.kind` | Phone particles. |
| `classic.wallpaper.image` | file | main wallpaper | The wallpaper in the Classic art style: a smooth 16:9 picture (≥ 1920×1080) or an SVG. Required inside `classic.wallpaper`. The main `top`/`bottom` colours still apply. |
| `classic.wallpaper.sprites` | as `wallpaper.sprites` | none | Animated layers over the classic image, drawn smooth. |
| `classic.phone.backdrop` | file | `classic.wallpaper.image` | The phone backdrop in Classic. |

**Files** are plain names in the theme folder: letters, digits, `.`, `_`, `-`,
ending in `.svg .png .jpg .jpeg .webp`.

**Ornaments** are names (`^[a-z][a-z0-9-]{0,31}$`) that resolve to
`<name>.svg` or `<name>.png` **in the theme folder first**, then to the built-in
ornaments. So you can restyle any ornament, including those the scenes use, by
shipping a file with the same name.

## Building blocks

### Decoration styles (`focus.style`, `panel.style`)

Drawn by the corner engine
([`CornerDecor.qml`](../apps/tv-shell/qml/CornerDecor.qml)) around the
top-right and bottom-left corners of the focused card
([`FocusDecor.qml`](../apps/tv-shell/qml/FocusDecor.qml)). The same styles hug
the featured panel's corners
([`HeroDecor.qml`](../apps/tv-shell/qml/HeroDecor.qml)).

| Style | Draws | Palette use | Grows over time |
|---|---|---|---|
| `vine` | a tapered stem, two-tone leaves with midribs, two curling tendrils | stem, dark→light leaves | your `extras` |
| `fern` | a thinner stem with 17 pinna pairs shrinking to a fiddlehead curl | stem, dark→light pinnae | your `extras` |
| `stars` | a constellation traced star by star (lines and four-point sparkles); a shooting star now and then | stem = lines, light = stars, glow = halos | more little stars, plus your `extras` |
| `embers` | a glowing, flickering trail with coals | glow = halo, bloom = core, light = coals and sparks | rising sparks, plus your `extras` |

Timing, the same for every style:
- **Grows** over 2.3 s when focus arrives.
- **Extras** open 3 s apart, then 6 s apart, up to six.
- **Fades** in 0.34 s when focus leaves.
- **Sways** at ~12 fps while awake, ~6 fps while resting.

### Particles (`ambient.kind`)

Drawn by [`Ambient.qml`](../apps/tv-shell/qml/Ambient.qml) on the TV and by
`app.css` (`html[data-particles]`) on phones.

| Kind | Looks like |
|---|---|
| `fireflies` | glowing motes (colour 0) and dust (colour 1) drifting and fading |
| `leaves` | leaves drifting down, turning as they fall |
| `stars` | stars twinkling in the upper sky, and a shooting star every 15–30 s |
| `embers` | embers rising from below, flickering |
| `snow` | flakes falling softly |

`Ambient.qml` also draws `rain` (thin streaks of 1×4–6 art pixels falling
fast with a little wind), but only for local weather (next section): it is
not a manifest kind, and `theme.schema.json`'s particle enums are not
extended.

### Weather in the scene

When the owner turns on Settings → Weather and "Weather in the scene"
(`state.weather.scene`), the Home backdrop follows the local weather in every
theme. The theme's own data stays untouched; [`World.qml`](../apps/tv-shell/qml/World.qml)
overrides what components read, and nothing branches on a theme id:

| Condition (`state.weather.current`) | Particles (replace the theme's) | Veil | Extra |
|---|---|---|---|
| `drizzle` | `rain`, 60% of the count | light grey | |
| `rain` | `rain` | grey | |
| `thunder` | `rain` | darker grey | a lightning flicker every 14–30 s |
| `snow` | `snow` | faint white | |
| `cloudy` | the theme's | grey | |
| `fog` | the theme's | pale, heavier | |
| `clear`, `partly-cloudy` | the theme's | none | |

- Count by intensity: light 24, moderate 40, heavy 56 (one `Rectangle`
  each, capped for the 2-core floor).
- Only while `status` is `ready` or `stale` and `current` is present.
- The veil and the flash are [`WeatherSky.qml`](../apps/tv-shell/qml/WeatherSky.qml),
  between the wallpaper and the particles. The veil is a static rectangle and
  stays with reduced motion. The particles don't (no rain with reduced motion
  or Performance). The flash steps on World's heartbeat and is gated like
  everything else (`alive`: not behind apps, resting, the screensaver or with
  reduced motion).
- The header chip (icon + temperature, dimmed when stale) uses the pixel
  icons `assets/pixel/weather-*.png` from
  [`tools/pixelart/weather.py`](../tools/pixelart/weather.py).

### Weather in the corner scene

With "Weather in the scene" on, the corner scene and the visiting bears
react too. [`World.qml`](../apps/tv-shell/qml/World.qml) turns the reading
into one **look**, `World.weatherLook`:

| Look | Conditions |
|---|---|
| `wet` | `drizzle`, `rain` |
| `storm` | `thunder` (everything `wet` has, plus the startle) |
| `snow` | `snow` |
| `fog` | `fog` |
| `night` | `clear` or `partly-cloudy` with `is_day` false |
| `""` (unchanged) | no weather in the scene, `cloudy`, or a clear day |

**How a scene declares its reactions.** The building block is
[`SceneWeather.qml`](../apps/tv-shell/qml/SceneWeather.qml). A scene places
two of them, `side: "back"` (behind its bears) and `side: "front"` (over
them), and gives each the pieces it wants per look, in its own units (art
pixels in Pixel, the 300-unit design grid in Classic). It is data; the engine
never branches on a theme or a scene:

```qml
SceneWeather {
    side: "back"
    unit: root.p                        // screen pixels per scene unit
    anchors.fill: parent
    reactions: ({
        wet: { shelter: { x: 30, y: 23 }, puddles: [ { x: 8, y: 80, w: 10 } ] },
        night: { stars: [ { x: 8, y: 6 }, { x: 47, y: 4 } ] }
    })
}
```

| Piece | Side | What it draws | Moves |
|---|---|---|---|
| `puddles: [{x, y, w}]` | back | wet ground | a glint runs along it |
| `shelter: {x, y, w}` | back | a tarp on two poles (`scene-wx-tarp.png`; in Classic `assets/classic/scene-tarp.svg`, `w` wide) | no |
| `stars: [{x, y}]` | back | stars over the scene | twinkle |
| `caps: [{x, y, w, a}]` | front | snow lying on a prop; `a` tilts it in Classic to follow a slope | no |
| `drips: [{x, y}]` | front | drops falling from an edge | fall a few pixels |
| `fade: 0.4` | front | nothing itself: the scene binds its props' opacity to `1 - fade` | no |
| `mist: [{y, h}]` | front | wisps of mist | drift |
| `startle: [{x, y}]` | front | a "!" (`scene-wx-startle.png` / `startle.svg`) over a bear on a lightning flash, for 1.5 s | shown on the flash |

The scene may also change its own art from `wet` and `startled` (the
campfire smoulders under its tarp; bears hop two pixels when startled).
What the built-in scenes do:

| Scene | Rain | Snow | Fog | Thunder | Clear night |
|---|---|---|---|---|---|
| campfire | tarp over the fire, which smoulders (dimmer, slower, one spark); puddles; drips from the tarp | on the log ends and the ground | logs and stones fade, mist | dad and the cub startle | stars |
| den | drips off the arch, puddles | along the arch | the rock fades, mist | dad and mama startle | stars |
| camp | puddles, drips from the lantern arm; no fireflies | on the tent's peak, a pine and the lantern arm; no fireflies | the camp fades as one layer (so the bears never show through the tent), mist; no fireflies | mama and the cub startle | stars |
| moon | drips from the crescent and the hanging stars | on the crescent's rim | the mobile fades, mist | the cub startles | more stars |

**Visiting bears** ([`BearVisitors.qml`](../apps/tv-shell/qml/BearVisitors.qml))
set `weatherDress` on their `BearPuppet`s: in `wet`/`storm` they hold a leaf
umbrella (`scene-wx-umbrella.png`, the stem drawn from the paw; Classic
`assets/classic/umbrella-leaf.svg`) instead of what they carry, and walk 1.4×
faster; in `snow` they wear the `hat-beanie` ornament. The bears inside the
corner scenes and the featured panel keep their own dress.

**Motion and cost.** A handful of `Rectangle`s and `Image`s, only for the
current look. Glints, drips, twinkles and mist move on `World.beat` while the
piece is `alive` (not resting, on the screensaver, behind apps or with
reduced motion); reduced motion shows the still version (the same pieces, a
drop hanging at each edge, no startle). The startle follows `World.flashing`,
which `WeatherSky` sets only while its own flash is alive. No Canvas. The
camp's fog layer (`layer.enabled`) exists only in fog.

**Checking it.** `scripts/sandbox.sh shot --theme campfire --apps-only
--weather rain` (conditions `clear`, `partly-cloudy`, `cloudy`, `fog`,
`drizzle`, `rain`, `snow`, `thunder`; add `:night` and `:light`/`:heavy`);
`--apps-only` leaves room for the corner scene; `--lightning` with
`--weather thunder` flashes every second (`BDTV_LIGHTNING_SECONDS=1`) so the
startle shows; `--reduced-motion` shows the still version. Tests:
`sceneWeatherPicksVariant`, `sceneWeatherEveryScene`,
`sceneWeatherMotionAndStill` in
[`tst_shell.cpp`](../apps/tv-shell/tests/tst_shell.cpp).

**A new scene** declares its own `reactions` (both art styles; ADR 0006).
**A new piece** is added to `SceneWeather.qml` for every scene: draw it in
both styles, put its sprite in
[`tools/pixelart/weatherprops.py`](../tools/pixelart/weatherprops.py) and its
SVG in [`tools/classicart/extras.py`](../tools/classicart/extras.py), move it
only on the heartbeat while `alive`, and list it in the table above.

### Corner scenes (`scene`)

| Scene | Component | What happens |
|---|---|---|
| `den` | [`DenFamily.qml`](../apps/tv-shell/qml/DenFamily.qml) | the family peeks out of the den by lantern light; blinks, the cub waves, asleep 22:00–06:00 |
| `camp` | [`CampScene.qml`](../apps/tv-shell/qml/CampScene.qml) | mama and the cub in a tent between pines; a lantern flickers, fireflies drift |
| `moon` | [`MoonScene.qml`](../apps/tv-shell/qml/MoonScene.qml) | the cub asleep on a hanging crescent moon with stars on threads; it rises and settles a pixel, "z"s rise |
| `campfire` | [`CampfireScene.qml`](../apps/tv-shell/qml/CampfireScene.qml) | dad and the cub toast marshmallows; the flame flickers, sparks rise |

Scenes are pixel art, 110×82 art pixels (440×330 on a 1080p TV), drawn on
their own sprites (`apps/tv-shell/assets/pixel/scene-*.png`, made by
`tools/pixelart/scenes.py`) with the pixel bears in front; small props
(`paw-grip`, `paw`) are ornaments a theme can override. Every scene reacts
to local weather ([Weather in the corner scene](#weather-in-the-corner-scene)).

### Built-in ornaments

`berries`, `blossom`, `controller`, `corner`, `daisy`, `den`, `divider`,
`flame`, `hat-beanie`, `hat-beret`, `hat-nightcap`, `heart`, `lantern`, `logs`,
`moon`, `mushroom`, `paw`, `paw-grip`, `phone`, `pine`, `pointer-up`, `popcorn`,
`pumpkin`, `remote`, `sparkle`, `sprig`, `star`, `tent`. The bear tips use
`hat-beret` (the Themes bear), `phone` (the phone-remote bear's prop) and
`pointer-up` (the arrow on the tip's sign).

Each comes twice, in
[`apps/tv-shell/assets/ornaments/`](../apps/tv-shell/assets/ornaments/): a
small pixel-art `<name>.png` (7–16 art pixels, most with a dark outline, made
by `tools/pixelart/ornaments.py`) for the Pixel art style, and a smooth
`<name>.svg` for Classic. The art style picks which is looked up first, in a
theme folder as among the built-ins; phones get them from
`/themes/_ornaments/<name>.png|svg`. A PNG ornament is drawn unsmoothed at a
whole multiple of the pixel grid, centred in the box it is given, never
rotated; an SVG one is drawn smooth at any size. A new built-in ornament needs
both files.

### Bears

What a theme can change about the bears: `bears.hat`, `bears.carry` and
`bears.chase` (see the [Manifest reference](#manifest-reference)), and any
ornament they use (`paw`, `paw-grip`, the hats) by shipping a file with the
same name. Everything else about them is engine art and code:
[Change the bears](#change-the-bears).

Visits happen on Home, Settings and Pair phone only
([`BearVisitors.qml`](../apps/tv-shell/qml/BearVisitors.qml)). The acts:
- walk across and wave;
- peek up from the bottom edge;
- hop onto an app tile and dance;
- a family parade;
- the cub chases the theme's `bears.chase`.

For checks, `BDTV_BEARS_SECONDS` sets a fixed gap between visits and
`BDTV_BEARS_ACT` (`walk|peek|hop|parade|chase`) picks the act
(`scripts/sandbox.sh shot --bears <act>` sets both).

The secret code on the remote (up, up, down, down, left, right, left, right,
OK; TV remote or phone) starts the family parade at once. The final OK is used
up by the parade; nothing else is unlocked.

### The featured panel

In the Bear Den style the big panel on Home has its own life
([`HeroPanel.qml`](../apps/tv-shell/qml/HeroPanel.qml)); Plain and Performance
keep it plain, reduced motion shows everything at once:

| What | Where | Behaviour |
|---|---|---|
| A room behind the app's icon | [`HeroScene.qml`](../apps/tv-shell/qml/HeroScene.qml), art `assets/pixel/hero-<scene>.png` + `HeroRig.js` from `tools/pixelart/hero.py` | Plex gets a cinema (curtains, projector beam, dust), YouTube a cabin with a TV whose static clears into the picture and a window that follows the clock (night, dawn, day, dusk), Moonlight an arcade with chasing lights, Spotify a music nook (a record player, rising notes, a bear in headphones bobbing to the beat; in Classic the lamp and notes glow instead), Jellyfin a home theatre in the woods (a screen between posts, string lights, a projector on a stump, fireflies), RetroArch a retro corner (a chunky CRT, a joystick, a lava lamp). The web apps share these rooms (Netflix the cinema, Disney+ the theatre, Hulu the cabin, the Browser the nook). Other apps get the cabin. Which app gets which room, and its bear's reaction, is data: `Apps.stage(adapter)`. The room shifts up to two art pixels as focus moves along the rail. |
| Typing | `HeroPanel.qml` | The title types itself in with a block cursor; the description, a how-to hint (`Apps.hint`, Spotify's "pick this TV in the device list"), state and button appear row by row. |
| A visiting bear | [`HeroLife.qml`](../apps/tv-shell/qml/HeroLife.qml) | 1.6 s after focus settles on an app, a bear walks in along the panel's bottom edge, reacts (the cub with popcorn, dad waving the remote, the cub on a controller) and walks off; at most once every 12 s. |
| A dozing bear | `HeroLife.qml` | While Bear Den rests, dad dozes on the panel's edge under "z"s (nothing moves); waking the TV makes him stretch. |
| The cub behind OK | `HeroPanel.qml` | After 7 s on one item the cub peeks over the OK button; it ducks when focus moves. |
| Seasons | `HeroPanel.qml` | Pumpkins in October, snow on the top edge in December. `BDTV_MONTH=1..12` pretends it is that month, for checking. |

All of it moves on World's heartbeat and stops while resting, behind apps and
on the screensaver.

## Preview and validate

| Step | How |
|---|---|
| Validate | `build/bin/bear-den-tv themes validate <dir>`: schema, id = folder, files present, every ornament resolvable. `bear-den-tv themes list` shows what loaded and why anything was skipped. |
| One screenshot | `BDTV_THEMES_DIR=<parent of your theme> scripts/sandbox.sh shot --theme <id>` (prints the PNG path). Options: `--screen`, `--no-weather`, `--weather rain[:night]`, `--apps-only` (room for the corner scene), `--lightning`, `--reduced-motion`, `--plain`, `--classic`, `--bears walk\|peek\|hop\|parade\|chase`, `--size 3840x2160`, `--fixture`, `--out`. See the header of [`scripts/sandbox.sh`](../scripts/sandbox.sh). |
| Every theme × main screens | `make shots` (built-in themes; PNGs in `build/shots/gallery/`). |
| Is it cheap enough? | `scripts/sandbox.sh perf --theme <id>` ([Performance rules](#performance-rules)). |
| Watch it animate | Run the shell with `--dev --screenshot-every 400` and reopen the file in a loop. |
| On the TV | Put the folder in the TV's themes folder, open Settings (themes reload), pick it. |

The default demo data has local weather with rain in the scene, which
replaces the theme's particles (see [Weather in the scene](#weather-in-the-scene)).
Add `--no-weather` to see the theme's own particles. `--theme` sets the
theme's accent colour for you.

Art tips:
- Pixel art in a few colours per object reads best on a TV: a 480×270
  wallpaper, 7–16 px ornaments with a dark outline, dithering instead of
  smooth gradients (see [Pixel art](#pixel-art)).
- If you use SVG anyway: Qt draws SVG Tiny, so avoid masks, filters,
  `<foreignObject>` and nested `<svg>`.
- Check the wallpaper under the dark scrims at the top and bottom of the
  screen, and keep what matters where Home's panels don't cover it: the top
  strip, the right half below the featured panel, and the bottom.

## Pixel art

In the Pixel art style (the default) everything Bear Den draws itself is
pixel art on one grid; [Classic art](#classic-art) is the smooth twin.

- **The grid.** `World.px` is the size of one art pixel on screen: 4 on a
  1080p TV (a 480×270 world), 8 at 4K. Every pixel-art item snaps to it, so
  no pixel is drawn at an in-between size.
- **Worlds.** `wallpaper.pixel: true` draws the wallpaper at a whole-number
  scale without smoothing, covering the screen; `wallpaper.sprites` adds
  animated layers (fire, rain, mist, lanterns, shimmer, an aurora), each a
  row of frames that advances on World's heartbeat and stops while resting.
- **Building blocks** in the shell:
  - [`PixelSprite`](../apps/tv-shell/qml/PixelSprite.qml): one of the shell's
    own sprites, optionally a frame of a sheet;
  - [`PixelBox`](../apps/tv-shell/qml/PixelBox.qml): the stand-in for a rounded
    `Rectangle` (colour, gradient, radius, border), with stair-step corners and
    borders of whole art pixels; tiles, panels, pills, rows, dialogs and the
    focus ring are PixelBoxes;
  - the corner engine, `BrandBackdrop` and the particles paint at 1/`World.px`
    of their size without antialiasing and are shown `World.px` times larger
    unsmoothed, so vines, glows and sparks are made of whole pixels;
  - pixel art is never rotated or scaled in between: bears change frames,
    tips pop open, flickers hop a pixel.
- **Making the art.** [`tools/pixelart/`](../tools/pixelart/) draws all of it
  from code with the Python standard library (no Pillow, no image editor):
  `python3 -B tools/pixelart/build.py` regenerates every world
  (`worlds/<id>.py`: backdrop, phone backdrop, sprite sheets, and the
  manifest's `wallpaper` block), the bears, the ornaments, the scenes, the
  featured-panel rooms and the weather icons; `--preview` also writes
  enlarged previews to `build/pixel-preview/`. The output is deterministic
  and committed. The tool's own guide, with every output path:
  [`tools/pixelart/README.md`](../tools/pixelart/README.md).
- **Not pixel art:** text (readability from the couch), the owner's brand
  art (brand folder) and the QR code. Bear Den's own app icons are pixel art
  ([App icons](#app-icons)).

## Classic art

**Themes → Art style → Classic** (`layout.ui.art_style: "classic"`) swaps
every piece of pixel art for a smooth one, with the same content and motion
([ADR 0006](decisions/0006-classic-art-style.md)):

- **Worlds.** A theme's optional `classic` block gives the Classic wallpaper
  (a picture or an SVG), its animated layers (`classic.wallpaper.sprites`,
  placed in the picture's own pixels and drawn smooth; they follow its crop
  and slow drift) and the phone backdrop. The four original worlds use their
  first painted pictures (`wallpaper.jpg`, `backdrop.jpg`); Winter's classic
  world is drawn by code. A theme without `classic` keeps its own wallpaper.
- **Chrome.** `PixelBox` becomes an antialiased rounded `Rectangle`; the focus
  ring is smooth with a glowing spark; the corner engine, `BrandBackdrop` and
  particles paint at full resolution, antialiased, and bloom, spin and drift
  continuously.
- **Bears.** `BearPuppet` loads `BearPuppetClassic.qml`, a jointed SVG rig
  (`assets/bear-*.svg`) with the same properties (walk, wave, sit, reach,
  sleep, hat, carry); `BearHead` and `BearMark` draw the SVG heads.
- **Scenes, rooms, icons.** `<Scene>Classic.qml` corner scenes; the
  featured panel's rooms, the seasonal pumpkin and snow, and the weather
  icons are SVGs in `assets/classic/` and `assets/ornaments/`.
- **Phones** follow `state.appearance.art_style`: rounded chrome, SVG art,
  round particles and curved vines.

**Making the art.** [`tools/classicart/`](../tools/classicart/README.md)
writes the Classic art that the repository generates (Winter, the rooms,
weather icons, seasonal art, the worlds' classic layers) as SVG or PNG from
code with the Python standard library, deterministically; the restored
original SVGs (ornaments, bears) are edited by hand. When you add a piece of
pixel art, add its Classic twin in the same change, and the reverse.

## Draw new pixel art

The built-in art is Python code that draws pixels, not image files you edit.
Change the code, rebuild, look. Full guide:
[`tools/pixelart/README.md`](../tools/pixelart/README.md).

1. Find the file that draws it:

   | Art | File | Output |
   |---|---|---|
   | A world (wallpaper, sprites, phone backdrop) | `tools/pixelart/worlds/<id>.py` | `themes/<id>/*.png` and the `wallpaper` block of `themes/<id>/theme.json` |
   | An ornament | `tools/pixelart/ornaments.py` (`SPRITES`, character grids) | `apps/tv-shell/assets/ornaments/<name>.png` |
   | The bears | `tools/pixelart/bears.py` | `apps/tv-shell/assets/pixel/{head,bear}-*.png`, `bear-mark.png`, `qml/BearRig.js`, phone copies |
   | Corner scenes | `tools/pixelart/scenes.py` | `apps/tv-shell/assets/pixel/scene-*.png` |
   | Featured-panel rooms | `tools/pixelart/hero.py` | `apps/tv-shell/assets/pixel/hero-*.png`, `qml/HeroRig.js` |
   | Weather icons | `tools/pixelart/weather.py` | `apps/tv-shell/assets/pixel/weather-*.png` |
   | Weather props (tarp, startle, leaf umbrella) | `tools/pixelart/weatherprops.py` | `apps/tv-shell/assets/pixel/scene-wx-*.png` |

2. Edit it. An ornament is a grid of letters, one letter per colour from
   `PAL`; `.` is transparent.
3. Rebuild only that part and write previews:

   ```sh
   python3 -B tools/pixelart/build.py --preview ornaments
   ```

   Names: `den forest midnight campfire winter bears ornaments scenes hero
   weather weatherprops`.
4. Look at `build/pixel-preview/` (for example `ornaments.png`).
5. Built-in art is compiled into the shell: `make shell`, then
   `scripts/sandbox.sh shot` and look.

For a **theme of your own**, you don't need the tool: draw a 480×270 PNG in
any pixel editor, put it in the theme folder and set `"pixel": true`.

Done when:
- `git status` shows only the PNGs (and generated `.js`/`theme.json`) you
  meant to change; running `build.py` a second time changes nothing;
- the preview and a sandbox screenshot look right to you;
- `make test-shell` and `go test ./internal/themes` pass.

## Change the bears

### What a theme can change

| Manifest field | Effect |
|---|---|
| `bears.hat` | an ornament the visiting bears wear |
| `bears.carry` | an ornament in their paw, or `stick` |
| `bears.chase` | what the cub runs after: `firefly`, `leaf`, `star`, `ember` or an ornament |
| ornament files `paw.png`, `paw-grip.png`, `hat-*.png` in the theme folder | restyle those props |
| `scene` | which corner scene (and so which bears) Home shows |

That is all. The bears' bodies, faces, colours and poses are **engine art**,
the same in every theme. There is no manifest field for them today.

### Where the bears are

| What | Files |
|---|---|
| Generator: faces, bodies, poses, the mark | [`tools/pixelart/bears.py`](../tools/pixelart/bears.py) |
| Generated art | `apps/tv-shell/assets/pixel/head-{dad,mama,cub}.png` (awake, blink, asleep), `bear-{dad,mama,cub}.png` (one column per pose, two rows), `bear-mark.png` (the logo) |
| Generated rig | [`apps/tv-shell/qml/BearRig.js`](../apps/tv-shell/qml/BearRig.js): frame sizes, pose order, paw and head-top per pose. Do not edit by hand. |
| Phone copies | `apps/remote-web/static/art/pixel/bear-mark.png`, `head-*.png`; sizes are also written in `ART` in [`icons.tsx`](../apps/remote-web/src/icons.tsx) |
| A head | [`BearHead.qml`](../apps/tv-shell/qml/BearHead.qml) (blinks, sleeps at night) |
| A whole bear | [`BearPuppet.qml`](../apps/tv-shell/qml/BearPuppet.qml) (poses, walking, what it holds and wears) |
| The logo | [`BearMark.qml`](../apps/tv-shell/qml/BearMark.qml) (header, screen frames, Home, lock and connecting screens) |
| Mascot in its den | [`DenMascot.qml`](../apps/tv-shell/qml/DenMascot.qml) (Home, lock and connecting screens) |
| Corner scenes | [`DenFamily.qml`](../apps/tv-shell/qml/DenFamily.qml), [`CampScene.qml`](../apps/tv-shell/qml/CampScene.qml), [`MoonScene.qml`](../apps/tv-shell/qml/MoonScene.qml), [`CampfireScene.qml`](../apps/tv-shell/qml/CampfireScene.qml); props from `scenes.py` |
| Visits | [`BearVisitors.qml`](../apps/tv-shell/qml/BearVisitors.qml) |
| Featured panel bears | [`HeroLife.qml`](../apps/tv-shell/qml/HeroLife.qml), the cub behind OK in [`HeroPanel.qml`](../apps/tv-shell/qml/HeroPanel.qml); per-app reactions in `Apps.stage()` ([`Apps.qml`](../apps/tv-shell/qml/Apps.qml)) |
| Other heads | the peeking cub in [`AppTile.qml`](../apps/tv-shell/qml/AppTile.qml), [`LaunchOverlay.qml`](../apps/tv-shell/qml/LaunchOverlay.qml), [`Screensaver.qml`](../apps/tv-shell/qml/Screensaver.qml) |

The QML asks for bears by **kind**: `dad`, `mama`, `cub`. It never draws a
bear itself; it shows frames from the generated sheets.

### Recipe: a different animal

This redraws the family (for example as foxes) without touching QML.

1. In [`tools/pixelart/bears.py`](../tools/pixelart/bears.py) change the
   colours at the top (`FUR`, `CREAM`, `INK`, …), the face in `head()`, the
   body in `bear()` and the logo grid `MARK`.
2. Keep these the same, because the QML depends on them:
   - the kind names `dad`, `mama`, `cub` (keys of `KINDS`);
   - the pose names and order in `POSES`;
   - three head frames (awake, blink, asleep) and two sheet rows (eyes
     open, blinking);
   - the mark at 16 art pixels wide (`BearMark.qml` draws it 16 art pixels
     square).
3. Sizes in `KINDS` (`hw`, `hh`, `fw`, `fh`) may change: `BearRig.js` is
   regenerated with them. If the head or mark size changes, also update the
   `bear-mark`/`bear-cub`/`bear-sleep` sizes in `ART` in
   [`apps/remote-web/src/icons.tsx`](../apps/remote-web/src/icons.tsx).
4. Rebuild the art and look at the sheet:

   ```sh
   python3 -B tools/pixelart/build.py --preview bears
   ```

   Open `build/pixel-preview/bears.png`.
5. Optional: the bear-themed props (`den`, `paw`, `paw-grip`, the hats) in
   `ornaments.py`, and the den and tent in `scenes.py`.
6. Rebuild and look at every place a bear appears:

   ```sh
   make shell
   make shots                                   # every theme: corner scenes, peeking cub
   for act in walk peek hop parade chase; do scripts/sandbox.sh shot --bears $act; done
   scripts/sandbox.sh shot --theme den --plain  # the logo without decorations
   make test-shell
   npm --prefix apps/remote-web run test:unit
   ```

7. Also check on the TV (or not at all, say so): the screensaver, the lock
   screen, the launch overlay and the featured panel's visiting bear. The
   sandbox has no switch for those screens.

Done when:
- `bears.png` and every screenshot above show the new animal, nothing
  cropped or floating;
- `make test-shell` and the phone unit tests pass;
- the words still fit (see [Rename the product](#rename-the-product) if the
  animal is no longer a bear).

## Rename the product

"Bear Den" is copy (words on screen), not an id. You can change the copy.
Renaming the **ids** (the `bear-den-tv` binaries, the `bear-den-tv` config
and data folders, the `BearDen` QML module, the `bdtv` Go package, the
`BDTV_*` variables, the `bear-den-tv-shell` window class the coordinator
matches) is a deep change that breaks existing installs; don't.

Where the name is shown (checked with `grep -rn "Bear Den"`):

| Part | Files |
|---|---|
| TV shell copy (`qsTr`) | `Header.qml` (the title; it also hides the device name when it equals `"Bear Den"`), `Main.qml` (window title), `SettingsScreen.qml`, `ConnectingScreen.qml`, `ErrorBanner.qml`, `LaunchOverlay.qml`, `Screensaver.qml`, `RemoteSetupScreen.qml`, `InstallCard.qml`, `AddAppsScreen.qml`, `AdvancedPlaybackScreen.qml`, `HeroPanel.qml`, `DiagnosticsScreen.qml`, all in [`apps/tv-shell/qml/`](../apps/tv-shell/qml/) |
| TV shell C++ | `src/main.cpp` (display name, `--help` text), `src/ShellController.cpp` (error messages) |
| Phone remote | [`apps/remote-web/src/i18n.ts`](../apps/remote-web/src/i18n.ts) (all phone copy, incl. `productName`), `static/index.html`, `static/manifest.webmanifest` |
| Coordinator | `device.display_name` default `"Bear Den"` in [`contracts/fixtures/config.default.valid.json`](../contracts/fixtures/config.default.valid.json); the mDNS default name `avahiDefaultName` in `internal/remote/mdns/mdns.go`; `ShellLabel` in `internal/session/coordinator.go`; `Product` sent to Plex in `internal/providers/plex/client.go`; tuning notes in `internal/applications/tuning/` |
| Desktop entries | `cmd/bear-den-tv/shortcut.go`, `cmd/bear-den-tv/autostart.go`, `internal/platform/autostart/autostart.go`, `packaging/bear-den-tv.desktop`, `packaging/autostart/bear-den-tv.desktop` |
| Docs | everywhere; leave history (ADRs, reports) as it is |

The name your TV shows to phones is also a setting: `device.display_name`
in `config.json`.

Done when:
- `grep -rn "Bear Den" apps/tv-shell/qml apps/tv-shell/src apps/remote-web/src apps/remote-web/static cmd internal packaging`
  lists only comments and names you chose to keep;
- `make test` passes (some Go tests use the name, for example
  `internal/remote/mdns/mdns_test.go`; update them with it);
- a sandbox Home and Settings screenshot and the phone show the new name.

## Phones

Paired phones receive `state.appearance`: the theme's name, accent, palette,
tile decoration (style and tip ornament URL), backdrop URL, veil and particle
kind, plus the installed themes for the layout editor. The coordinator
resolves it with
[`internal/themes.Registry.Appearance`](../internal/themes/themes.go) and
serves the art under `/themes/<id>/<file>`. Only image files of loaded themes
and the built-in ornaments are served, never manifests.

On the phone:
- [`main.tsx`](../apps/remote-web/src/main.tsx) sets the CSS variables;
- [`vines.tsx`](../apps/remote-web/src/vines.tsx) draws the tile decoration;
- [`app.css`](../apps/remote-web/src/app.css) holds the particles.

## Performance rules

Every decoration must be free when idle and cheap when not, on a 2-core
Celeron driving a 120 Hz output.

1. **Behind apps, nothing moves.** Every animation requires
   `Session.target.kind === "shell"`.
2. **Screensaver, nothing moves.** `Theme.screensaver` stops everything.
3. **Resting (45 s without input):**
   - Background loops stop: particles, wallpaper drift, scenes, panel sway,
     the peeking cub's blinks.
   - Still running, slowly: the focused decoration (~6 fps) and rarer bear
     visits.
4. **Reduced motion:** decorations appear fully grown and still; no visits
   and no peeking cub.
5. **Frame budget:**
   - Decorations, particles, the wallpaper drift and visiting bears move on
     one clock, `World.beat(dt)` (`qml/World.qml`): 20 beats a second awake,
     4 while resting (full speed while a bear is out), none otherwise. Using
     the one clock puts every change in the same frame; separate timers
     would each trigger frames and their rates would add up. Never use
     free-running 60–120 fps animations for anything that loops.
   - Canvas items repaint only when their inputs change.
   - No custom GPU shaders (Canvas/QPainter, Image, Rectangle).
6. **Plain turns everything off.**
7. **Memory and start-up: build and decode only what shows.**
   - Images decode at the size they are drawn: `sourceSize` on every
     `Image`; `RoundedImage` does it itself, on first paint. A 2560×1440
     wallpaper on a 360-pixel theme card is a few hundred kilobytes, not
     15 MB.
   - A component with a Pixel and a Classic version creates only the current
     one (a `Repeater` of 0 or 1, or a `Loader`), never both with one hidden:
     a hidden Canvas still costs an item, a 2D context and a texture, and
     `PixelBox` alone appears hundreds of times.
   - Screens other than Home are built when first opened (`ScreenSlot` in
     `ShellRoot.qml`); each takes a few milliseconds (up to ~0.1 s on the
     Celeron), inside its fade-in.
   - Measured with the reference TV's own state (Classic art, Midnight),
     offscreen on a desktop: the shell's private memory went from 322 MB to
     90 MB when these three landed (pixel art: about 150 MB before, as
     much or less after). On the TV it was about 460 MB resident before.

Check with `make perf` (`scripts/perf-sandbox.sh`): it runs the shell here,
offscreen on two cores, and fails when resting or the screensaver draws more
than its budget; `--theme <id>` checks a theme. Then measure on the TV with
`scripts/measure-target.sh 20` (CPU %) and `BDTV_FPS_LOG=1` (frames per
second) while awake and while resting.

## Extending the engine (new building blocks)

A new **style**, **particle kind** or **scene** is code, and needs the same
change in every place that knows the vocabulary. A theme can then pick it by
name; engine code never checks a theme id.

1. **Contract:** add the value to the enum in
   [`contracts/theme.schema.json`](../contracts/theme.schema.json), and to
   `state.appearance` in
   [`contracts/state.schema.json`](../contracts/state.schema.json)
   (`focus.style`, `phone.particles`) when phones should show it
   ([`contracts/AGENTS.md`](../contracts/AGENTS.md#change-a-contract)).
2. **TV loader:** add it to the allowed lists in
   [`apps/tv-shell/src/ThemeRegistry.cpp`](../apps/tv-shell/src/ThemeRegistry.cpp)
   (`kStyles`, `kParticles`, `kScenes`; `kChases` for a new `bears.chase` kind).
3. **TV drawing:**
   - a style: a `paint<Style>()` function and a `case` in `paintAll()` in
     [`CornerDecor.qml`](../apps/tv-shell/qml/CornerDecor.qml);
   - a particle kind: a `Repeater` gated on `root.kind` in
     [`Ambient.qml`](../apps/tv-shell/qml/Ambient.qml), moving on
     `World.beat`;
   - a scene: a component plus the `Loader` map in
     [`HomeScreen.qml`](../apps/tv-shell/qml/HomeScreen.qml); its art in
     `tools/pixelart/scenes.py`.
4. **Phone:**
   - a style: an entry in `SPRITES` and `STEM_OPACITY` in
     [`vines.tsx`](../apps/remote-web/src/vines.tsx) (and `.vine-style-<style>`
     rules in `app.css` if it animates);
   - a particle kind: `:root[data-particles='<kind>']` rules in
     [`app.css`](../apps/remote-web/src/app.css);
   - extend `FocusStyle`/`ParticleKind` in
     [`contract.ts`](../apps/remote-web/src/contract.ts).
5. **Go:** the coordinator validates manifests against the schema, so a new
   enum value needs nothing more. New manifest *fields* need
   `internal/themes.Manifest` and `Appearance` in
   [`internal/themes/themes.go`](../internal/themes/themes.go).
6. **Prove it:**
   - use it in a built-in theme (`go test ./internal/themes` validates every
     built-in theme; `make test-shell` loads them all);
   - `make shots` and look; `make perf`;
   - measure on the TV, or write "not measured".
7. **Document it** in this file's tables in the same change.

Not every kind is a manifest value. `rain` is drawn by `Ambient.qml` but set
only by [`World.qml`](../apps/tv-shell/qml/World.qml) for local weather
([Weather in the scene](#weather-in-the-scene)); it is not in
`theme.schema.json` or `kParticles`. To let themes pick rain too, follow the
steps above for it (schema, `kParticles`, phone CSS, `contract.ts`).

## Art and brands

Bear Den bundles only its own art: the pixel worlds, bears and ornaments are
generated by `tools/pixelart` (the bears keep the colours and features of the
owner's original mark), and so are its own app icons ([App icons](#app-icons)).
We bundle our own original art, never third-party logos: nothing trademarked
is committed. By default an installed app shows the icon its own Flatpak
exports, read from the owner's disk at run time
([ADR 0012](decisions/0012-app-icons-apps-own-by-default.md)); that is not
bundled either. An owner who wants an app's official logo or a different
icon puts it in the brand folder
(`~/.local/share/bear-den-tv/brand/<adapter>/{icon,logo,background}.*`),
which wins over both.

## App icons

Every supported app has an original Bear Den icon in both art styles: a
badge in the app's brand colours with a pair of bear ears on top, holding a
motif that says what the app is for. None copies the app's logo. It is what
a tile shows with **Bear Den style**, and with the default **App's own**
whenever the app's own icon is not available (the app is not installed, it
is a streaming site, or its Flatpak exports none).

| Adapter | Motif |
|---|---|
| `plex-htpc` | a film reel with a strip of film |
| `vacuumtube` | a chunky vintage TV with a bear on its screen |
| `moonlight` | a crescent moon over a game controller |
| `spotify` | a record player |
| `jellyfin` | a home-theatre screen with two bears on a couch |
| `retroarch` | an arcade cabinet with a bear on its screen |
| `netflix` | a striped popcorn bucket (web app; badge in Bear Den's plum, not the service's colours) |
| `disney-plus` | a magic wand with a star and sparkles (no castle, no arc) |
| `hulu` | a mint-green 1960s TV set on a stand, one antenna, a moonlit hill on its screen (not YouTube's red TV) |
| `browser` | a globe with a gold compass needle |

- **Pixel:** 32×32 art pixels, drawn by `tools/pixelart/appicons.py`
  (`apps/tv-shell/assets/pixel/app-<adapter>.png`); `AppIcon` shows it at the
  largest whole-number scale that fits, unsmoothed.
- **Classic:** the same design smooth, `tools/classicart/appicons.py`
  (`apps/tv-shell/assets/classic/app-<adapter>.svg`, a 32×32 view box).
- **Phones:** both tools copy their icons to `apps/remote-web/static/art/`
  (`pixel/app-<adapter>.png`, `app-<adapter>.svg`); the remote's app tiles use
  them (`AppArt` in `icons.tsx`) whenever the coordinator has no icon of the
  app's own for them.
- **App's own or Bear Den style** (Themes → App icons, the phone's Layout;
  layout `ui.app_icons`, owner decision: the app's own by default;
  [ADR 0012](decisions/0012-app-icons-apps-own-by-default.md)).
- **Order**, everywhere an app is shown (TV tiles, the featured panel and
  its room's screen, the launch overlay, the install card, Add apps, phone
  tiles):
  1. the owner's brand folder (`brand/<adapter>/icon.*`);
  2. with "App's own": the icon the **installed** Flatpak exports, when that
     Flatpak is the app itself. The streaming sites (Netflix, Disney+, Hulu)
     run in Google Chrome and never show its icon; the Browser tile may show its browser's. An app
     that is not installed has no export;
  3. Bear Den's icon;
  4. a monogram.

  The TV resolves this in `Shell.appArt`; phones get rungs 1 and 2 from
  [`GET /api/v1/apps/{adapter}/icon`](../contracts/http.md#app-icons) as PNG
  (never SVG: an SVG-only brand icon or export is skipped on phones) and
  draw Bear Den's icon when it answers 404. The featured panel's rooms stay
  Bear Den's art; the room's screen shows the chosen icon.
- Regenerate: `python3 -B tools/pixelart/build.py --preview appicons`
  (preview `build/pixel-preview/appicons.png`) and
  `python3 -B tools/classicart/build.py appicons`.

## UI icons

The shell's own pictures for its pills, settings rows and setup cards are
generated in both art styles, in the same family as the [app icons](#app-icons):
the bear-ear badge, each in its own den colour, holding one bold cream motif
that reads at 48–96 screen pixels from the couch. Original art only.

| Name | Motif | For |
|---|---|---|
| `gear` | a chunky cog | Settings |
| `apps` | a 2×2 grid of tiles | Apps |
| `themes` | a painter's palette with a brush | Themes |
| `phone` | a phone with a bear face on its screen | Pair phone; Phones & remote |
| `display` | a TV screen with an eye | Display & accessibility |
| `home` | a little log cabin | Home screen |
| `play` | a play triangle on a film frame | Playback |
| `power` | a crescent moon with a power symbol | Power & TV |
| `about` | a round "i" | About |
| `plus` | a big bold "+" | Add apps |
| `globe` | a wireframe globe | Streaming sites |
| `refresh` | two circular arrows | Keep apps up to date |
| `medal` | a gold medal with a paw | Den badges |
| `wave` | a bear paw waving | Welcome / setup |

- **Where:** `apps/tv-shell/assets/pixel/icon-<name>.png` (32×32 art pixels,
  `tools/pixelart/uiicons.py`) and `apps/tv-shell/assets/classic/icon-<name>.svg`
  (a 32×32 view box, `tools/classicart/uiicons.py`, colours read from the
  pixel module). Both folders are globbed into the shell build. Icons listed
  in `PHONE` (today `plus`) are also copied to `apps/remote-web/static/art/`.
- **Generated:** edit the code, never the files. Regenerate with
  `python3 -B tools/pixelart/build.py --preview uiicons` (look at
  `build/pixel-preview/uiicons.png` and `uiicons-small.png`) and
  `python3 -B tools/classicart/build.py uiicons`.
- **Add one:** a colour in `BADGE` and a motif function in `ICONS` in
  `tools/pixelart/uiicons.py`, the smooth twin with the same name in
  `tools/classicart/uiicons.py`, a row in the table above; rebuild both and
  look at the preview at 1× and 2×.

## Den badges

Each Den badge ([`docs/operations.md`](operations.md#den-badges)) has an
original medal in both art styles: a round medallion with a gold rim, a pair
of bear ears on top and two ribbon tails, in the badge's own colours, holding
a motif. A badge not earned yet shows its silhouette: the same shape in one
slate colour with a question mark.

| Badge | Motif |
|---|---|
| `first-night-in` | a striped popcorn bucket |
| `movie-night` | a film clapperboard |
| `couch-explorer` | a little sofa |
| `night-owl` | an owl's face under a crescent moon |
| `early-cub` | the sun rising over a hill |
| `rainy-day` | an umbrella in the rain |
| `snow-day` | a snowflake |
| `thunder-buddy` | a cloud with a lightning bolt |
| `all-seasons` | a tree, one quarter per season |
| `style-switcher` | a brush painting half a pixel, half a curve |
| `theme-tourist` | a suitcase with travel stickers |
| `family-den` | three bear heads, big to small |
| `good-host` | a guest ticket |
| `sleepy-bear` | a sleeping bear's head with a "z" |
| `parade-spotter` | a drum with two sticks |
| `loyal-den` | a calendar page with a heart |

- **Pixel:** 32×32 art pixels, `tools/pixelart/badges.py`
  (`apps/tv-shell/assets/pixel/badge-<id>.png` and `badge-<id>-locked.png`);
  `BadgeMedal` draws one art pixel per world pixel (128 screen pixels at
  1080p), unsmoothed.
- **Classic:** the same medals smooth, `tools/classicart/badges.py`
  (`apps/tv-shell/assets/classic/badge-<id>.svg`, `-locked.svg`, a 32×32 view
  box). It reads the colours from the pixel module, so the styles never drift.
- **Phones:** both tools copy the medals to `apps/remote-web/static/art/`
  (`pixel/badge-<id>.png`, `badge-<id>.svg`, and the `-locked` twins).
- A new badge is a row in `Badges` in `internal/achievements`, its name and
  hint in `qml/Badges.qml` and in the phone's `i18n.ts`, and a motif in both
  tools; `TestCatalogue` fails until its art exists in all eight places.
- Regenerate: `python3 -B tools/pixelart/build.py --preview badges`
  (preview `build/pixel-preview/badges.png`) and
  `python3 -B tools/classicart/build.py badges`.
