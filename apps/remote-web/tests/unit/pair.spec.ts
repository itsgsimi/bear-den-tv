// Pairing on the phone (src/views/pair.tsx, the name prefill in src/app.ts):
// UX-02, Connect used to stay greyed out with no reason until a name was
// typed. Now the name starts filled from the browser (never empty), Connect
// waits only for the six digits, and the reason is written under it.
import { describe, expect, it } from 'vitest';
import type { ApiEnvironment, SocketLike } from '../../src/api.ts';
import { createApp, type PageEnvironment, type WindowEnvironment } from '../../src/app.ts';
import { t } from '../../src/i18n.ts';
import { pairBlocker } from '../../src/views/pair.tsx';

function boot(userAgent: string, stored: string | null) {
  const socket: SocketLike = { readyState: 0, send: () => undefined, close: () => undefined, onopen: null, onclose: null, onerror: null, onmessage: null };
  const env: ApiEnvironment = {
    fetch: () => new Promise<Response>(() => undefined),
    createSocket: () => socket,
    setTimeout: () => 0,
    clearTimeout: () => undefined,
    setInterval: () => 0,
    clearInterval: () => undefined,
    random: () => 0.5,
    origin: 'http://192.0.2.10:8090',
  };
  const page: PageEnvironment = { hidden: false, addEventListener: () => undefined, removeEventListener: () => undefined };
  const storage = new Map<string, string>(stored === null ? [] : [['bdtv.device_name', stored]]);
  const win: WindowEnvironment = {
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    location: { hash: '', pathname: '/', search: '' },
    history: { replaceState: () => undefined },
    navigator: { userAgent },
    localStorage: { getItem: (k: string) => storage.get(k) ?? null, setItem: (k: string, v: string) => void storage.set(k, v) },
  };
  return createApp(env, page, win, { now: () => 1000 });
}

describe('pairing needs only the code', () => {
  it('Connect waits for six digits and says so', () => {
    expect(pairBlocker('', false)).toBe(t.pair.needSixDigits);
    expect(pairBlocker('12345', false)).toBe(t.pair.needSixDigits);
    expect(pairBlocker('123456', false)).toBeNull();
    expect(pairBlocker('12', true)).toBeNull(); // connecting: the button says so
  });

  it('the name starts filled from the browser, or the name kept from last time', () => {
    const ua = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';
    expect(boot(ua, null).store.getState().pair.device_name).toBe('iPhone (Safari)');
    expect(boot('unknown agent', null).store.getState().pair.device_name).toBe('Phone');
    expect(boot(ua, 'Kitchen tablet').store.getState().pair.device_name).toBe('Kitchen tablet');
  });
});
