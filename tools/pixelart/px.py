"""Tiny pixel-art canvas for Bear Den's generated art (tools/pixelart; guide
docs/THEMES.md → Pixel art). Pure Python standard library: no Pillow, so the
art can be regenerated anywhere `python3` runs.

An Img is RGBA. Drawing snaps to whole pixels and never antialiases; colours
blend only where a caller passes an alpha. Gradients and glows use ordered
(Bayer) dithering between palette colours, the classic pixel-art look, rather
than smooth ramps.
"""

import math
import struct
import zlib

BAYER4 = [
    [0, 8, 2, 10],
    [12, 4, 14, 6],
    [3, 11, 1, 9],
    [15, 7, 13, 5],
]


def rgb(c):
    """'#RRGGBB' | (r, g, b) | (r, g, b, a) → (r, g, b, a)."""
    if isinstance(c, str):
        c = c.lstrip('#')
        return (int(c[0:2], 16), int(c[2:4], 16), int(c[4:6], 16), 255)
    if len(c) == 3:
        return (c[0], c[1], c[2], 255)
    return tuple(c)


def mix(a, b, t):
    a, b = rgb(a), rgb(b)
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(3)) + (255,)


def dither(x, y, t):
    """True when a pixel at (x, y) should take the next colour for fraction t."""
    return t * 16 > BAYER4[y & 3][x & 3] + 0.5


class Rng:
    """Deterministic random numbers (so regenerated art is identical)."""

    def __init__(self, seed):
        self.s = seed & 0xFFFFFFFF or 1

    def next(self):
        self.s ^= (self.s << 13) & 0xFFFFFFFF
        self.s ^= self.s >> 17
        self.s ^= (self.s << 5) & 0xFFFFFFFF
        return self.s / 0xFFFFFFFF

    def uniform(self, a, b):
        return a + (b - a) * self.next()

    def int(self, a, b):
        return int(a + (b - a + 1) * self.next()) if b > a else a

    def choice(self, seq):
        return seq[min(len(seq) - 1, int(self.next() * len(seq)))]


def noise1(seed, x):
    """Smooth value noise in [0, 1] along one axis."""
    i = math.floor(x)
    f = x - i
    def h(n):
        n = (n * 374761393 + seed * 668265263) & 0xFFFFFFFF
        n = ((n ^ (n >> 13)) * 1274126177) & 0xFFFFFFFF
        return (n ^ (n >> 16)) / 0xFFFFFFFF
    u = f * f * (3 - 2 * f)
    return h(i) * (1 - u) + h(i + 1) * u


def fbm1(seed, x, octaves=4):
    v, amp, tot = 0.0, 1.0, 0.0
    for o in range(octaves):
        v += noise1(seed + o * 97, x * (2 ** o)) * amp
        tot += amp
        amp *= 0.5
    return v / tot


def noise2(seed, x, y):
    """Smooth value noise in [0, 1] on a plane."""
    ix, iy = math.floor(x), math.floor(y)
    fx, fy = x - ix, y - iy
    def h(a, b):
        n = (a * 374761393 + b * 668265263 + seed * 1442695041) & 0xFFFFFFFF
        n = ((n ^ (n >> 13)) * 1274126177) & 0xFFFFFFFF
        return (n ^ (n >> 16)) / 0xFFFFFFFF
    ux, uy = fx * fx * (3 - 2 * fx), fy * fy * (3 - 2 * fy)
    top = h(ix, iy) * (1 - ux) + h(ix + 1, iy) * ux
    bot = h(ix, iy + 1) * (1 - ux) + h(ix + 1, iy + 1) * ux
    return top * (1 - uy) + bot * uy


def fbm2(seed, x, y, octaves=4):
    v, amp, tot = 0.0, 1.0, 0.0
    for o in range(octaves):
        f = 2 ** o
        v += noise2(seed + o * 131, x * f, y * f) * amp
        tot += amp
        amp *= 0.5
    return v / tot


class Img:
    def __init__(self, w, h, fill=(0, 0, 0, 0)):
        self.w, self.h = w, h
        f = rgb(fill)
        self.px = [f] * (w * h)

    # --- pixels ---------------------------------------------------------
    def get(self, x, y):
        if 0 <= x < self.w and 0 <= y < self.h:
            return self.px[y * self.w + x]
        return (0, 0, 0, 0)

    def put(self, x, y, c, a=None):
        x, y = int(x), int(y)
        if not (0 <= x < self.w and 0 <= y < self.h):
            return
        c = rgb(c)
        alpha = c[3] if a is None else int(a * 255)
        if alpha >= 255:
            self.px[y * self.w + x] = c[:3] + (255,)
            return
        if alpha <= 0:
            return
        d = self.px[y * self.w + x]
        t = alpha / 255.0
        da = d[3] / 255.0
        oa = t + da * (1 - t)
        if oa <= 0:
            return
        out = tuple(int(round((c[i] * t + d[i] * da * (1 - t)) / oa)) for i in range(3))
        self.px[y * self.w + x] = out + (int(round(oa * 255)),)

    def clear(self, x, y):
        if 0 <= x < self.w and 0 <= y < self.h:
            self.px[y * self.w + x] = (0, 0, 0, 0)

    # --- shapes ---------------------------------------------------------
    def rect(self, x, y, w, h, c, a=None):
        for yy in range(int(y), int(y + h)):
            for xx in range(int(x), int(x + w)):
                self.put(xx, yy, c, a)

    def hline(self, x0, x1, y, c, a=None):
        for x in range(int(min(x0, x1)), int(max(x0, x1)) + 1):
            self.put(x, y, c, a)

    def vline(self, x, y0, y1, c, a=None):
        for y in range(int(min(y0, y1)), int(max(y0, y1)) + 1):
            self.put(x, y, c, a)

    def line(self, x0, y0, x1, y1, c, a=None):
        x0, y0, x1, y1 = int(round(x0)), int(round(y0)), int(round(x1)), int(round(y1))
        dx, dy = abs(x1 - x0), -abs(y1 - y0)
        sx, sy = (1 if x0 < x1 else -1), (1 if y0 < y1 else -1)
        err = dx + dy
        while True:
            self.put(x0, y0, c, a)
            if x0 == x1 and y0 == y1:
                break
            e2 = 2 * err
            if e2 >= dy:
                err += dy
                x0 += sx
            if e2 <= dx:
                err += dx
                y0 += sy

    def disc(self, cx, cy, r, c, a=None):
        r2 = r * r + r * 0.8
        for y in range(int(cy - r - 1), int(cy + r + 2)):
            for x in range(int(cx - r - 1), int(cx + r + 2)):
                if (x - cx) ** 2 + (y - cy) ** 2 <= r2:
                    self.put(x, y, c, a)

    def ellipse(self, cx, cy, rx, ry, c, a=None):
        for y in range(int(cy - ry - 1), int(cy + ry + 2)):
            for x in range(int(cx - rx - 1), int(cx + rx + 2)):
                if rx > 0 and ry > 0 and ((x - cx) / (rx + 0.4)) ** 2 + ((y - cy) / (ry + 0.4)) ** 2 <= 1:
                    self.put(x, y, c, a)

    def poly(self, pts, c, a=None):
        """Fill a polygon (even-odd scanline)."""
        ys = [p[1] for p in pts]
        for y in range(int(math.floor(min(ys))), int(math.ceil(max(ys))) + 1):
            yc = y + 0.5
            xs = []
            for i in range(len(pts)):
                (x0, y0), (x1, y1) = pts[i], pts[(i + 1) % len(pts)]
                if (y0 <= yc < y1) or (y1 <= yc < y0):
                    xs.append(x0 + (yc - y0) * (x1 - x0) / (y1 - y0))
            xs.sort()
            for i in range(0, len(xs) - 1, 2):
                for x in range(int(math.ceil(xs[i] - 0.5)), int(math.floor(xs[i + 1] - 0.5)) + 1):
                    self.put(x, y, c, a)

    # --- dithered fills -------------------------------------------------
    def vgradient(self, x, y, w, h, stops):
        """Vertical dithered gradient; stops = [(pos 0..1, colour), ...]."""
        for yy in range(h):
            t = yy / max(1, h - 1)
            for i in range(len(stops) - 1):
                if stops[i][0] <= t <= stops[i + 1][0]:
                    a, b = stops[i], stops[i + 1]
                    f = (t - a[0]) / max(1e-6, b[0] - a[0])
                    break
            for xx in range(w):
                self.put(x + xx, y + yy, b[1] if dither(x + xx, y + yy, f) else a[1])

    def glow(self, cx, cy, r, c, strength=0.5, ry=None, bands=4):
        """A radial glow in a few hard-edged, dithered bands (never smooth)."""
        ry = ry or r
        c = rgb(c)
        for y in range(int(cy - ry), int(cy + ry) + 1):
            for x in range(int(cx - r), int(cx + r) + 1):
                d = math.sqrt(((x - cx) / r) ** 2 + ((y - cy) / ry) ** 2)
                if d >= 1:
                    continue
                v = (1 - d) ** 1.6 * bands
                band = math.floor(v)
                frac = v - band
                if dither(x, y, frac):
                    band += 1
                if band > 0:
                    self.put(x, y, c, min(1.0, strength * band / bands))

    # --- images ---------------------------------------------------------
    def blit(self, src, x, y, flip=False, alpha=1.0):
        for sy in range(src.h):
            for sx in range(src.w):
                p = src.px[sy * src.w + (src.w - 1 - sx if flip else sx)]
                if p[3]:
                    self.put(x + sx, y + sy, p, p[3] / 255 * alpha)

    def crop(self, x, y, w, h):
        out = Img(w, h)
        for yy in range(h):
            for xx in range(w):
                out.px[yy * w + xx] = self.get(x + xx, y + yy)
        return out

    def outline(self, c, diagonal=False):
        """Draw a 1-pixel outline around every opaque pixel (in place)."""
        c = rgb(c)
        opaque = {(i % self.w, i // self.w) for i, p in enumerate(self.px) if p[3] > 0}
        n = [(1, 0), (-1, 0), (0, 1), (0, -1)]
        if diagonal:
            n += [(1, 1), (-1, 1), (1, -1), (-1, -1)]
        for (x, y) in list(opaque):
            for dx, dy in n:
                q = (x + dx, y + dy)
                if q not in opaque and 0 <= q[0] < self.w and 0 <= q[1] < self.h:
                    self.px[q[1] * self.w + q[0]] = c
        return self

    def scaled(self, k):
        out = Img(self.w * k, self.h * k)
        for y in range(out.h):
            row = y // k * self.w
            for x in range(out.w):
                out.px[y * out.w + x] = self.px[row + x // k]
        return out

    # --- file -----------------------------------------------------------
    def save(self, path):
        raw = bytearray()
        for y in range(self.h):
            raw.append(0)
            for p in self.px[y * self.w:(y + 1) * self.w]:
                raw.extend(p)
        def chunk(t, d):
            return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xFFFFFFFF)
        png = b'\x89PNG\r\n\x1a\n'
        png += chunk(b'IHDR', struct.pack('>IIBBBBB', self.w, self.h, 8, 6, 0, 0, 0))
        png += chunk(b'IDAT', zlib.compress(bytes(raw), 9))
        png += chunk(b'IEND', b'')
        with open(path, 'wb') as f:
            f.write(png)


def from_ascii(rows, palette):
    """A sprite from rows of characters; palette maps char → colour ('.' and
    ' ' are transparent)."""
    rows = [r for r in rows]
    w = max(len(r) for r in rows)
    img = Img(w, len(rows))
    for y, r in enumerate(rows):
        for x, ch in enumerate(r):
            if ch in palette and palette[ch] is not None:
                img.put(x, y, palette[ch])
    return img


def sheet(frames):
    """Frames side by side in one row (the wallpaper sprite layout)."""
    w, h = frames[0].w, frames[0].h
    out = Img(w * len(frames), h)
    for i, f in enumerate(frames):
        out.blit(f, i * w, 0)
    out.frames = len(frames)
    return out
