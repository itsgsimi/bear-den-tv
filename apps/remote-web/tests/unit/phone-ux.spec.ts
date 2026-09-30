// The phone's second UX round (UX audit 2026-09-29): a lost TV dims the
// controls and says since when (UX-12); the Devices tab shows the list
// read-only where it cannot manage (UX-13); a greyed control's reason sits
// under its group (UX-14); About keeps ids under Details and Log out asks
// first (UX-15, UX-19); the QR path asks for the phone's name (UX-16);
// something playing comes right under the D-pad (UX-21); a guest who comes
// back after the pass ended is told so (UX-29); Play/Pause and Mute are
// single buttons (UX-33).
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { ApiEnvironment, SocketLike } from '../../src/api.ts';
import { createApp, type PageEnvironment, type WindowEnvironment } from '../../src/app.ts';
import type { Device, Session, StateSnapshot } from '../../src/contract.ts';
import { t } from '../../src/i18n.ts';
import { type AppState, initialState, isMuted, mutedByThisPhone, reduce, tvLost } from '../../src/state.ts';
import { LogoutControl } from '../../src/views/about.tsx';
import { DeviceRows } from '../../src/views/devices.tsx';
import { groupReason, playButtons, playbackFirst } from '../../src/views/remote.tsx';
import { LostBanner } from '../../src/views/shell.tsx';

type Props = Record<string, unknown> & { children?: unknown };
function walk(node: unknown, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) for (const c of node) walk(c, out);
  else if (node && typeof node === 'object' && 'props' in node) {
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

const fixture = (name: string): StateSnapshot => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as StateSnapshot;
const online = (snap: StateSnapshot, permissions: Session['permissions']): AppState => {
  const s = reduce(initialState('Phone'), { type: 'session_started', session: { device_id: 'd', device_name: 'Phone', permissions, csrf_token: 'c', transport_secure: false } });
  return reduce(s, { type: 'state_received', snapshot: snap, at: 1 });
};

describe('a lost TV (UX-12)', () => {
  it('dims the controls and says since when, until it is back', () => {
    let s = online(fixture('state.phone-controller.valid.json'), ['controller']);
    s = reduce(s, { type: 'connection_changed', connection: 'online', at: 10 });
    expect(tvLost(s)).toBe(false);
    s = reduce(s, { type: 'connection_changed', connection: 'reconnecting', at: Date.UTC(2026, 8, 29, 20, 42) });
    expect(tvLost(s)).toBe(true);
    const lostAt = s.lostAt;
    s = reduce(s, { type: 'connection_changed', connection: 'offline', at: lostAt! + 60_000 });
    expect(s.lostAt).toBe(lostAt); // the first moment, not the latest
    const banner = LostBanner({ state: s });
    expect(text(banner)).toMatch(/^Lost the TV at .+ Trying again…$/);
    s = reduce(s, { type: 'connection_changed', connection: 'online', at: lostAt! + 90_000 });
    expect(tvLost(s)).toBe(false);
    expect(s.lostAt).toBeNull();
    expect(LostBanner({ state: s })).toBeNull();
  });
});

describe('the Remote tab', () => {
  it('puts a greyed group’s reason under it (UX-14)', () => {
    const s = online(fixture('state.phone-controller.valid.json'), ['controller']);
    const reason = groupReason(s, ['text.submit']);
    expect(reason).toBe(s.snapshot!.capabilities['text.submit']!.reason);
    expect(groupReason(s, ['nav.up', 'select'])).toBeNull();
  });

  it('shows one Play/Pause that follows what is playing, and playback first (UX-21, UX-33)', () => {
    const playing = online(fixture('state.phone-now-playing.valid.json'), ['controller']);
    expect(playbackFirst(playing)).toBe(true);
    const status = playing.snapshot!.now_playing!.status;
    expect(playButtons(playing)).toEqual([status === 'playing' ? 'pause' : 'play']);
    const paused = online({ ...playing.snapshot!, now_playing: { ...playing.snapshot!.now_playing!, status: status === 'playing' ? 'paused' : 'playing' } }, ['controller']);
    expect(playButtons(paused)).toEqual([status === 'playing' ? 'play' : 'pause']);
    const idle = online(fixture('state.phone-controller.valid.json'), ['controller']);
    expect(playbackFirst(idle)).toBe(false);
  });

  it('Mute is one toggle, from what this phone last sent (UX-33)', () => {
    let s = online(fixture('state.phone-controller.valid.json'), ['controller']);
    expect(mutedByThisPhone(s)).toBe(false);
    const req = { protocol: 1 as const, request_id: '00000000-0000-4000-8000-000000000001', context_epoch: 1, target: 'active' as const, action: 'audio.mute' as const, args: { muted: true } };
    s = reduce(s, { type: 'action_sent', request: req, at: 5 });
    expect(mutedByThisPhone(s)).toBe(false); // not accepted yet
    s = reduce(s, { type: 'action_result', result: { protocol: 1, request_id: req.request_id, outcome: 'delivered', code: 'ok', message: '', context_epoch: 1, target: { kind: 'shell' } } as never, at: 6 });
    expect(mutedByThisPhone(s)).toBe(true);
  });
});

describe('Devices and About', () => {
  const devices: Device[] = [{ id: 'd1', name: 'DEMO phone', permissions: ['controller'], connected: true } as Device];
  it('lists phones read-only without Remove where the phone cannot manage them (UX-13)', () => {
    const props = { devices, self: null, busy: false, confirming: null, onAsk: () => undefined, onRemove: () => undefined };
    expect(byTestId(walk(DeviceRows({ ...props, manageable: false })), 'revoke-device')).toBeUndefined();
    expect(byTestId(walk(DeviceRows({ ...props, manageable: false })), 'device-row')).toBeDefined();
    expect(byTestId(walk(DeviceRows({ ...props, manageable: true })), 'revoke-device')).toBeDefined();
  });

  it('Log out asks first, Cancel first (UX-19)', () => {
    const asked: boolean[] = [];
    let out = 0;
    let tree = walk(LogoutControl({ asking: false, onAsk: (a) => asked.push(a), onLogout: () => out++ }));
    (byTestId(tree, 'logout')!.props.onClick as () => void)();
    expect(asked).toEqual([true]);
    expect(out).toBe(0);
    tree = walk(LogoutControl({ asking: true, onAsk: (a) => asked.push(a), onLogout: () => out++ }));
    expect(text(tree[0])).toContain(t.about.logoutAsk);
    (byTestId(tree, 'logout-cancel')!.props.onClick as () => void)();
    expect(asked).toEqual([true, false]);
    (byTestId(tree, 'logout-yes')!.props.onClick as () => void)();
    expect(out).toBe(1);
  });
});

function boot(hash: string, stored: Record<string, string>, sessionStatus: number) {
  const requests: { path: string; body: string }[] = [];
  const socket: SocketLike = { readyState: 0, send: () => undefined, close: () => undefined, onopen: null, onclose: null, onerror: null, onmessage: null };
  const env: ApiEnvironment = {
    fetch: async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), 'http://192.0.2.10:8090').pathname;
      requests.push({ path, body: String(init?.body ?? '') });
      if (path === '/api/v1/info') return new Response(JSON.stringify({ protocol: 1, device_name: 'TV', transport: 'trusted-lan-http', https: false, pairing_required: true }), { status: 200 });
      if (path === '/api/v1/session') return new Response(JSON.stringify({ error: 'unauthenticated' }), { status: sessionStatus });
      return new Response(JSON.stringify({ error: 'invalid_invitation' }), { status: 401 });
    },
    createSocket: () => socket,
    setTimeout: () => 0,
    clearTimeout: () => undefined,
    setInterval: () => 0,
    clearInterval: () => undefined,
    random: () => 0.5,
    origin: 'http://192.0.2.10:8090',
  };
  const page: PageEnvironment = { hidden: false, addEventListener: () => undefined, removeEventListener: () => undefined };
  const storage = new Map(Object.entries(stored));
  const win: WindowEnvironment = {
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    location: { hash, pathname: '/', search: '' },
    history: { replaceState: () => undefined },
    navigator: { userAgent: 'test' },
    localStorage: { getItem: (k: string) => storage.get(k) ?? null, setItem: (k: string, v: string) => void storage.set(k, v) },
  };
  return { app: createApp(env, page, win, { now: () => 1000 }), requests, storage };
}

describe('pairing again', () => {
  it('the QR path asks for the name before claiming (UX-16)', async () => {
    const { app, requests } = boot('#pair=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', {}, 401);
    await app.start();
    expect(app.store.getState().pair.invite).toBe(true);
    expect(requests.some((r) => r.path === '/api/v1/pair/claim')).toBe(false);
    app.setDeviceName('Kitchen tablet');
    await app.pairWithInvitation();
    const claim = requests.find((r) => r.path === '/api/v1/pair/claim');
    expect(claim).toBeDefined();
    expect(JSON.parse(claim!.body)).toMatchObject({ invitation: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', device_name: 'Kitchen tablet' });
  });

  it('a guest back after the pass ended is told so (UX-29)', async () => {
    const { app, storage } = boot('', { 'bdtv.guest': '1' }, 401);
    await app.start();
    expect(app.store.getState().screen).toBe('pair');
    expect(app.store.getState().pair.notice).toBe('pass_ended');
    expect(storage.get('bdtv.guest')).toBe('');
    const plain = boot('', {}, 401);
    await plain.app.start();
    expect(plain.app.store.getState().pair.notice).toBeNull();
  });
});

describe('the TV’s real mute state (state.audio)', () => {
  it('wins over what this phone last sent', () => {
    const audio = fixture('state.phone-audio.valid.json');
    let s = online(audio, ['controller']);
    expect(isMuted(s)).toBe(true);
    const req = { protocol: 1 as const, request_id: '00000000-0000-4000-8000-000000000002', context_epoch: 1, target: 'active' as const, action: 'audio.mute' as const, args: { muted: false } };
    s = reduce(s, { type: 'action_sent', request: req, at: 5 });
    s = reduce(s, { type: 'action_result', result: { protocol: 1, request_id: req.request_id, outcome: 'delivered', code: 'ok', message: '', context_epoch: 1, target: { kind: 'shell' } } as never, at: 6 });
    expect(isMuted(s)).toBe(true); // the TV still says muted until its next state
    s = reduce(s, { type: 'state_received', snapshot: { ...audio, audio: { muted: false, volume_percent: 65 } }, at: 7 });
    expect(isMuted(s)).toBe(false);
    // Without state.audio, the phone's own last press.
    const plain = { ...audio };
    delete plain.audio;
    s = reduce(s, { type: 'state_received', snapshot: plain, at: 8 });
    expect(isMuted(s)).toBe(mutedByThisPhone(s));
  });
});
