"""The Classic worlds' animated layers (tools/classicart): soft, translucent
sprite sheets drawn over each theme's Classic picture, the smooth counterpart
of the pixel worlds' `wallpaper.sprites` (tools/pixelart/worlds/<id>.py).

Writes themes/<id>/classic-*.png (RGBA, frames side by side in one row) and
the `classic.wallpaper.sprites` list of each themes/<id>/theme.json. Sprite
`x`/`y` are in the Classic picture's own pixels (2560x1440); the positions
below were read off the pictures (den's cave mouth and dark arch, forest's
misty valley, midnight's moon path and brightest stars, campfire's lit smoke,
Winter's aurora, chimney and windows in tools/classicart/winter.py).

Memory: every sheet is a texture on the TV (width x height x 4 bytes), so the
regions are small, the frames few (3-6) and the rates low (2-8 fps). Nothing
here covers the whole screen. Frames loop seamlessly: every motion is a
whole cycle over the frames.

Pure standard library, deterministic: a second run changes nothing.
"""

import json
import math
import os
import struct
import zlib

from svg import Rng

TAU = 2 * math.pi


# --- a tiny antialiased RGBA canvas (premultiplied floats) -----------------

def _rgb(c):
    c = c.lstrip('#')
    return tuple(int(c[i:i + 2], 16) / 255 for i in (0, 2, 4))


class Canvas:
    def __init__(self, w, h):
        self.w, self.h = w, h
        self.r = [0.0] * (w * h)
        self.g = [0.0] * (w * h)
        self.b = [0.0] * (w * h)
        self.a = [0.0] * (w * h)

    def _over(self, i, c, k):
        if k <= 0.0:
            return
        if k > 1.0:
            k = 1.0
        j = 1.0 - k
        self.r[i] = c[0] * k + self.r[i] * j
        self.g[i] = c[1] * k + self.g[i] * j
        self.b[i] = c[2] * k + self.b[i] * j
        self.a[i] = k + self.a[i] * j

    def blob(self, cx, cy, rx, ry, colour, alpha, wrap_x=False, wrap_y=False):
        """A soft ellipse: alpha at the centre, fading smoothly to nothing at
        its rim (a radial gradient). Wraps around the frame when asked, so
        tiles and loops join without seams."""
        c = _rgb(colour)
        xs = [cx]
        ys = [cy]
        if wrap_x:
            xs += [cx - self.w, cx + self.w]
        if wrap_y:
            ys += [cy - self.h, cy + self.h]
        for ox in xs:
            for oy in ys:
                x0, x1 = max(0, int(ox - rx)), min(self.w, int(ox + rx) + 1)
                y0, y1 = max(0, int(oy - ry)), min(self.h, int(oy + ry) + 1)
                for y in range(y0, y1):
                    dy = (y + 0.5 - oy) / ry
                    dy *= dy
                    if dy >= 1:
                        continue
                    row = y * self.w
                    for x in range(x0, x1):
                        dx = (x + 0.5 - ox) / rx
                        t = dx * dx + dy
                        if t < 1:
                            f = 1 - t
                            self._over(row + x, c, alpha * f * f)

    def streak(self, x0, y0, x1, y1, width, colour, alpha, wrap_y=False):
        """An antialiased line that tapers at both ends (a raindrop)."""
        c = _rgb(colour)
        ys = [0, -self.h, self.h] if wrap_y else [0]
        vx, vy = x1 - x0, y1 - y0
        ll = vx * vx + vy * vy
        for off in ys:
            ay, by = y0 + off, y1 + off
            bx0, bx1 = max(0, int(min(x0, x1) - width - 1)), min(self.w, int(max(x0, x1) + width + 2))
            by0, by1 = max(0, int(min(ay, by) - width - 1)), min(self.h, int(max(ay, by) + width + 2))
            for y in range(by0, by1):
                py = y + 0.5 - ay
                row = y * self.w
                for x in range(bx0, bx1):
                    px = x + 0.5 - x0
                    u = (px * vx + py * vy) / ll
                    if u < 0 or u > 1:
                        continue
                    dx, dy = px - u * vx, py - u * vy
                    d = math.sqrt(dx * dx + dy * dy)
                    if d < width:
                        self._over(row + x, c, alpha * (1 - d / width) * math.sin(math.pi * u))

    def fade(self, fn):
        """Multiplies every pixel by fn(x, y) in [0, 1] (soft edges)."""
        for y in range(self.h):
            row = y * self.w
            for x in range(self.w):
                k = fn(x, y)
                if k < 1:
                    i = row + x
                    self.r[i] *= k
                    self.g[i] *= k
                    self.b[i] *= k
                    self.a[i] *= k


def edge(v, size, soft):
    """1 inside, easing to 0 over `soft` pixels at both ends of [0, size)."""
    d = min(v + 0.5, size - v - 0.5) / soft
    if d >= 1:
        return 1.0
    return max(0.0, d * d * (3 - 2 * d))


def save_sheet(path, frames):
    """Frames side by side in one row, as an RGBA PNG (straight alpha)."""
    w, h = frames[0].w, frames[0].h
    raw = bytearray()
    for y in range(h):
        raw.append(0)
        for f in frames:
            row = y * w
            for x in range(w):
                i = row + x
                a = f.a[i]
                if a <= 0.002:
                    raw.extend(b'\0\0\0\0')
                    continue
                raw.extend((min(255, int(f.r[i] / a * 255 + 0.5)), min(255, int(f.g[i] / a * 255 + 0.5)),
                            min(255, int(f.b[i] / a * 255 + 0.5)), min(255, int(a * 255 + 0.5))))

    def chunk(t, d):
        return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xFFFFFFFF)
    png = b'\x89PNG\r\n\x1a\n'
    png += chunk(b'IHDR', struct.pack('>IIBBBBB', w * len(frames), h, 8, 6, 0, 0, 0))
    png += chunk(b'IDAT', zlib.compress(bytes(raw), 9))
    png += chunk(b'IEND', b'')
    with open(path, 'wb') as fh:
        fh.write(png)


# --- the layers --------------------------------------------------------------

def mist(w, h, frames, seed, colour, alpha, count):
    """A drifting band of mist: soft wide blobs swaying on a whole cycle."""
    rng = Rng(seed)
    blobs = [(rng.uniform(0.05, 0.95) * w, rng.uniform(0.32, 0.68) * h, rng.uniform(0.14, 0.3) * w,
              rng.uniform(0.18, 0.3) * h, rng.uniform(0, TAU), rng.uniform(0.6, 1.0)) for _ in range(count)]
    out = []
    for f in range(frames):
        ph = TAU * f / frames
        c = Canvas(w, h)
        for bx, by, rx, ry, p, k in blobs:
            c.blob(bx + 0.06 * w * math.sin(ph + p), by + 0.08 * h * math.sin(ph * 2 + p),
                   rx, ry, colour, alpha * k * (0.75 + 0.25 * math.sin(ph + p * 1.7)))
        c.fade(lambda x, y: edge(x, w, w * 0.25) * edge(y, h, h * 0.35))
        out.append(c)
    return out


def glow(size, frames, seed, colour, core, alpha, flicker):
    """A warm light (lantern, window) whose glow flickers; the core stays."""
    rng = Rng(seed)
    levels = [1 + flicker * (rng.next() * 2 - 1) for _ in range(frames)]
    out = []
    for f in range(frames):
        c = Canvas(size, size)
        m = size / 2
        c.blob(m, m, m * 0.8 * levels[f], m * 0.8 * levels[f], colour, alpha * levels[f])
        c.blob(m, m, m * 0.35, m * 0.35, colour, alpha * 0.8 * levels[f])
        if core:
            c.blob(m, m, 6, 8, core, 0.75)
        out.append(c)
    return out


def shafts(w, h, frames):
    """Den: light shafts through the cave mouth breathing, with dust motes."""
    rng = Rng(11)
    # Bands along the light's direction (down and to the right).
    ang = math.atan2(1, 0.85)
    dx, dy = math.cos(ang), math.sin(ang)
    bands = [(rng.uniform(0.1, 0.9) * w, rng.uniform(18, 40), rng.uniform(0, TAU)) for _ in range(6)]
    motes = [(rng.uniform(0, w), rng.uniform(0, h), rng.uniform(1.4, 2.6), rng.uniform(0, TAU)) for _ in range(40)]
    out = []
    for f in range(frames):
        ph = TAU * f / frames
        c = Canvas(w, h)
        col = _rgb('#FFF4CC')
        for y in range(h):
            row = y * w
            for x in range(w):
                # Distance across the shafts (perpendicular to the light).
                across = (x - 0.15 * w) * dy - y * dx
                k = 0.0
                for bx, bw, p in bands:
                    d = (across - (bx - 0.5 * w) * dy) / bw
                    if -3 < d < 3:
                        k += math.exp(-d * d) * (0.5 + 0.5 * math.sin(ph + p))
                if k > 0.005:
                    c._over(row + x, col, 0.09 * k)
        for mx, my, r, p in motes:
            c.blob(mx + 6 * math.sin(ph + p), (my + h * f / frames * 0.15) % h, r, r, '#FFF8DC',
                   0.5 + 0.35 * math.sin(ph + p), wrap_y=True)
        c.fade(lambda x, y: edge(x, w, w * 0.3) * edge(y, h, h * 0.3))
        out.append(c)
    return out


def moon_path(w, h, frames):
    """Midnight: glinting dashes on the water under the moon."""
    out = []
    for f in range(frames):
        rng = Rng(300 + f)
        c = Canvas(w, h)
        for _ in range(38):
            y = rng.uniform(0.04, 0.96) * h
            spread = 22 + 60 * (y / h)
            x = w / 2 + rng.uniform(-1, 1) * spread * rng.next()
            half = rng.uniform(6, 20) * (0.6 + y / h)
            k = 0.55 * (1 - abs(x - w / 2) / (spread + 1)) * edge(y, h, h * 0.25)
            c.streak(x - half, y, x + half, y, 2.2, '#FFF1CC', k)
        out.append(c)
    return out


def twinkle(size, frames):
    """Midnight: a star's sparkle growing and shrinking."""
    out = []
    m = size / 2
    for f in range(frames):
        k = 0.5 + 0.5 * math.cos(TAU * f / frames)
        c = Canvas(size, size)
        c.blob(m, m, 4 + 4 * k, 4 + 4 * k, '#E8F0FF', 0.2 + 0.3 * k)
        arm = 4 + 9 * k
        c.streak(m - arm, m, m + arm, m, 1.1, '#FFFFFF', 0.25 + 0.45 * k)
        c.streak(m, m - arm, m, m + arm, 1.1, '#FFFFFF', 0.25 + 0.45 * k)
        out.append(c)
    return out


def rain(w, h, frames):
    """Campfire: a tile of falling rain; the pattern wraps both ways so tiles
    join, and it moves one whole tile height per loop. Fades out downwards."""
    rng = Rng(77)
    drops = [(rng.uniform(0, w), rng.uniform(0, h), rng.uniform(34, 70), rng.uniform(0.9, 1.6),
              rng.uniform(0.14, 0.3)) for _ in range(70)]
    out = []
    for f in range(frames):
        c = Canvas(w, h)
        for x, y, ln, wd, a in drops:
            yy = (y + h * f / frames) % h
            xx = (x - 0.18 * (h * f / frames)) % w
            c.streak(xx, yy, xx - 0.18 * ln, yy + ln, wd, '#D6E4DC', a, wrap_y=True)
            if xx < 20:  # the tile's left edge continues on the right
                c.streak(xx + w, yy, xx + w - 0.18 * ln, yy + ln, wd, '#D6E4DC', a, wrap_y=True)
        c.fade(lambda x, y: 1 - max(0.0, (y - 0.45 * h) / (0.55 * h)) ** 0.8 if y > 0.45 * h else 1.0)
        out.append(c)
    return out


def smoke_puffs(w, h, frames, x0, y0, colour, alpha, count, drift):
    """Soft puffs rising from (x0, y0), growing and fading; puff k takes puff
    k+1's place over one loop, so the column never jumps."""
    out = []
    for f in range(frames):
        c = Canvas(w, h)
        for k in range(count):
            t = (k + f / frames) / count          # 0 at the source, 1 at the top
            px = x0 + drift * t + 14 * math.sin(t * 5.0)
            py = y0 - t * (y0 - 40)
            r = 16 + 70 * t
            a = alpha * min(1.0, t * 5) * (1 - t) ** 1.2
            c.blob(px, py, r * 1.2, r, colour, a)
        out.append(c)
    return out


def wisps(w, h, frames):
    """Campfire: slow wisps curling through the lit smoke."""
    rng = Rng(55)
    ws = [(rng.uniform(0.1, 0.9) * w, rng.uniform(0.1, 0.9) * h, rng.uniform(40, 90), rng.uniform(14, 30),
           rng.uniform(0, TAU)) for _ in range(22)]
    out = []
    for f in range(frames):
        ph = TAU * f / frames
        c = Canvas(w, h)
        for x, y, rx, ry, p in ws:
            c.blob(x + 18 * math.sin(ph + p), y + 10 * math.cos(ph + p), rx, ry, '#CFE0C4',
                   0.07 + 0.05 * math.sin(ph + 2 * p))
        c.fade(lambda x, y: edge(x, w, w * 0.3) * edge(y, h, h * 0.3))
        out.append(c)
    return out


def aurora_rays(w, h, frames, seed):
    """Winter: the aurora's rays brightening and dimming in turn."""
    rng = Rng(seed)
    rays = [(rng.uniform(0, w), rng.uniform(0.5, 0.7) * h, rng.uniform(10, 24), rng.uniform(0.3, 0.45) * h,
             rng.uniform(0, TAU), '#9CFFD8' if rng.next() < 0.75 else '#C4B2FF') for _ in range(24)]
    out = []
    for f in range(frames):
        ph = TAU * f / frames
        c = Canvas(w, h)
        for x, y, rx, ry, p, col in rays:
            k = 0.5 + 0.5 * math.sin(ph + p)
            c.blob(x + 8 * math.sin(ph + p * 2), y, rx, ry, col, 0.5 * k * k, wrap_x=True)
        c.fade(lambda x, y: edge(x, w, w * 0.2) * edge(y, h, h * 0.2))
        out.append(c)
    return out


# --- the worlds --------------------------------------------------------------

def _sprite(sheet, frames, fps, x, y):
    return {'sheet': sheet, 'frames': frames, 'fps': fps, 'x': x, 'y': y}


def worlds():
    """Each world: its sheets (file → frames) and its sprite list."""
    return {
        'den': (
            {'classic-shafts.png': lambda: shafts(360, 420, 4),
             'classic-mist.png': lambda: mist(640, 180, 4, 21, '#B4C6B6', 0.05, 24),
             'classic-lantern.png': lambda: glow(240, 4, 3, '#FFB457', '#FFE9A8', 0.30, 0.18)},
            # The light through the cave mouth (top left), mist on the floor
            # below it, a lantern deep in the dark arch (top right).
            [_sprite('classic-shafts.png', 4, 3, 220, 60),
             _sprite('classic-mist.png', 4, 2, 60, 470),
             _sprite('classic-mist.png', 4, 3, 620, 520),
             _sprite('classic-lantern.png', 4, 6, 1920, 250)]),
        'forest': (
            {'classic-mist.png': lambda: mist(720, 280, 4, 31, '#C4CEC8', 0.045, 26)},
            # Mist in the valley under the mountains, and low along the
            # tree line on both sides.
            [_sprite('classic-mist.png', 4, 2, 640, 210),
             _sprite('classic-mist.png', 4, 3, 1100, 260),
             _sprite('classic-mist.png', 4, 2, 180, 330),
             _sprite('classic-mist.png', 4, 3, 1700, 300)]),
        'midnight': (
            {'classic-shimmer.png': lambda: moon_path(200, 380, 4),
             'classic-lakemist.png': lambda: mist(720, 90, 4, 41, '#A8B8D8', 0.1, 22),
             'classic-twinkle.png': lambda: twinkle(48, 4)},
            # The moon's path on the lake (under the moon, left), mist along
            # the far shore, the brightest stars twinkling.
            [_sprite('classic-shimmer.png', 4, 4, 236, 575),
             _sprite('classic-lakemist.png', 4, 2, 480, 495),
             _sprite('classic-lakemist.png', 4, 3, 1320, 505)]
            + [_sprite('classic-twinkle.png', 4, fps, x - 24, y - 24)
               for (x, y), fps in zip([(709, 63), (1116, 179), (2239, 65), (1779, 230), (657, 183), (1769, 71)],
                                      [2, 3, 4, 3, 2, 4])]),
        'campfire': (
            {'classic-rain.png': lambda: rain(512, 560, 3),
             'classic-smoke.png': lambda: wisps(440, 360, 4)},
            # Rain over the top of the picture, where it catches the light,
            # and wisps curling through the lit smoke on both sides.
            [_sprite('classic-rain.png', 3, 8, x, 0) for x in (0, 512, 1024, 1536, 2048)]
            + [_sprite('classic-smoke.png', 4, 2, 40, 40),
               _sprite('classic-smoke.png', 4, 3, 1900, 120)]),
        'winter': (
            {'classic-aurora.png': lambda: aurora_rays(480, 340, 4, 61),
             'classic-smoke.png': lambda: smoke_puffs(280, 440, 6, 92, 432, '#D4DEEC', 0.5, 6, 90),
             'classic-window.png': lambda: glow(120, 4, 8, '#FFC46E', None, 0.32, 0.2)},
            # Rays along the aurora's lower edge, smoke from the chimney
            # (its top is at 972, 992 in winter.py), the windows' glow.
            [_sprite('classic-aurora.png', 4, fps, x, 330) for x, fps in ((80, 3), (700, 4), (1320, 3), (1940, 4))]
            + [_sprite('classic-smoke.png', 6, 4, 880, 560),
               _sprite('classic-window.png', 4, 5, 793, 1112),
               _sprite('classic-window.png', 4, 6, 905, 1112)]),
    }


def write_sprites(path, sprites):
    """Sets theme.json's classic.wallpaper.sprites, keeping everything else
    (2-space JSON, key order)."""
    with open(path) as fh:
        data = json.load(fh)
    classic = data.setdefault('classic', {})
    classic.setdefault('wallpaper', {})['sprites'] = sprites
    text = json.dumps(data, indent=2, ensure_ascii=False) + '\n'
    with open(path) as fh:
        if fh.read() == text:
            return
    with open(path, 'w') as fh:
        fh.write(text)


def build(root):
    files = []
    for tid, (sheets, sprites) in worlds().items():
        folder = os.path.join(root, 'themes', tid)
        for name, make in sheets.items():
            frames = make()
            save_sheet(os.path.join(folder, name), frames)
            files.append(f'themes/{tid}/{name}')
        write_sprites(os.path.join(folder, 'theme.json'), sprites)
        files.append(f'themes/{tid}/theme.json')
    return files
