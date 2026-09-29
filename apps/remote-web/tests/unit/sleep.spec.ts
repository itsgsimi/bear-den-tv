// Unit tests for the Sleep section (src/views/sleep.tsx) and its controller
// wiring (src/app.ts): the time left is counted from sleep_at_ms on the
// coordinator's clock, the countdown ticks only while a timer runs and the
// page is visible, chips and Screen off are gated by their capabilities,
// the chosen chip is marked, Cancel appears only with a timer, the warning
// and screen-off states say so, and the power actions are sent to the shell
// target with a display_off result shown as a gentle toast.
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import { createApp, type PageEnvironment, type WindowEnvironment } from '../../src/app.ts';
import type { ApiEnvironment, SocketLike } from '../../src/api.ts';
import type { ActionResult, Power } from '../../src/contract.ts';
import { powerOf, remainingMs, shouldTickSleep, SLEEP_CHOICES, SleepSection, type SleepSectionProps } from '../../src/views/sleep.tsx';

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

const open = { listed: true, disabled: false, reason: null };
const TIMER: Power = { sleep_at_ms: 2_685_000, sleep_minutes: 45, warning: false, display: 'on' };

function render(overrides: Partial<SleepSectionProps> = {}) {
  const calls: (number | 'off')[] = [];
  const props: SleepSectionProps = {
    power: { sleep_at_ms: null, warning: false, display: 'on' },
    remaining: null,
    sleepGate: open,
    offGate: open,
    classic: false,
    onSleep: (m) => calls.push(m),
    onScreenOff: () => calls.push('off'),
    ...overrides,
  };
  const vnode = SleepSection(props);
  const tree = walk(vnode);
  return { vnode, tree, calls, text: text(vnode) };
}

const chips = (tree: VNode<Props>[]) => tree.filter((v) => v.type === 'button' && v.props['data-minutes'] !== undefined && v.props['data-minutes'] !== 0);
const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);

describe('sleep countdown', () => {
  it('counts down from sleep_at_ms on the coordinator clock', () => {
    // 45 s left when the snapshot was made, 5 s ago on the phone.
    expect(remainingMs(TIMER, 2_640_000, 5_000)).toBe(40_000);
    expect(remainingMs(TIMER, 2_640_000, -3_000)).toBe(45_000); // a clock step back never adds time
    expect(remainingMs(TIMER, 2_640_000, 60_000)).toBe(0);
    expect(remainingMs({ ...TIMER, sleep_at_ms: null }, 2_640_000, 0)).toBeNull();
    expect(remainingMs(null, 2_640_000, 0)).toBeNull();
  });

  it('ticks only while a timer runs and the page is visible', () => {
    expect(shouldTickSleep(TIMER, false)).toBe(true);
    expect(shouldTickSleep(TIMER, true)).toBe(false);
    expect(shouldTickSleep({ ...TIMER, sleep_at_ms: null }, false)).toBe(false);
    expect(shouldTickSleep(null, false)).toBe(false);
  });

  it('reads state.power and tolerates its absence', () => {
    expect(powerOf(null)).toBeNull();
    expect(powerOf({ power: TIMER } as never)).toBe(TIMER);
    expect(powerOf({} as never)).toBeNull();
  });
});

describe('SleepSection', () => {
  it('renders nothing when neither action is listed', () => {
    const closed = { listed: false, disabled: true, reason: null };
    expect(render({ sleepGate: closed, offGate: closed }).vnode).toBeNull();
  });

  it('offers the six choices and Screen off, no Cancel without a timer', () => {
    const { tree, calls, text: shown } = render();
    expect(chips(tree).map((c) => c.props['data-minutes'])).toEqual([...SLEEP_CHOICES]);
    expect(chips(tree).map((c) => text(c))).toEqual(['15 min', '30 min', '45 min', '1 h', '1 h 30', '2 h']);
    expect(byTestId(tree, 'sleep-cancel')).toBeUndefined();
    expect(shown).toContain('No sleep timer');
    (chips(tree)[3]!.props.onClick as () => void)();
    (byTestId(tree, 'screen-off')!.props.onClick as () => void)();
    expect(calls).toEqual([60, 'off']);
  });

  it('marks the running choice, counts down and cancels with 0', () => {
    const { tree, calls, text: shown } = render({ power: TIMER, remaining: 42 * 60_000 + 5_000 });
    const chosen = chips(tree).filter((c) => c.props['aria-pressed'] === true);
    expect(chosen.map((c) => c.props['data-minutes'])).toEqual([45]);
    expect(shown).toContain('Going to sleep in 42:05');
    expect(tree.some((v) => typeof v.type === 'function' && v.props.name === 'bear-sleep')).toBe(true);
    (byTestId(tree, 'sleep-cancel')!.props.onClick as () => void)();
    expect(calls).toEqual([0]);
  });

  it('says press anything during the warning, and that the screen is off', () => {
    expect(render({ power: { ...TIMER, warning: true }, remaining: 38_000 }).text).toContain('Going to sleep in 0:38. Press anything to stay awake.');
    const off = render({ power: { sleep_at_ms: null, warning: false, display: 'off' } });
    expect(off.text).toContain('The screen is off');
    expect(byTestId(off.tree, 'screen-off')!.props.disabled).toBe(true);
  });

  it('disables with the server reason when a capability is unavailable', () => {
    const { tree } = render({
      sleepGate: { listed: true, disabled: true, reason: 'The TV session is locked. Unlock it on the TV.' },
      offGate: { listed: true, disabled: true, reason: 'The screen cannot be turned off here: the X server has no DPMS extension' },
    });
    for (const chip of chips(tree)) {
      expect(chip.props.disabled).toBe(true);
      expect(chip.props.title).toBe('The TV session is locked. Unlock it on the TV.');
    }
    expect(byTestId(tree, 'screen-off')!.props.title).toContain('no DPMS extension');
    expect(byTestId(tree, 'screen-off')!.props.disabled).toBe(true);
  });

  it('shows only the listed control', () => {
    const { tree } = render({ offGate: { listed: false, disabled: true, reason: null } });
    expect(byTestId(tree, 'screen-off')).toBeUndefined();
    expect(chips(tree).length).toBe(6);
  });
});

describe('controller', () => {
  function harness(result: (body: Record<string, unknown>) => ActionResult) {
    const sent: Record<string, unknown>[] = [];
    const fetch = async (path: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
      if (path === '/api/v1/actions') sent.push(body);
      return new Response(JSON.stringify(result(body)), { status: 200 });
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
    return { app: createApp(env, page, win, { now: () => 1000 }), sent };
  }
  const answer =
    (outcome: ActionResult['outcome'], code: ActionResult['code'], message = '') =>
    (body: Record<string, unknown>): ActionResult => ({
      protocol: 1,
      request_id: String(body.request_id),
      outcome,
      code,
      message,
      context_epoch: 1,
      target: { kind: 'shell', app_id: null, label: 'Bear Den TV' },
      detail: {},
    });

  it('sends the power actions to the shell target', async () => {
    const { app, sent } = harness(answer('observed', 'ok'));
    await app.tap('power.sleep_timer', { minutes: 30 });
    await app.tap('display.off', {});
    expect(sent.map((r) => [r.action, r.target, r.args])).toEqual([
      ['power.sleep_timer', 'shell', { minutes: 30 }],
      ['display.off', 'shell', {}],
    ]);
  });

  it('shows a press that only woke the screen as information, not an error', async () => {
    const { app } = harness(answer('failed', 'display_off', 'The screen was off. This press turned it on; press again.'));
    await app.tap('select', {});
    const toast = app.store.getState().toast;
    expect(toast?.kind).toBe('info');
    expect(toast?.text).toBe('The screen was off. This press turned it on; press again.');
  });
});
