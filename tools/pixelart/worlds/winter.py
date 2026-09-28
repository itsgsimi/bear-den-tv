"""Winter world backdrop: a snowy night. Northern lights ripple over snowy
hills, a cabin glows among snow-capped pines with smoke from its chimney.
Snow falls (ambient); the cub asleep on the moon mobile is Home's corner
scene."""

import math

from px import Img, Rng, fbm1, dither
import common as c

ID = 'winter'
SNOW, SNOW_D, SNOW_S = '#E6EEF8', '#B4C6DC', '#8CA2C0'


def hill(img, seed, base, amp, scale, fill, shade):
    for x in range(img.w):
        top = int(base - fbm1(seed, x / scale, 3) * amp)
        for y in range(top, img.h):
            img.put(x, y, shade if (y - top) > 2 and dither(x, y, 0.25 + 0.3 * math.sin(x / 17)) else fill)


def build():
    W, H = c.W, c.H
    img = Img(W, H, '#050A18')
    img.vgradient(0, 0, W, 200, [(0, '#050A18'), (0.55, '#10203C'), (1, '#27406A')])
    c.stars(img, 6, 200, 0, 150, ['#F4FAFF', '#CFE6FF', '#9FC3E0'], bright=10)
    hill(img, 3, 190, 34, 90, '#9FB4D0', '#8CA2C0')
    c.forest(img, 50, 206, 90, 10, 22, '#16263E', snow='#C8D8EC')
    hill(img, 5, 222, 20, 60, SNOW_D, SNOW_S)
    c.forest(img, 51, 236, 40, 16, 34, '#0F1C30', snow=SNOW)
    hill(img, 7, 248, 10, 50, SNOW, SNOW_D)
    # The cabin: log walls, a snowy roof, warm windows, a chimney.
    cx, cy = 150, 240
    img.rect(cx, cy - 16, 40, 16, '#4A3326')
    for y in range(cy - 16, cy, 3):
        img.hline(cx, cx + 39, y, '#5E4232')
    img.poly([(cx - 5, cy - 16), (cx + 20, cy - 32), (cx + 45, cy - 16)], '#3A2A20')
    img.poly([(cx - 5, cy - 16), (cx + 20, cy - 32), (cx + 45, cy - 16), (cx + 40, cy - 18), (cx + 20, cy - 29), (cx, cy - 18)], SNOW)
    img.rect(cx + 30, cy - 34, 5, 10, '#3A2A20')
    img.rect(cx + 29, cy - 35, 7, 2, SNOW)
    for wx in (cx + 6, cx + 26):
        img.rect(wx, cy - 11, 8, 7, '#FFC878')
        img.vline(wx + 4, cy - 11, cy - 5, '#4A3326')
        img.hline(wx, wx + 7, cy - 8, '#4A3326')
    img.glow(cx + 20, cy - 4, 60, '#FFB45E', 0.3, ry=18, bands=4)
    img.rect(cx + 16, cy - 10, 7, 10, '#3A2A20')
    # A fence half-buried in snow.
    for x in range(20, 130, 9):
        img.rect(x, 252, 2, 8, '#4A3A30')
        img.put(x, 251, SNOW)
    img.hline(20, 128, 255, '#4A3A30')
    sprites = [
        ('aurora.png', c.sheet(aurora_frames(W, 70, 6)), 5, 0, 16),
        ('smoke.png', c.sheet(c.smoke_frames(26, 56, 8, '#B8C4D8', seed=9)), 5, cx + 30, cy - 90),
    ]
    return img, sprites


def aurora_frames(w, h, frames):
    """Curtains of green and teal light rippling (loops over `frames`)."""
    out = []
    cols = ['#6FE3B0', '#4FC3A0', '#58A8D8']
    for f in range(frames):
        img = Img(w, h)
        ph = f / frames * 2 * math.pi
        for x in range(w):
            wave = math.sin(x / 38 + ph) * 8 + math.sin(x / 17 - ph * 2) * 3
            strength = max(0.0, math.sin(x / 130 + 0.35)) ** 0.8 * (0.7 + 0.3 * math.sin(x / 9 + ph * 3))
            top = int(18 + wave)
            length = int(20 + 22 * strength)
            for i in range(length):
                y = top + i
                if not (0 <= y < h):
                    continue
                t = i / max(1, length)
                a = strength * (1 - t) * 0.55
                if a > 0.04 and dither(x, y, min(1, a * 2.2)):
                    img.put(x, y, cols[0] if t < 0.35 else cols[1] if t < 0.7 else cols[2], min(0.8, a + 0.2))
        out.append(img)
    return out


def phone(img):
    return img.crop(96, 0, 168, 270)
