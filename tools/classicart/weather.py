"""Classic weather icons (tools/classicart): the smooth counterparts of
tools/pixelart/weather.py, used by the Home header chip and Settings → Weather
in the Classic art style. Writes apps/tv-shell/assets/classic/weather-<name>.svg:

    sun, moon, sun-cloud, moon-cloud, cloud, fog, drizzle, rain, snow, thunder

Each icon is a 64×64 drawing (the pixel icons are 16×16 art pixels, shown at
the same size on screen) built from the same parts (sun, moon, cloud, drops,
flakes, bolt), with a soft dark rim so it reads over any wallpaper.
"""

import math
import os

from svg import Svg, n

RIM = '#17130F'
RIM_W = 3.2


def sun(s, cx, cy, r, rays=True):
    if rays:
        for k in range(8):
            a = k * math.pi / 4
            x1, y1 = cx + math.cos(a) * r * 1.35, cy + math.sin(a) * r * 1.35
            x2, y2 = cx + math.cos(a) * r * 1.8, cy + math.sin(a) * r * 1.8
            s.line(x1, y1, x2, y2, RIM, r * 0.32 + RIM_W, 0.55)
            s.line(x1, y1, x2, y2, '#F7C95A', r * 0.32)
    s.circle(cx, cy, r + RIM_W / 2, RIM, 0.6)
    s.glow(cx, cy, r * 2.2, r * 2.2, '#FFE9A0', 0.35)
    s.circle(cx, cy, r, s.radial([(0, '#FFF8D8'), (0.55, '#FFE08A'), (1, '#E9A93A')], fx=0.38, fy=0.36))


def moon(s, cx, cy, r):
    top, bot = (cx + r * 0.25, cy - r * 0.97), (cx + r * 0.25, cy + r * 0.97)
    d = (f'M{n(top[0])} {n(top[1])} A{n(r)} {n(r)} 0 1 0 {n(bot[0])} {n(bot[1])} '
         f'A{n(r * 0.78)} {n(r * 0.78)} 0 0 1 {n(top[0])} {n(top[1])} Z')
    s.glow(cx, cy, r * 1.5, r * 1.5, '#DCE6FF', 0.25)
    s.path(d, 'none', stroke=RIM, sw=RIM_W, extra=' stroke-opacity="0.6"')
    s.path(d, s.radial([(0, '#FFFFFF'), (0.6, '#EEF2FF'), (1, '#AEBBDA')], cx=0.3, cy=0.4, r=0.8))


def cloud(s, x, y, k, tone='light'):
    """A cloud whose base line is at y, left edge at x, k = size."""
    puffs = [(x + 11 * k, y - 8 * k, 8 * k), (x + 22 * k, y - 14 * k, 11 * k), (x + 33 * k, y - 9 * k, 8.5 * k)]
    base = (x + 3 * k, y - 11 * k, 38 * k, 11 * k)
    top, bottom = {'light': ('#FFFFFF', '#C9D2E0'), 'grey': ('#C4CBD8', '#7E889C'),
                   'dark': ('#9AA3B5', '#586277')}[tone]
    for cx, cy, r in puffs:
        s.circle(cx, cy, r, RIM, stroke=RIM, sw=RIM_W * 2, extra=' stroke-opacity="0.6" fill-opacity="0.6"')
    s.rect(*base, RIM, rx=5.5 * k, stroke=RIM, sw=RIM_W * 2, extra=' stroke-opacity="0.6" fill-opacity="0.6"')
    g = s.linear_abs([(0, top), (1, bottom)], 0, y - 25 * k, 0, y)
    for cx, cy, r in puffs:
        s.circle(cx, cy, r, g)
    s.rect(*base, g, rx=5.5 * k)
    # A soft highlight on the top puff.
    s.ellipse(x + 19 * k, y - 19 * k, 5 * k, 3 * k, '#FFFFFF', 0.45)


def drops(s, xs, y, length):
    for i, x in enumerate(xs):
        yy = y + (i % 2) * length * 0.5
        d = (f'M{n(x)} {n(yy)} Q{n(x + 3.4)} {n(yy + length * 0.62)} {n(x + 2.2)} {n(yy + length * 0.8)} '
             f'A3 3 0 0 1 {n(x - 3.2)} {n(yy + length * 0.66)} Q{n(x - 2)} {n(yy + length * 0.3)} {n(x)} {n(yy)} Z')
        s.path(d, 'none', stroke=RIM, sw=RIM_W, extra=' stroke-opacity="0.55"')
        s.path(d, s.linear([(0, '#A8D0FF'), (1, '#4A7FC4')]))


def flakes(s, xs, y):
    for i, x in enumerate(xs):
        yy = y + (i % 2) * 7
        for a in range(3):
            ang = a * math.pi / 3
            dx, dy = math.cos(ang) * 4.2, math.sin(ang) * 4.2
            s.line(x - dx, yy - dy, x + dx, yy + dy, RIM, 2 + RIM_W, 0.5)
        for a in range(3):
            ang = a * math.pi / 3
            dx, dy = math.cos(ang) * 4.2, math.sin(ang) * 4.2
            s.line(x - dx, yy - dy, x + dx, yy + dy, '#F4FAFF', 2)


def bolt(s, x, y):
    p = [(x + 6, y), (x - 3, y + 13), (x + 3, y + 13), (x - 2, y + 25), (x + 11, y + 9), (x + 4, y + 9), (x + 9, y)]
    d = 'M' + ' L'.join(f'{n(a)} {n(b)}' for a, b in p) + ' Z'
    s.path(d, 'none', stroke=RIM, sw=RIM_W, extra=' stroke-opacity="0.7"')
    s.path(d, s.linear([(0, '#FFF3B0'), (1, '#F2B63A')]))


def fog(s):
    for i, (x0, x1, y, col) in enumerate(((8, 44, 20, '#F7F3EA'), (18, 58, 30, '#C9CFDA'),
                                         (4, 48, 40, '#F7F3EA'), (14, 54, 50, '#C9CFDA'))):
        s.line(x0, y, x1, y, RIM, 6 + RIM_W, 0.5)
        s.line(x0, y, x1, y, col, 6)


ICONS = {
    'sun': lambda s: sun(s, 32, 32, 13),
    'moon': lambda s: moon(s, 30, 32, 19),
    'sun-cloud': lambda s: (sun(s, 24, 24, 10), cloud(s, 12, 54, 1.25)),
    'moon-cloud': lambda s: (moon(s, 26, 24, 14), cloud(s, 12, 54, 1.25)),
    'cloud': lambda s: (cloud(s, 18, 36, 1.05, 'grey'), cloud(s, 4, 52, 1.3)),
    'fog': fog,
    'drizzle': lambda s: (cloud(s, 6, 38, 1.3, 'grey'), drops(s, (20, 34, 48), 43, 10)),
    'rain': lambda s: (cloud(s, 6, 36, 1.3, 'dark'), drops(s, (14, 25, 36, 47), 40, 16)),
    'snow': lambda s: (cloud(s, 6, 36, 1.3), flakes(s, (15, 29, 43), 46)),
    'thunder': lambda s: (cloud(s, 6, 34, 1.3, 'dark'), drops(s, (14, 48), 40, 13), bolt(s, 27, 36)),
}


def build(root):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    os.makedirs(out, exist_ok=True)
    files = []
    for name, draw in ICONS.items():
        s = Svg(64, 64, f'weather: {name}', f'Weather icon "{name}", Classic: tools/classicart/weather.py')
        draw(s)
        path = os.path.join(out, f'weather-{name}.svg')
        s.save(path)
        files.append(path)
    return files
