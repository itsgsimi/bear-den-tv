"""Shared scenery for the worlds' pixel backdrops (tools/pixelart): ridges,
pines, clouds, rain and flames. Every backdrop is 480×270 art pixels, drawn
4× on a 1080p TV (docs/THEMES.md → Pixel art)."""

import math

from px import Img, Rng, fbm1, fbm2, dither, mix, rgb, sheet  # noqa: F401 (sheet re-exported)

W, H = 480, 270


def ridge(img, seed, base, amp, scale, color, rim=None, rim_side=-1, top=None):
    """A mountain ridge: fbm skyline from `base - amp` up; filled down to the
    bottom. `rim` lights the upper edge (1 px) on slopes facing `rim_side`."""
    heights = []
    for x in range(img.w):
        v = fbm1(seed, x / scale, 5)
        heights.append(int(base - v * amp))
    for x, hy in enumerate(heights):
        img.vline(x, hy, img.h - 1 if top is None else top, color)
    if rim:
        for x in range(1, img.w - 1):
            slope = heights[x + 1] - heights[x - 1]
            if slope * rim_side > 0 or (slope == 0 and x % 3 == 0):
                img.put(x, heights[x], rim)
    return heights


def pine(img, x, base, h, color, rim=None, rim_side=1, snow=None):
    """A pine silhouette in tiers; `rim` lights one side, `snow` caps tiers."""
    tiers = max(3, h // 7)
    maxw = max(3, int(h * 0.36))
    trunk = max(1, h // 10)
    img.rect(x - (1 if h > 24 else 0), base - trunk, 1 + (2 if h > 24 else 1), trunk, color)
    top = base - h
    body = h - trunk
    for row in range(body):
        t = row / body
        tier_t = (row * tiers / body) % 1.0
        half = maxw * t * (0.5 + 0.55 * tier_t)
        half = max(0, int(round(half)))
        y = top + row
        img.hline(x - half, x + half, y, color)
        if rim and half > 0:
            img.put(x + rim_side * half, y, rim)
        if snow and half > 1 and tier_t < 0.18:
            img.hline(x - half + 1, x + half - 1, y, snow)
    img.put(x, top - 1, color)


def forest(img, seed, y_base, count, hmin, hmax, color, rim=None, rim_side=1, x0=0, x1=None, snow=None, density=None):
    rng = Rng(seed)
    x1 = img.w if x1 is None else x1
    trees = []
    for i in range(count):
        x = rng.int(x0 - 6, x1 + 6)
        if density and rng.next() > density(x):
            continue
        h = rng.int(hmin, hmax)
        trees.append((x, y_base + rng.int(-2, 3), h))
    trees.sort(key=lambda t: t[2])
    for x, b, h in trees:
        pine(img, x, b, h, color, rim, rim_side, snow)


def clouds(img, seed, y0, y1, color, light=None, dark=None, cover=0.5, scale=60.0, light_from=None):
    """Cloud banks between y0 and y1 (2-D noise, stretched wide): `light`
    rims their tops (brighter towards `light_from` = (x, y), the moon) and
    `dark` shades their undersides. Edges are dithered, never smooth."""
    for y in range(y0, y1):
        fade = min(1.0, (y - y0) / 8.0, (y1 - y) / 8.0)
        for x in range(img.w):
            v = fbm2(seed, x / scale, y / (scale * 0.28), 5)
            d = (v - (1 - cover)) * fade
            if d <= 0 or (d < 0.05 and not dither(x, y, d / 0.05)):
                continue
            img.put(x, y, color)
            above = fbm2(seed, x / scale, (y - 2) / (scale * 0.28), 5) - (1 - cover)
            below = fbm2(seed, x / scale, (y + 3) / (scale * 0.28), 5) - (1 - cover)
            if light and above < d - 0.02:
                near = 1.0
                if light_from:
                    dist = ((x - light_from[0]) ** 2 + ((y - light_from[1]) * 2) ** 2) ** 0.5
                    near = max(0.0, 1 - dist / 220)
                if dither(x, y, 0.35 + 0.65 * near):
                    img.put(x, y, light)
            elif dark and below < d - 0.015 and dither(x, y, 0.6):
                img.put(x, y, dark)


def stars(img, seed, n, y0, y1, colors, bright=None):
    rng = Rng(seed)
    for _ in range(n):
        x, y = rng.int(0, img.w - 1), rng.int(y0, y1)
        img.put(x, y, rng.choice(colors))
    for _ in range(bright or 0):
        x, y = rng.int(2, img.w - 3), rng.int(y0, y1)
        c = colors[0]
        img.put(x, y, c)
        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
            img.put(x + dx, y + dy, c, 0.45)


def rain_frames(seed, w, h, frames, n, color, length=(4, 7), alpha=(0.25, 0.55), slant=1):
    """Looping rain: every drop moves h/frames pixels per frame (wrapping)."""
    rng = Rng(seed)
    drops = [(rng.int(0, w - 1), rng.int(0, h - 1), rng.int(*length), rng.uniform(*alpha)) for _ in range(n)]
    step = h // frames
    out = []
    for f in range(frames):
        img = Img(w, h)
        for (x, y, ln, a) in drops:
            yy = (y + f * step) % h
            xx = (x + slant * (f * step) // 6) % w
            for i in range(ln):
                img.put((xx + slant * i // 6) % w, (yy + i) % h, color, a * (0.5 + 0.5 * i / ln))
        out.append(img)
    return out


FIRE = ['#FFF4C8', '#FFD490', '#FFB45E', '#FF7A2F', '#C9552B', '#7A2E1A']


def flame_frames(w, h, frames, seed=3, palette=FIRE):
    """A flickering flame: a teardrop whose edge wobbles per frame, banded hot
    (core) to cool (edge)."""
    out = []
    for f in range(frames):
        rng = Rng(seed * 31 + f * 7)
        img = Img(w, h)
        cx = (w - 1) / 2
        phase = f / frames * 2 * math.pi
        for y in range(h):
            t = y / (h - 1)                       # 0 top → 1 bottom
            sway = math.sin(phase + t * 4) * (1 - t) * w * 0.12
            half = (w / 2 - 0.5) * (math.sin(math.pi * min(1, t * 1.05)) ** 0.7) * (0.35 + 0.65 * t ** 0.5)
            half *= 0.88 + 0.12 * math.sin(phase * 2 + y)
            for x in range(w):
                d = abs(x - cx - sway) / max(0.6, half)
                if d > 1 or (y < h * 0.25 and rng.next() < 0.18):
                    continue
                heat = (1 - d) * 0.7 + t * 0.5 - 0.15
                idx = 5 - min(5, max(0, int(heat * 6)))
                if y > h * 0.8 and idx < 2:
                    idx = 2
                img.put(x, y, palette[idx])
        # a detached flicker above now and then
        if f % 2 == 0:
            img.put(int(cx + math.sin(phase) * 1.5), 0, palette[2])
        out.append(img)
    return out


def smoke_frames(w, h, frames, color, seed=5, puffs=6):
    """Smoke rising and drifting right, looping: each puff rises h/frames·k."""
    rng = Rng(seed)
    specs = [(rng.uniform(0, 1), rng.uniform(1.5, 3.2)) for _ in range(puffs)]
    out = []
    for f in range(frames):
        img = Img(w, h)
        for (off, r) in specs:
            p = (off + f / frames) % 1.0
            y = h - 3 - p * (h - 6)
            x = 4 + p * p * (w - 10) + math.sin(p * 6 + off * 9) * 1.5
            rr = r * (0.6 + p * 1.2)
            a = 0.5 * (1 - p) * min(1, p * 6)
            img.ellipse(x, y, rr, rr * 0.8, color, a)
        out.append(img)
    return out


def ripple_frames(w, h, frames, color):
    """Rain rings spreading on a puddle."""
    out = []
    for f in range(frames):
        img = Img(w, h)
        for (cx, cy, off) in ((w * 0.3, h * 0.5, 0), (w * 0.72, h * 0.45, 0.5)):
            p = (f / frames + off) % 1.0
            rx = 0.5 + p * (w * 0.22)
            for a in range(0, 360, 12):
                x = cx + math.cos(math.radians(a)) * rx
                y = cy + math.sin(math.radians(a)) * rx * 0.35
                img.put(int(round(x)), int(round(y)), color, 0.7 * (1 - p))
        out.append(img)
    return out


def blink_frames(w, h, frames, color, on=(0, 1, 2), glow=None):
    """A small light that flickers: brighter on the frames listed in `on`."""
    out = []
    for f in range(frames):
        img = Img(w, h)
        a = 1.0 if f in on else 0.55
        if glow:
            img.glow(w / 2, h / 2, w / 2, glow, 0.35 * a, bands=3)
        img.rect(w // 2 - 1, h // 2 - 1, 2, 2, color, a)
        out.append(img)
    return out


def mist_frames(w, h, frames, color='#AEBAC0', alpha=0.22):
    """A band of mist sliding slowly sideways (loops over `frames`)."""
    out = []
    for f in range(frames):
        img = Img(w, h)
        for y in range(h):
            for x in range(w):
                v = fbm2(4, ((x + f * w / frames) % w) / 40, y / 5, 3)
                edge = min(1.0, x / 30, (w - x) / 30, y / 3, (h - y) / 3)
                a = (v - 0.45) * 0.9 * edge
                if a > 0 and dither(x, y, min(1, a * 3)):
                    img.put(x, y, color, alpha)
        out.append(img)
    return out
