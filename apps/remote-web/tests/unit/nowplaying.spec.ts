// Unit tests for the Now playing card (src/views/nowplaying.tsx): it renders
// the reading, extrapolates the position locally from position_at + rate,
// ticks only while visible and playing, and renders nothing when the
// snapshot carries no now_playing. Also the store facts it reads
// (snapshotAt, hidden) in src/state.ts.
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { NowPlaying, StateSnapshot } from '../../src/contract.ts';
import { targetFor } from '../../src/app.ts';
import { behindHomeApp, initialState, reduce } from '../../src/state.ts';
import { appLabelFor, extrapolatePosition, formatClock, NowPlayingCard, nowPlayingOf, shouldTick } from '../../src/views/nowplaying.tsx';

type Props = Record<string, unknown> & { children?: unknown };

/** Every vnode in a tree, with stateless function components expanded. */
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

function text(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(text).join('');
  if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    if (typeof v.type === 'function') return text((v.type as (p: Props) => unknown)(v.props));
    return text(v.props.children);
  }
  return '';
}

const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);

const DEMO: NowPlaying = {
  app_id: 'plex-htpc',
  title: 'DEMO Episode 3: The Long Winter',
  subtitle: 'DEMO Show',
  status: 'playing',
  length_ms: 2_640_000,
  position_ms: 754_000,
  position_at: 203_500,
  rate: 1,
};

function snapshot(np?: NowPlaying | null): StateSnapshot {
  return {
    protocol: 1,
    context_epoch: 52,
    generated_at_ms: 204_000,
    device_name: 'Bear Den',
    dev_mode: true,
    config_revision: 3,
    session: { locked: false, display_session: 'x11', desktop_adapter: 'fake', shell_connected: true, shell_state: 'running' },
    target: { kind: 'app', app_id: 'plex-htpc', label: 'Plex', observed: true },
    capabilities: {},
    shell: { screen: 'home', focus: { section_id: null, item_id: null } },
    applications: [
      { id: 'plex-htpc', label: 'Plex', adapter: 'plex-htpc', installed: true, version: null, installation: 'user', running: true, foreground: true, launch_state: 'running', last_error: null },
    ],
    remote: {
      enabled: true,
      transport: 'trusted-lan-http',
      listening: true,
      addresses: [],
      https: false,
      http_layout_editing: false,
      paired_device_count: 1,
      now_playing: true,
      hold: { active: false, device_id: null, action: null },
      limits: { repeat_delay_ms: 350, repeat_hz: 6, hold_renew_ms: 200, hold_expiry_ms: 600, max_message_bytes: 16384 },
    },
    notifications: [],
    ...(np === undefined ? {} : { now_playing: np }),
  } as StateSnapshot;
}

describe('Now playing card', () => {
  it('renders the app, title, subtitle, status and a progress bar', () => {
    const tree = walk(NowPlayingCard({ np: DEMO, appLabel: 'Plex', positionMs: 754_000 }));
    const card = byTestId(tree, 'now-playing');
    expect(card?.props['data-status']).toBe('playing');
    expect(card?.props.class).toContain('np-playing');
    expect(text(byTestId(tree, 'now-playing-title'))).toBe(DEMO.title);
    expect(byTestId(tree, 'now-playing-title')?.props.title).toBe(DEMO.title); // full text on long-press when ellipsized
    expect(text(byTestId(tree, 'now-playing-status'))).toBe('Playing');
    const all = text(NowPlayingCard({ np: DEMO, appLabel: 'Plex', positionMs: 754_000 }));
    expect(all).toContain('Plex');
    expect(all).toContain('DEMO Show');
    const bar = byTestId(tree, 'now-playing-bar');
    expect([bar?.type, bar?.props.max, bar?.props.value]).toEqual(['progress', 2_640_000, 754_000]);
    expect(bar?.props['aria-label']).toBe('12:34 of 44:00');
    expect(text(byTestId(tree, 'now-playing-position'))).toBe('12:34');
  });

  it('shows paused, and leaves out what the player does not report', () => {
    const bare: NowPlaying = { app_id: 'youtube', title: 'DEMO', status: 'paused', position_at: 0, rate: 1 };
    const tree = walk(NowPlayingCard({ np: bare, appLabel: 'YouTube', positionMs: null }));
    expect(byTestId(tree, 'now-playing')?.props['data-status']).toBe('paused');
    expect(text(byTestId(tree, 'now-playing-status'))).toBe('Paused');
    expect(tree.some((v) => v.props.class === 'np-subtitle small')).toBe(false);
    expect(byTestId(tree, 'now-playing-bar')).toBeUndefined();
    expect(byTestId(tree, 'now-playing-position')).toBeUndefined();
    // A position without a length: the time, no bar.
    const live = walk(NowPlayingCard({ np: { ...bare, position_ms: 61_000 }, appLabel: 'YouTube', positionMs: 61_000 }));
    expect(byTestId(live, 'now-playing-bar')).toBeUndefined();
    expect(text(byTestId(live, 'now-playing-position'))).toBe('1:01');
  });

  it('renders nothing when the snapshot has no now_playing', () => {
    expect(NowPlayingCard({ np: null, appLabel: 'Plex', positionMs: null })).toBeNull();
    expect(nowPlayingOf(null)).toBeNull();
    expect(nowPlayingOf(snapshot())).toBeNull();
    expect(nowPlayingOf(snapshot(null))).toBeNull();
    expect(nowPlayingOf(snapshot(DEMO))).toBe(DEMO);
  });

  it('labels the reading with its own app', () => {
    expect(appLabelFor(snapshot(DEMO), DEMO)).toBe('Plex');
    expect(appLabelFor(snapshot(DEMO), { ...DEMO, app_id: 'gone' })).toBe('Plex'); // the target label
  });
});

describe('behind Home', () => {
  const BEHIND: NowPlaying = { ...DEMO, app_id: 'youtube', foreground: false, title: 'DEMO Video: Building a Cabin', subtitle: 'DEMO Channel' };

  it('says the app plays behind Home', () => {
    const tree = walk(NowPlayingCard({ np: BEHIND, appLabel: 'YouTube', positionMs: 180_000 }));
    const card = byTestId(tree, 'now-playing');
    expect(card?.props['data-behind']).toBe('true');
    expect(card?.props.class).toContain('np-behind');
    expect(card?.props['aria-label']).toBe('Playing in YouTube · behind Home');
    expect(text(byTestId(tree, 'now-playing-behind'))).toBe('Playing in YouTube · behind Home');
    const paused = walk(NowPlayingCard({ np: { ...BEHIND, status: 'paused' }, appLabel: 'YouTube', positionMs: 180_000 }));
    expect(text(byTestId(paused, 'now-playing-behind'))).toBe('Paused in YouTube · behind Home');
  });

  it('says nothing about Home for the app in front', () => {
    for (const np of [DEMO, { ...DEMO, foreground: true }]) {
      const tree = walk(NowPlayingCard({ np, appLabel: 'Plex', positionMs: 754_000 }));
      expect(byTestId(tree, 'now-playing')?.props['data-behind']).toBe('false');
      expect(byTestId(tree, 'now-playing-behind')).toBeUndefined();
    }
  });

  it('names the app behind Home as the playback target, never "active"', () => {
    const shell = { ...snapshot(BEHIND), target: { kind: 'shell', app_id: null, label: 'Bear Den TV', observed: true } } as StateSnapshot;
    expect(behindHomeApp(shell)).toBe('youtube');
    expect(targetFor('media.pause', shell)).toBe('youtube');
    expect(targetFor('media.play', shell)).toBe('youtube');
    expect(targetFor('media.seek_relative', shell)).toBe('youtube');
    expect(targetFor('nav.left', shell)).toBe('active');
    expect(targetFor('home', shell)).toBe('shell');
    // In front, or nothing known: the active window, as before.
    expect(behindHomeApp(snapshot(DEMO))).toBeNull();
    expect(targetFor('media.pause', snapshot(DEMO))).toBe('active');
    expect(targetFor('media.pause', snapshot({ ...DEMO, foreground: true }))).toBe('active');
    expect(targetFor('media.pause', null)).toBe('active');
  });
});

describe('from the Plex server', () => {
  it('says the reading is read-only and from the Plex server', () => {
    const plex: NowPlaying = { ...DEMO, foreground: false, source: 'plex_server' };
    const tree = walk(NowPlayingCard({ np: plex, appLabel: 'Plex', positionMs: 754_000 }));
    expect(byTestId(tree, 'now-playing')?.props['data-source']).toBe('plex_server');
    expect(text(byTestId(tree, 'now-playing-source'))).toBe('From your Plex server · read-only');
    expect(text(byTestId(tree, 'now-playing-behind'))).toBe('Playing in Plex · behind Home');
    for (const np of [DEMO, { ...DEMO, source: 'mpris' as const }]) {
      expect(byTestId(walk(NowPlayingCard({ np, appLabel: 'Plex', positionMs: 1 })), 'now-playing-source')).toBeUndefined();
    }
  });
});

describe('position extrapolation', () => {
  it('advances from position_at on the coordinator clock plus the time since arrival, at the rate', () => {
    // Read 500 ms before the snapshot was generated; the snapshot arrived 2 s ago.
    expect(extrapolatePosition(DEMO, 204_000, 2_000)).toBe(754_000 + 500 + 2_000);
    expect(extrapolatePosition({ ...DEMO, rate: 2 }, 204_000, 2_000)).toBe(754_000 + 2 * 2_500);
    expect(extrapolatePosition(DEMO, 203_500, 0)).toBe(754_000);
  });

  it('stands still unless playing, stops at the length and never goes negative', () => {
    expect(extrapolatePosition({ ...DEMO, status: 'paused' }, 204_000, 60_000)).toBe(754_000);
    expect(extrapolatePosition({ ...DEMO, status: 'stopped' }, 204_000, 60_000)).toBe(754_000);
    expect(extrapolatePosition(DEMO, 204_000, 10 * 3_600_000)).toBe(2_640_000);
    expect(extrapolatePosition(DEMO, 204_000, -5_000)).toBe(754_500); // a clock step back is ignored
    expect(extrapolatePosition({ ...DEMO, position_ms: undefined }, 204_000, 1_000)).toBeNull();
  });

  it('formats clocks', () => {
    expect([formatClock(0), formatClock(59_999), formatClock(61_000), formatClock(3_600_000), formatClock(3_725_000)]).toEqual(['0:00', '0:59', '1:01', '1:00:00', '1:02:05']);
  });

  it('ticks only while playing, visible and with a position', () => {
    expect(shouldTick(DEMO, false)).toBe(true);
    expect(shouldTick(DEMO, true)).toBe(false);
    expect(shouldTick({ ...DEMO, status: 'paused' }, false)).toBe(false);
    expect(shouldTick({ ...DEMO, position_ms: undefined }, false)).toBe(false);
    expect(shouldTick({ ...DEMO, rate: 0 }, false)).toBe(false);
    expect(shouldTick(null, false)).toBe(false);
  });
});

describe('store facts the card reads', () => {
  it('remembers when each snapshot arrived and whether the page is hidden', () => {
    let s = initialState();
    expect([s.snapshotAt, s.hidden]).toEqual([0, false]);
    s = reduce(s, { type: 'state_received', snapshot: snapshot(DEMO), at: 1234 });
    expect(s.snapshotAt).toBe(1234);
    const hidden = reduce(s, { type: 'visibility_changed', hidden: true });
    expect(hidden.hidden).toBe(true);
    expect(reduce(hidden, { type: 'visibility_changed', hidden: true })).toBe(hidden);
  });
});
