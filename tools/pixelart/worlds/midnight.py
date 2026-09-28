"""Midnight world backdrop: a moonlit lake. A starry sky with the Milky Way,
the moon low over dark hills, pines along the shore, and the moon's path
shimmering on the water. The cub asleep on the moon mobile is Home's corner
scene (MoonScene.qml)."""

import math

from px import Img, Rng, fbm2, dither
import common as c

ID = 'midnight'
MOON = (330, 150)


def build():
    W, H = c.W, c.H
    img = Img(W, H, '#060914')
    img.vgradient(0, 0, W, 190, [(0, '#060914'), (0.5, '#0E1630'), (1, '#1E2C52')])
    # The Milky Way: a faint dithered band across the sky.
    for y in range(0, 170):
        for x in range(W):
            d = abs((y - 20) - (x - 60) * 0.32) / 26
            if d < 1:
                v = fbm2(8, x / 24, y / 24, 4) * (1 - d)
                if v > 0.28 and dither(x, y, min(1, (v - 0.28) * 3)):
                    img.put(x, y, '#3A4A78', 0.6)
    c.stars(img, 4, 260, 0, 180, ['#E8EEFF', '#C8D4F4', '#9DB2DD', '#F4E3A1'], bright=16)
    img.glow(MOON[0], MOON[1], 90, '#8FA6E0', 0.4, ry=70, bands=6)
    img.disc(MOON[0], MOON[1], 13, '#DCE4F6')
    img.disc(MOON[0] - 1, MOON[1] - 1, 12, '#F2F5FC')
    for (dx, dy, r) in ((-4, -3, 2), (3, 2, 3), (5, -5, 1), (-3, 5, 1)):
        img.disc(MOON[0] + dx, MOON[1] + dy, r, '#D2DAEE')
    c.ridge(img, 17, 176, 40, 80, '#141C34', rim='#34457A', rim_side=-1)
    c.ridge(img, 19, 186, 22, 40, '#0E142A', rim='#26335E', rim_side=-1)
    c.forest(img, 30, 194, 90, 10, 26, '#090E1E')
    # The lake: the sky mirrored, darker, with the moon's path on it.
    for y in range(194, H):
        my = 194 - (y - 194)
        for x in range(W):
            p = img.get(x, max(0, my - 6))
            if p[0] > 150:                      # the moon's own reflection is the shimmer sprite
                p = (40, 56, 100, 255)
            img.put(x, y, (int(p[0] * 0.55), int(p[1] * 0.6), int(p[2] * 0.75)))
        if y % 3 == 0:
            img.hline(0, W - 1, y, '#1A2548', 0.35)
    c.forest(img, 31, 197, 60, 3, 8, '#070B18')                  # reflected shoreline hint
    # A little dock on the left, and reeds.
    img.rect(40, 214, 46, 3, '#3A2E28')
    img.hline(40, 85, 214, '#5A4A3C')
    for x in (44, 60, 76):
        img.vline(x, 217, 226, '#2A201C')
    rng = Rng(9)
    for _ in range(60):
        x = rng.int(0, W - 1)
        if 30 < x < 100:
            continue
        h = rng.int(3, 9)
        img.vline(x, 262 - h, 269, '#0B1224')
        img.put(x, 262 - h, '#1E2A4A')
    sprites = [
        ('shimmer.png', c.sheet(shimmer_frames(34, 72, 4)), 5, MOON[0] - 17, 196),
    ]
    return img, sprites


def shimmer_frames(w, h, frames):
    """The moon's path on the water: short bright dashes that shift."""
    out = []
    for f in range(frames):
        rng = Rng(100 + f)
        img = Img(w, h)
        for y in range(0, h, 2):
            spread = w / 2 * (0.35 + 0.65 * y / h)
            for _ in range(3):
                x = w / 2 + rng.uniform(-spread, spread)
                ln = rng.int(2, 6)
                col = '#F2F5FC' if abs(x - w / 2) < spread * 0.4 else '#9DB2DD'
                img.hline(x, x + ln, y, col, 0.9 - 0.5 * y / h)
        out.append(img)
    return out


def phone(img):
    return img.crop(300, 0, 168, 270)
