"""The props the world puts on when the local weather reaches the scene
(tools/pixelart; guide docs/THEMES.md → Weather in the corner scene):

- scene-wx-tarp.png: a sagging canvas tarp on two poles that a corner scene
  can pitch over something to keep it dry (the campfire, in the rain);
- scene-wx-startle.png: the "!" that pops over a bear startled by lightning;
- scene-wx-umbrella.png: a big leaf a visiting bear holds over its head in
  the rain (the stem is drawn by BearPuppet.qml, from the paw up).

Classic twins: tools/classicart/extras.py (assets/classic/scene-tarp.svg,
startle.svg, umbrella-leaf.svg). Writes apps/tv-shell/assets/pixel/.
"""

import os

from px import Img

OUT = '#15110E'
POLE, POLE_LIT = '#6B4A30', '#8A6443'
CANVAS, CANVAS_LIT, CANVAS_DARK = '#5F7447', '#7E9458', '#465736'
DROP = '#B8D0EE'


def tarp():
    """50×54: canvas along the top (sagging in the middle), poles to the
    ground at x 3 and 46, a drop hanging from the lowest point."""
    img = Img(50, 54)
    for x in (3, 46):
        img.vline(x, 4, 53, POLE)
        img.vline(x + 1, 4, 53, POLE_LIT)
    for x in range(0, 50):
        sag = round(3 * (1 - ((x - 24.5) / 25) ** 2))       # 0 at the poles, 3 in the middle
        top = 1 + sag
        img.put(x, top, CANVAS_LIT)
        img.vline(x, top + 1, top + 3, CANVAS)
        img.put(x, top + 4, CANVAS_DARK)
    for x in (12, 24, 37):                                   # seams
        sag = round(3 * (1 - ((x - 24.5) / 25) ** 2))
        img.vline(x, 2 + sag, 4 + sag, CANVAS_DARK)
    img.outline(OUT)
    img.put(24, 9, DROP)
    img.put(25, 11, DROP)
    return img


def startle():
    """5×9: a warm "!" with a dark outline."""
    img = Img(5, 9)
    img.rect(1, 0, 3, 5, '#FFE6A8')
    img.vline(2, 0, 4, '#FFFFFF')
    img.rect(1, 7, 3, 2, '#FFE6A8')
    img = pad(img)
    img.outline(OUT)
    return img


def umbrella():
    """24×10: a leaf canopy, a dome with a midrib and veins, a notched rim."""
    img = Img(24, 10)
    for y in range(10):
        for x in range(24):
            dx, dy = (x - 11.5) / 11.5, (9 - y) / 9
            if dx * dx + dy * dy <= 1 and y <= 8:
                img.put(x, y, '#6FA35A' if dy > 0.5 else '#5E8F4C')
    for x in range(1, 23, 4):                                # notched rim
        img.clear(x, 8)
    img.vline(11, 1, 8, '#A8D07E')                           # midrib
    for (x0, x1) in ((4, 10), (13, 19)):                     # veins
        img.line(x0, 7, x1, 3, '#8CBF6A')
    img.put(11, 0, '#4E7A3E')
    img = pad(img)
    img.outline(OUT)
    return img


def pad(img):
    """One transparent pixel all round, so the outline fits."""
    out = Img(img.w + 2, img.h + 2)
    out.blit(img, 1, 1)
    return out


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    os.makedirs(out, exist_ok=True)
    parts = (('tarp', tarp()), ('startle', startle()), ('umbrella', umbrella()))
    board = Img(120, 60, '#2A2F3A')
    x = 2
    for name, img in parts:
        img.save(os.path.join(out, f'scene-wx-{name}.png'))
        board.blit(img, x, 2)
        x += img.w + 4
    if preview:
        os.makedirs(preview, exist_ok=True)
        board.scaled(4).save(os.path.join(preview, 'weatherprops.png'))
    print('weatherprops: tarp, startle, umbrella')
