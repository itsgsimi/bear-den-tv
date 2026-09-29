"""Den badges in the Classic art style (tools/classicart): the smooth twins of
tools/pixelart/badges.py, on the same 32×32 grid with the same medal (a
round medallion with a gold rim, bear ears on top and two ribbon tails), the
same motifs and the same colours (FACE is read from the pixel module, so the
two styles never drift apart). Every medal also gets a silhouette, the same
drawing in one slate colour, for badges not earned yet.

Writes apps/tv-shell/assets/classic/badge-<id>.svg and badge-<id>-locked.svg,
and the phone remote's copies in apps/remote-web/static/art/.
"""

import importlib.util
import math
import os
import re
import shutil

from svg import Svg

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location('pixel_badges', os.path.join(HERE, '..', 'pixelart', 'badges.py'))
_px = importlib.util.module_from_spec(_spec)
import sys  # noqa: E402
sys.path.insert(0, os.path.join(HERE, '..', 'pixelart'))
_spec.loader.exec_module(_px)
sys.path.pop(0)

FACE = _px.FACE
INK = '#1A1210'
CREAM = '#F6EAD0'
CREAM_D = '#D8C6A2'
OUTLINE = '#120E0C'
BROWN, BROWN_D, BROWN_L = '#8A5A34', '#5A3A20', '#B8844E'
LOCKED, LOCKED_RIM = _px.LOCKED, _px.LOCKED_RIM


def darker(c, t=0.35):
    c = c.lstrip('#')
    r, g, b = (int(c[i:i + 2], 16) for i in (0, 2, 4))
    return '#%02X%02X%02X' % (round(r * (1 - t)), round(g * (1 - t)), round(b * (1 - t)))


def medal(s, face):
    top, bottom, ribbon = face
    for sign in (-1, 1):                                             # ribbon tails
        cx = 15.5 + sign * 7.2
        pts = [(cx - sign * 2.6, 22.2), (cx + sign * 3.2, 22.6), (cx + sign * 3.6, 31), (cx + sign * 0.8, 29.2), (cx - sign * 1.6, 31.2)]
        s.poly(pts, OUTLINE)
        inner = [(x + (15.5 - x) * 0.0, y) for x, y in pts]
        s.poly([(inner[0][0], inner[0][1] + 0.6), (inner[1][0] - sign * 0.7, inner[1][1] + 0.6), (inner[2][0] - sign * 0.7, inner[2][1] - 0.9),
                (inner[3][0], inner[3][1] - 0.9), (inner[4][0] + sign * 0.6, inner[4][1] - 0.9)],
               s.linear([(0, ribbon), (1, darker(ribbon))]))
    for cx in (8.2, 22.8):                                            # bear ears
        s.circle(cx, 6.2, 3.9, OUTLINE)
        s.circle(cx, 6.2, 3.1, s.radial([(0, BROWN_L), (1, BROWN)], fx=0.35, fy=0.3))
        s.circle(cx, 6.1, 1.4, '#D8A870')
    s.circle(15.5, 16, 12.6, OUTLINE)                                 # the rim
    s.circle(15.5, 16, 11.8, s.linear([(0, '#FFE08A'), (0.5, '#E8B84A'), (1, '#9A6A1A')], x1=0, y1=0, x2=1, y2=1))
    s.circle(15.5, 16, 9.9, darker('#9A6A1A', 0.2))
    s.circle(15.5, 16, 9.5, s.linear([(0, top), (1, bottom)]))        # the face
    s.path('M8.2 12.6 A8.4 8.4 0 0 1 19.4 8.4', 'none', extra=' stroke="#FFFFFF" stroke-opacity="0.3" stroke-width="1.1" stroke-linecap="round"')


# --- motifs (inside x 8..23, y 9..24; the pixel twins' shapes, smoothed) ----

def popcorn(s):
    s.path('M9 14 L22 14 L20.2 24 L10.8 24 Z', '#F6F0E6')
    for x0 in (10.2, 14.3, 18.4):
        s.path(f'M{x0} 14 L{x0 + 2} 14 L{x0 + 1.6} 24 L{x0 + 0.5} 24 Z', '#E23A2E')
    for (x, y) in ((10.5, 12.5), (13, 11.2), (16, 12), (18.8, 11.2), (21, 13), (12.5, 13.4), (15.4, 10.2), (18.2, 13.4)):
        s.circle(x, y, 1.8, s.radial([(0, '#FFFFFF'), (1, '#FFE9A8')], fx=0.35, fy=0.3))


def clapper(s):
    s.rect(9, 15, 14, 9, '#20242E', rx=1)
    for x in (11, 15, 19):
        s.rect(x, 17.6, 2, 0.9, CREAM_D, rx=0.3)
    s.rect(10.5, 20.8, 11, 0.9, CREAM_D, rx=0.3)
    s.path('M9 11.4 L22 9 L22.8 12.2 L9.8 14.4 Z', '#20242E')
    for i, x in enumerate((11, 15, 19)):
        dy = -i * 0.72
        s.path(f'M{x} {10.9 + dy} L{x + 2} {10.5 + dy} L{x + 2.6} {13.3 + dy} L{x + 0.6} {13.7 + dy} Z', '#F6F0E6')


def sofa(s):
    s.rect(10, 10.6, 12, 6.4, '#9A6A2A', rx=2)
    s.rect(11, 11.6, 10, 4.4, '#E0A04A', rx=1.5)
    s.rect(8, 15, 16, 6, s.linear([(0, '#F2C47A'), (1, '#C88A3A')]), rx=1.5)
    s.rect(7.6, 13.6, 3.4, 7.6, '#9A6A2A', rx=1.6)
    s.rect(21, 13.6, 3.4, 7.6, '#9A6A2A', rx=1.6)
    s.rect(9, 21, 2, 2, INK, rx=0.5)
    s.rect(21, 21, 2, 2, INK, rx=0.5)
    s.rect(13.8, 11.8, 4.4, 3.2, '#D84A3C', rx=1)


def owl(s):
    s.path('M20 7.8 A3.3 3.3 0 1 0 23.2 12.4 A2.7 2.7 0 1 1 20 7.8 Z', '#FFF6D8')
    s.circle(10, 9, 0.5, '#FFF6D8')
    s.circle(13, 11, 0.4, '#FFF6D8')
    s.ellipse(14, 19, 5.2, 5.2, s.radial([(0, '#A8845E'), (1, '#6A4A30')], fx=0.4, fy=0.3))
    s.path('M9 14.2 L11 11.6 L12.2 15 Z', '#5A3E28')
    s.path('M19 14.2 L17 11.6 L15.8 15 Z', '#5A3E28')
    for cx in (12, 16):
        s.circle(cx, 17, 2.1, '#FFF2C0')
        s.circle(cx, 17.2, 1, INK)
        s.circle(cx - 0.5, 16.6, 0.4, '#FFFFFF')
    s.path('M13 19 L15 19 L14 21.2 Z', '#F2A63A')


def sunrise(s):
    s.circle(15.5, 19, 7.8, s.radial([(0, '#FFE08A', 0.7), (1, '#FFE08A', 0)]))
    s.circle(15.5, 19, 5.4, s.radial([(0, '#FFF6C8'), (1, '#FFD86A')]))
    for k in range(7):
        a = math.pi + k * math.pi / 6
        s.line(15.5 + 7 * math.cos(a), 19 + 7 * math.sin(a), 15.5 + 8.8 * math.cos(a), 19 + 8.8 * math.sin(a), '#FFE08A', 0.9)
    s.path('M6.2 20.4 Q11 18.2 15.5 20.6 T25 20 L24.6 23 A9.5 9.5 0 0 1 6.6 23 Z', s.linear([(0, '#6A9A56'), (1, '#2E5A30')]))


def umbrella(s):
    for (x, y) in ((9, 10), (12, 13), (21, 11), (22, 15), (10, 18), (20, 20)):
        s.line(x, y, x - 0.3, y + 1.4, '#A8D8FF', 0.6)
    s.path('M8 15.6 Q15.5 5.2 23 15.6 Q21.1 14.4 19.3 15.6 Q17.4 14.4 15.5 15.6 Q13.6 14.4 11.7 15.6 Q9.8 14.4 8 15.6 Z',
           s.linear([(0, '#FF7A6A'), (1, '#C0302A')]))
    s.path('M15.5 15.4 L15.5 21.8 Q15.5 23.4 13.9 23.2 Q12.8 23 12.9 21.9', 'none',
           extra=' stroke="#E6E6EC" stroke-width="0.9" stroke-linecap="round"')


def snowflake(s):
    cx, cy = 15.5, 16
    for k in range(6):
        a = k * math.pi / 3 + math.pi / 2
        s.line(cx, cy, cx + 7 * math.cos(a), cy + 7 * math.sin(a), '#FFFFFF', 1.1)
        mx, my = cx + 4.2 * math.cos(a), cy + 4.2 * math.sin(a)
        for sg in (-1, 1):
            b = a + sg * math.pi / 4
            s.line(mx, my, mx + 2.3 * math.cos(b), my + 2.3 * math.sin(b), '#CFE6FF', 0.9)
    s.circle(cx, cy, 1.5, '#FFFFFF')


def storm(s):
    g = s.linear([(0, '#E6E6F0'), (1, '#9A9AB0')])
    for (x, y, r) in ((12, 13.4, 3.4), (16, 11.2, 4), (20, 13.4, 3.2), (15.5, 14.2, 3.4)):
        s.circle(x, y, r, g)
    s.rect(9, 13.8, 13.8, 3.2, g, rx=1.6)
    s.path('M16 16 L19.2 16 L16.6 19.4 L19.2 19.4 L12.8 25.2 L14.9 20.6 L12.4 20.6 Z', s.linear([(0, '#FFF2A0'), (1, '#F2B82A')]))


def seasons(s):
    cx, cy = 15.5, 14
    quads = (('#8AD86A', 180, 270), ('#3A9A3A', 270, 360), ('#E8A03A', 90, 180), ('#FFFFFF', 0, 90))
    for col, a0, a1 in quads:
        r0, r1 = math.radians(a0), math.radians(a1)
        x0, y0 = cx + 7 * math.cos(r0), cy + 6 * math.sin(r0)
        x1, y1 = cx + 7 * math.cos(r1), cy + 6 * math.sin(r1)
        s.path(f'M{cx} {cy} L{x0:.2f} {y0:.2f} A7 6 0 0 1 {x1:.2f} {y1:.2f} Z', col)
    for (x, y) in ((11, 11), (13, 9.2), (10.2, 13)):
        s.circle(x, y, 0.7, '#FF9AC8')
    s.rect(14, 17.6, 3, 6.4, s.linear([(0, BROWN_L), (1, BROWN_D)], x2=1, y2=0), rx=0.8)
    s.rect(10, 23.6, 11.6, 1, '#3A5A3A', rx=0.5)


def brush(s):
    s.path('M8 20.5 L9 19.5 L10 18.5 L11 17.5 L13 17.5', 'none', extra=' stroke="#FFD84A" stroke-width="1.2" stroke-linejoin="miter"')
    s.path('M15 20 Q19 13.2 23.4 20', 'none', extra=' stroke="#7AE0FF" stroke-width="1.3" stroke-linecap="round"')
    s.line(13.5, 20, 20.5, 10, '#B07A48', 1.5)
    s.rect(10.8, 19.6, 3.4, 2.2, '#C9C9D0', rx=0.5)
    s.path('M10.6 21.6 L13.4 21.6 L12 24.4 Z', INK)


def suitcase(s):
    s.path('M12.5 13 L12.5 11 Q12.5 10 13.5 10 L18.5 10 Q19.5 10 19.5 11 L19.5 13', 'none',
           extra=' stroke="#8A4A1E" stroke-width="1.2"')
    s.rect(8, 13, 16, 10, s.linear([(0, '#E09050'), (1, '#B06A2E')]), rx=1.6)
    s.rect(11.8, 13, 1, 10, '#8A4A1E')
    s.rect(18.8, 13, 1, 10, '#8A4A1E')
    s.rect(14, 15, 4, 3, '#4AD8C8', rx=0.8)
    s.circle(10, 19, 1.1, '#FFD84A')
    s.rect(20, 15, 3, 3, '#E86A8A', rx=1.5)


def family(s):
    def head(cx, cy, r, fur, ear, line):
        for ex in (cx - r * 0.72, cx + r * 0.72):
            s.circle(ex, cy - r * 0.72, r * 0.42, line)
            s.circle(ex, cy - r * 0.72, r * 0.3, ear)
        s.circle(cx, cy, r + 0.5, line)
        s.circle(cx, cy, r, s.radial([(0, fur), (1, ear)], fx=0.4, fy=0.3))
        s.ellipse(cx, cy + r * 0.38, r * 0.42, r * 0.27, CREAM)
        s.circle(cx, cy + r * 0.22, r * 0.12, INK)
        for ex in (cx - r * 0.42, cx + r * 0.42):
            s.circle(ex, cy - r * 0.18, r * 0.1, INK)
    head(11, 14, 3.4, '#8A5A34', '#4A2A14', '#2A1810')
    head(20.5, 14.5, 3.0, '#D09A68', '#8A5A30', '#4A2A14')
    head(15.5, 20.5, 2.5, '#F2CC98', '#C08A58', '#6A4424')


def ticket(s):
    s.path('M8 13 L23 11 L23.5 14.6 A1.4 1.4 0 0 0 23.9 17.4 L24 20 L9 22 L8.6 19 A1.4 1.4 0 0 0 8.2 16.2 Z',
           s.linear([(0, '#FFF6D8'), (1, '#F2D898')]))
    s.path('M19 11.8 L19.9 21', 'none', extra=' stroke="#D8B878" stroke-width="0.5" stroke-dasharray="1 1"')
    s.line(11, 15.2, 17, 14.4, '#C0463C', 0.9)
    s.line(11, 17.2, 16, 16.6, '#D8B878', 0.7)
    s.line(11, 19.2, 15, 18.7, '#D8B878', 0.7)
    s.circle(21.5, 16, 1.1, '#C0463C')


def sleepy(s):
    cx, cy, r = 14, 18, 5.5
    for ex in (cx - r * 0.72, cx + r * 0.72):
        s.circle(ex, cy - r * 0.72, r * 0.36, BROWN_D)
    s.circle(cx, cy, r, s.radial([(0, BROWN_L), (1, BROWN)], fx=0.4, fy=0.3))
    s.ellipse(cx, cy + r * 0.42, r * 0.45, r * 0.3, CREAM)
    s.circle(cx, cy + r * 0.25, 0.6, INK)
    s.path('M11 17 Q12 18 13 17 M15 17 Q16 18 17 17', 'none', extra=' stroke="#1A1210" stroke-width="0.6" stroke-linecap="round"')
    s.path('M18 9.2 L21.6 9.2 L18 12.4 L21.6 12.4', 'none', extra=' stroke="#FFFFFF" stroke-width="0.8" stroke-linejoin="round" stroke-linecap="round"')
    s.path('M22 13.2 L23.6 13.2 L22 15 L23.6 15', 'none', extra=' stroke="#CFE6FF" stroke-width="0.6" stroke-linejoin="round" stroke-linecap="round"')


def drum(s):
    s.line(9, 9, 14, 14, '#E6D2A8', 0.9)
    s.line(22, 9, 17, 14, '#E6D2A8', 0.9)
    s.circle(9, 9, 1.1, '#FFFFFF')
    s.circle(22, 9, 1.1, '#FFFFFF')
    s.rect(8.6, 15, 13.8, 7, s.linear([(0, '#6A9AF0'), (1, '#2A4A9A')]))
    s.ellipse(15.5, 22, 6.9, 1.9, '#2A4A9A')
    s.path('M9 16 L11.6 21 L14.2 16 L16.8 21 L19.4 16 L22 21', 'none', extra=' stroke="#FFD84A" stroke-width="0.7" stroke-linejoin="round"')
    s.ellipse(15.5, 15, 6.9, 1.9, s.radial([(0, '#FFFFFF'), (1, '#E6DCC8')]))
    s.ellipse(15.5, 15, 6.9, 1.9, 'none', extra=' stroke="#FFD84A" stroke-width="0.6"')


def calendar(s):
    s.rect(9, 10, 14, 13, '#FFF6E8', rx=1.2)
    s.path('M9 14 L9 11.2 Q9 10 10.2 10 L21.8 10 Q23 10 23 11.2 L23 14 Z', '#C0463C')
    s.rect(10.6, 8.8, 1, 3, INK, rx=0.5)
    s.rect(20.4, 8.8, 1, 3, INK, rx=0.5)
    for x in range(10, 22, 3):
        for y in (15.5, 18.5, 21.5):
            s.circle(x + 0.5, y, 0.35, CREAM_D)
    s.path('M16 21 C12 18.4 12.6 15.6 14.4 15.8 C15.3 15.9 15.8 16.6 16 17.2 C16.2 16.6 16.7 15.9 17.6 15.8 C19.4 15.6 20 18.4 16 21 Z', '#E24A6A')


MOTIFS = {
    'first-night-in': popcorn,
    'movie-night': clapper,
    'couch-explorer': sofa,
    'night-owl': owl,
    'early-cub': sunrise,
    'rainy-day': umbrella,
    'snow-day': snowflake,
    'thunder-buddy': storm,
    'all-seasons': seasons,
    'style-switcher': brush,
    'theme-tourist': suitcase,
    'family-den': family,
    'good-host': ticket,
    'sleepy-bear': sleepy,
    'parade-spotter': drum,
    'loyal-den': calendar,
}

_COLOUR = re.compile(r'#[0-9A-Fa-f]{6}\b')


def silhouette(text):
    """Every colour of the medal becomes slate (the outline stays dark and
    the rim a lighter slate), and a question mark sits on the face."""
    def swap(m):
        c = m.group(0).upper()
        if c == OUTLINE:
            return _px.LOCKED_LINE
        return LOCKED
    body = _COLOUR.sub(swap, text)
    q = (f'  <path d="M13.4 13.6 Q13.4 11.4 15.6 11.4 Q17.8 11.4 17.8 13.4 Q17.8 14.8 15.8 15.8 L15.8 17.4" fill="none" '
         f'stroke="{LOCKED_RIM}" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>\n'
         f'  <circle cx="15.8" cy="19.8" r="0.8" fill="{LOCKED_RIM}"/>\n')
    return body.replace('</svg>', q + '</svg>')


def build(root):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    phone = os.path.join(root, 'apps', 'remote-web', 'static', 'art')
    os.makedirs(out, exist_ok=True)
    files = []
    for bid, draw in MOTIFS.items():
        s = Svg(128, 128, f'{bid} badge', f'Den badge {bid}, Classic: tools/classicart/badges.py', 32, 32)
        medal(s, FACE[bid])
        draw(s)
        text = s.text()
        for name, body in ((f'badge-{bid}.svg', text), (f'badge-{bid}-locked.svg', silhouette(text))):
            path = os.path.join(out, name)
            with open(path, 'w', newline='\n') as f:
                f.write(body)
            shutil.copyfile(path, os.path.join(phone, name))
            files += [path, os.path.join(phone, name)]
    return files
