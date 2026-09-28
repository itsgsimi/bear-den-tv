// UUID v4 generation for request and hold ids.
// Contract: `uuidV4()` returns a lower-case RFC 4122 version-4 UUID. It prefers
// `crypto.randomUUID`, which browsers expose only in secure contexts, and falls
// back to `crypto.getRandomValues` so plain-HTTP LAN pages still get random ids.
// It throws when no cryptographic random source exists; ids are never derived
// from Math.random.

export interface RandomSource {
  randomUUID?: () => string;
  getRandomValues?: <T extends ArrayBufferView | null>(array: T) => T;
}

const HEX: string[] = [];
for (let i = 0; i < 256; i += 1) HEX.push((i + 0x100).toString(16).slice(1));

/**
 * @param source Random source; defaults to the global `crypto` object.
 * @returns A version-4 UUID string such as `b1ed0b5e-e592-4b33-b7a7-9029a434a818`.
 */
export function uuidV4(source: RandomSource | undefined = globalThis.crypto as RandomSource | undefined): string {
  if (source && typeof source.randomUUID === 'function') {
    return source.randomUUID.call(source).toLowerCase();
  }
  if (source && typeof source.getRandomValues === 'function') {
    const bytes = new Uint8Array(16);
    source.getRandomValues.call(source, bytes);
    return formatV4(bytes);
  }
  throw new Error('No cryptographic random source is available');
}

/**
 * @param bytes Sixteen random bytes; the version and variant bits are overwritten.
 * @returns The formatted UUID.
 */
export function formatV4(bytes: Uint8Array): string {
  if (bytes.length !== 16) throw new Error('UUID needs 16 bytes');
  const b = Uint8Array.from(bytes);
  b[6] = ((b[6] as number) & 0x0f) | 0x40;
  b[8] = ((b[8] as number) & 0x3f) | 0x80;
  let out = '';
  for (let i = 0; i < 16; i += 1) {
    if (i === 4 || i === 6 || i === 8 || i === 10) out += '-';
    out += HEX[b[i] as number];
  }
  return out;
}

export const UUID_V4_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
