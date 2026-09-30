"""Bear Den's UI icons in the Classic art style (tools/classicart; guide
docs/THEMES.md → UI icons): the smooth twins of tools/pixelart/uiicons.py, on
the same 32×32 grid, in the app icons' badge (tools/classicart/appicons.py:
a rounded square with bear ears on top) and the same colours (BADGE and
PHONE are read from the pixel module, so the two styles never drift apart).
Original art, never a third-party logo:

    gear     a chunky cog                         (Settings)
    apps     a 2×2 grid of little tiles           (Apps)
    themes   a painter's palette with a brush     (Themes)
    phone    a phone with a bear face on screen   (Pair phone; Phones & remote)
    display  a TV screen with an eye              (Display & accessibility)
    home     a little log cabin                   (Home screen)
    play     a play triangle on a film frame      (Playback)
    power    a crescent moon with a power symbol  (Power & TV)
    about    a round "i"                          (About)
    plus     a big bold "+"                       (Add apps)
    globe    a wireframe globe                    (Streaming sites)
    refresh  two circular arrows                  (Keep apps up to date)
    medal    a gold medal with a paw              (Den badges)
    wave     a bear paw waving                    (Welcome / setup)

Each motif is drawn twice: first in the badge's ear colour, nudged down and
right, as a solid drop shadow; then in cream with light from the top left.
Only SVG Tiny 1.2: paths (M, L, Q, Z), circles, ellipses, rects, gradients and
a translated group; no arcs, filters, masks or clip paths.

Writes apps/tv-shell/assets/classic/icon-<name>.svg and, for the icons the
phone remote shows, copies in apps/remote-web/static/art/icon-<name>.svg.
"""

import importlib.util
import math
import os
import shutil
import sys

from appicons import badge
from svg import Svg, n

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location('pixel_uiicons', os.path.join(HERE, '..', 'pixelart', 'uiicons.py'))
_px = importlib.util.module_from_spec(_spec)
sys.path.insert(0, os.path.join(HERE, '..', 'pixelart'))
_saved = sys.modules.get('appicons')
sys.modules.pop('appicons', None)       # the pixel module imports its own appicons
_spec.loader.exec_module(_px)
if _saved is not None:
    sys.modules['appicons'] = _saved
sys.path.pop(0)

BADGE = _px.BADGE
PHONE = _px.PHONE
CREAM, CREAM_D, CREAM_L = _px.CREAM, _px.CREAM_D, _px.CREAM_L
INK = _px.INK
BROWN, BROWN_D, BROWN_L = _px.BROWN, _px.BROWN_D, _px.BROWN_L
GOLD, GOLD_D, GOLD_L = _px.GOLD, _px.GOLD_D, _px.GOLD_L


def mixh(a, b, t):
    a, b = a.lstrip('#'), b.lstrip('#')
    ca = [int(a[i:i + 2], 16) for i in (0, 2, 4)]
    cb = [int(b[i:i + 2], 16) for i in (0, 2, 4)]
    return '#%02X%02X%02X' % tuple(round(ca[i] + (cb[i] - ca[i]) * t) for i in range(3))


class Paint:
    """Colours for one pass: the real ones, or all the shadow colour."""

    def __init__(self, s, shadow=None):
        self.shadow = shadow
        self.cream = shadow or s.linear([(0, CREAM_L), (0.55, CREAM), (1, CREAM_D)], x1=0, y1=0, x2=1, y2=1)

    def __call__(self, col):
        return self.shadow or col


def circle_pts(cx, cy, rx, ry=None, k=32, a0=0.0, a1=360.0):
    ry = rx if ry is None else ry
    out = []
    for i in range(k + 1 if a1 - a0 < 360 else k):
        a = math.radians(a0 + (a1 - a0) * i / k)
        out.append((cx + rx * math.cos(a), cy + ry * math.sin(a)))
    return out


def d_of(points, close=True):
    return 'M' + ' L'.join(f'{n(x)} {n(y)}' for x, y in points) + (' Z' if close else '')


# --- motifs -------------------------------------------------------------------

def gear(s, P, pal):
    cx, cy = 16.5, 18.5
    outer = []
    for k in range(8):
        a = k * math.pi / 4
        for (r, da) in ((6.8, -0.36), (10, -0.17), (10, 0.17), (6.8, 0.36)):
            outer.append((cx + r * math.cos(a + da), cy + r * math.sin(a + da)))
        for j in (1, 2):                               # the rim between teeth
            b = a + 0.36 + j * (math.pi / 4 - 0.72) / 3
            outer.append((cx + 6.8 * math.cos(b), cy + 6.8 * math.sin(b)))
    hole = circle_pts(cx, cy, 2.9, k=20)
    s.path(d_of(outer) + ' ' + d_of(hole), P.cream, extra=' fill-rule="evenodd"')


def apps(s, P, pal):
    for x0 in (8, 17):
        for y0 in (10, 19):
            s.rect(x0, y0, 7, 7, P.cream, rx=1.6)


def themes(s, P, pal):
    body = circle_pts(15, 19.5, 9.2, 7.2, k=36)
    s.path(d_of(body) + ' ' + d_of(circle_pts(10.3, 23.2, 1.5, k=16)), P.cream, extra=' fill-rule="evenodd"')
    for (x, y, c) in ((9.5, 16.5, '#D8404A'), (13.5, 13.5, '#F2C14E'), (18.5, 13.5, '#4A8AD8'), (20.5, 18.5, '#5AB85A')):
        s.circle(x, y, 1.6, P(c))
    s.line(26.6, 7.4, 20.6, 15.4, P(BROWN), 2.2)
    s.line(26.2, 7.2, 20.4, 14.8, P(BROWN_L), 0.7)
    s.poly([(19.4, 15.2), (21.6, 17), (19.6, 19.4), (17.4, 17.6)], P('#C9C9D0'))
    s.path('M17.2 17.8 L19.4 19.6 Q17 22.6 13.4 23.4 Q14.2 20.2 17.2 17.8 Z', P('#8A2E7A'))


def phone(s, P, pal):
    s.rect(10, 7, 12, 22, P.cream, rx=2.2)
    s.rect(14, 8.2, 4, 0.9, P(CREAM_D), rx=0.45)
    s.rect(12, 10, 8, 14, P(s.linear([(0, '#2E4058'), (1, '#1A2434')])), rx=0.8)
    s.circle(13.9, 14, 1.3, P('#8A5A30'))
    s.circle(19.1, 14, 1.3, P('#8A5A30'))
    s.circle(16.5, 17.3, 3.3, P(s.radial([(0, '#E0AA70'), (1, '#B8804A')], fx=0.35, fy=0.3)))
    s.ellipse(16.5, 18.7, 1.7, 1.2, P('#F0D0A0'))
    s.circle(15, 16.6, 0.5, P(INK))
    s.circle(18, 16.6, 0.5, P(INK))
    s.ellipse(16.5, 18.2, 0.6, 0.45, P(INK))
    s.rect(14, 26, 4, 1, P(CREAM_D), rx=0.5)


def display(s, P, pal):
    s.rect(14, 24, 4, 3.4, P(CREAM_D))
    s.rect(10, 26.8, 12, 2.2, P.cream, rx=1)
    s.rect(4, 8, 24, 17, P.cream, rx=2)
    s.rect(6, 10, 20, 13, P(s.linear([(0, '#22344A'), (1, '#101A24')])), rx=1)
    s.path('M8.4 16.5 Q16.5 10.4 24.6 16.5 Q16.5 22.6 8.4 16.5 Z', P(CREAM_L))
    s.circle(16.5, 16.5, 2.9, P(s.radial([(0, '#8AE8E8'), (1, '#2A9A9C')], fx=0.35, fy=0.3)))
    s.circle(16.5, 16.5, 1.3, P(INK))
    s.circle(15.6, 15.5, 0.55, P('#FFFFFF'))


def home(s, P, pal):
    roof, roof_d = '#6A3A1E', '#4A2410'
    s.rect(20, 7, 3, 7, P('#8A7A6A'))
    s.rect(19.6, 6.6, 3.8, 1.2, P('#B0A090'), rx=0.4)
    s.rect(7, 15.5, 18, 12.5, P.cream, rx=0.6)
    for y in (18.5, 21.5, 24.5):
        s.rect(7, y - 0.35, 18, 0.7, P(CREAM_D))
    s.path('M3.4 17.6 L16.5 6.8 L29.6 17.6 Q29.8 18.4 29 18.4 L4 18.4 Q3.2 18.4 3.4 17.6 Z', P(roof))
    s.poly([(6.4, 17.6), (16.5, 9.4), (26.6, 17.6)], P(roof_d))
    s.line(4.6, 16.8, 15.8, 7.6, P('#9A5A30'), 0.8)
    s.rect(9, 20, 5, 4, P(s.linear([(0, '#FFF0A8'), (1, '#FFC83A')])), rx=0.4)
    s.rect(11.2, 20, 0.6, 4, P(roof_d))
    s.path('M17 28 L17 21.6 Q19.5 19.4 22 21.6 L22 28 Z', P(roof_d))
    s.circle(20.6, 24.6, 0.55, P(GOLD))


def play(s, P, pal):
    s.rect(4, 8, 24, 20, P.cream, rx=1.6)
    for x in range(6, 27, 3):
        s.rect(x, 9, 2, 2, P(INK), rx=0.4)
        s.rect(x, 24, 2, 2, P(INK), rx=0.4)
    s.rect(5, 12, 22, 11, P('#2A1A14'), rx=0.6)
    s.poly([(12.6, 13.6), (12.6, 22.4), (21.4, 18)], P(CREAM_L), stroke=P(CREAM_L), sw=1.4)


def power(s, P, pal):
    oc, orr = (15.5, 18.5), 9.6
    ic, ir = (20.5, 14.5), 7.2
    outer = [p for p in circle_pts(*oc, orr, k=72) if math.dist(p, ic) > ir]
    inner = [p for p in circle_pts(*ic, ir, k=72) if math.dist(p, oc) < orr]
    # Rotate each list so it runs along its arc without a jump, then join.
    def along(pts, circle_c):
        angs = sorted(pts, key=lambda p: math.atan2(p[1] - circle_c[1], p[0] - circle_c[0]) % (2 * math.pi))
        gaps = [(math.atan2(angs[(i + 1) % len(angs)][1] - circle_c[1], angs[(i + 1) % len(angs)][0] - circle_c[0])
                 - math.atan2(angs[i][1] - circle_c[1], angs[i][0] - circle_c[0])) % (2 * math.pi) for i in range(len(angs))]
        cut = gaps.index(max(gaps)) + 1
        return angs[cut:] + angs[:cut]
    o = along(outer, oc)
    i = along(inner, ic)
    s.path(d_of(o + i[::-1]), P.cream)
    pc = (21, 16)
    arc = circle_pts(*pc, 3.7, k=20, a0=-50, a1=230)
    s.path(d_of(arc, close=False), 'none', stroke=P(CREAM_L), sw=1.9)
    s.line(21, 10.2, 21, 15.6, P(CREAM_L), 2)
    s.circle(8.5, 11.5, 0.6, P(CREAM_L))
    s.circle(26.5, 24.5, 0.6, P(CREAM_L))


def about(s, P, pal):
    ink = mixh(pal['bottom'], INK, 0.3)
    s.circle(16.5, 18.5, 9.8, P.cream)
    s.circle(16.5, 13, 1.7, P(ink))
    s.path('M13 16 L18 16 L18 23 L19 23 Q19.4 23 19.4 23.5 L19.4 24.6 Q19.4 25 19 25 L14 25 Q13.6 25 13.6 24.6 '
           'L13.6 23.5 Q13.6 23 14 23 L15 23 L15 17.6 L13.4 17.6 Q13 17.6 13 17.2 Z', P(ink))


def plus(s, P, pal):
    s.rect(13, 7, 6, 21, P.cream, rx=1.6)
    s.rect(6, 15, 20, 6, P.cream, rx=1.6)


def globe(s, P, pal):
    cx, cy, r = 16.5, 18.5, 9.8
    line = mixh(pal['bottom'], INK, 0.2)
    s.circle(cx, cy, r, P.cream)
    s.line(cx, cy - r, cx, cy + r, P(line), 1)
    s.line(cx - r, cy, cx + r, cy, P(line), 1)
    s.ellipse(cx, cy, 4.8, r, 'none', stroke=P(line), sw=1)
    for sgn in (-1, 1):
        y0 = cy + sgn * 5.5
        s.path(f'M{n(cx - 8.2)} {n(y0 - sgn * 1.2)} Q{n(cx)} {n(y0 + sgn * 1.2)} {n(cx + 8.2)} {n(y0 - sgn * 1.2)}',
               'none', stroke=P(line), sw=1)
    s.circle(cx, cy, r, 'none', stroke=P(line), sw=0.6)


def refresh(s, P, pal):
    cx, cy = 16.5, 18.5
    for (a0, a1) in ((185, 288), (5, 108)):
        s.path(d_of(circle_pts(cx, cy, 6.7, k=16, a0=a0, a1=a1), close=False), 'none', stroke=P.cream, sw=3.6)
        a, t = math.radians(a1), math.radians(a1 + 50)
        s.poly([(cx + 2 * math.cos(a), cy + 2 * math.sin(a)), (cx + 11.4 * math.cos(a), cy + 11.4 * math.sin(a)),
                (cx + 6.7 * math.cos(t), cy + 6.7 * math.sin(t))], P.cream, stroke=P(CREAM), sw=0.6)


def medal(s, P, pal):
    s.poly([(9, 7), (13, 7), (17.4, 17), (14.4, 18.2)], P('#4A7AD8'))
    s.poly([(24, 7), (20, 7), (15.6, 17), (18.6, 18.2)], P('#E8E0D0'))
    s.circle(16.5, 21.5, 7.2, P(GOLD_D))
    s.circle(16.2, 21.2, 6.3, P(s.radial([(0, GOLD_L), (0.6, GOLD), (1, '#D8A030')], fx=0.35, fy=0.3)))
    s.ellipse(16.5, 23.4, 2.5, 1.8, P(BROWN_D))
    for (x, y) in ((13, 20.8), (15.3, 18.9), (17.7, 18.9), (20, 20.8)):
        s.circle(x, y, 1, P(BROWN_D))


def wave(s, P, pal):
    toes = ((9.1, 14.9), (12.9, 11.3), (17.5, 10.7), (21.7, 13.1))
    s.rect(11, 24, 9, 5, P.cream, rx=1)
    s.ellipse(15.5, 21, 7.2, 6.2, P.cream)
    for (x, y) in toes:
        s.circle(x, y, 2.7, P.cream)
    pad = '#B8704A'
    s.ellipse(15.5, 21.5, 4, 3.1, P(pad))
    for (x, y) in toes:
        s.circle(x, y + 0.4, 1.3, P(pad))
    for (x0, y0) in ((24.4, 16.6), (25.4, 20.6)):
        s.line(x0, y0, x0 + 2.2, y0 - 1.1, P(CREAM_L), 0.9)
    s.line(5.6, 21.4, 4.6, 23.6, P(CREAM_L), 0.9)


ICONS = {
    'gear': gear,
    'apps': apps,
    'themes': themes,
    'phone': phone,
    'display': display,
    'home': home,
    'play': play,
    'power': power,
    'about': about,
    'plus': plus,
    'globe': globe,
    'refresh': refresh,
    'medal': medal,
    'wave': wave,
}


def build(root):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    phone_dir = os.path.join(root, 'apps', 'remote-web', 'static', 'art')
    os.makedirs(out, exist_ok=True)
    files = []
    for name, draw in ICONS.items():
        pal = BADGE[name]
        s = Svg(128, 128, f'{name} icon', f'Bear Den UI icon "{name}", Classic: tools/classicart/uiicons.py', 32, 32)
        badge(s, pal)
        s.raw('<g transform="translate(0.8 0.8)">')
        draw(s, Paint(s, pal['ear']), pal)
        s.raw('</g>')
        draw(s, Paint(s), pal)
        path = os.path.join(out, f'icon-{name}.svg')
        s.save(path)
        files.append(path)
        if name in PHONE:
            copy = os.path.join(phone_dir, f'icon-{name}.svg')
            shutil.copyfile(path, copy)
            files.append(copy)
    return files
