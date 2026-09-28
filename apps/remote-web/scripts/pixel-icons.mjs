// Generates the PWA and favicon icons in static/icons/ from the pixel bear mark
// (static/art/pixel/bear-mark.png) by nearest-neighbour upscaling onto the
// charcoal app background, so every art pixel stays a crisp square. Referenced
// by static/manifest.webmanifest and static/index.html. Run by hand after the
// mark changes (`node scripts/pixel-icons.mjs`); the output is deterministic and
// committed. Node standard library only (node:zlib for PNG deflate).
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateSync, inflateSync } from 'node:zlib';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const BACKGROUND = [0x16, 0x19, 0x1c, 255]; // --charcoal, the manifest background_color

/** Decodes an 8-bit RGBA or RGB, non-interlaced PNG into { w, h, px: Uint8Array RGBA }. */
function decodePng(buf) {
  let off = 8;
  let w = 0;
  let h = 0;
  let type = 0;
  const idat = [];
  while (off < buf.length) {
    const len = buf.readUInt32BE(off);
    const tag = buf.toString('latin1', off + 4, off + 8);
    const data = buf.subarray(off + 8, off + 8 + len);
    if (tag === 'IHDR') {
      w = data.readUInt32BE(0);
      h = data.readUInt32BE(4);
      type = data[9];
      if (data[8] !== 8 || (type !== 6 && type !== 2) || data[12] !== 0) throw new Error('need an 8-bit RGB(A), non-interlaced PNG');
    } else if (tag === 'IDAT') idat.push(data);
    off += 12 + len;
  }
  const bpp = type === 6 ? 4 : 3;
  const raw = inflateSync(Buffer.concat(idat));
  const stride = w * bpp;
  const px = new Uint8Array(w * h * 4);
  let prev = new Uint8Array(stride);
  for (let y = 0; y < h; y++) {
    const filter = raw[y * (stride + 1)];
    const line = Uint8Array.from(raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1)));
    for (let x = 0; x < stride; x++) {
      const a = x >= bpp ? line[x - bpp] : 0;
      const b = prev[x];
      const c = x >= bpp ? prev[x - bpp] : 0;
      let p = 0;
      if (filter === 1) p = a;
      else if (filter === 2) p = b;
      else if (filter === 3) p = (a + b) >> 1;
      else if (filter === 4) {
        const pa = Math.abs(b - c);
        const pb = Math.abs(a - c);
        const pc = Math.abs(a + b - 2 * c);
        p = pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      line[x] = (line[x] + p) & 255;
    }
    for (let x = 0; x < w; x++) {
      px.set([line[x * bpp], line[x * bpp + 1], line[x * bpp + 2], bpp === 4 ? line[x * bpp + 3] : 255], (y * w + x) * 4);
    }
    prev = line;
  }
  return { w, h, px };
}

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
function crc32(buf) {
  let c = 0xffffffff;
  for (const byte of buf) c = CRC_TABLE[(c ^ byte) & 255] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}
function chunk(tag, data) {
  const out = Buffer.alloc(12 + data.length);
  out.writeUInt32BE(data.length, 0);
  out.write(tag, 4, 'latin1');
  data.copy(out, 8);
  out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
  return out;
}
/** Encodes an opaque RGBA image as an 8-bit RGB PNG (filter 0, max deflate). */
function encodePng(w, h, px) {
  const raw = Buffer.alloc(h * (w * 3 + 1));
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4;
      raw.set([px[i], px[i + 1], px[i + 2]], y * (w * 3 + 1) + 1 + x * 3);
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr.set([8, 2, 0, 0, 0], 8);
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw, { level: 9 })), chunk('IEND', Buffer.alloc(0))]);
}

/** The mark scaled by `scale` (nearest neighbour), centred on a `size` square of BACKGROUND. */
function render(mark, size, scale) {
  const px = new Uint8Array(size * size * 4);
  for (let i = 0; i < size * size; i++) px.set(BACKGROUND, i * 4);
  const ox = Math.floor((size - mark.w * scale) / 2);
  const oy = Math.floor((size - mark.h * scale) / 2);
  for (let y = 0; y < mark.h * scale; y++) {
    for (let x = 0; x < mark.w * scale; x++) {
      const s = (Math.floor(y / scale) * mark.w + Math.floor(x / scale)) * 4;
      const alpha = mark.px[s + 3] / 255;
      const d = ((oy + y) * size + ox + x) * 4;
      for (let k = 0; k < 3; k++) px[d + k] = Math.round(mark.px[s + k] * alpha + px[d + k] * (1 - alpha));
    }
  }
  return encodePng(size, size, px);
}

const mark = decodePng(readFileSync(join(root, 'static/art/pixel/bear-mark.png')));
// [file, canvas size, whole-number scale]. "any" icons fill ~75% of the canvas;
// maskable ones keep the mark inside the central 60% safe zone.
const ICONS = [
  ['bear-den-48.png', 48, 2],
  ['bear-den-192.png', 192, 9],
  ['bear-den-512.png', 512, 24],
  ['bear-den-maskable-192.png', 192, 6],
  ['bear-den-maskable-512.png', 512, 16],
];
for (const [file, size, scale] of ICONS) {
  if (Math.max(mark.w, mark.h) * scale > size) throw new Error(`${file}: mark does not fit`);
  writeFileSync(join(root, 'static/icons', file), render(mark, size, scale));
}
