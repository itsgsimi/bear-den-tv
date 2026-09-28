// Growing corner decorations on app tiles: the web twin of the TV's focus
// decoration, drawn as pixel art in the TV theme's style and colours
// (state.appearance.focus and .palette; resolved by internal/themes from the
// theme package):
//   vine    stem with two-tone leaves     fern    fronds of leaflets
//   stars   a constellation of plus stars embers  a glowing trail with coals
// and the theme's tip ornament (a pixel PNG from /themes/) at the end.
// Contract: purely decorative (`aria-hidden`), no inline styles (CSP): geometry
// lives in SVG attributes and all motion in app.css. Everything sits on a
// GRID x GRID cell grid (one cell = one art pixel, 2 CSS px on a tile): the
// stem is an axis-aligned stepped path with square caps and every leaf is a
// handful of unit rects, all with `shape-rendering="crispEdges"`. The SVG is
// drawn for the top-left corner and mirrored to the bottom-right with a CSS
// rotation (a 180-degree turn keeps the grid); the tip is an <img> outside the
// SVG, drawn at a whole-number scale with `image-rendering: pixelated` and never
// rotated (`upright` tips such as flames stand on the stem end instead of
// centring on it). The path uses `pathLength="1"` so CSS grows it with
// stroke-dashoffset 1 -> 0; each leaf's base is its group's origin so CSS scale
// pivots on it. A `.vine-host` ancestor decides when they are grown (see
// app.css). Nothing is drawn when the style is empty (Plain).
//
// Classic art style (appearance.art_style = 'classic', ADR 0006) draws the
// smooth twin instead (ClassicCorner): a Bezier stem on a 100x100 viewBox with
// round caps, vector leaves per style that CSS unfurls and sways, and the tip
// (an SVG from /themes/) as an SVG <image> blooming at the stem's end; the
// whole SVG is mirrored to the bottom-right with a CSS rotation.
import type { JSX } from 'preact';
import type { Appearance, ArtStyle, FocusStyle } from './contract.ts';
import { artStyleOf } from './icons.tsx';

type Palette = { stem: string; light: string; dark: string; bloom: string; glow: string };
const DEN: Palette = { stem: '#5E8C6E', light: '#A3D1AE', dark: '#6A9C7A', bloom: '#F7F0E2', glow: '#E3B35C' };

/**
 * The tile decoration for the TV's appearance: den vines until the TV reports
 * its theme, with the daisy tip in the art style's format (PNG or SVG).
 */
export function decorOf(
  appearance: Appearance | null | undefined,
  art: ArtStyle = 'pixel',
): { style: FocusStyle; tip: string; upright: boolean; palette: Palette } {
  if (!appearance) return { style: 'vine', tip: art === 'classic' ? '/themes/_ornaments/daisy.svg' : '/themes/_ornaments/daisy.png', upright: false, palette: DEN };
  return {
    style: appearance.focus?.style ?? '',
    tip: appearance.focus?.tip ?? '',
    upright: appearance.focus?.tip_upright ?? false,
    palette: { ...DEN, ...(appearance.palette ?? {}) } as Palette,
  };
}

/** Cells per side; the card's corner sits at cell (5.6, 5.6) and the stem runs in cells 2-3. */
export const GRID = 40;

// The stem's centre line (2 cells wide, so its edges fall on whole cells): in
// along the top, down a two-step staircase at the corner, then down the side.
export const STEM = 'M39 3 H7 V5 H5 V7 H3 V36';

type Dir = 'n' | 's' | 'e' | 'w';
// [x, y, outward direction] of each leaf's base, in growth order; out and in alternate.
// n/s leaves sit above/below the top run (base row 2 / 4), w/e beside the side run (base column 2 / 4).
export const LEAVES: ReadonlyArray<readonly [number, number, Dir]> = [
  [34, 2, 'n'],
  [29, 4, 's'],
  [23, 2, 'n'],
  [17, 4, 's'],
  [11, 2, 'n'],
  [2, 12, 'w'],
  [4, 18, 'e'],
  [2, 24, 'w'],
  [4, 30, 'e'],
];

/** Where the tip ornament sits: the stem's end, as a fraction of the corner box. */
export const TIP_AT = { x: 3 / GRID, y: 37 / GRID };

type Ink = 'light' | 'dark' | 'glow';
// A sprite is unit cells [dx, dy, ink, opacity] drawn pointing "north" (outward = -y)
// from its base cell (0, -1); leafCells turns it to face the leaf's direction.
type Sprite = ReadonlyArray<readonly [number, number, Ink, number]>;
const SPRITES: Record<Exclude<FocusStyle, ''>, readonly [Sprite, Sprite]> = {
  // [regular, variant for every third leaf]
  vine: [
    [[0, -1, 'dark', 1], [1, -1, 'light', 1], [1, -2, 'light', 1], [2, -2, 'light', 1], [2, -3, 'dark', 1]],
    [[0, -1, 'dark', 1], [-1, -1, 'light', 1], [-1, -2, 'light', 1], [-2, -2, 'light', 1], [-2, -3, 'dark', 1]],
  ],
  fern: [
    [[0, -1, 'dark', 1], [0, -2, 'dark', 1], [-1, -2, 'light', 1], [1, -2, 'light', 1], [0, -3, 'light', 1], [-1, -4, 'light', 1], [1, -4, 'light', 1]],
    [[0, -1, 'dark', 1], [0, -2, 'dark', 1], [-1, -3, 'light', 1], [1, -3, 'light', 1], [0, -4, 'light', 1]],
  ],
  stars: [
    [[0, -2, 'light', 1], [-1, -2, 'glow', 0.5], [1, -2, 'glow', 0.5], [0, -1, 'glow', 0.5], [0, -3, 'glow', 0.5]],
    [[0, -3, 'light', 1], [-1, -3, 'light', 1], [1, -3, 'light', 1], [0, -2, 'light', 1], [0, -4, 'light', 1], [-2, -3, 'glow', 0.45], [2, -3, 'glow', 0.45], [0, -1, 'glow', 0.45], [0, -5, 'glow', 0.45]],
  ],
  embers: [
    [[0, -1, 'light', 1], [-1, -1, 'glow', 0.35], [1, -1, 'glow', 0.35], [0, -2, 'glow', 0.35]],
    [[0, -2, 'light', 1], [1, -2, 'light', 1], [0, -1, 'glow', 0.35], [1, -1, 'glow', 0.35], [-1, -2, 'glow', 0.35], [2, -2, 'glow', 0.35], [0, -3, 'glow', 0.35], [1, -3, 'glow', 0.35]],
  ],
};

/**
 * Turns a north-facing sprite to face `dir` by rotating each cell's centre a
 * multiple of 90 degrees about the base origin, so cells stay on the grid.
 * @returns [x, y, ink, opacity] of each unit cell's top-left corner.
 */
export function leafCells(sprite: Sprite, dir: Dir): Array<[number, number, Ink, number]> {
  return sprite.map(([dx, dy, ink, op]) => {
    const cx = dx + 0.5;
    const cy = dy + 0.5;
    const [rx, ry] = dir === 'n' ? [cx, cy] : dir === 's' ? [-cx, -cy] : dir === 'e' ? [-cy, cx] : [cy, -cx];
    return [rx - 0.5, ry - 0.5, ink, op];
  });
}

const STEM_OPACITY: Record<Exclude<FocusStyle, ''>, number> = { vine: 1, fern: 1, stars: 0.45, embers: 0.8 };

function Corner({ corner, decor }: { corner: 'tl' | 'br'; decor: ReturnType<typeof decorOf> }): JSX.Element | null {
  const style = decor.style;
  if (style === '') return null;
  const ink = (i: Ink) => (i === 'light' ? decor.palette.light : i === 'dark' ? decor.palette.dark : decor.palette.glow);
  return (
    <span class={`vine vine-${corner} vine-style-${style}`} aria-hidden="true">
      <svg class="vine-art" viewBox={`0 0 ${GRID} ${GRID}`} shape-rendering="crispEdges" focusable="false">
        <path
          class="vine-stem"
          d={STEM}
          pathLength={1}
          fill="none"
          stroke={decor.palette.stem}
          stroke-opacity={STEM_OPACITY[style]}
          stroke-width={2}
          stroke-linecap="square"
          stroke-linejoin="miter"
        />
        {LEAVES.map(([x, y, dir], i) => (
          <g key={i} transform={`translate(${x} ${y})`}>
            <g class={`vine-sway vine-sway-${i % 3}`}>
              <g class={`vine-leaf vine-leaf-${i + 1}`}>
                {leafCells(SPRITES[style][i % 3 === 1 ? 1 : 0], dir).map(([cx, cy, c, op], j) => (
                  <rect key={j} x={cx} y={cy} width={1} height={1} fill={ink(c)} opacity={op === 1 ? undefined : op} />
                ))}
              </g>
            </g>
          </g>
        ))}
      </svg>
      {decor.tip ? (
        <span class="vine-bud">
          <img class={`vine-tip${decor.upright ? ' tip-upright' : ''}`} src={decor.tip} alt="" draggable={false} />
        </span>
      ) : null}
    </span>
  );
}

// Classic (smooth) renderer, restored from before the pixel-art switch.
// viewBox units; the card's corner sits at (14, 14) and the path runs ~7 units outside it.
export const CLASSIC_STEM = 'M98 7 C88 4 80 10 70 7 S52 4 44 7 S30 7 26 7 Q7 7 7 26 C5 36 9 44 7 54 S5 74 8 90';

// [x, y, angle in degrees] along the path in growth order; outward and inward alternate.
export const CLASSIC_LEAVES: ReadonlyArray<readonly [number, number, number]> = [
  [90, 6, -150],
  [78, 8, 115],
  [61, 6, -140],
  [45, 7, 130],
  [25, 7, -135],
  [11, 17, 45],
  [7, 38, 160],
  [8, 55, 35],
  [6, 74, -160],
];

/** The Classic tip's position: the stem's end, in viewBox units. */
export const CLASSIC_TIP: readonly [number, number] = [8, 90];

function ClassicLeaf({ style, index, palette }: { style: FocusStyle; index: number; palette: Palette }): JSX.Element {
  switch (style) {
    case 'fern':
      // A fern pinna: three narrow leaflets fanning out.
      return (
        <>
          <path d="M0 0 C4 -2.5 9 -2.5 13 0 C9 2.5 4 2.5 0 0 Z" fill={palette.light} />
          <path d="M0 0 C3 -4 7 -7 10 -8 C8 -5 4 -2 0 0 Z" fill={palette.dark} />
          <path d="M0 0 C3 4 7 7 10 8 C8 5 4 2 0 0 Z" fill={palette.dark} />
        </>
      );
    case 'stars':
      // A star: soft glow and a four-point sparkle.
      return (
        <>
          <circle r={index % 3 === 1 ? 5.5 : 4.2} fill={palette.glow} opacity="0.22" />
          <path d={index % 3 === 1 ? 'M0 -4 Q0 0 4 0 Q0 0 0 4 Q0 0 -4 0 Q0 0 0 -4 Z' : 'M0 -3 Q0 0 3 0 Q0 0 0 3 Q0 0 -3 0 Q0 0 0 -3 Z'} fill={palette.light} />
        </>
      );
    case 'embers':
      // A glowing coal.
      return (
        <>
          <circle r="4.6" fill={palette.glow} opacity="0.3" />
          <circle r="2" fill={palette.light} />
        </>
      );
    default:
      return (
        <>
          <path d="M0 0 C4 -6 11 -6 16 0 C11 6 4 6 0 0 Z" fill={palette.light} />
          <path d="M0 0 C4 6 11 6 16 0 Z" fill={palette.dark} />
        </>
      );
  }
}

function ClassicTip({ url, upright }: { url: string; upright: boolean }): JSX.Element | null {
  if (!url) return null;
  return (
    <g class={upright ? 'tip-upright' : undefined}>
      <image href={url} x="-9" y={upright ? -14 : -9} width="18" height="18" />
    </g>
  );
}

export const CLASSIC_STROKE_WIDTH: Record<Exclude<FocusStyle, ''>, number> = { vine: 4.5, fern: 3.2, stars: 1.3, embers: 2.6 };

/** @returns One smooth corner (Classic art style), or null for Plain. */
export function ClassicCorner({ corner, decor }: { corner: 'tl' | 'br'; decor: ReturnType<typeof decorOf> }): JSX.Element | null {
  const style = decor.style;
  if (style === '') return null;
  const flat = style === 'stars' || style === 'embers';
  return (
    <svg class={`vine vine-classic vine-${corner} vine-style-${style}`} viewBox="0 0 100 100" aria-hidden="true" focusable="false">
      <path class="vine-stem" d={CLASSIC_STEM} pathLength={1} fill="none" stroke={decor.palette.stem} stroke-width={CLASSIC_STROKE_WIDTH[style]} stroke-linecap="round" />
      {CLASSIC_LEAVES.map(([x, y, angle], i) => (
        <g key={i} transform={`translate(${x} ${y}) rotate(${flat ? 0 : angle})`}>
          <g class={`vine-sway vine-sway-${i % 3}`}>
            <g class={`vine-leaf vine-leaf-${i + 1}`}>
              <ClassicLeaf style={style} index={i} palette={decor.palette} />
            </g>
          </g>
        </g>
      ))}
      <g transform={`translate(${CLASSIC_TIP[0]} ${CLASSIC_TIP[1]})`}>
        <g class="vine-bud">
          <ClassicTip url={decor.tip} upright={decor.upright} />
        </g>
      </g>
    </svg>
  );
}

/**
 * @param props The TV's appearance, and the art style (defaults to the
 *   appearance's; the pair screen passes it without an appearance).
 * @returns The two corner decorations (top-left and mirrored bottom-right) for a `.vine-host`.
 */
export function Vines({ appearance, art }: { appearance?: Appearance | null; art?: ArtStyle }): JSX.Element {
  const style = art ?? artStyleOf(appearance);
  const decor = decorOf(appearance, style);
  const C = style === 'classic' ? ClassicCorner : Corner;
  return (
    <>
      <C corner="tl" decor={decor} />
      <C corner="br" decor={decor} />
    </>
  );
}
