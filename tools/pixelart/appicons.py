"""Bear Den's own app icons in pixel art (tools/pixelart; guide
docs/THEMES.md → App icons). One original icon per adapter, never a copy of
the app's official logo: each is a Bear Den badge (a rounded square in the
app's brand colours with a pair of bear ears on top) holding a motif:

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

All share one 32×32 grid, the same badge, light from the top left and a dark
1-pixel outline. Writes apps/tv-shell/assets/pixel/app-<adapter>.png and the
phone remote's copies in apps/remote-web/static/art/pixel/. The Classic twins
are drawn by tools/classicart/appicons.py with the same palettes.
"""

import os
import shutil

import math

from px import Img, mix

S = 32
OUTLINE = '#120E0C'
CREAM = '#F6EAD0'
CREAM_D = '#D8C6A2'
INK = '#1A1210'

# Badge colours per adapter: gradient top/bottom and the ear colours
# (the Classic twin reads the same table).
BADGE = {
    'plex-htpc': {'top': '#F2B53A', 'bottom': '#B5780A', 'ear': '#8A5A06', 'inner': '#F6D27A'},
    'vacuumtube': {'top': '#4A1C18', 'bottom': '#1E0A08', 'ear': '#2A0E0C', 'inner': '#8A3A30'},
    'moonlight': {'top': '#5A6880', 'bottom': '#1E2636', 'ear': '#2A3244', 'inner': '#8A9CB8'},
    'spotify': {'top': '#2FCF6A', 'bottom': '#10703A', 'ear': '#0C5A2C', 'inner': '#7FE0A2'},
    'jellyfin': {'top': '#9A5CC8', 'bottom': '#1E78B8', 'ear': '#5A2E82', 'inner': '#C9A0E8'},
    'retroarch': {'top': '#3E3478', 'bottom': '#141030', 'ear': '#221C4A', 'inner': '#7A6AC8'},
    # Web apps (Google Chrome, Brave): our own colours, never the services' logos.
    'netflix': {'top': '#5A2A40', 'bottom': '#220E18', 'ear': '#3A1626', 'inner': '#9A5A74'},
    'disney-plus': {'top': '#3A4AA8', 'bottom': '#141A48', 'ear': '#222C6A', 'inner': '#7A8AE0'},
    'hulu': {'top': '#1E5A3A', 'bottom': '#0A2416', 'ear': '#123A24', 'inner': '#5A9A74'},
    'browser': {'top': '#2A7A90', 'bottom': '#0E3440', 'ear': '#16505E', 'inner': '#6AB4C8'},
}




def badge(img, pal):
    """The shared frame: ears behind a rounded square, gradient, top sheen."""
    for cx in (7, 24):
        img.disc(cx, 5, 3.4, pal['ear'])
        img.rect(cx - 1, 4, 3, 2, pal['inner'])
    x0, y0, x1, y1 = 2, 5, 29, 30          # inclusive
    for y in range(y0, y1 + 1):
        t = (y - y0) / (y1 - y0)
        col = mix(pal['top'], pal['bottom'], t)
        # stair corners: 3, 2, 1 pixels in from the top and bottom rows
        inset = {0: 3, 1: 2, 2: 1}.get(min(y - y0, y1 - y), 0)
        for x in range(x0 + inset, x1 - inset + 1):
            img.put(x, y, col)
    img.hline(5, 26, y0 + 1, '#FFFFFF', 0.22)   # sheen
    img.hline(4, 27, y0 + 2, '#FFFFFF', 0.1)


def reel(img):
    """Plex: a film reel with a strip of film unspooling to the right."""
    cx, cy = 14, 18
    img.rect(18, 22, 10, 6, INK)                 # the strip
    for x in range(19, 28, 2):
        img.put(x, 23, CREAM_D)
        img.put(x, 26, CREAM_D)
    img.rect(19, 24, 8, 2, '#6A4A1A')
    img.disc(cx, cy, 8, CREAM)
    for x in range(cx - 8, cx + 9):                # shade the lower right rim
        for y in range(cy - 8, cy + 9):
            if (x - cx) ** 2 + (y - cy) ** 2 > 56 and (x - cx) + (y - cy) > 2 and img.get(x, y)[3]:
                img.put(x, y, CREAM_D)
    for k in range(5):
        a = -math.pi / 2 + k * 2 * math.pi / 5
        hx, hy = cx + 4.6 * math.cos(a), cy + 4.6 * math.sin(a)
        img.disc(round(hx), round(hy), 1.3, '#8A5A06')
    img.rect(cx - 1, cy - 1, 2, 2, INK)


def tv(img):
    """YouTube: a chunky vintage TV on legs, a bear's head on its screen."""
    red, red_d = '#E23A2E', '#9A1E16'
    img.line(15, 12, 11, 7, '#C9C9D0')           # rabbit ears
    img.line(17, 12, 21, 7, '#C9C9D0')
    img.put(11, 7, CREAM)
    img.put(21, 7, CREAM)
    img.rect(5, 12, 22, 14, red)                 # the cabinet
    img.hline(6, 25, 12, '#FF6A5A')
    img.rect(5, 24, 22, 2, red_d)
    img.rect(7, 27, 2, 2, INK)                   # legs
    img.rect(23, 27, 2, 2, INK)
    img.rect(7, 14, 13, 9, INK)                  # screen bezel
    img.rect(8, 15, 11, 7, '#9FC8E8')            # the picture: a bear
    img.rect(8, 19, 11, 3, '#7AAFD8')
    img.disc(11, 16, 1, '#5A3A28')
    img.disc(16, 16, 1, '#5A3A28')
    img.disc(13.5, 18.5, 2.6, '#5A3A28')
    img.put(12, 18, CREAM)
    img.put(15, 18, CREAM)
    img.rect(13, 19, 2, 1, INK)
    img.rect(22, 15, 3, 3, CREAM)                # knobs
    img.rect(22, 19, 3, 2, CREAM_D)
    img.hline(21, 25, 23, red_d)


def moon_pad(img):
    """Moonlight: a crescent moon above a game controller with grips."""
    pale, pale_d = '#E6EEF8', '#A9C1DC'
    for x in range(8, 24):                        # a crescent: one disc minus another
        for y in range(6, 19):
            if (x - 15) ** 2 + (y - 12) ** 2 <= 30 and (x - 18) ** 2 + (y - 10) ** 2 > 20:
                img.put(x, y, pale if y < 13 else pale_d)
    img.put(23, 8, pale)                          # two stars
    img.put(8, 9, pale_d)
    # A modern pad in dark slate: two grips, a stick on each side.
    body, body_l, body_d = '#8494AC', '#B4C2D6', '#3A4658'
    img.rect(8, 19, 16, 5, body)
    img.rect(6, 20, 20, 3, body)
    img.poly([(5, 21), (11, 21), (10, 28), (6, 28)], body)      # grips
    img.poly([(21, 21), (27, 21), (26, 28), (22, 28)], body)
    img.hline(9, 22, 19, body_l)
    img.rect(6, 27, 4, 1, body_d)
    img.rect(22, 27, 4, 1, body_d)
    for (x, y) in ((10, 21), (20, 22)):           # the sticks
        img.rect(x - 1, y - 1, 3, 3, body_d)
        img.rect(x - 1, y - 1, 2, 2, '#1E2430')
    img.rect(14, 21, 4, 1, pale_d)                # a light bar


def turntable(img):
    """Spotify: a record player with its tonearm on the record."""
    wood, wood_d, wood_l = '#9A6438', '#5E3A1E', '#C08A58'
    img.rect(4, 13, 24, 14, wood)
    img.hline(4, 27, 13, wood_l)
    img.rect(4, 25, 24, 2, wood_d)
    img.rect(5, 27, 3, 2, INK)
    img.rect(24, 27, 3, 2, INK)
    img.disc(14, 19, 6, '#161616')                # the record
    for (r, col) in ((4.2, '#3A3A3A'), (2.6, '#262626')):
        for x in range(8, 21):
            for y in range(13, 26):
                d = ((x - 14) ** 2 + (y - 19) ** 2) ** 0.5
                if abs(d - r) < 0.5:
                    img.put(x, y, col)
    img.disc(14, 19, 1.4, '#F2C14E')              # its label
    img.put(14, 19, INK)
    img.put(11, 16, '#6A6A6A')                    # a glint
    img.put(12, 15, '#6A6A6A')
    img.disc(24, 15, 1.2, '#C9C9D0')              # tonearm
    img.line(24, 15, 23, 21, '#E6E6EC')
    img.line(23, 21, 20, 22, '#E6E6EC')
    img.rect(19, 22, 2, 1, INK)
    img.rect(23, 23, 3, 1, '#2FCF6A')              # power light


def theatre(img):
    """Jellyfin: a big screen between curtains, two bears on a couch."""
    cur, cur_d = '#5A2E82', '#3A1C58'
    img.rect(6, 8, 20, 11, '#DCE8F8')             # the screen: woods at night
    img.rect(6, 8, 20, 5, '#B8C8F0')
    img.rect(18, 9, 2, 2, '#FFF6D8')              # the moon
    for (x, h) in ((8, 5), (11, 7), (14, 4), (22, 6), (24, 4)):
        img.vline(x, 18 - h, 18, '#3A5A6A')
        img.hline(x - 1, x + 1, 18 - h // 2, '#3A5A6A')
    img.rect(6, 17, 20, 2, '#5A7A7A')
    img.rect(6, 8, 20, 1, '#FFFFFF')
    img.rect(4, 7, 3, 13, cur)                    # curtains
    img.rect(25, 7, 3, 13, cur)
    img.vline(5, 7, 19, cur_d)
    img.vline(26, 7, 19, cur_d)
    img.rect(4, 6, 24, 2, cur_d)
    for (cx, fur, ear) in ((12, '#B8844E', '#8A5A30'), (19, '#E0B880', '#B8844E')):  # two bears' heads
        img.rect(cx - 3, 19, 2, 2, ear)
        img.rect(cx + 2, 19, 2, 2, ear)
        img.rect(cx - 2, 20, 5, 4, fur)
        img.rect(cx - 3, 21, 7, 2, fur)
    couch, couch_d = '#3A2458', '#24163A'
    img.rect(5, 24, 22, 4, couch)                 # the couch
    img.rect(4, 22, 3, 6, couch_d)
    img.rect(25, 22, 3, 6, couch_d)
    img.hline(7, 24, 24, '#6A4A98')
    img.rect(6, 28, 2, 1, INK)
    img.rect(24, 28, 2, 1, INK)


def cabinet(img):
    """RetroArch: an arcade cabinet, a bear's face on its screen."""
    body, body_d, body_l = '#E0564A', '#8A2A24', '#F08070'
    img.poly([(9, 7), (23, 7), (23, 29), (9, 29)], body)
    img.rect(9, 7, 14, 4, '#FFD34F')              # marquee
    img.hline(10, 22, 8, '#FFF4C8')
    img.rect(8, 7, 1, 23, body_d)
    img.rect(23, 7, 1, 23, body_d)
    img.rect(11, 12, 10, 8, INK)                  # screen
    img.rect(12, 13, 8, 6, '#2A4A6A')
    img.rect(13, 13, 2, 2, '#C89A6A')             # bear face on screen
    img.rect(18, 13, 2, 2, '#C89A6A')
    img.rect(14, 14, 5, 4, '#E8C89A')
    img.rect(13, 15, 7, 2, '#E8C89A')
    img.put(15, 15, INK)
    img.put(17, 15, INK)
    img.put(16, 16, INK)
    img.poly([(8, 21), (24, 21), (25, 24), (7, 24)], body_l)   # control panel
    img.vline(12, 19, 21, '#2A2A2E')              # joystick
    img.rect(11, 18, 3, 2, '#3A6AD8')
    img.rect(16, 22, 2, 1, '#FFD34F')
    img.rect(19, 22, 2, 1, '#4FD86A')
    img.rect(13, 26, 6, 2, body_d)                # coin door
    img.put(15, 26, '#FFD34F')


def popcorn(img):
    """Netflix tile: a striped popcorn bucket, heaped, one kernel falling."""
    red, red_d, white, white_d = '#D8404A', '#8A1E28', '#F6EAD0', '#C8B894'
    corn, corn_d, corn_l = '#FFE6A0', '#E0B050', '#FFF8DC'
    # The heap: overlapping kernels above the rim.
    for (x, y, r) in ((11, 13, 2.6), (15, 11, 3.0), (20, 12.5, 2.6), (13, 15, 2.4), (18, 15, 2.6), (9, 16, 1.8), (22, 16, 1.8)):
        img.disc(x, y, r, corn)
    for (x, y) in ((10, 14), (14, 12), (19, 13), (16, 15), (21, 15), (12, 16)):
        img.put(x, y, corn_d)
    for (x, y) in ((14, 9), (19, 11), (11, 12)):
        img.put(x, y, corn_l)
    img.disc(25, 9, 1.2, corn)                    # a kernel on its way out
    img.put(25, 9, corn_d)
    # The bucket: wider at the top, red and cream stripes.
    top, bottom = 17, 28
    for y in range(top, bottom + 1):
        inset = (y - top) * 3 // (bottom - top)
        x0, x1 = 8 + inset, 23 - inset
        for x in range(x0, x1 + 1):
            stripe = ((x - 8) // 3) % 2 == 0
            col = (red if stripe else white) if x < x1 - 1 else (red_d if stripe else white_d)
            img.put(x, y, col)
    img.hline(7, 24, 17, white)                   # the rim
    img.hline(7, 24, 18, white_d)
    img.hline(11, 20, 28, red_d)


def wand(img):
    """Disney+ tile: a magic wand with a star tip and a trail of sparkles."""
    gold, gold_d, gold_l = '#FFD34F', '#C89A1E', '#FFF4C8'
    stick, stick_l = '#9A9AB8', '#F2F2FA'
    img.line(9, 27, 18, 16, stick)                # the wand, lower left to the star
    img.line(10, 27, 19, 16, stick)
    img.line(9, 26, 18, 15, stick_l)
    img.rect(8, 26, 3, 3, gold_d)                 # its golden end cap
    # A five-pointed star at the tip.
    cx, cy, ro, ri = 21, 12, 6.2, 2.6
    pts = []
    for k in range(10):
        a = -math.pi / 2 + k * math.pi / 5
        r = ro if k % 2 == 0 else ri
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    img.poly(pts, gold)
    img.put(20, 10, gold_l)
    img.put(21, 11, gold_l)
    img.put(23, 15, gold_d)
    img.put(19, 15, gold_d)
    # Sparkles: small four-point stars.
    for (x, y, c) in ((8, 11, gold_l), (13, 8, gold), (26, 21, gold_l), (15, 22, gold)):
        img.put(x, y, c)
        img.put(x - 1, y, c, 0.6)
        img.put(x + 1, y, c, 0.6)
        img.put(x, y - 1, c, 0.6)
        img.put(x, y + 1, c, 0.6)
    img.put(24, 25, gold_d)
    img.put(11, 19, gold_d)


def retro_set(img):
    """Hulu tile: a boxy mint-green 1960s TV set on a stand, one antenna,
    a dial and a speaker grille, a moonlit hill on its screen (not
    YouTube's red TV with rabbit ears and a bear)."""
    mint, mint_d, mint_l = '#4ED88A', '#1E8A4E', '#9AF0BE'
    img.line(19, 11, 24, 6, '#C9C9D0')            # one antenna, to the right
    img.put(24, 6, CREAM)
    img.rect(4, 11, 24, 14, mint)                 # the cabinet
    img.hline(5, 26, 11, mint_l)
    img.rect(4, 23, 24, 2, mint_d)
    img.rect(6, 13, 14, 9, INK)                   # screen, rounded corners
    img.rect(7, 14, 12, 7, '#1E3A5A')
    img.put(7, 14, INK)
    img.put(18, 14, INK)
    img.put(7, 20, INK)
    img.put(18, 20, INK)
    img.disc(15, 16, 1.2, '#FFF6D8')             # the moon
    img.rect(8, 19, 10, 2, '#2E6A4A')             # a hill
    img.rect(10, 18, 5, 1, '#2E6A4A')
    img.disc(23.5, 15, 2, CREAM)                  # the dial
    img.put(23, 14, INK)
    for y in (19, 21):                            # speaker grille
        img.hline(21, 26, y, mint_d)
    img.rect(12, 25, 8, 1, mint_d)                # the stand
    img.line(10, 28, 13, 25, INK)
    img.line(22, 28, 19, 25, INK)


def compass(img):
    """Browser tile: a globe with a gold compass needle across it."""
    sea, sea_d, land, land_d = '#5AB8E8', '#2A7AB0', '#6AD08A', '#3A9A5A'
    cx, cy = 16, 18
    img.disc(cx, cy, 9, sea)
    for x in range(cx - 9, cx + 10):              # shade the lower right
        for y in range(cy - 9, cy + 10):
            if (x - cx) ** 2 + (y - cy) ** 2 <= 81 and (x - cx) + (y - cy) > 6 and img.get(x, y)[3]:
                img.put(x, y, sea_d)
    img.poly([(10, 13), (15, 12), (16, 16), (12, 19), (9, 17)], land)       # two continents
    img.poly([(18, 19), (23, 17), (24, 22), (20, 25)], land)
    img.put(12, 18, land_d)
    img.put(22, 23, land_d)
    for y in (14, 22):                            # latitude lines
        for x in range(cx - 8, cx + 9):
            if (x - cx) ** 2 + (y - cy) ** 2 < 78 and img.get(x, y)[3]:
                img.put(x, y, '#FFFFFF', 0.3)
    # The compass needle: red north, cream south, a pivot.
    img.poly([(16, 17), (21, 11), (17, 18)], '#FFD34F')
    img.poly([(16, 19), (11, 25), (15, 18)], '#8A5A30')
    img.disc(16, 18, 1.2, INK)
    img.put(16, 18, CREAM)


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


def icon(adapter):
    img = Img(S, S)
    badge(img, BADGE[adapter])
    ICONS[adapter](img)
    # The outline hugs the whole badge (ears included), inside the 32×32 grid.
    shape = Img(S, S)
    for i, p in enumerate(img.px):
        if p[3]:
            shape.px[i] = p
    shape.outline(OUTLINE)
    return shape


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    phone = os.path.join(root, 'apps', 'remote-web', 'static', 'art', 'pixel')
    os.makedirs(out, exist_ok=True)
    os.makedirs(phone, exist_ok=True)
    made = []
    for adapter in ICONS:
        img = icon(adapter)
        path = os.path.join(out, f'app-{adapter}.png')
        img.save(path)
        shutil.copyfile(path, os.path.join(phone, f'app-{adapter}.png'))
        made.append(img)
    if preview:
        os.makedirs(preview, exist_ok=True)
        board = Img((S + 4) * len(made), S + 4, '#2A2F3A')
        for i, img in enumerate(made):
            board.blit(img, i * (S + 4) + 2, 2)
        board.scaled(6).save(os.path.join(preview, 'appicons.png'))
    print('appicons: ' + ', '.join(ICONS))
