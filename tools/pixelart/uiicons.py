"""Bear Den's UI icons in pixel art (tools/pixelart; guide docs/THEMES.md →
UI icons): the pictures on the shell's pills, settings rows and setup cards.
Each is the app icons' badge (tools/pixelart/appicons.py: a rounded square
with a pair of bear ears on top, light from the top left, a dark 1-pixel
outline) in its own den colour, holding one bold cream motif that reads at
48–96 screen pixels from the couch. Original art, never a third-party logo:

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

Every motif is drawn on its own layer, gets a solid drop shadow (one pixel
down and right, in the badge's ear colour) and a cream shade on its lower
right edges, then sits on the badge. Writes
apps/tv-shell/assets/pixel/icon-<name>.png and, for the icons the phone
remote shows (PHONE), copies in apps/remote-web/static/art/pixel/. The Classic
twins are tools/classicart/uiicons.py, which reads BADGE from here.
"""

import math
import os
import shutil

from appicons import OUTLINE, badge
from px import Img, mix

S = 32
CREAM = '#F6EAD0'
CREAM_D = '#D8C6A2'
CREAM_L = '#FFFAEC'
INK = '#1A1210'
BROWN, BROWN_D, BROWN_L = '#8A5A34', '#5A3A20', '#B8844E'
GOLD, GOLD_D, GOLD_L = '#F2C14E', '#B8841E', '#FFE6A0'


def _hex(c):
    return '#%02X%02X%02X' % c[:3]


def _pal(top, bottom):
    """A badge palette in the app icons' shape: gradient, ears, inner ears."""
    return {'top': top, 'bottom': bottom, 'ear': _hex(mix(top, bottom, 0.75)),
            'inner': _hex(mix(top, '#FFFFFF', 0.35))}


# One den colour per icon (gradient top, bottom); the Classic twin reads this.
BADGE = {
    'gear': _pal('#8E7058', '#44322A'),       # taupe
    'apps': _pal('#F0BC48', '#A8700E'),       # honey amber
    'themes': _pal('#9C4E8C', '#461840'),     # plum
    'phone': _pal('#5070B0', '#1C2A56'),      # dusk blue
    'display': _pal('#2A8A8C', '#0C3A3E'),    # deep teal
    'home': _pal('#5A9A40', '#1E4414'),       # forest green
    'play': _pal('#D0502E', '#701C0C'),       # brick red
    'power': _pal('#4A3E8A', '#140E34'),      # midnight indigo
    'about': _pal('#8E963A', '#3C4012'),      # olive moss
    'plus': _pal('#F08A3A', '#A0400C'),       # pumpkin
    'globe': _pal('#A0603A', '#4A2412'),      # chestnut
    'refresh': _pal('#4AA6D0', '#1A5A80'),    # sky blue
    'medal': _pal('#9A3050', '#40101E'),      # wine
    'wave': _pal('#6AB894', '#246048'),       # sage
}

# Icons the phone remote shows too (copied to apps/remote-web/static/art/).
PHONE = ('plus',)


# --- motifs (a layer each, inside x 5..27, y 8..28) ---------------------------

def gear(m, pal):
    cx, cy = 16, 18
    for k in range(8):
        a = k * math.pi / 4
        dx, dy = math.cos(a), math.sin(a)
        px_, py_ = -dy, dx
        m.poly([(cx + dx * 5 + px_ * 2, cy + dy * 5 + py_ * 2), (cx + dx * 9.8 + px_ * 1.6, cy + dy * 9.8 + py_ * 1.6),
                (cx + dx * 9.8 - px_ * 1.6, cy + dy * 9.8 - py_ * 1.6), (cx + dx * 5 - px_ * 2, cy + dy * 5 - py_ * 2)], CREAM)
    m.disc(cx, cy, 6.6, CREAM)
    for x in range(cx - 3, cx + 4):
        for y in range(cy - 3, cy + 4):
            if (x - cx + 0.5) ** 2 + (y - cy + 0.5) ** 2 <= 7.5:
                m.clear(x, y)


def apps(m, pal):
    for x0 in (8, 17):
        for y0 in (10, 19):
            m.rect(x0, y0, 7, 7, CREAM)
            for (x, y) in ((x0, y0), (x0 + 6, y0), (x0, y0 + 6), (x0 + 6, y0 + 6)):
                m.clear(x, y)
    m.rect(9, 11, 2, 1, CREAM_L)


def themes(m, pal):
    m.ellipse(14.5, 19, 9, 7, CREAM)
    m.disc(10, 23, 1.6, CREAM)
    m.clear(9, 22)                                  # the thumb hole
    m.clear(10, 22)
    m.clear(9, 23)
    m.clear(10, 23)
    for (x, y, c) in ((9, 16, '#D8404A'), (13, 13, '#F2C14E'), (18, 13, '#4A8AD8'), (20, 18, '#5AB85A')):
        m.disc(x, y, 1.4, c)
    # The brush: a handle from the top right, a ferrule, a plum tip.
    m.line(26, 7, 20, 15, BROWN)
    m.line(27, 8, 21, 16, BROWN_D)
    m.line(25, 7, 19, 15, BROWN_L)
    m.rect(17, 16, 3, 3, '#C9C9D0')
    m.poly([(16, 18), (18, 20), (14, 23), (13, 22)], '#8A2E7A')


def phone(m, pal):
    m.rect(10, 7, 12, 22, CREAM)
    for (x, y) in ((10, 7), (21, 7), (10, 28), (21, 28)):
        m.clear(x, y)
    m.rect(14, 8, 4, 1, CREAM_D)                   # speaker
    m.rect(12, 10, 8, 14, '#1E2A3A')                # the screen
    m.rect(12, 10, 8, 1, '#2E4058')
    fur, fur_d, snout = '#C8905A', '#8A5A30', '#F0D0A0'
    m.disc(13.5, 13.5, 1.2, fur_d)                 # a bear's face
    m.disc(18.5, 13.5, 1.2, fur_d)
    m.disc(16, 17, 3.2, fur)
    m.rect(15, 18, 3, 2, snout)
    m.put(14, 16, INK)
    m.put(18, 16, INK)
    m.put(16, 18, INK)
    m.rect(14, 26, 4, 1, CREAM_D)                  # home bar


def display(m, pal):
    m.rect(4, 8, 24, 17, CREAM)                     # the frame
    for (x, y) in ((4, 8), (27, 8), (4, 24), (27, 24)):
        m.clear(x, y)
    m.rect(6, 10, 20, 13, '#162230')                # the screen
    m.ellipse(16, 16, 7, 3.4, CREAM_L)              # an eye
    for (x, y) in ((8, 16), (24, 16)):
        m.put(x, y, CREAM_L)
    m.disc(16, 16, 2.6, '#4AC0C0')
    m.disc(16, 16, 1.2, INK)
    m.put(15, 15, '#FFFFFF')
    m.rect(14, 25, 4, 2, CREAM_D)                   # the stand
    m.rect(10, 27, 12, 2, CREAM)


def home(m, pal):
    roof, roof_d = '#6A3A1E', '#4A2410'
    m.rect(20, 7, 3, 6, '#8A7A6A')                  # chimney
    m.rect(20, 7, 3, 1, '#B0A090')
    m.rect(7, 16, 18, 12, CREAM)                    # log walls
    for y in (18, 21, 24, 27):
        m.hline(7, 24, y, CREAM_D)
    m.poly([(3.5, 17), (16, 7), (28.5, 17)], roof)  # the roof
    m.poly([(5.5, 17), (16, 9), (26.5, 17)], roof_d)
    m.line(4, 16, 15, 7, '#9A5A30')
    m.hline(4, 27, 17, roof)
    m.rect(9, 20, 5, 4, '#FFD34F')                  # a warm window
    m.rect(9, 20, 5, 1, '#FFF0A8')
    m.vline(11, 20, 23, roof_d)
    m.rect(17, 20, 5, 8, roof_d)                    # the door
    m.put(20, 24, GOLD)


def play(m, pal):
    m.rect(4, 8, 24, 20, CREAM)                     # the film frame
    for x in range(6, 27, 3):                       # sprocket holes
        m.rect(x, 9, 2, 2, INK)
        m.rect(x, 24, 2, 2, INK)
    m.rect(5, 12, 22, 11, '#2A1A14')
    m.poly([(12, 13), (12, 23), (21.5, 18)], CREAM_L)   # the play triangle
    m.hline(12, 12, 13, '#FFFFFF')


def power(m, pal):
    cx, cy = 15, 18
    for x in range(4, 26):                          # a crescent: a disc minus a disc
        for y in range(8, 29):
            if (x - cx) ** 2 + (y - cy) ** 2 <= 92 and (x - 20) ** 2 + (y - 14) ** 2 > 52:
                m.put(x, y, CREAM)
    pcx, pcy = 20.5, 15.5                           # a power symbol in the moon's bite
    for x in range(14, 27):
        for y in range(9, 22):
            d = math.hypot(x + 0.5 - pcx, y + 0.5 - pcy)
            ang = math.degrees(math.atan2(y + 0.5 - pcy, x + 0.5 - pcx)) % 360
            if 2.7 <= d <= 4.6 and not (228 < ang < 312):
                m.put(x, y, CREAM_L)
    m.rect(20, 9, 2, 7, CREAM_L)
    m.put(8, 11, CREAM_L)                           # a star
    m.put(26, 24, CREAM_L)


def about(m, pal):
    m.disc(16, 18, 9.5, CREAM)
    ink = _hex(mix(pal['bottom'], INK, 0.3))
    m.disc(16, 12.5, 1.6, ink)                      # the "i"
    m.rect(14, 16, 4, 8, ink)
    m.rect(13, 16, 1, 1, ink)
    m.rect(13, 23, 6, 2, ink)


def plus(m, pal):
    m.rect(13, 7, 6, 21, CREAM)
    m.rect(6, 15, 20, 6, CREAM)
    for (x, y) in ((13, 7), (18, 7), (13, 27), (18, 27), (6, 15), (6, 20), (25, 15), (25, 20)):
        m.clear(x, y)
    m.rect(14, 8, 2, 3, CREAM_L)


def globe(m, pal):
    cx, cy, r = 16, 18, 9.5
    line = _hex(mix(pal['bottom'], INK, 0.2))
    m.disc(cx, cy, r, CREAM)
    for x in range(4, 29):
        for y in range(6, 30):
            if not m.get(x, y)[3]:
                continue
            X, Y = x - cx, y - cy
            on = abs(X) <= 0.5 or abs(Y) <= 0.5                          # meridian, equator
            on = on or abs((X / 4.6) ** 2 + (Y / r) ** 2 - 1) < 0.2       # a curved meridian
            on = on or abs(Y + 5.5 - 0.02 * X * X) <= 0.5                # two latitudes, bowed
            on = on or abs(Y - 5.5 + 0.02 * X * X) <= 0.5
            if on:
                m.put(x, y, line)


def refresh(m, pal):
    cx, cy = 16, 18
    for x in range(4, 29):
        for y in range(6, 30):
            d = math.hypot(x + 0.5 - cx, y + 0.5 - cy)
            ang = math.degrees(math.atan2(y + 0.5 - cy, x + 0.5 - cx)) % 360
            if 4.8 <= d <= 8.6 and (185 <= ang <= 292 or 5 <= ang <= 112):
                m.put(x, y, CREAM)
    for end in (292, 112):                          # arrowheads, pointing clockwise
        a = math.radians(end)
        t = math.radians(end + 52)
        m.poly([(cx + 1.8 * math.cos(a), cy + 1.8 * math.sin(a)), (cx + 11.8 * math.cos(a), cy + 11.8 * math.sin(a)),
                (cx + 6.8 * math.cos(t), cy + 6.8 * math.sin(t))], CREAM)


def medal(m, pal):
    m.poly([(9, 7), (13, 7), (17, 17), (14, 18)], '#4A7AD8')     # ribbon
    m.poly([(23, 7), (19, 7), (15, 17), (18, 18)], '#E8E0D0')
    m.disc(16, 21, 7.2, GOLD_D)                                   # the medal
    m.disc(15.6, 20.6, 6.4, GOLD)
    for x in range(9, 23):
        for y in range(14, 28):
            if 5.2 < math.hypot(x - 15.6, y - 20.6) <= 6.8 and x + y < 34 and m.get(x, y)[3]:
                m.put(x, y, GOLD_L)
    m.ellipse(16, 23, 2.4, 1.6, BROWN_D)                          # a paw
    for (x, y) in ((12.6, 20.4), (14.8, 18.4), (17.4, 18.4), (19.6, 20.4)):
        m.disc(x, y, 0.9, BROWN_D)


def wave(m, pal):
    toes = ((8.6, 14.4), (12.4, 10.8), (17, 10.2), (21.2, 12.6))
    m.ellipse(15, 20.5, 7, 6, CREAM)                # the palm
    for (x, y) in toes:
        m.disc(x, y, 2.6, CREAM)                    # toes
    m.rect(11, 25, 9, 4, CREAM)                     # the wrist
    pad = '#B8704A'
    m.ellipse(15, 21, 4, 3, pad)                    # pads
    for (x, y) in toes:
        m.disc(x, y + 0.4, 1.3, pad)
    for (x0, y0) in ((24, 16), (25, 20)):           # motion marks, to the right
        m.line(x0, y0, x0 + 2, y0 - 1, CREAM_L)
    m.line(5, 21, 4, 23, CREAM_L)


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


def icon(name):
    pal = BADGE[name]
    img = Img(S, S)
    badge(img, pal)
    m = Img(S, S)
    ICONS[name](m, pal)
    cream = tuple(int(CREAM[i:i + 2], 16) for i in (1, 3, 5)) + (255,)
    for y in range(S):                              # shade the lower right edges
        for x in range(S):
            if m.get(x, y) == cream and not m.get(x + 1, y + 1)[3]:
                m.put(x, y, CREAM_D)
    for y in range(S):                              # a solid drop shadow
        for x in range(S):
            if m.get(x, y)[3] and img.get(x + 1, y + 1)[3] and not m.get(x + 1, y + 1)[3]:
                img.put(x + 1, y + 1, pal['ear'])
    img.blit(m, 0, 0)
    shape = Img(S, S)
    for i, p in enumerate(img.px):
        if p[3]:
            shape.px[i] = p
    shape.outline(OUTLINE)
    return shape


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    phone_dir = os.path.join(root, 'apps', 'remote-web', 'static', 'art', 'pixel')
    os.makedirs(out, exist_ok=True)
    os.makedirs(phone_dir, exist_ok=True)
    made = []
    for name in ICONS:
        img = icon(name)
        path = os.path.join(out, f'icon-{name}.png')
        img.save(path)
        if name in PHONE:
            shutil.copyfile(path, os.path.join(phone_dir, f'icon-{name}.png'))
        made.append(img)
    if preview:
        os.makedirs(preview, exist_ok=True)
        cols = 7
        rows = (len(made) + cols - 1) // cols
        board = Img((S + 4) * cols, (S + 4) * rows, '#2A2F3A')
        for i, img in enumerate(made):
            board.blit(img, (i % cols) * (S + 4) + 2, (i // cols) * (S + 4) + 2)
        board.scaled(5).save(os.path.join(preview, 'uiicons.png'))
        board.scaled(2).save(os.path.join(preview, 'uiicons-small.png'))
    print('uiicons: ' + ', '.join(ICONS))
