# ADR 0006 — A Classic art style beside pixel art

Date: 2026-09-23. Status: accepted. Amends [ADR 0005](0005-pixel-art.md).

The owner asked to switch between the pixel-art look and the original smooth
(SVG and picture) look, with Classic matching everything pixel art does,
including the parts that were only ever drawn as pixels.

**Decision.**
- A new, optional `layout.ui.art_style`: `pixel` (default; missing means
  pixel) or `classic`. It is independent of `ui.theme` (Style), so Plain and
  Performance work with either. Additive under protocol 1 (ADR 0004).
- Phones mirror it as `state.appearance.art_style`; `appearance.pixel` still
  says whether the backdrop in use is pixel art.
- Themes carry Classic art in an optional `classic` block
  (`classic.wallpaper.image`/`.sprites`, `classic.phone.backdrop`). A theme
  without it keeps its own wallpaper in Classic, so existing owner themes
  need no change.
- Ornament names resolve style-first: the `.png` in Pixel, the `.svg` in
  Classic, theme folder before built-in. Built-in ornaments ship both.
- The shell resolves every theme once per art style
  (`Themes.get(id, artStyle)`); components branch on `World.pixel` /
  `World.classic`. Pixel components keep their grid; Classic ones draw
  antialiased, continuous and rotated as they did before ADR 0005.

**Consequences.** Two sets of art to keep in step: a new pixel feature needs a
Classic counterpart (and the reverse). Classic art that the repository
generates is SVG written by code, like `tools/pixelart`; the four original
world pictures are kept as JPEGs because nothing here can regenerate them.
