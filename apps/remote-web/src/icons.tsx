// Inline SVG glyphs for controls, and the illustrations (`Art`) in both art
// styles (docs/decisions/0006-classic-art-style.md): Pixel draws same-origin
// PNGs from static/art/pixel at whole-number scales, Classic the smooth SVGs
// from static/art. Contract: every icon is decorative (`aria-hidden`); the
// owning button carries the accessible label. Icon paths use `currentColor` so
// the theme controls their colour.
import type { JSX } from 'preact';
import type { Appearance, ArtStyle } from './contract.ts';

export type IconName =
  | 'up'
  | 'down'
  | 'left'
  | 'right'
  | 'back'
  | 'home'
  | 'play'
  | 'pause'
  | 'rewind'
  | 'forward'
  | 'volume-down'
  | 'volume-up'
  | 'mute'
  | 'unmute'
  | 'keyboard'
  | 'app'
  | 'remote'
  | 'layout'
  | 'devices'
  | 'about'
  | 'restart'
  | 'close'
  | 'moon'
  | 'screen-off';

const PATHS: Record<IconName, string> = {
  up: 'M12 6l7 8H5z',
  down: 'M12 18l-7-8h14z',
  left: 'M6 12l8-7v14z',
  right: 'M18 12l-8 7V5z',
  back: 'M10 6L4 12l6 6v-4h6a3 3 0 0 1 0 6h-2v2h2a5 5 0 0 0 0-10h-6z',
  home: 'M12 3l9 8h-3v9h-5v-6h-2v6H6v-9H3z',
  play: 'M7 5l12 7-12 7z',
  pause: 'M6 5h4v14H6zm8 0h4v14h-4z',
  rewind: 'M11 12l9-6v12zM3 12l9-6v12z',
  forward: 'M13 12L4 6v12zm8 0l-9-6v12z',
  'volume-down': 'M4 9v6h4l5 4V5L8 9zm11 0h6v2h-6z',
  'volume-up': 'M4 9v6h4l5 4V5L8 9zm11 0h6v2h-6zm2-3h2v8h-2z',
  mute: 'M4 9v6h4l5 4V5L8 9zm11.5 1.1l1.4-1.4 1.6 1.6 1.6-1.6 1.4 1.4-1.6 1.6 1.6 1.6-1.4 1.4-1.6-1.6-1.6 1.6-1.4-1.4 1.6-1.6z',
  unmute: 'M4 9v6h4l5 4V5L8 9zm12.5 3a4.5 4.5 0 0 0-2.5-4v8a4.5 4.5 0 0 0 2.5-4zm-2.5-8v2a6 6 0 0 1 0 12v2a8 8 0 0 0 0-16z',
  keyboard: 'M3 6h18v12H3zm2 2v2h2V8zm4 0v2h2V8zm4 0v2h2V8zm4 0v2h2V8zM5 11v2h2v-2zm4 0v2h2v-2zm4 0v2h2v-2zm4 0v2h2v-2zM7 14v2h10v-2z',
  app: 'M4 4h7v7H4zm9 0h7v7h-7zM4 13h7v7H4zm9 0h7v7h-7z',
  remote: 'M8 2h8a2 2 0 0 1 2 2v16a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm4 4a2 2 0 1 0 0 4 2 2 0 0 0 0-4zm-3 7v2h6v-2zm0 4v2h6v-2z',
  layout: 'M3 4h18v5H3zm0 7h8v9H3zm10 0h8v9h-8z',
  devices: 'M4 4h10v14H4zm2 2v10h6V6zm10 3h4v11h-4zm1 2v7h2v-7z',
  about: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v2h-2zm0 4h2v6h-2z',
  restart: 'M12 4a8 8 0 1 0 7.3 4.8l-1.8.8A6 6 0 1 1 12 6v3l4-4-4-4z',
  close: 'M6.4 5l12.6 12.6-1.4 1.4L5 6.4zM5 17.6L17.6 5 19 6.4 6.4 19z',
  moon: 'M14.5 3a8.5 8.5 0 1 0 6.5 13.9A7 7 0 0 1 14.5 3z',
  'screen-off': 'M3 4h18v12h-7v2h3v2H7v-2h3v-2H3zm2 2v8h14V6zm4.5 1.5l5 5-1 1-5-5zm5 0l1 1-5 5-1-1z',
};

/**
 * @param props Icon name and optional size in CSS pixels.
 * @returns A decorative inline SVG.
 */
export function Icon({ name, size = 24 }: { name: IconName; size?: number }): JSX.Element {
  return (
    <svg class="icon" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} fill="currentColor" />
    </svg>
  );
}

export type ArtName = 'bear-mark' | 'bear-cub' | 'bear-sleep' | 'paw' | 'heart' | 'sparkle' | 'sprig' | 'divider' | 'den';

/** Pixel-art files in `art/pixel/` (static/art/pixel) and their native size in art pixels. */
export const ART: Record<ArtName, { file: string; w: number; h: number }> = {
  'bear-mark': { file: 'bear-mark', w: 16, h: 17 },
  'bear-cub': { file: 'head-cub', w: 21, h: 19 },
  'bear-sleep': { file: 'head-cub-sleep', w: 21, h: 19 },
  paw: { file: 'paw', w: 7, h: 7 },
  heart: { file: 'heart', w: 9, h: 8 },
  sparkle: { file: 'sparkle', w: 7, h: 7 },
  sprig: { file: 'sprig', w: 13, h: 7 },
  divider: { file: 'divider', w: 21, h: 3 },
  den: { file: 'den', w: 12, h: 8 },
};

/** Smooth files in `art/` (static/art) for the Classic style. */
export const CLASSIC_ART: Record<ArtName, string> = {
  'bear-mark': 'bear-mark',
  'bear-cub': 'bear-cub',
  'bear-sleep': 'bear-sleep',
  paw: 'paw',
  heart: 'heart',
  sparkle: 'sparkle',
  sprig: 'sprig',
  divider: 'divider',
  den: 'den',
};

/**
 * The art style the phone draws in: the TV's `appearance.art_style`, Pixel
 * when it is missing or unknown (and before pairing, when there is no appearance).
 */
export function artStyleOf(appearance: Appearance | null | undefined): ArtStyle {
  return appearance?.art_style === 'classic' ? 'classic' : 'pixel';
}

/**
 * Decorative same-origin illustration. Contract: always `alt=""` and
 * `aria-hidden`; any meaning it carries is also present as text.
 * Pixel (default): `art/pixel/*.png` at a whole-number multiple of its native
 * size with `image-rendering: pixelated` (the `.art` class in app.css), so every
 * art pixel is a crisp square. Classic: `art/*.svg` at `width` x `height` CSS px.
 * @param props Art name, CSS class, integer pixel scale (CSS px per art pixel),
 *   the Classic size (height defaults to width) and the art style.
 * @returns A decorative image.
 */
export function Art({
  name,
  class: cls,
  scale,
  width,
  height,
  art = 'pixel',
}: {
  name: ArtName;
  class?: string;
  scale: number;
  width: number;
  height?: number;
  art?: ArtStyle;
}): JSX.Element {
  if (art === 'classic') {
    return <img class={`art art-classic ${cls ?? ''}`} src={`art/${CLASSIC_ART[name]}.svg`} width={width} height={height ?? width} alt="" aria-hidden="true" draggable={false} />;
  }
  const px = ART[name];
  const n = Math.max(1, Math.round(scale));
  return <img class={`art ${cls ?? ''}`} src={`art/pixel/${px.file}.png`} width={px.w * n} height={px.h * n} alt="" aria-hidden="true" draggable={false} />;
}
