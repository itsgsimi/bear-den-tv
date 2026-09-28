"""Home's corner scenes in pixel art (tools/pixelart): the props each scene is
drawn on, 110×82 art pixels (440×330 on a 1080p TV), plus their animated
parts. The bears themselves are the sprites from bears.py, placed by the
scene's QML (CampfireScene, CampScene, MoonScene, DenFamily). Writes
apps/tv-shell/assets/pixel/scene-*.png.
"""

import math
import os

from px import Img, Rng, dither
import common as c
from ornaments import SPRITES, sprite

SW, SH = 110, 82
OUT = '#15110E'


def campfire():
    img = Img(SW, SH)
    # Two sitting logs (bark, cut ends with rings).
    for (x0, y0, w, flip) in ((2, 70, 30, False), (78, 71, 30, True)):
        img.rect(x0, y0, w, 7, '#6B4A30')
        img.hline(x0 + 2, x0 + w - 6, y0 + 2, '#825B3B')
        img.hline(x0 + 4, x0 + w - 3, y0 + 5, '#553A26')
        ex = x0 + (0 if flip else w - 7)
        img.ellipse(ex + 3, y0 + 3, 3, 3, '#C89A68')
        img.ellipse(ex + 3, y0 + 3, 1, 1, '#A87B50')
    # The fire pit: stones in a ring, crossed logs.
    for i in range(9):
        a = math.pi * (0.05 + 0.9 * i / 8)
        x = 55 + math.cos(a) * 16
        y = 76 - math.sin(a) * 3
        img.ellipse(x, y, 3, 2, '#6E6A66' if i % 2 else '#85807A')
        img.put(int(x) - 1, int(y) - 1, '#A9A39C')
    img.line(42, 76, 66, 70, '#4A3222')
    img.line(42, 75, 66, 69, '#6B4A30')
    img.line(46, 70, 68, 76, '#5A3E28')
    img.line(46, 69, 68, 75, '#7A5536')
    img.outline(OUT)
    fire = []
    flames = c.flame_frames(18, 26, 6, seed=8)
    small = c.flame_frames(10, 14, 6, seed=2)
    for f in range(6):
        fr = Img(48, 44)
        fr.glow(24, 36, 24 + (f % 2), '#FF8A3D', 0.3 + 0.05 * (f % 3), ry=14, bands=4)
        fr.blit(flames[f], 15, 12)
        fr.blit(small[(f + 3) % 6], 8, 24)
        fr.blit(small[f], 30, 25)
        fire.append(fr)
    return img, {'fire': c.sheet(fire)}


def camp():
    img = Img(SW, SH)
    # Two pines, kept whole inside the scene (the scene's edge would crop them).
    c.pine(img, 18, 80, 52, '#1E3A2A', rim='#3C604F', rim_side=1)
    c.pine(img, 92, 81, 46, '#1E3A2A', rim='#3C604F', rim_side=-1)
    # The tent: sunlit canvas, a shaded side, an open door glowing inside.
    top, left, right, base = (50, 20), 14, 88, 80
    img.poly([(left, base), top, (right, base)], '#C9A36A')
    img.poly([top, (right, base), (60, base)], '#9C7A4C')
    img.line(top[0], top[1], left, base, '#E0C28E')
    door = [(34, base), (50, 44), (66, base)]
    back = Img(SW, SH)
    back.poly(door, '#2A2118')
    back.glow(50, 76, 16, '#FFC46E', 0.55, ry=12, bands=4)
    for y in range(SH):                                   # the door opening: see through
        for x in range(SW):
            if back.get(x, y)[3]:
                img.clear(x, y)
    img.poly([(34, base), (50, 44), (40, base)], '#B08A58')        # tied-back flap
    img.line(top[0], top[1] - 3, top[0], top[1], '#6B4A32')
    img.line(left, base, left - 6, base - 2, '#8A7A60')          # guy ropes
    img.line(right, base, right + 6, base - 2, '#8A7A60')
    # The lantern post.
    img.rect(96, 36, 2, 45, '#6B4A32')
    img.rect(86, 35, 12, 2, '#6B4A32')
    img.vline(88, 37, 40, '#3B424A')
    img.outline(OUT)
    lantern = []
    for f in range(4):
        fr = Img(21, 21)
        fr.glow(10, 12, 10, '#FFC46E', 0.32 + 0.08 * (f % 2), bands=3)
        fr.rect(7, 6, 7, 10, '#22262E')
        fr.rect(8, 5, 5, 1, '#22262E')
        fr.rect(8, 7, 5, 8, ['#FFD490', '#FFE6A8', '#FFC878', '#FFDA98'][f])
        fr.put(10, 10 + f % 2, '#FFF4C8')
        lantern.append(fr)
    return img, {'lantern': c.sheet(lantern), 'back': back}


def moon():
    img = Img(SW, SH)
    # Threads from above, stars hanging on them, the crescent in the middle.
    for (x, y1) in ((42, 26), (14, 30), (98, 38), (6, 58)):
        img.vline(x, 0, y1, '#9DB2DD', 0.7)
    star = sprite(*SPRITES['star'])
    for (x, y) in ((11, 30), (95, 38), (3, 58)):
        img.blit(star, x, y)
    cx, cy, r = 56, 50, 26
    for y in range(cy - r, cy + r + 1):
        for x in range(cx - r, cx + r + 1):
            d1 = math.hypot(x - cx, y - cy)
            d2 = math.hypot(x - (cx + 11), y - (cy - 8))
            if d1 <= r and d2 > r - 3:
                col = '#F2F5FC' if d1 < r - 3 else '#C8D4EE'
                if (x - cx) * 0.6 + (y - cy) > 8 and d1 > r - 7:
                    col = '#B7C3DF'
                img.put(x, y, col)
    for (x, y, rr) in ((40, 52, 2), (46, 66, 3), (38, 40, 1), (54, 72, 1)):
        img.disc(x, y, rr, '#D2DAEE')
    z = Img(5, 5)
    for x in range(5):
        z.put(x, 0, '#EDE3D1')
        z.put(x, 4, '#EDE3D1')
        z.put(4 - x, x, '#EDE3D1')
    return img, {'z': z}


def den():
    img = Img(SW, SH)
    rng = Rng(4)
    # A rocky nook: a rounded arch of stone around a dark, lantern-lit hollow.
    for y in range(SH):
        for x in range(SW):
            outer = ((x - 55) / 55) ** 2 + ((y - 84) / 72) ** 2
            inner = ((x - 55) / 42) ** 2 + ((y - 86) / 60) ** 2
            if outer <= 1 and inner > 1:
                n = c.fbm2(3, x / 9, y / 7, 3)
                img.put(x, y, '#5A5048' if n > 0.6 else '#433A33' if n > 0.4 else '#352D28')
    back = Img(SW, SH)
    for y in range(SH):
        for x in range(SW):
            if ((x - 55) / 42) ** 2 + ((y - 86) / 60) ** 2 <= 1:
                back.put(x, y, '#1A1411')
    back.glow(55, 80, 40, '#E3B35C', 0.4, ry=34, bands=5)
    for _ in range(40):                                  # moss along the arch
        a = rng.uniform(math.pi * 1.05, math.pi * 1.95)
        x = 55 + math.cos(a) * rng.uniform(44, 54)
        y = 84 + math.sin(a) * rng.uniform(62, 70)
        img.put(x, y, '#4E7A5C')
        img.put(x + 1, y, '#6A9C7A')
    sprig, daisy = sprite(*SPRITES['sprig']), sprite(*SPRITES['daisy'])
    img.blit(sprig, 0, 70)
    img.blit(daisy, 6, 64)
    img.blit(daisy, 96, 60)
    img.outline(OUT)
    return img, {'back': back}


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    os.makedirs(out, exist_ok=True)
    board = Img(SW * 4 + 10, SH * 2 + 60, '#2A2F3A')
    for i, (name, fn) in enumerate((('campfire', campfire), ('camp', camp), ('moon', moon), ('den', den))):
        img, extra = fn()
        img.save(os.path.join(out, f'scene-{name}.png'))
        for k, s in extra.items():
            s.save(os.path.join(out, f'scene-{name}-{k}.png'))
        board.blit(img, 2 + i * (SW + 2), 2)
        y = SH + 4
        for s in extra.values():
            board.blit(s.crop(0, 0, min(s.w, SW), s.h), 2 + i * (SW + 2), y)
            y += s.h + 2
    if preview:
        os.makedirs(preview, exist_ok=True)
        board.scaled(3).save(os.path.join(preview, 'scenes.png'))
    print('scenes: campfire, camp, moon, den')
