// HoldController lease tests (contracts/actions.md#holds) with vitest fake timers.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { HoldMessage } from '../../src/contract.ts';
import { HoldController, bindHoldLifecycle, type HoldEndReason, type HoldListener } from '../../src/hold.ts';

const RENEW_MS = 200;

function setup(open = true) {
  const sent: HoldMessage[] = [];
  const transport = { open, send: (m: HoldMessage) => (transport.open ? (sent.push(m), true) : false) };
  const ends: Array<{ id: string; reason: HoldEndReason }> = [];
  const listener: HoldListener = {
    onStart: vi.fn(),
    onEnd: (id, reason) => ends.push({ id, reason }),
    onUnavailable: vi.fn(),
  };
  let n = 0;
  const controller = new HoldController(transport, { renewMs: RENEW_MS, newId: () => `hold-${++n}` }, listener);
  return { sent, transport, ends, listener, controller };
}

describe('HoldController', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('sends start, renews every renewMs, and stops on release', () => {
    const { sent, controller, ends } = setup();
    expect(controller.begin('nav.right', 47)).toBe('hold-1');
    expect(sent).toEqual([{ type: 'hold.start', hold_id: 'hold-1', action: 'nav.right', context_epoch: 47 }]);
    vi.advanceTimersByTime(RENEW_MS * 3);
    expect(sent.filter((m) => m.type === 'hold.renew')).toHaveLength(3);
    controller.release();
    expect(sent.at(-1)).toEqual({ type: 'hold.stop', hold_id: 'hold-1' });
    expect(ends).toEqual([{ id: 'hold-1', reason: 'released' }]);
    vi.advanceTimersByTime(RENEW_MS * 5);
    expect(sent.filter((m) => m.type === 'hold.renew')).toHaveLength(3);
    expect(controller.active).toBe(false);
  });

  it('reports unavailable and starts nothing when the socket is closed', () => {
    const { controller, listener, sent } = setup(false);
    expect(controller.begin('nav.up', 1)).toBeNull();
    expect(listener.onUnavailable).toHaveBeenCalledWith('nav.up');
    expect(sent).toEqual([]);
    vi.advanceTimersByTime(RENEW_MS * 2);
    expect(controller.active).toBe(false);
  });

  it('ends as disconnected without hold.stop when a renew cannot be sent', () => {
    const { controller, transport, sent, ends } = setup();
    controller.begin('nav.down', 1);
    transport.open = false;
    vi.advanceTimersByTime(RENEW_MS);
    expect(ends).toEqual([{ id: 'hold-1', reason: 'disconnected' }]);
    expect(sent.some((m) => m.type === 'hold.stop')).toBe(false);
    transport.open = true;
    vi.advanceTimersByTime(RENEW_MS * 3);
    expect(sent).toHaveLength(1);
  });

  it('cancel(disconnected) sends nothing; cancel(hidden) sends hold.stop', () => {
    const a = setup();
    a.controller.begin('nav.left', 1);
    a.controller.cancel('disconnected');
    expect(a.sent.map((m) => m.type)).toEqual(['hold.start']);
    const b = setup();
    b.controller.begin('nav.left', 1);
    b.controller.cancel('hidden');
    expect(b.sent.map((m) => m.type)).toEqual(['hold.start', 'hold.stop']);
    expect(b.ends).toEqual([{ id: 'hold-1', reason: 'hidden' }]);
  });

  it('replaces an existing hold when a new one begins', () => {
    const { controller, sent, ends } = setup();
    controller.begin('nav.left', 1);
    controller.begin('nav.right', 1);
    expect(sent.map((m) => m.type)).toEqual(['hold.start', 'hold.stop', 'hold.start']);
    expect(ends).toEqual([{ id: 'hold-1', reason: 'replaced' }]);
    expect(controller.holdId).toBe('hold-2');
  });

  it('stops renewing on a server end event for the current hold only', () => {
    const { controller, sent, ends } = setup();
    controller.begin('nav.up', 1);
    controller.serverEvent({ type: 'hold', hold_id: 'other', state: 'expired' });
    controller.serverEvent({ type: 'hold', hold_id: 'hold-1', state: 'active' });
    expect(controller.active).toBe(true);
    controller.serverEvent({ type: 'hold', hold_id: 'hold-1', state: 'busy', reason: 'Another phone' });
    expect(controller.active).toBe(false);
    expect(ends).toEqual([{ id: 'hold-1', reason: 'server' }]);
    const before = sent.length;
    vi.advanceTimersByTime(RENEW_MS * 3);
    expect(sent.length).toBe(before);
    expect(sent.some((m) => m.type === 'hold.stop')).toBe(false);
  });

  it('bindHoldLifecycle cancels on visibilitychange to hidden and on blur, and unbinds', () => {
    const { controller, ends } = setup();
    const docListeners = new Set<() => void>();
    const winListeners = new Set<() => void>();
    const doc = {
      hidden: false,
      addEventListener: (_: 'visibilitychange', l: () => void) => docListeners.add(l),
      removeEventListener: (_: 'visibilitychange', l: () => void) => docListeners.delete(l),
    };
    const win = {
      addEventListener: (_: 'blur', l: () => void) => winListeners.add(l),
      removeEventListener: (_: 'blur', l: () => void) => winListeners.delete(l),
    };
    const unbind = bindHoldLifecycle(controller, doc, win);

    controller.begin('nav.up', 1);
    docListeners.forEach((l) => l());
    expect(controller.active).toBe(true);
    doc.hidden = true;
    docListeners.forEach((l) => l());
    expect(ends.at(-1)).toEqual({ id: 'hold-1', reason: 'hidden' });

    controller.begin('nav.up', 1);
    winListeners.forEach((l) => l());
    expect(ends.at(-1)).toEqual({ id: 'hold-2', reason: 'blur' });

    unbind();
    expect(docListeners.size).toBe(0);
    expect(winListeners.size).toBe(0);
  });
});
