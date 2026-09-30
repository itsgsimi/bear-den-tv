// Inline SVG glyphs for controls, the app tiles' icons (`AppArt`: the app's own
// icon from the TV when it has one, else Bear Den's own, never a bundled
// official logo; docs/THEMES.md → App icons), and the illustrations (`Art`) in both art
// styles (docs/decisions/0006-classic-art-style.md): Pixel draws same-origin
// PNGs from static/art/pixel at whole-number scales, Classic the smooth SVGs
// from static/art, and the Add apps tile's "+" (`PlusArt`). Contract: every icon is decorative (`aria-hidden`); the
// owning button carries the accessible label. Icon paths use `currentColor` so
// the theme controls their colour.
import type { JSX } from 'preact';
import type { AppIcons, Appearance, ArtStyle } from './contract.ts';

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
  | 'badge'
  | 'restart'
  | 'close'
  | 'moon'
  | 'screen-off'
  | 'power';

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
  badge: 'M12 2a7 7 0 1 0 0 14 7 7 0 0 0 0-14zm0 3l1.5 3 3.3.5-2.4 2.3.6 3.2L12 12.5 9 14l.6-3.2-2.4-2.3 3.3-.5zM7.5 15.2L5.5 22l3-1.4 2.2 2.2 1-5.3a9 9 0 0 1-4.2-2.3zm9 0a9 9 0 0 1-4.2 2.3l1 5.3 2.2-2.2 3 1.4z',
  restart: 'M12 4a8 8 0 1 0 7.3 4.8l-1.8.8A6 6 0 1 1 12 6v3l4-4-4-4z',
  close: 'M6.4 5l12.6 12.6-1.4 1.4L5 6.4zM5 17.6L17.6 5 19 6.4 6.4 19z',
  moon: 'M14.5 3a8.5 8.5 0 1 0 6.5 13.9A7 7 0 0 1 14.5 3z',
  'screen-off': 'M3 4h18v12h-7v2h3v2H7v-2h3v-2H3zm2 2v8h14V6zm4.5 1.5l5 5-1 1-5-5zm5 0l1 1-5 5-1-1z',
  power: 'M11 2h2v10h-2zM7.1 5.3l1.4 1.4a7 7 0 1 0 7 0l1.4-1.4a9 9 0 1 1-9.8 0z',
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

/** The pixel grid of Bear Den's app icons (and the Add apps tile's "+"). */
const APP_ICON_GRID = 32;

/**
 * The "+" of the Add apps tile: `art/pixel/icon-plus.png` (Pixel) or
 * `art/icon-plus.svg` (Classic), shown only once it has loaded (`data-art`
 * set by its load/error handlers, like AppArt's own icon); until then, or
 * when the file is missing, a "+" drawn in CSS (`.plus-fallback`).
 */
export function PlusArt({ art = 'pixel' }: { art?: ArtStyle }): JSX.Element {
  const src = art === 'classic' ? 'art/icon-plus.svg' : 'art/pixel/icon-plus.png';
  return (
    <span class="plus-art" key={src} data-art="pending">
      <span class="plus-fallback" aria-hidden="true" />
      <img
        class={`art plus-img ${art === 'classic' ? 'art-classic' : ''}`}
        src={src}
        width={APP_ICON_GRID}
        height={APP_ICON_GRID}
        alt=""
        aria-hidden="true"
        draggable={false}
        onLoad={(ev) => ev.currentTarget.parentElement?.setAttribute('data-art', 'ok')}
        onError={(ev) => ev.currentTarget.parentElement?.setAttribute('data-art', 'none')}
      />
    </span>
  );
}

/** Adapters with Bear Den's own icon (tools/pixelart and tools/classicart appicons.py). */
export const APP_ICONS: readonly string[] = ['plex-htpc', 'vacuumtube', 'moonlight', 'spotify', 'jellyfin', 'retroarch', 'netflix', 'disney-plus', 'hulu', 'browser'];

/**
 * Where the TV serves an app's own icon (contracts/http.md#app-icons), for an
 * adapter the phone knows only. The server ignores the query: it changes
 * with the TV's choice and when the app is installed, so the image (and the
 * browser's cache) follow both. Same origin (CSP img-src 'self').
 * @param adapter The application's adapter.
 * @param choice `appearance.app_icons` (missing means app).
 * @param installed The application is installed (applications[].installed).
 * @returns The path, or null for an unknown adapter.
 */
export function appIconUrl(adapter: string, choice: AppIcons | undefined, installed = true): string | null {
  if (!APP_ICONS.includes(adapter)) return null;
  return `/api/v1/apps/${encodeURIComponent(adapter)}/icon?icons=${choice === 'bear_den' ? 'bear_den' : 'app'}&installed=${installed ? 1 : 0}`;
}

/**
 * An app tile's icon: the app's own icon from the TV (the owner's brand icon,
 * or with "App's own" the installed Flatpak's; `appIconUrl`) when the TV has
 * one, otherwise Bear Den's own icon for the adapter, the same art as the TV's
 * tiles. Pixel: `art/pixel/app-<adapter>.png` (32×32 art pixels) at the
 * largest whole-number scale that fits `size`; Classic: `art/app-<adapter>.svg`
 * at `size`. An adapter without one gets the generic grid glyph. The TV's
 * icon loads hidden behind ours and replaces it only once it has loaded (a
 * 404 leaves ours), through a data attribute on the wrapper, so the view
 * keeps no state of its own.
 * @param props Adapter name, size in CSS px, the art style, the TV's icon
 * choice (`appearance.app_icons`) and whether the app is installed (a new
 * image once it is); `own: false` skips the TV's icon.
 * @returns A decorative image or glyph.
 */
export function AppArt({ adapter, size, art = 'pixel', icons, own = true, installed = true }: { adapter: string; size: number; art?: ArtStyle; icons?: AppIcons; own?: boolean; installed?: boolean }): JSX.Element {
  if (!APP_ICONS.includes(adapter)) return <Icon name="app" size={Math.round(size * 0.7)} />;
  const ours =
    art === 'classic' ? (
      <img class="art art-classic app-art app-art-ours" src={`art/app-${adapter}.svg`} width={size} height={size} alt="" aria-hidden="true" draggable={false} />
    ) : (
      <img class="art app-art app-art-ours" src={`art/pixel/app-${adapter}.png`} width={Math.max(1, Math.floor(size / APP_ICON_GRID)) * APP_ICON_GRID} height={Math.max(1, Math.floor(size / APP_ICON_GRID)) * APP_ICON_GRID} alt="" aria-hidden="true" draggable={false} />
    );
  const url = own ? appIconUrl(adapter, icons, installed) : null;
  if (!url) return ours;
  return (
    <span class="app-art-stack" key={url} data-own="pending">
      {ours}
      <img
        class="app-art app-art-own"
        src={url}
        width={size}
        height={size}
        alt=""
        aria-hidden="true"
        draggable={false}
        onLoad={(ev) => ev.currentTarget.parentElement?.setAttribute('data-own', 'ok')}
        onError={(ev) => ev.currentTarget.parentElement?.setAttribute('data-own', 'none')}
      />
    </span>
  );
}
