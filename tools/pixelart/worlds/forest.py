"""Forest world backdrop: misty pines at dawn. Ridges of pines fade into the
mist towards a gold sky, light falls through in shafts, ferns, mushrooms and
berries fill the foreground and a creek glints. The bears' tent is Home's
corner scene (CampScene.qml)."""

import math

from px import Img, Rng, dither, mix
import common as c
from ornaments import SPRITES, sprite

ID = 'forest'


def fern(img, x, y, size, color, light):
    """A fern: fronds fanning up and out from one point."""
    for k, ang in enumerate((-70, -42, -18, 6, 30, 56)):
        a = math.radians(ang - 90)
        ln = size * (0.75 + 0.25 * math.cos(math.radians(ang)))
        for i in range(int(ln)):
            t = i / ln
            bend = t * t * 0.6 * (1 if ang > -10 else -1)
            px_ = x + math.cos(a + bend) * i
            py_ = y + math.sin(a + bend) * i
            img.put(px_, py_, color)
            if i % 2 == 0 and 1 < i < ln - 1:
                side = int(round((1 - t) * 2.2))
                for s in range(1, side + 1):
                    img.put(px_ + math.cos(a + bend + 1.4) * s, py_ + math.sin(a + bend + 1.4) * s, light if s == side else color)
                    img.put(px_ + math.cos(a + bend - 1.4) * s, py_ + math.sin(a + bend - 1.4) * s, color)


def build():
    W, H = c.W, c.H
    img = Img(W, H, '#1E3527')
    img.vgradient(0, 0, W, 200, [(0, '#1D3A30'), (0.4, '#4F7766'), (0.75, '#A9C4A6'), (1, '#EFE2B4')])
    img.glow(330, 150, 150, '#FFF0C0', 0.45, ry=70, bands=5)
    # Pine ridges, far (hazy, pale) to near (deep green).
    layers = [
        (186, 10, 22, '#8DAE98', 190),
        (196, 14, 30, '#6F9582', 160),
        (208, 18, 40, '#557C69', 120),
        (222, 24, 54, '#3C604F', 80),
        (238, 30, 70, '#264536', 40),
    ]
    for i, (base, hmin, hmax, col, n) in enumerate(layers):
        c.forest(img, 40 + i, base, n, hmin, hmax, col)
        img.rect(0, base - 8, W, 10, '#DCE6D2', 0.12 + 0.02 * (4 - i))
    # Light shafts from the top right.
    for k, x0 in enumerate((330, 372, 430)):
        for y in range(110, 234):
            x = x0 - (y - 110) * 0.55
            fade = min(1.0, (y - 110) / 40, (234 - y) / 30)
            for w in range(7 + k % 2 * 4):
                if dither(int(x) + w, y, 0.35 * fade):
                    img.put(x + w, y, '#FFF6D8', 0.22)
    # Forest floor.
    img.vgradient(0, 236, W, H - 236, [(0, '#1E3527'), (1, '#122018')])
    # A creek winding across the floor.
    for x in range(W):
        cy = 250 + math.sin(x / 38) * 5 + math.sin(x / 13) * 1.5
        for y in range(int(cy) - 2, int(cy) + 2):
            img.put(x, y, '#5E8A8A' if y < cy else '#3E6A70')
        if x % 7 == 0:
            img.put(x, int(cy) - 1, '#CFE6D8')
    rng = Rng(12)
    for _ in range(18):
        x = rng.int(0, W - 1)
        fern(img, x, rng.int(262, 270), rng.int(8, 14), '#3E7A4E', '#8CC47A')
    for _ in range(10):
        x = rng.int(0, W - 1)
        fern(img, x, rng.int(238, 244), rng.int(6, 10), '#35684A', '#7FB06E')
    mush, berries = sprite(*SPRITES['mushroom']), sprite(*SPRITES['berries'])
    for (x, y) in ((18, 256), (64, 262), (212, 258), (300, 262)):
        img.blit(mush, x, y)
    for (x, y) in ((110, 258), (250, 262), (40, 240)):
        img.blit(berries, x, y)
    sprites = [
        ('mist.png', c.sheet(c.mist_frames(240, 14, 8, '#E6EEE0', 0.3)), 4, 20, 196),
        ('mist.png', None, 3, 250, 214),
        ('glints.png', c.sheet(glint_frames(60, 8, 4)), 6, 150, 246),
    ]
    return img, sprites


def glint_frames(w, h, frames):
    out = []
    rng = Rng(3)
    spots = [(rng.int(0, w - 1), rng.int(1, h - 2)) for _ in range(9)]
    for f in range(frames):
        img = Img(w, h)
        for i, (x, y) in enumerate(spots):
            if (i + f) % frames == 0:
                img.put(x, y, '#F4FAF0')
                img.put(x - 1, y, '#CFE6D8', 0.6)
                img.put(x + 1, y, '#CFE6D8', 0.6)
        out.append(img)
    return out


def phone(img):
    return img.crop(220, 0, 168, 270)
