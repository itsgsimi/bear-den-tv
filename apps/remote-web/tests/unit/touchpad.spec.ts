// Touchpad gestures (src/touchpad.ts) and when the touchpad is drawn
// (src/views/touchpad.tsx touchpadShown): drag, tap, two-finger tap and
// scroll become pointer actions within the contract's limits, one request
// in flight at a time; the pad shows only with a web app in front that
// accepts pointer input, and never to a guest.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { Session, StateSnapshot } from '../../src/contract.ts';
import { type AppState, initialState, reduce } from '../../src/state.ts';
import { MAX_MOVE, MIN_GAP_MS, TAP_MS, Touchpad, type PointerSink } from '../../src/touchpad.ts';
import { touchpadShown } from '../../src/views/touchpad.tsx';

interface Sent {
  kind: 'move' | 'scroll' | 'click';
  a: number | string;
  b?: number;
}

function rig() {
  let clock = 0;
  const timers: { at: number; fn: () => void }[] = [];
  const sent: Sent[] = [];
  const pending: (() => void)[] = [];
  const sink: PointerSink = {
    move: (dx, dy) => {
      sent.push({ kind: 'move', a: dx, b: dy });
      return new Promise<void>((resolve) => pending.push(resolve));
    },
    scroll: (dy) => {
      sent.push({ kind: 'scroll', a: dy });
      return new Promise<void>((resolve) => pending.push(resolve));
    },
    click: (button) => {
      sent.push({ kind: 'click', a: button });
    },
  };
  const pad = new Touchpad(sink, { now: () => clock, schedule: (fn, ms) => timers.push({ at: clock + ms, fn }), sensitivity: 1, scrollGain: 1 });
  const advance = async (ms: number) => {
    clock += ms;
    for (;;) {
      const due = timers.filter((x) => x.at <= clock);
      if (due.length === 0) break;
      for (const d of due) timers.splice(timers.indexOf(d), 1);
      due.forEach((d) => d.fn());
      await Promise.resolve();
    }
  };
  const answer = async () => {
    pending.splice(0).forEach((r) => r());
    for (let i = 0; i < 4; i++) await Promise.resolve();
  };
  return { pad, sent, advance, answer };
}

describe('touchpad gestures', () => {
  it('a drag moves the pointer, coalesced, one request in flight', async () => {
    const { pad, sent, advance, answer } = rig();
    pad.down(1, 100, 100);
    pad.move(1, 110, 105);
    pad.move(1, 130, 100);
    await advance(0);
    expect(sent).toEqual([{ kind: 'move', a: 30, b: 0 }]);
    pad.move(1, 140, 90); // while the first is in flight: held back
    await advance(MIN_GAP_MS);
    expect(sent).toHaveLength(1);
    await answer();
    await advance(MIN_GAP_MS);
    expect(sent[1]).toEqual({ kind: 'move', a: 10, b: -10 });
    pad.up(1);
    expect(sent.some((s) => s.kind === 'click')).toBe(false); // a drag is not a tap
  });

  it('a big fling is split into moves within the contract limit', async () => {
    const { pad, sent, advance, answer } = rig();
    pad.down(1, 0, 0);
    pad.move(1, 1000, 0);
    for (let i = 0; i < 4; i++) {
      await advance(MIN_GAP_MS);
      await answer();
    }
    expect(sent.map((s) => s.a)).toEqual([MAX_MOVE, MAX_MOVE, 200]);
  });

  it('a quick tap clicks, a two-finger tap right-clicks, a slow press does not', async () => {
    const { pad, sent, advance } = rig();
    pad.down(1, 50, 50);
    await advance(TAP_MS / 2);
    pad.up(1);
    pad.down(1, 50, 50);
    pad.down(2, 80, 50);
    pad.up(2);
    pad.up(1);
    pad.down(1, 50, 50);
    await advance(TAP_MS + 10);
    pad.up(1);
    expect(sent.filter((s) => s.kind === 'click').map((s) => s.a)).toEqual(['left', 'right']);
  });

  it('two fingers dragging scroll the page (natural direction), not the pointer', async () => {
    const { pad, sent, advance } = rig();
    pad.down(1, 100, 300);
    pad.down(2, 160, 300);
    pad.move(1, 100, 200);
    pad.move(2, 160, 200);
    await advance(0);
    expect(sent).toEqual([{ kind: 'scroll', a: 100 }]);
  });
});

describe('when the touchpad is drawn', () => {
  const fixture = (name: string): StateSnapshot => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as StateSnapshot;
  const online = (snap: StateSnapshot, permissions: Session['permissions']): AppState => {
    const s = reduce(initialState('Phone'), { type: 'session_started', session: { device_id: 'd', device_name: 'Phone', permissions, csrf_token: 'c', transport_secure: false } });
    return reduce(s, { type: 'state_received', snapshot: snap, at: 1 });
  };
  it('with a web app in front for a family phone only', () => {
    const web = fixture('state.phone-web-app.valid.json');
    expect(touchpadShown(online(web, ['controller']))).toBe(true);
    const asGuest = { ...web, me: { ...web.me!, permissions: ['guest' as const], expires_at_ms: 2_000_000_000_000 } };
    expect(touchpadShown(online(asGuest, ['guest']))).toBe(false);
    const shell = fixture('state.phone-controller.valid.json');
    expect(touchpadShown(online(shell, ['controller']))).toBe(false);
    const off = { ...web, capabilities: { ...web.capabilities, 'pointer.move': { available: false, reason: 'The touchpad works only in web apps.' } } };
    expect(touchpadShown(online(off, ['controller']))).toBe(false);
  });
});
