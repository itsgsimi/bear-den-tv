// Den badges on the phone (src/views/badges.tsx, visibleTabs in src/state.ts,
// badgeDay in src/i18n.ts): the tab exists only when the coordinator sent
// state.achievements (a controller phone; never a guest pass), the shelf keeps
// the TV's order with the phone's copy, earned days read "2 Sep 2026",
// progress never passes the goal, the medals follow the art style, and an id
// this phone does not know still draws. Uses the contract fixtures.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { Achievements, Session, StateSnapshot } from '../../src/contract.ts';
import { badgeDay, t } from '../../src/i18n.ts';
import { type AppState, initialState, reduce, visibleTabs } from '../../src/state.ts';
import { BadgesView, shelf } from '../../src/views/badges.tsx';

const fixture = <T>(name: string): T => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as T;
const guestSnap = fixture<StateSnapshot>('state.phone-guest.valid.json');
const familySnap = fixture<StateSnapshot>('state.phone-controller.valid.json');

type Props = Record<string, unknown> & { children?: unknown };
function walk(node: unknown, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) {
    for (const c of node) walk(c, out);
  } else if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    out.push(v);
    if (typeof v.type === 'function') walk((v.type as (p: Props) => unknown)(v.props), out);
    else walk(v.props.children, out);
  }
  return out;
}

function online(snap: StateSnapshot): AppState {
  const me = snap.me!;
  const session: Session = { device_id: me.device_id, device_name: me.device_name, permissions: me.permissions, csrf_token: 'c', transport_secure: false };
  const s = reduce(initialState('Phone'), { type: 'session_started', session });
  return reduce(s, { type: 'state_received', snapshot: snap, at: 1 });
}

const styled = (snap: StateSnapshot, art: 'pixel' | 'classic'): StateSnapshot => ({ ...snap, appearance: { ...snap.appearance!, art_style: art } });
const srcs = (tree: VNode<Props>[]) => tree.filter((v) => v.type === 'img').map((v) => String(v.props.src));

describe('Den badges on the phone', () => {
  it('shows the tab only when the TV sent badges', () => {
    expect(visibleTabs(online(familySnap))).toContain('badges');
    expect(visibleTabs(online(guestSnap))).not.toContain('badges');
    const locked = { ...familySnap, achievements: undefined };
    expect(visibleTabs(online(locked))).not.toContain('badges');
  });

  it('keeps the TV order, the copy, the day and the capped progress', () => {
    const a = familySnap.achievements!;
    const items = shelf(a);
    expect(items.map((b) => b.id)).toEqual(a.progress.map((p) => p.id));
    const first = items[0]!;
    expect(first).toMatchObject({ id: 'first-night-in', name: 'First Night In', earned: true, day: '2 Sep 2026' });
    const movie = items.find((b) => b.id === 'movie-night')!;
    expect(movie).toMatchObject({ earned: false, count: 4, goal: 10, day: '' });
    expect(movie.hint).toBe(t.badges.names['movie-night']![1]);
    const over: Achievements = { enabled: true, earned: [], progress: [{ id: 'loyal-den', count: 99, goal: 30 }] };
    expect(shelf(over)[0]!.count).toBe(30);
  });

  it('has copy for every badge the TV knows', () => {
    for (const p of familySnap.achievements!.progress) expect(t.badges.names[p.id], p.id).toBeDefined();
  });

  it('draws medals in the art style, silhouettes until earned', () => {
    const pixel = srcs(walk(BadgesView({ state: online(styled(familySnap, 'pixel')) })));
    expect(pixel).toContain('art/pixel/badge-first-night-in.png');
    expect(pixel).toContain('art/pixel/badge-movie-night-locked.png');
    const classic = srcs(walk(BadgesView({ state: online(styled(familySnap, 'classic')) })));
    expect(classic).toContain('art/badge-first-night-in.svg');
    expect(classic).toContain('art/badge-movie-night-locked.svg');
  });

  it('draws an unknown badge plainly and says when badges are hidden', () => {
    const newer = { ...styled(familySnap, 'pixel'), achievements: { enabled: true, earned: [{ id: 'future-badge', day: '2027-01-01' }], progress: [{ id: 'future-badge', count: 1, goal: 1 }] } };
    const tree = walk(BadgesView({ state: online(newer) }));
    expect(srcs(tree)).toEqual(['art/pixel/badge-first-night-in-locked.png']);
    expect(shelf(newer.achievements)[0]!.name).toBe('future-badge');
    const hidden = walk(BadgesView({ state: online(guestSnap) }));
    expect(hidden.some((v) => v.props.children === t.badges.hidden)).toBe(true);
  });

  it('formats days and leaves anything else as sent', () => {
    expect(badgeDay('2026-12-31')).toBe('31 Dec 2026');
    expect(badgeDay('soon')).toBe('soon');
    expect(badgeDay('2026-13-01')).toBe('2026-13-01');
  });
});
