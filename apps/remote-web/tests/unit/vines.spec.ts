// Unit tests for the pixel-art tile vines (src/vines.tsx): every leaf cell sits
// on the whole-cell grid, touches the stem it grows from and points the way its
// direction says; the pre-theme default tip is a pixel PNG.
import { describe, expect, it } from 'vitest';
import { decorOf, GRID, LEAVES, leafCells } from '../../src/vines.tsx';

// Stem cells from STEM ('M39 3 H7 V5 H5 V7 H3 V36', 2 cells wide, square caps).
const stem = new Set<string>();
const band = (x0: number, x1: number, y0: number, y1: number) => {
  for (let x = x0; x < x1; x++) for (let y = y0; y < y1; y++) stem.add(`${x},${y}`);
};
band(6, 40, 2, 4);
band(4, 8, 4, 6);
band(2, 6, 6, 8);
band(2, 4, 8, 37);

describe('vines', () => {
  it('defaults to the den vine with a pixel daisy tip', () => {
    expect(decorOf(null).tip).toBe('/themes/_ornaments/daisy.png');
  });

  it('turns sprites by quarter turns and keeps cells on the grid', () => {
    const north = [[0, -1, 'dark', 1], [1, -2, 'light', 1]] as const;
    expect(leafCells(north, 'n').map(([x, y]) => [x, y])).toEqual([[0, -1], [1, -2]]);
    expect(leafCells(north, 's').map(([x, y]) => [x, y])).toEqual([[-1, 0], [-2, 1]]);
    expect(leafCells(north, 'e').map(([x, y]) => [x, y])).toEqual([[0, 0], [1, 1]]);
    expect(leafCells(north, 'w').map(([x, y]) => [x, y])).toEqual([[-1, -1], [-2, -2]]);
  });

  it('grows every leaf off the stem, outside it, on whole cells', () => {
    const base = [[0, -1, 'light', 1]] as const;
    for (const [x, y, dir] of LEAVES) {
      const [[cx, cy]] = leafCells(base, dir).map(([dx, dy]) => [x + dx, y + dy]) as [[number, number]];
      expect(Number.isInteger(cx) && Number.isInteger(cy)).toBe(true);
      expect(cx >= 0 && cx < GRID && cy >= 0 && cy < GRID).toBe(true);
      expect(stem.has(`${cx},${cy}`)).toBe(false);
      const touches = [[1, 0], [-1, 0], [0, 1], [0, -1]].some(([ax, ay]) => stem.has(`${cx + (ax ?? 0)},${cy + (ay ?? 0)}`));
      expect(touches).toBe(true);
    }
  });
});
