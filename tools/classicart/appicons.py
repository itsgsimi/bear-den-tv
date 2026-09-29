"""Bear Den's own app icons in the Classic art style (tools/classicart): the
smooth twins of tools/pixelart/appicons.py, drawn on the same 32×32 grid with
the same badge (a rounded square in the app's brand colours, bear ears on
top), the same motifs and the same palettes. Original art, never an official
logo:

    plex-htpc   a film reel with a strip of film
    vacuumtube  a chunky vintage TV with a bear on its screen
    moonlight   a crescent moon over a game controller
    spotify     a record player
    jellyfin    a home-theatre screen with two bears on a couch
    retroarch   an arcade cabinet with a bear on its screen
    netflix     a striped popcorn bucket (web app)
    disney-plus a magic wand with a star and sparkles (web app)
    hulu        a mint-green 1960s TV set on a stand (web app)
    browser     a globe with a gold compass needle (web app)

Writes apps/tv-shell/assets/classic/app-<adapter>.svg and the phone remote's
copies in apps/remote-web/static/art/app-<adapter>.svg.
"""

import math
import os
import shutil

from svg import Svg

INK = '#1A1210'
CREAM = '#F6EAD0'
CREAM_D = '#D8C6A2'
OUTLINE = '#120E0C'

# The same badge colours as tools/pixelart/appicons.py (BADGE).
BADGE = {
    'plex-htpc': {'top': '#F2B53A', 'bottom': '#B5780A', 'ear': '#8A5A06', 'inner': '#F6D27A'},
    'vacuumtube': {'top': '#4A1C18', 'bottom': '#1E0A08', 'ear': '#2A0E0C', 'inner': '#8A3A30'},
    'moonlight': {'top': '#5A6880', 'bottom': '#1E2636', 'ear': '#2A3244', 'inner': '#8A9CB8'},
    'spotify': {'top': '#2FCF6A', 'bottom': '#10703A', 'ear': '#0C5A2C', 'inner': '#7FE0A2'},
    'jellyfin': {'top': '#9A5CC8', 'bottom': '#1E78B8', 'ear': '#5A2E82', 'inner': '#C9A0E8'},
    'retroarch': {'top': '#3E3478', 'bottom': '#141030', 'ear': '#221C4A', 'inner': '#7A6AC8'},
    'netflix': {'top': '#5A2A40', 'bottom': '#220E18', 'ear': '#3A1626', 'inner': '#9A5A74'},
    'disney-plus': {'top': '#3A4AA8', 'bottom': '#141A48', 'ear': '#222C6A', 'inner': '#7A8AE0'},
    'hulu': {'top': '#1E5A3A', 'bottom': '#0A2416', 'ear': '#123A24', 'inner': '#5A9A74'},
    'browser': {'top': '#2A7A90', 'bottom': '#0E3440', 'ear': '#16505E', 'inner': '#6AB4C8'},
}


def badge(s, pal):
    for cx in (7.5, 24.5):
        s.circle(cx, 4.9, 3.9, OUTLINE)
        s.circle(cx, 4.9, 3.1, pal['ear'])
        s.circle(cx, 4.6, 1.5, pal['inner'])
    s.rect(1.6, 4.6, 28.8, 26.8, OUTLINE, rx=5.4)
    s.rect(2.4, 5.4, 27.2, 25.2, s.linear([(0, pal['top']), (1, pal['bottom'])]), rx=4.8)
    s.rect(4.5, 6.2, 23, 3, s.linear([(0, '#FFFFFF', 0.28), (1, '#FFFFFF', 0)]), rx=1.5)


def reel(s):
    s.rect(18, 22, 10, 6, INK, rx=0.6)
    for x in range(19, 28, 2):
        s.rect(x, 22.8, 1, 0.9, CREAM_D, rx=0.2)
        s.rect(x, 26.2, 1, 0.9, CREAM_D, rx=0.2)
    s.rect(19, 24, 8, 2, '#6A4A1A')
    cx, cy = 14.5, 18.5
    s.circle(cx + 0.4, cy + 0.6, 8.4, '#000000', 0.25)
    s.circle(cx, cy, 8.2, s.radial([(0, '#FFF6E2'), (0.75, CREAM), (1, CREAM_D)], fx=0.35, fy=0.3))
    for k in range(5):
        a = -math.pi / 2 + k * 2 * math.pi / 5
        s.circle(cx + 4.7 * math.cos(a), cy + 4.7 * math.sin(a), 1.7, '#8A5A06')
    s.circle(cx, cy, 1.2, INK)


def tv(s):
    red = s.linear([(0, '#FF5A4A'), (0.5, '#E23A2E'), (1, '#9A1E16')])
    s.line(15.5, 12, 11, 7, '#C9C9D0', 0.8)
    s.line(16.5, 12, 21, 7, '#C9C9D0', 0.8)
    s.circle(11, 7, 0.8, CREAM)
    s.circle(21, 7, 0.8, CREAM)
    s.rect(7, 25.5, 2, 3.2, INK, rx=0.6)
    s.rect(23, 25.5, 2, 3.2, INK, rx=0.6)
    s.rect(4.6, 11.6, 22.8, 15, red, rx=2.6)
    s.rect(6.6, 13.6, 14, 10, INK, rx=2.2)
    s.rect(7.6, 14.6, 12, 8, s.linear([(0, '#B8DAF2'), (1, '#6A9CC8')]), rx=1.6)
    bear = '#5A3A28'
    s.circle(11, 16.4, 1.2, bear)
    s.circle(16.2, 16.4, 1.2, bear)
    s.ellipse(13.6, 18.9, 3, 2.7, bear)
    s.ellipse(13.6, 19.9, 1.3, 1, '#8A6A50')
    s.circle(12.4, 18.2, 0.4, CREAM)
    s.circle(14.8, 18.2, 0.4, CREAM)
    s.circle(13.6, 19.5, 0.45, INK)
    s.circle(23.5, 16.4, 1.5, s.radial([(0, '#FFFFFF'), (1, CREAM_D)], fx=0.35, fy=0.3))
    s.circle(23.5, 20.4, 1.2, CREAM_D)


def moon_pad(s):
    pale = s.linear([(0, '#F2F6FC'), (1, '#A9C1DC')])
    s.glow(15, 12, 9, 8, '#DCE8FF', 0.35)
    s.path('M14.2 6.7 A5.5 5.5 0 1 0 20.4 14.3 A4.6 4.6 0 0 1 14.2 6.7 Z', pale)
    s.circle(23, 8.4, 0.5, '#F2F6FC')
    s.circle(8.4, 9.4, 0.4, '#A9C1DC')
    body = s.linear([(0, '#B4C2D6'), (0.4, '#8494AC'), (1, '#4A5668')])
    s.path('M9 19 L23 19 Q27 19 27.2 23 L27 26.6 Q26.8 28.4 25 28.2 L23 27.8 Q21.6 26.6 20.8 24.4 '
           'L11.2 24.4 Q10.4 26.6 9 27.8 L7 28.2 Q5.2 28.4 5 26.6 L4.8 23 Q5 19 9 19 Z', body)
    for x, y in ((10, 21.6), (20.4, 22.4)):
        s.circle(x, y, 1.7, '#3A4658')
        s.circle(x - 0.2, y - 0.2, 1.1, '#1E2430')
    s.rect(14, 21, 4, 1, '#A9C1DC', rx=0.5)


def turntable(s):
    s.rect(5, 26, 3, 2.8, INK, rx=0.6)
    s.rect(24, 26, 3, 2.8, INK, rx=0.6)
    s.rect(4, 13, 24, 14, s.linear([(0, '#C08A58'), (0.15, '#9A6438'), (1, '#5E3A1E')]), rx=1.6)
    s.circle(14.5, 19.5, 6.4, s.radial([(0, '#3A3A3A'), (0.25, '#161616'), (0.6, '#262626'), (0.62, '#161616'),
                                         (0.85, '#2A2A2A'), (1, '#111111')]))
    s.path('M10 15.6 A5.4 5.4 0 0 1 13 14.4', 'none', stroke='#8A8A8A', sw=0.5)
    s.circle(14.5, 19.5, 1.6, '#F2C14E')
    s.circle(14.5, 19.5, 0.35, INK)
    s.circle(24, 15.4, 1.4, s.radial([(0, '#FFFFFF'), (1, '#9A9AA4')], fx=0.35, fy=0.3))
    s.path('M24 15.4 L23.3 21.2 L20.4 22.6', 'none', stroke='#E6E6EC', sw=0.8)
    s.rect(19.2, 22.2, 2, 1, INK, rx=0.3)
    s.rect(23, 23.4, 3, 1, '#2FCF6A', rx=0.5)


def theatre(s):
    s.rect(6, 8, 20, 11, s.linear([(0, '#9AB0E8'), (1, '#DCE8F8')]))
    s.glow(19, 10, 4, 3, '#FFF6D8', 0.8)
    s.circle(19, 10, 1.2, '#FFF6D8')
    for x, h in ((8.5, 5), (11.5, 7), (14.5, 4), (22.5, 6), (24.5, 4)):
        s.poly([(x, 18.5 - h), (x + 1.6, 18.5), (x - 1.6, 18.5)], '#3A5A6A')
    s.rect(6, 17, 20, 2, '#5A7A7A')
    curtain = s.linear([(0, '#6A3A98'), (1, '#3A1C58')], x2=1, y2=0)
    s.path('M4 6 L8 6 Q6.6 12 7.4 19.6 L4 19.6 Z', curtain)
    s.path('M28 6 L24 6 Q25.4 12 24.6 19.6 L28 19.6 Z', curtain)
    s.rect(4, 5.8, 24, 2, '#3A1C58', rx=0.6)
    for cx, fur, ear in ((12, '#B8844E', '#8A5A30'), (19.5, '#E0B880', '#B8844E')):
        s.circle(cx - 2.2, 19.4, 1.3, ear)
        s.circle(cx + 2.2, 19.4, 1.3, ear)
        s.ellipse(cx, 22, 3.3, 2.8, fur)
    s.rect(5, 23.6, 22, 4.6, s.linear([(0, '#6A4A98'), (0.2, '#3A2458'), (1, '#24163A')]), rx=1.4)
    s.rect(3.6, 21.8, 3.4, 6.4, '#24163A', rx=1.2)
    s.rect(25, 21.8, 3.4, 6.4, '#24163A', rx=1.2)


def cabinet(s):
    s.rect(8.4, 6.6, 15.2, 23, s.linear([(0, '#8A2A24'), (0.12, '#E0564A'), (0.88, '#E0564A'), (1, '#8A2A24')], x2=1, y2=0), rx=1.4)
    s.rect(9, 7, 14, 4, s.linear([(0, '#FFF4C8'), (1, '#FFD34F')]), rx=1)
    s.rect(10.6, 11.8, 10.8, 8.4, INK, rx=1.2)
    s.rect(11.6, 12.8, 8.8, 6.4, s.radial([(0, '#3A6A9A'), (1, '#1A2A4A')]), rx=0.8)
    s.circle(14, 13.9, 0.9, '#C89A6A')
    s.circle(18, 13.9, 0.9, '#C89A6A')
    s.ellipse(16, 16, 2.8, 2.3, '#E8C89A')
    s.circle(15, 15.4, 0.35, INK)
    s.circle(17, 15.4, 0.35, INK)
    s.circle(16, 16.5, 0.4, INK)
    s.path('M8 21 L24 21 L25.2 24.4 L6.8 24.4 Z', s.linear([(0, '#F8A090'), (1, '#E0564A')]))
    s.line(12, 22.4, 12, 19.6, '#2A2A2E', 0.7)
    s.circle(12, 19.2, 1.3, s.radial([(0, '#8AB0FF'), (1, '#2A5AC8')], fx=0.35, fy=0.3))
    s.circle(17, 22.6, 0.8, '#FFD34F')
    s.circle(20, 22.6, 0.8, '#4FD86A')
    s.rect(13, 25.6, 6, 2.4, '#8A2A24', rx=0.5)
    s.rect(15.5, 26.2, 1, 1, '#FFD34F', rx=0.2)


def popcorn(s):
    corn = s.radial([(0, '#FFF8DC'), (0.7, '#FFE6A0'), (1, '#E0B050')], fx=0.35, fy=0.3)
    for x, y, r in ((11, 13.4, 2.8), (15.5, 11.2, 3.2), (20.4, 12.8, 2.8), (13.2, 15.4, 2.6), (18.4, 15.4, 2.8), (9.2, 16.2, 2), (22.6, 16.2, 2)):
        s.circle(x, y, r, corn)
    s.circle(25.2, 9, 1.3, corn)
    stripes = s.linear([(0, '#D8404A'), (0.1875, '#D8404A'), (0.1875, '#F6EAD0'), (0.375, '#F6EAD0'), (0.375, '#D8404A'),
                        (0.5625, '#D8404A'), (0.5625, '#F6EAD0'), (0.75, '#F6EAD0'), (0.75, '#D8404A'), (1, '#D8404A')], x2=1, y2=0)
    s.path('M7.6 17.4 L24.4 17.4 L21.6 28.6 L10.4 28.6 Z', stripes)
    s.path('M7.6 17.4 L24.4 17.4 L21.6 28.6 L10.4 28.6 Z', s.linear([(0, '#000000', 0), (0.7, '#000000', 0), (1, '#000000', 0.3)], x2=1, y2=0))
    s.rect(6.8, 16.6, 18.4, 2, s.linear([(0, '#FFFFFF'), (1, '#D8C6A2')]), rx=0.8)


def wand(s):
    s.line(9.4, 27, 18.6, 15.8, '#F2F2FA', 1.4)
    s.line(10, 27.4, 19, 16.4, '#9A9AB8', 0.6)
    s.rect(7.8, 25.6, 3.2, 3.2, '#C89A1E', rx=0.8)
    cx, cy, ro, ri = 21, 12, 6.4, 2.7
    pts = []
    for k in range(10):
        a = -math.pi / 2 + k * math.pi / 5
        r = ro if k % 2 == 0 else ri
        pts.append((round(cx + r * math.cos(a), 2), round(cy + r * math.sin(a), 2)))
    s.glow(cx, cy, 7, 7, '#FFF4C8', 0.35)
    s.poly(pts, s.radial([(0, '#FFF4C8'), (0.6, '#FFD34F'), (1, '#C89A1E')], fx=0.4, fy=0.35))
    for x, y, r in ((8, 11, 1.3), (13, 8, 1.1), (26, 21, 1.3), (15, 22, 1)):
        s.path(f'M{x} {y - r * 1.6} Q{x} {y} {x + r * 1.6} {y} Q{x} {y} {x} {y + r * 1.6} Q{x} {y} {x - r * 1.6} {y} Q{x} {y} {x} {y - r * 1.6} Z', '#FFF4C8')


def retro_set(s):
    s.line(19, 11, 24, 6, '#C9C9D0', 0.8)
    s.circle(24, 6, 0.8, CREAM)
    s.line(10, 28.6, 13, 25, INK, 1)
    s.line(22, 28.6, 19, 25, INK, 1)
    s.rect(4, 11, 24, 14.4, s.linear([(0, '#9AF0BE'), (0.12, '#4ED88A'), (0.85, '#4ED88A'), (1, '#1E8A4E')]), rx=2)
    s.rect(6, 13, 14, 9.4, INK, rx=2.4)
    s.rect(7, 14, 12, 7.4, s.linear([(0, '#1E3A5A'), (1, '#2A4A6A')]), rx=1.8)
    s.circle(15, 16.2, 1.3, '#FFF6D8')
    s.path('M7.4 20.6 Q11 17.4 14.6 19.2 Q16.6 18.6 18.6 20 L18.6 21 Q18.6 21.4 18 21.4 L8 21.4 Q7.4 21.4 7.4 20.8 Z', '#2E6A4A')
    s.circle(23.5, 15.2, 2.1, s.radial([(0, '#FFFFFF'), (1, CREAM_D)], fx=0.35, fy=0.3))
    s.line(23.5, 15.2, 22.8, 14, INK, 0.5)
    for y in (19, 21):
        s.rect(21, y - 0.4, 5.4, 0.8, '#1E8A4E', rx=0.4)
    s.rect(12, 24.8, 8, 1.2, '#1E8A4E', rx=0.4)


def compass(s):
    cx, cy = 16, 18.4
    s.circle(cx, cy, 9.4, s.radial([(0, '#8AD8F8'), (0.7, '#5AB8E8'), (1, '#2A7AB0')], fx=0.35, fy=0.3))
    s.path('M9.6 13 L15 11.8 L16.2 16.2 L12.2 19.6 L8.8 17.4 Z', '#6AD08A')
    s.path('M18 19 L23.4 17 L24.4 22.4 L20 25.4 Z', '#6AD08A')
    for y in (14, 22.8):
        s.line(cx - 7.8, y, cx + 7.8, y, '#FFFFFF', 0.4, 0.35)
    s.poly([(16, 17.4), (21.4, 11), (17, 18.6)], '#FFD34F')
    s.poly([(16, 19.4), (10.6, 25.8), (15, 18.2)], '#8A5A30')
    s.circle(16, 18.4, 1.3, INK)
    s.circle(16, 18.4, 0.5, CREAM)


ICONS = {
    'plex-htpc': reel,
    'vacuumtube': tv,
    'moonlight': moon_pad,
    'spotify': turntable,
    'jellyfin': theatre,
    'retroarch': cabinet,
    'netflix': popcorn,
    'disney-plus': wand,
    'hulu': retro_set,
    'browser': compass,
}


def build(root):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    phone = os.path.join(root, 'apps', 'remote-web', 'static', 'art')
    os.makedirs(out, exist_ok=True)
    files = []
    for adapter, draw in ICONS.items():
        s = Svg(128, 128, f'{adapter} icon', f'Bear Den\'s own icon for {adapter}, Classic: tools/classicart/appicons.py', 32, 32)
        badge(s, BADGE[adapter])
        draw(s)
        path = os.path.join(out, f'app-{adapter}.svg')
        s.save(path)
        shutil.copyfile(path, os.path.join(phone, f'app-{adapter}.svg'))
        files += [path, os.path.join(phone, f'app-{adapter}.svg')]
    return files
