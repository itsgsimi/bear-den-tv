// Unit tests for the TV section (src/views/tv.tsx): the TV power buttons are
// drawn only while tv.power is available and send tv.power on/standby; the
// status line follows state.cec.tv_power; the volume heading says TV volume
// only while HDMI-CEC is on with the TV chosen; tv.power goes to the shell
// target.
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import { createApp, type PageEnvironment, type WindowEnvironment } from '../../src/app.ts';
import type { ApiEnvironment, SocketLike } from '../../src/api.ts';
import type { ActionResult, Cec, StateSnapshot } from '../../src/contract.ts';
import { cecOf, TvSection, type TvSectionProps, volumeDrivesTV, volumeHeading } from '../../src/views/tv.tsx';

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

const ON: Cec = { available: true, enabled: true, volume_target: 'pc', tv_power: 'on' };

function render(overrides: Partial<TvSectionProps> = {}) {
  const calls: string[] = [];
  const props: TvSectionProps = { available: true, cec: ON, onPower: (p) => calls.push(p), ...overrides };
  const vnode = TvSection(props);
  return { vnode, tree: walk(vnode), calls, text: text(vnode) };
}

const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);
const snap = (cec?: Cec) => ({ cec }) as unknown as StateSnapshot;

describe('TV section', () => {
  it('is not drawn unless tv.power is available', () => {
    expect(render({ available: false }).vnode).toBeNull();
    expect(render({ available: false, cec: { available: false, reason: 'No HDMI-CEC device', enabled: false, volume_target: 'pc', tv_power: 'unknown' } }).vnode).toBeNull();
  });

  it('sends tv.power on and standby', () => {
    const { tree, calls } = render();
    (byTestId(tree, 'tv-on')?.props.onClick as () => void)();
    (byTestId(tree, 'tv-standby')?.props.onClick as () => void)();
    expect(calls).toEqual(['on', 'standby']);
  });

  it('says what the TV last reported', () => {
    expect(render().text).toContain('The TV is on.');
    expect(render({ cec: { ...ON, tv_power: 'standby' } }).text).toContain('standby.');
    expect(render({ cec: { ...ON, tv_power: 'unknown' } }).text).toContain('did not say');
  });
});

describe('volume heading', () => {
  it('says TV volume only while HDMI-CEC is on with the TV chosen', () => {
    expect(volumeHeading(null)).toBe('PC volume');
    expect(volumeHeading(snap())).toBe('PC volume');
    expect(volumeHeading(snap(ON))).toBe('PC volume');
    expect(volumeHeading(snap({ ...ON, volume_target: 'tv' }))).toBe('TV volume');
    expect(volumeDrivesTV(snap({ ...ON, volume_target: 'tv', enabled: false }))).toBe(false);
    expect(cecOf(snap())).toBeNull();
  });
});

describe('tv.power request', () => {
  it('targets the shell', async () => {
    const sent: Record<string, unknown>[] = [];
    const fetch = async (path: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
      if (path === '/api/v1/actions') sent.push(body);
      const result: ActionResult = {
        protocol: 1, request_id: String(body.request_id), outcome: 'observed', code: 'ok', message: '',
        context_epoch: 1, target: { kind: 'shell', app_id: null, label: 'Bear Den TV' }, detail: { tv_power: 'on' },
      };
      return new Response(JSON.stringify(result), { status: 200 });
    };
    const socket: SocketLike = { readyState: 0, send: () => undefined, close: () => undefined, onopen: null, onclose: null, onerror: null, onmessage: null };
    const env: ApiEnvironment = {
      fetch,
      createSocket: () => socket,
      setTimeout: () => 0,
      clearTimeout: () => undefined,
      setInterval: () => 0,
      clearInterval: () => undefined,
      random: () => 0.5,
      origin: 'http://192.0.2.10:8090',
    };
    const page: PageEnvironment = { hidden: false, addEventListener: () => undefined, removeEventListener: () => undefined };
    const win: WindowEnvironment = {
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      location: { hash: '', pathname: '/', search: '' },
      history: { replaceState: () => undefined },
      navigator: { userAgent: 'test' },
      localStorage: null,
    };
    const app = createApp(env, page, win, { now: () => 1000 });
    await app.tap('tv.power', { power: 'standby' });
    expect(sent.map((r) => [r.action, r.target, r.args])).toEqual([['tv.power', 'shell', { power: 'standby' }]]);
  });
});
