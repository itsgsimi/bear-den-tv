// Text entry surfaced on the Remote (textEntrySurfaced in src/state.ts, used
// by src/views/remote.tsx): while the TV has a text field focused the server
// lists text.submit as available, and the Remote brings text entry to the
// top; when the field loses focus (text.submit unavailable) it goes back.
// A phone that may not send text.submit never gets it surfaced.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { GUEST_ACTIONS, type Session, type StateSnapshot } from '../../src/contract.ts';
import { type AppState, initialState, reduce, textEntrySurfaced } from '../../src/state.ts';

const fixture = (name: string): StateSnapshot => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as StateSnapshot;
const online = (snap: StateSnapshot, permissions: Session['permissions']): AppState => {
  const s = reduce(initialState('Phone'), { type: 'session_started', session: { device_id: 'd', device_name: 'Phone', permissions, csrf_token: 'c', transport_secure: false } });
  return reduce(s, { type: 'state_received', snapshot: snap, at: 1 });
};

describe('text entry surfaced while a TV text field has focus', () => {
  const focused = fixture('state.phone-web-app.valid.json');
  const blurred: StateSnapshot = { ...focused, capabilities: { ...focused.capabilities, 'text.submit': { available: false, reason: 'No text field is focused.' } } };

  it('comes to the top when a field is focused and goes back when it leaves', () => {
    let s = online(focused, ['controller']);
    expect(textEntrySurfaced(s)).toBe(true);
    s = reduce(s, { type: 'state_received', snapshot: blurred, at: 2 });
    expect(textEntrySurfaced(s)).toBe(false);
    s = reduce(s, { type: 'state_received', snapshot: focused, at: 3 });
    expect(textEntrySurfaced(s)).toBe(true);
  });

  it('for owners and family phones; a guest by the same rule the server enforces', () => {
    expect(textEntrySurfaced(online(focused, ['owner', 'controller']))).toBe(true);
    const asGuest = { ...focused, me: { ...focused.me!, permissions: ['guest' as const], expires_at_ms: 2_000_000_000_000 } };
    expect(textEntrySurfaced(online(asGuest, ['guest']))).toBe(GUEST_ACTIONS.has('text.submit'));
  });

  it('never without a snapshot or when text.submit is unlisted', () => {
    expect(textEntrySurfaced(initialState('Phone'))).toBe(false);
    const rest = { ...focused.capabilities };
    delete rest['text.submit'];
    expect(textEntrySurfaced(online({ ...focused, capabilities: rest }, ['controller']))).toBe(false);
  });
});
