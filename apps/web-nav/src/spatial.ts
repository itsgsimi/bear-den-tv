// Spatial navigation geometry for the navigation script (nav.ts): given the
// focused rectangle and the candidates' rectangles, pick the best one in a
// direction, the way a TV remote moves between tiles. Pure functions; tested
// through the Playwright suite (tests/nav.spec.ts).

export interface Box {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export type Direction = 'up' | 'down' | 'left' | 'right';

/** Gap between two 1-D ranges; 0 when they overlap. */
function gap(a0: number, a1: number, b0: number, b1: number): number {
  if (b1 < a0) return a0 - b1;
  if (b0 > a1) return b0 - a1;
  return 0;
}

/**
 * Score of moving from `from` to `to` in `dir`, or null when `to` is not in
 * that direction. Lower is better: the distance along the direction, plus the
 * sideways gap weighted three times (so a tile in the same row beats a closer
 * one in the next row), plus a small centre-offset tie-breaker.
 */
export function score(from: Box, to: Box, dir: Direction): number | null {
  const fcx = (from.left + from.right) / 2;
  const fcy = (from.top + from.bottom) / 2;
  const tcx = (to.left + to.right) / 2;
  const tcy = (to.top + to.bottom) / 2;
  const slack = 1;
  let primary: number;
  let side: number;
  let offset: number;
  switch (dir) {
    case 'right':
      if (tcx <= fcx || to.left < from.left + slack) return null;
      primary = Math.max(0, to.left - from.right);
      side = gap(from.top, from.bottom, to.top, to.bottom);
      offset = Math.abs(tcy - fcy);
      break;
    case 'left':
      if (tcx >= fcx || to.right > from.right - slack) return null;
      primary = Math.max(0, from.left - to.right);
      side = gap(from.top, from.bottom, to.top, to.bottom);
      offset = Math.abs(tcy - fcy);
      break;
    case 'down':
      if (tcy <= fcy || to.top < from.top + slack) return null;
      primary = Math.max(0, to.top - from.bottom);
      side = gap(from.left, from.right, to.left, to.right);
      offset = Math.abs(tcx - fcx);
      break;
    case 'up':
      if (tcy >= fcy || to.bottom > from.bottom - slack) return null;
      primary = Math.max(0, from.top - to.bottom);
      side = gap(from.left, from.right, to.left, to.right);
      offset = Math.abs(tcx - fcx);
      break;
  }
  return primary + side * 3 + offset * 0.05;
}

/** Index of the best candidate in `dir`, or -1 when there is none. */
export function pick(from: Box, candidates: Box[], dir: Direction): number {
  let best = -1;
  let bestScore = Infinity;
  candidates.forEach((c, i) => {
    const s = score(from, c, dir);
    if (s !== null && s < bestScore) {
      best = i;
      bestScore = s;
    }
  });
  return best;
}
