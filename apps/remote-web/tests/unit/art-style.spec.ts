// Unit tests for the runtime art style switch (ADR 0006): artStyleOf (what
// main.tsx writes to <html data-art>), the Art component choosing the pixel PNG
// or the classic SVG, and the Classic (smooth) vine renderer in src/vines.tsx.
import { existsSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { Appearance } from '../../src/contract.ts';
import { AppArt, APP_ICONS, Art, artStyleOf } from '../../src/icons.tsx';
import { CLASSIC_LEAVES, CLASSIC_STEM, ClassicCorner, decorOf, Vines } from '../../src/vines.tsx';

type Props = Record<string, unknown> & { children?: unknown };
const props = (n: unknown): Props => (n as VNode<Props>).props;

/**
 * Every vnode in a tree. With `expand`, stateless function components are
 * called and their output walked too (a tiny render without a DOM).
 */
function walk(node: unknown, expand = false, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) {
    for (const c of node) walk(c, expand, out);
  } else if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    out.push(v);
    if (expand && typeof v.type === 'function') walk((v.type as (p: Props) => unknown)(v.props), expand, out);
    else walk(v.props.children, expand, out);
  }
  return out;
}

describe('art style', () => {
  it('is pixel without an appearance, when missing or unknown, classic only when the TV says so', () => {
    expect(artStyleOf(null)).toBe('pixel');
    expect(artStyleOf(undefined)).toBe('pixel');
    expect(artStyleOf({} as Appearance)).toBe('pixel');
    expect(artStyleOf({ art_style: 'pixel' } as Appearance)).toBe('pixel');
    expect(artStyleOf({ art_style: 'bogus' } as unknown as Appearance)).toBe('pixel');
    expect(artStyleOf({ art_style: 'classic' } as Appearance)).toBe('classic');
  });

  it('Art draws the pixel PNG at a whole-number scale by default', () => {
    const img = props(Art({ name: 'bear-cub', scale: 4, width: 96 }));
    expect(img.src).toBe('art/pixel/head-cub.png');
    expect([img.width, img.height]).toEqual([84, 76]);
  });

  it('Art draws the smooth SVG at its classic size in the classic style', () => {
    const img = props(Art({ name: 'bear-cub', scale: 4, width: 96, art: 'classic' }));
    expect(img.src).toBe('art/bear-cub.svg');
    expect([img.width, img.height]).toEqual([96, 96]);
    const den = props(Art({ name: 'den', scale: 16, width: 200, height: 150, art: 'classic' }));
    expect([den.src, den.width, den.height]).toEqual(['art/den.svg', 200, 150]);
    expect(props(Art({ name: 'bear-sleep', scale: 2, width: 40, art: 'classic' })).src).toBe('art/bear-sleep.svg');
  });
});

describe('classic vines', () => {
  it('defaults to the SVG daisy tip in classic and the PNG one in pixel', () => {
    expect(decorOf(null, 'classic').tip).toBe('/themes/_ornaments/daisy.svg');
    expect(decorOf(null).tip).toBe('/themes/_ornaments/daisy.png');
  });

  it('Vines picks the renderer from the art style', () => {
    const pick = (a: Appearance) => walk(props(Vines({ appearance: a })).children).map((n) => n.type);
    const classic = { art_style: 'classic', focus: { style: 'vine', tip: '/t.svg' } } as unknown as Appearance;
    const pixel = { art_style: 'pixel', focus: { style: 'vine', tip: '/t.png' } } as unknown as Appearance;
    expect(pick(classic).every((t) => t === ClassicCorner)).toBe(true);
    expect(pick(pixel).some((t) => t === ClassicCorner)).toBe(false);
    expect(walk(props(Vines({ art: 'classic' })).children).every((n) => n.type === ClassicCorner)).toBe(true);
  });

  it('draws a round-capped Bezier stem, a vector leaf per point and the tip as an SVG <image>', () => {
    const svg = ClassicCorner({ corner: 'br', decor: decorOf(null, 'classic') });
    const nodes = walk(svg, true);
    expect(props(svg).class).toContain('vine-classic');
    expect(props(svg).class).toContain('vine-br');
    const stem = nodes.find((n) => n.props.class === 'vine-stem');
    expect(stem?.props.d).toBe(CLASSIC_STEM);
    expect(stem?.props['stroke-linecap']).toBe('round');
    const leaves = nodes.filter((n) => typeof n.props.class === 'string' && n.props.class.startsWith('vine-leaf '));
    expect(leaves.length).toBe(CLASSIC_LEAVES.length);
    // Each leaf is a smooth path (curves), not grid cells.
    for (const leaf of leaves) expect(walk(leaf.props.children, true).some((n) => n.type === 'path' || n.type === 'circle')).toBe(true);
    // Leaves sit on the stem's run: along the top (y < 12) or down the side (x < 12).
    for (const [x, y] of CLASSIC_LEAVES) expect(x < 12 || y < 12).toBe(true);
    const images = nodes.filter((n) => n.type === 'image');
    expect(images.map((n) => n.props.href)).toEqual(['/themes/_ornaments/daisy.svg']);
  });

  it('draws nothing for Plain', () => {
    const plain = { art_style: 'classic', focus: { style: '' } } as unknown as Appearance;
    expect(ClassicCorner({ corner: 'tl', decor: decorOf(plain, 'classic') })).toBeNull();
  });
});

describe('app tile icons', () => {
  it('every app uses Bear Den\'s own icon: the pixel PNG at a whole-number scale, or the classic SVG', () => {
    expect(APP_ICONS).toEqual(['plex-htpc', 'vacuumtube', 'moonlight', 'spotify', 'jellyfin', 'retroarch']);
    for (const adapter of APP_ICONS) {
      const px = props(AppArt({ adapter, size: 70 }));
      expect(px.src).toBe(`art/pixel/app-${adapter}.png`);
      expect([px.width, px.height]).toEqual([64, 64]);
      const smooth = props(AppArt({ adapter, size: 32, art: 'classic' }));
      expect(smooth.src).toBe(`art/app-${adapter}.svg`);
      expect([smooth.width, smooth.height]).toEqual([32, 32]);
      expect(existsSync(new URL(`../../static/art/pixel/app-${adapter}.png`, import.meta.url))).toBe(true);
      expect(existsSync(new URL(`../../static/art/app-${adapter}.svg`, import.meta.url))).toBe(true);
    }
  });

  it('an adapter without an icon gets the generic glyph, never a guessed file', () => {
    const glyph = AppArt({ adapter: 'kodi', size: 32 }) as VNode<Props>;
    expect(glyph.props.src).toBeUndefined();
    expect(glyph.props.name).toBe('app');
  });

  it('the remote\'s app tiles draw AppArt, not the generic glyph', () => {
    const src = readFileSync(new URL('../../src/views/remote.tsx', import.meta.url), 'utf8');
    const tile = src.slice(src.indexOf('function AppButton'));
    expect(tile).toContain('<AppArt adapter={application.adapter}');
    expect(tile).not.toContain('<Icon name="app"');
  });
});
