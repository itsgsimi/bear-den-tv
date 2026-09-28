"""Den world backdrop: looking out of the bears' cosy cave at dusk. Rock frames
the view, vines with daisies and blossoms hang from the rim, mist drifts
through the pines outside, and lanterns glow on the cave floor. The family
(DenFamily.qml) peeks out of their nook in the bottom-right corner."""

import math

from px import Img, Rng, fbm2, dither, mix
import common as c
from ornaments import SPRITES, sprite

ID = 'den'

ROCK, ROCK_D, ROCK_L, RIM = '#2A2320', '#211B18', '#342B25', '#5A4A3C'
LEAF, LEAF_D, LEAF_L, STEM = '#6A9C7A', '#4E7A5C', '#A3D1AE', '#5E8C6E'


def opening(x, y):
    """Inside the cave mouth (the view outside)?"""
    a = math.atan2(y - 168, x - 240)
    wobble = 1 + (c.fbm1(3, a * 5, 3) - 0.5) * 0.14
    return ((x - 240) / 212) ** 2 + ((y - 168) / 150) ** 2 < wobble * wobble and y < 226


def build():
    W, H = c.W, c.H
    img = Img(W, H, '#000000')
    # Outside: dusk sky, low sun, layered pines fading into mist.
    img.vgradient(0, 0, W, 190, [(0, '#1C2E44'), (0.45, '#3B5468'), (0.8, '#8C8A84'), (1, '#E0A868')])
    img.glow(300, 158, 120, '#FFD08A', 0.5, ry=50, bands=5)
    img.disc(300, 160, 8, '#FFE6B0')
    c.clouds(img, 31, 40, 120, '#4A5F74', light='#9AA6B0', dark='#33465A', cover=0.4, scale=90, light_from=(300, 160))
    c.ridge(img, 12, 176, 26, 70, '#4E6170', rim='#8E8A80', rim_side=1)
    c.forest(img, 21, 186, 150, 10, 22, '#3A4E5C')
    img.rect(0, 178, W, 10, '#8C9AA2', 0.18)
    c.forest(img, 22, 200, 110, 14, 30, '#2A3B47')
    img.rect(0, 192, W, 8, '#9AA8B0', 0.14)
    c.forest(img, 23, 222, 70, 22, 44, '#1B2830')
    img.vgradient(0, 214, W, 12, [(0, '#1F2C2A'), (1, '#18201E')])
    # The cave: rock everywhere outside the mouth, textured, rimmed where the
    # evening light catches it.
    for y in range(H):
        for x in range(W):
            if opening(x, y):
                continue
            n = fbm2(7, x / 40, y / 14, 4)               # strata, wider than tall
            col = ROCK_L if n > 0.64 else ROCK_D if n < 0.38 else ROCK
            if 0.5 < n < 0.515:
                col = '#1A1512'                           # cracks
            img.put(x, y, col)
    for y in range(H):
        for x in range(W):
            if opening(x, y) and not opening(x, y - 1) and y < 226:
                pass
            if not opening(x, y) and any(opening(x + dx, y + dy) for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1))):
                img.put(x, y, RIM)
                if not opening(x, y - 2) and dither(x, y, 0.5):
                    img.put(x, y - 1, '#4A3D31')
    # Cave floor inside, warm where the lanterns are.
    for y in range(226, H):
        for x in range(W):
            n = fbm2(9, x / 30, y / 8, 3)
            img.put(x, y, '#2A211C' if n > 0.5 else '#231B17')
    img.hline(0, W - 1, 226, '#3B2E25')
    img.glow(36, 250, 90, '#FFB45E', 0.4, ry=40, bands=5)
    img.glow(340, 252, 80, '#FFB45E', 0.34, ry=34, bands=5)
    img.glow(452, 250, 70, '#FFB45E', 0.3, ry=40, bands=4)
    # Moss and pebbles on the floor.
    rng = Rng(5)
    for _ in range(260):
        x, y = rng.int(0, W - 1), rng.int(228, H - 1)
        r = rng.next()
        if r < 0.55:
            img.put(x, y, '#3E5A44')
            img.put(x + 1, y, '#4E7056')
        elif r < 0.8:
            img.rect(x, y, 2, 1, '#4A3C32')
        else:
            img.put(x, y, '#5A4A3C')
    # A woven rug near the middle lantern.
    for yy in range(4):
        for xx in range(34):
            img.put(300 + xx - yy, 258 + yy, ['#8A4A3A', '#C9A65A', '#6A3A2E'][(xx // 3 + yy) % 3])
    # Vines hanging from the rim.
    flowers = [sprite(*SPRITES['daisy']), sprite(*SPRITES['blossom'])]
    rng = Rng(8)
    for i in range(46):
        x = rng.int(20, W - 20)
        top = next((y for y in range(H) if opening(x, y)), None)
        if top is None:
            continue
        length = rng.int(8, 46) if 60 < x < 420 else rng.int(20, 70)
        sway = rng.uniform(-0.12, 0.12)
        prev = None
        for k in range(length):
            vx = int(round(x + math.sin(k * 0.22 + i) * 1.4 + k * sway))
            vy = top - 1 + k
            img.put(vx, vy, STEM)
            if k % 4 == 2:
                side = 1 if (k // 4) % 2 else -1
                img.put(vx + side, vy, LEAF)
                img.put(vx + 2 * side, vy, LEAF_L if k % 8 == 2 else LEAF)
                img.put(vx + side, vy + 1, LEAF_D)
            prev = (vx, vy)
        if prev and rng.next() < 0.45:
            f = flowers[rng.int(0, 1)]
            img.blit(f, prev[0] - f.w // 2, prev[1] - 2)
    # Lantern posts (their light is the animated sprite).
    for (lx, ly) in ((30, 222), (336, 226)):
        img.vline(lx + 3, ly + 8, ly + 26, '#3A2A20')
        img.rect(lx + 1, ly + 26, 5, 1, '#3A2A20')
    sprites = [
        ('lantern.png', c.sheet(lantern_frames()), 6, 30, 214),
        ('lantern.png', None, 6, 336, 218),
        ('mist.png', c.sheet(c.mist_frames(200, 12, 8)), 4, 150, 184),
    ]
    return img, sprites


def lantern_frames():
    out = []
    for f in range(4):
        img = Img(7, 12)
        img.rect(1, 2, 5, 8, '#22262E')
        img.rect(2, 1, 3, 1, '#22262E')
        img.put(3, 0, '#22262E')
        glow = ['#FFD490', '#FFE6A8', '#FFC878', '#FFDA98'][f]
        img.rect(2, 3, 3, 6, glow)
        img.put(3, 5 + (f % 2), '#FFF4C8')
        img.put(3, 4, '#FFF4C8')
        out.append(img)
    return out


def phone(img):
    return img.crop(180, 0, 168, 270)
