// Guest pass UI gating (contracts/http.md#guest-passes): the selectors in
// src/state.ts (isGuest, mayUse, guestEndsAt, visibleTabs), the "pass ended"
// session end, the chip in src/views/shell.tsx and passEndLabel in i18n.ts.
// Uses the contract fixture state.phone-guest.valid.json as the guest's view.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import { type ActionName, GUEST_ACTIONS, type Session, type StateSnapshot } from '../../src/contract.ts';
import { passEndLabel } from '../../src/i18n.ts';
import { type AppState, guestEndsAt, initialState, isGuest, mayUse, reduce, visibleTabs } from '../../src/state.ts';
import { GuestChip } from '../../src/views/shell.tsx';

const fixture = <T>(name: string): T => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as T;
const guestSnap = fixture<StateSnapshot>('state.phone-guest.valid.json');
const familySnap = fixture<StateSnapshot>('state.phone-controller.valid.json');

// Every action in the protocol (contracts/action.schema.json actionName).
const ALL_ACTIONS: ActionName[] = [
  'nav.up', 'nav.down', 'nav.left', 'nav.right', 'select', 'back', 'home', 'app.launch', 'app.close',
  'media.play', 'media.pause', 'media.seek_relative', 'audio.volume_delta', 'audio.mute', 'text.submit', 'shell.restart', 'power.sleep_timer', 'display.off',
];

function sessionFor(snap: StateSnapshot): Session {
  const me = snap.me!;
  return { device_id: me.device_id, device_name: me.device_name, permissions: me.permissions, csrf_token: 'csrf', transport_secure: false };
}

function online(snap: StateSnapshot, at = 1_790_620_000_000): AppState {
  const s = reduce(initialState('Phone'), { type: 'session_started', session: sessionFor(snap) });
  return reduce(s, { type: 'state_received', snapshot: snap, at });
}

describe('guest pass gating', () => {
  it('hides exactly what a guest may not send', () => {
    const guest = online(guestSnap);
    expect(isGuest(guest)).toBe(true);
    for (const action of ALL_ACTIONS) expect(mayUse(guest, action), action).toBe(GUEST_ACTIONS.has(action));
    expect(mayUse(guest, 'app.close')).toBe(false);
    expect(mayUse(guest, 'shell.restart')).toBe(false);
    expect(mayUse(guest, 'power.sleep_timer')).toBe(false);
    expect(mayUse(guest, 'display.off')).toBe(false);
    expect(mayUse(guest, 'nav.up')).toBe(true);
    expect(mayUse(guest, 'media.pause')).toBe(true);
  });

  it('leaves a family phone to its capabilities', () => {
    const family = online(familySnap);
    expect(isGuest(family)).toBe(false);
    for (const action of ALL_ACTIONS) expect(mayUse(family, action), action).toBe(true);
    expect(guestEndsAt(family)).toBeNull();
  });

  it('gives a guest only the remote and about tabs', () => {
    expect(visibleTabs(online(guestSnap))).toEqual(['remote', 'about']);
  });

  it('reads the pass end from me.expires_at_ms', () => {
    expect(guestEndsAt(online(guestSnap))).toBe(guestSnap.me!.expires_at_ms);
  });

  it('shows "Your guest pass has ended" when a guest is revoked or loses its session', () => {
    for (const reason of ['revoked', 'unauthenticated'] as const) {
      const ended = reduce(online(guestSnap), { type: 'session_ended', reason });
      expect(ended.screen).toBe('pair');
      expect(ended.pair.notice, reason).toBe('pass_ended');
    }
    expect(reduce(online(guestSnap), { type: 'session_ended', reason: 'logout' }).pair.notice).toBe('logout');
    expect(reduce(online(familySnap), { type: 'session_ended', reason: 'revoked' }).pair.notice).toBe('revoked');
  });
});

describe('guest chip', () => {
  it('says when the pass ends, and is absent for family phones', () => {
    const ends = guestSnap.me!.expires_at_ms!;
    const strip = GuestChip({ state: online(guestSnap, ends - 5 * 3600 * 1000) }) as VNode<{ children?: unknown }>;
    const chip = strip.props.children as VNode<{ children?: unknown; 'data-testid'?: string }>;
    expect(chip.props['data-testid']).toBe('guest-chip');
    expect(String(chip.props.children)).toBe(`Guest · ends ${passEndLabel(ends, ends - 5 * 3600 * 1000)}`);
    expect(GuestChip({ state: online(familySnap) })).toBeNull();
  });

  it('formats a time within 20 hours and a weekday further out', () => {
    const end = new Date(2026, 8, 29, 4, 0).getTime(); // local 04:00
    expect(passEndLabel(end, end - 7 * 3600 * 1000)).toBe('04:00');
    expect(passEndLabel(end, end - 3 * 24 * 3600 * 1000)).toBe('Tue 04:00');
  });
});
