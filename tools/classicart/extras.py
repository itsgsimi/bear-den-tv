"""Small Classic pieces with no older SVG (tools/classicart):

- apps/tv-shell/assets/ornaments/pumpkin.svg: the October pumpkin (ornament;
  the pixel one is pumpkin.png from tools/pixelart/ornaments.py);
- apps/tv-shell/assets/classic/snowcap.svg: December snow on the featured
  panel's top edge, a tile that repeats seamlessly (128×20, the pixel tile's
  32×5 at 4×);
- apps/tv-shell/assets/classic/sleep-z.svg: the "z" that floats over a dozing
  bear (the pixel one is scene-moon-z.png, 5×5 art pixels).
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


def build(root):
    orn = os.path.join(root, 'apps', 'tv-shell', 'assets', 'ornaments', 'pumpkin.svg')
    cl = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    os.makedirs(cl, exist_ok=True)
    pumpkin(orn)
    snowcap(os.path.join(cl, 'snowcap.svg'))
    sleep_z(os.path.join(cl, 'sleep-z.svg'))
    return [orn, os.path.join(cl, 'snowcap.svg'), os.path.join(cl, 'sleep-z.svg')]
