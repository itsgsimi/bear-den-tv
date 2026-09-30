"""Small Classic pieces with no older SVG (tools/classicart):

- apps/tv-shell/assets/ornaments/pumpkin.svg: the October pumpkin (ornament;
  the pixel one is pumpkin.png from tools/pixelart/ornaments.py);
- apps/tv-shell/assets/ornaments/hat-beret.svg, phone.svg, pointer-up.svg:
  the bear tips' painter's beret, tiny phone and sign arrow (ornaments; the
  pixel ones come from tools/pixelart/ornaments.py);
- apps/tv-shell/assets/classic/snowcap.svg: December snow on the featured
  panel's top edge, a tile that repeats seamlessly (128×20, the pixel tile's
  32×5 at 4×);
- apps/tv-shell/assets/classic/sleep-z.svg: the "z" that floats over a dozing
  bear (the pixel one is scene-moon-z.png, 5×5 art pixels);
- the weather props of the corner scenes and visiting bears (the pixel ones
  are scene-wx-*.png from tools/pixelart/weatherprops.py, same proportions):
  apps/tv-shell/assets/classic/scene-tarp.svg (a tarp on two poles, 200×216),
  startle.svg (the "!" over a startled bear, 20×36) and umbrella-leaf.svg
  (the leaf canopy a bear holds in the rain, 104×44).
"""

import math
import os

from svg import Svg, n


def pumpkin(path):
    s = Svg(70, 64, 'pumpkin', 'October pumpkin ornament, Classic: tools/classicart/extras.py')
    s.ellipse(35, 60, 28, 3.5, s.radial([(0, '#000000', 0.35), (1, '#000000', 0)]))
    body = s.linear([(0, '#FFB14A'), (0.55, '#F07F22'), (1, '#B8521A')])
    side = s.linear([(0, '#F59A36'), (1, '#A94A16')])
    for cx, rx in ((17, 15), (53, 15)):
        s.ellipse(cx, 38, rx, 20, side, stroke='#7A3510', sw=1.5)
    for cx, rx in ((26, 14), (44, 14)):
        s.ellipse(cx, 38, rx, 21.5, body, stroke='#8A3E12', sw=1.5)
    s.ellipse(35, 38, 11, 22.5, body, stroke='#8A3E12', sw=1.5)
    s.ellipse(31, 27, 4, 8, '#FFE0A8', 0.35)
    # Stem and a curling vine leaf.
    s.path('M33 18 Q32 10 36 5 Q39 6 38.5 9 Q36 12 37.5 18 Z', s.linear([(0, '#8BA356'), (1, '#4E6630')]),
           stroke='#3A4A22', sw=1.2)
    s.path('M38 12 Q48 4 56 10 Q49 16 38 12 Z', s.linear([(0, '#9CC468'), (1, '#5E8A3A')]), stroke='#3A5A22', sw=1.2)
    s.path('M36 14 Q30 12 28 8 Q27 5 30 5', 'none', stroke='#6E8A3E', sw=1.4)
    s.save(path)


def snowcap(path):
    w, h = 128, 20
    s = Svg(w, h, 'snow', 'December snowcap tile (repeats sideways), Classic: tools/classicart/extras.py')

    def top(x):
        t = x / w * 2 * math.pi
        return 5.5 + 2.4 * math.sin(t) + 1.4 * math.sin(3 * t + 1) + 0.8 * math.sin(5 * t + 2)

    xs = [i * 4 for i in range(w // 4 + 1)]
    d = f'M0 {n(top(0))}' + ''.join(f' L{n(x)} {n(top(x))}' for x in xs[1:])
    s.path(d + f' L{w} 17 Q{w * 0.75} 18.5 {w / 2} 17 Q{w / 4} 15.5 0 17 Z',
           s.linear([(0, '#FFFFFF'), (0.6, '#F1F7FF'), (1, '#C8D8EA')]))
    s.path(d, 'none', stroke='#FFFFFF', sw=1.2)
    s.path(f'M0 17 Q{w / 4} 15.5 {w / 2} 17 Q{w * 0.75} 18.5 {w} 17 L{w} 20 L0 20 Z',
           s.linear([(0, '#8FA6C2', 0.55), (1, '#8FA6C2', 0)]))
    s.save(path)


def sleep_z(path):
    s = Svg(20, 20, 'z', 'The "z" over a dozing bear, Classic: tools/classicart/extras.py')
    d = 'M4 4.5 H16 L7.5 15 H16'
    s.path(d, 'none', stroke='#17130F', sw=5, extra=' stroke-opacity="0.5"')
    s.path(d, 'none', stroke='#EEF2FF', sw=2.8)
    s.save(path)


def tarp(path):
    s = Svg(200, 216, 'tarp', 'Rain tarp over a corner scene, Classic: tools/classicart/extras.py')
    pole = s.linear([(0, '#8A6443'), (1, '#5A3E28')], 0, 0, 1, 0)
    for x in (12, 184):
        s.rect(x, 14, 7, 202, pole, rx=3)
    canvas = s.linear([(0, '#8FA466'), (0.5, '#6B8050'), (1, '#4C5E3A')])
    s.path('M0 8 Q100 34 200 8 L200 26 Q100 50 0 26 Z', canvas, stroke='#3A4A2A', sw=2)
    for x in (50, 100, 150):
        s.line(x, 12 + 18 * (1 - ((x - 100) / 100) ** 2), x, 30 + 18 * (1 - ((x - 100) / 100) ** 2), '#3E5030', sw=1.5)
    s.path('M0 8 Q100 34 200 8', 'none', stroke='#B4C88A', sw=2)
    s.ellipse(100, 44, 2.6, 3.6, '#CFE0F6', 0.9)
    s.ellipse(101, 52, 2, 2.8, '#CFE0F6', 0.7)
    s.save(path)


def startle(path):
    s = Svg(20, 36, '!', 'The "!" over a bear startled by lightning, Classic: tools/classicart/extras.py')
    s.path('M5 3 H15 L12.5 22 H7.5 Z', '#FFE6A8', stroke='#17130F', sw=2.5)
    s.circle(10, 29.5, 4, '#FFE6A8', stroke='#17130F', sw=2.5)
    s.path('M9 5 H11 L10.3 18 H9.7 Z', '#FFFFFF', 0.8)
    s.save(path)


def umbrella(path):
    s = Svg(104, 44, 'leaf', 'Leaf umbrella for a bear in the rain, Classic: tools/classicart/extras.py')
    body = s.linear([(0, '#8CC46E'), (0.6, '#6FA35A'), (1, '#4E7A3E')])
    rim = ''.join(f' Q{n(96 - i * 12 + 6)} {n(44 if i % 2 else 38)} {n(96 - (i + 1) * 12)} 40' for i in range(7))
    s.path('M4 40 Q6 4 52 2 Q98 4 100 40 L96 40' + rim + ' Z', body, stroke='#2F4A24', sw=2.2)
    s.path('M52 3 L52 40', 'none', stroke='#C4E09A', sw=2)
    for (x0, y0, x1, y1) in ((52, 18, 22, 34), (52, 12, 30, 22), (52, 18, 82, 34), (52, 12, 74, 22)):
        s.path(f'M{x0} {y0} Q{(x0 + x1) / 2} {y1 - 8} {x1} {y1}', 'none', stroke='#A6D07E', sw=1.4)
    s.save(path)


def beret(path):
    s = Svg(100, 44, 'beret', "The Themes tip bear's painter's beret, Classic: tools/classicart/extras.py")
    felt = s.linear([(0, '#E0685C'), (0.6, '#C8453D'), (1, '#8E2C2A')])
    s.rect(47, 2, 6, 9, '#5A2320', rx=3)
    s.path('M4 30 Q8 8 50 8 Q94 8 98 26 Q96 36 50 38 Q8 38 4 30 Z', felt, stroke='#6E2422', sw=2.2)
    s.path('M14 34 Q50 42 88 32', 'none', stroke='#6E2422', sw=3)
    s.ellipse(34, 20, 6, 4, '#F2C14E')
    s.ellipse(42, 25, 4, 3, '#7FB56F')
    s.path('M20 18 Q40 10 66 12', 'none', stroke='#F2A094', sw=2, extra=' stroke-opacity="0.6"')
    s.save(path)


def phone(path):
    s = Svg(40, 64, 'phone', "The tiny phone a phone tip's bear holds, Classic: tools/classicart/extras.py")
    s.rect(3, 2, 34, 60, '#2B3036', rx=7, stroke='#17130F', sw=2)
    s.rect(7, 8, 26, 42, s.linear([(0, '#9CC2F2'), (1, '#4E7FC4')]), rx=3)
    s.circle(20, 29, 7, '#EEF2FF', 0.85)
    s.path('M17.5 25 L24.5 29 L17.5 33 Z', '#4E7FC4')
    s.circle(20, 56, 2.6, '#8A93A6')
    s.save(path)


def pointer_up(path):
    s = Svg(40, 48, 'arrow', "The wooden arrow on a bear tip's sign, Classic: tools/classicart/extras.py")
    wood = s.linear([(0, '#E3C08A'), (0.5, '#A87A50'), (1, '#6B4A30')], 0, 0, 1, 0)
    s.path('M20 2 L38 22 L27 22 L27 46 L13 46 L13 22 L2 22 Z', wood, stroke='#3A2818', sw=2.4)
    s.path('M20 7 L31 19', 'none', stroke='#F2DDB4', sw=1.6, extra=' stroke-opacity="0.7"')
    s.save(path)


def build(root):
    orn = os.path.join(root, 'apps', 'tv-shell', 'assets', 'ornaments', 'pumpkin.svg')
    cl = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    os.makedirs(cl, exist_ok=True)
    pumpkin(orn)
    orns = os.path.dirname(orn)
    beret(os.path.join(orns, 'hat-beret.svg'))
    phone(os.path.join(orns, 'phone.svg'))
    pointer_up(os.path.join(orns, 'pointer-up.svg'))
    snowcap(os.path.join(cl, 'snowcap.svg'))
    sleep_z(os.path.join(cl, 'sleep-z.svg'))
    tarp(os.path.join(cl, 'scene-tarp.svg'))
    startle(os.path.join(cl, 'startle.svg'))
    umbrella(os.path.join(cl, 'umbrella-leaf.svg'))
    return [orn] + [os.path.join(orns, f) for f in ('hat-beret.svg', 'phone.svg', 'pointer-up.svg')] + [os.path.join(cl, f) for f in ('snowcap.svg', 'sleep-z.svg', 'scene-tarp.svg', 'startle.svg',
                                                   'umbrella-leaf.svg')]
