// readInvitation (src/app.ts): the phone must redeem the link the TV's QR code
// opens. The coordinator builds it as base + "#pair=" + token
// (internal/pairing/pairing.go issue(), pinned by pairing_test.go); the
// contract fixture state.shell-guest-pass.valid.json carries one.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { StateSnapshot } from '../../src/contract.ts';
import { readInvitation, type WindowEnvironment } from '../../src/app.ts';

function windowAt(url: string): WindowEnvironment & { replaced: string | null } {
  const u = new URL(url);
  const env = {
    replaced: null as string | null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    location: { hash: u.hash, pathname: u.pathname, search: u.search },
    history: {
      replaceState: (_d: unknown, _u: string, next?: string) => {
        env.replaced = next ?? null;
      },
    },
    navigator: { userAgent: 'test' },
  };
  return env;
}

describe('readInvitation', () => {
  it('reads the token from the URL the TV puts in its QR code, and strips it', () => {
    const shell = JSON.parse(readFileSync(new URL('../../../../contracts/fixtures/state.shell-guest-pass.valid.json', import.meta.url), 'utf8')) as StateSnapshot & {
      pairing: { url: string };
    };
    const win = windowAt(shell.pairing.url);
    expect(readInvitation(win)).toBe('DEMO');
    expect(win.replaced).toBe('/');
    expect(readInvitation(windowAt('http://192.0.2.10:8090/#pair=abc_DEF-123'))).toBe('abc_DEF-123');
  });

  it('still accepts #invite= and ignores other fragments', () => {
    expect(readInvitation(windowAt('http://192.0.2.10:8090/#invite=tok'))).toBe('tok');
    expect(readInvitation(windowAt('http://192.0.2.10:8090/#settings'))).toBeNull();
    expect(readInvitation(windowAt('http://192.0.2.10:8090/'))).toBeNull();
  });
});
