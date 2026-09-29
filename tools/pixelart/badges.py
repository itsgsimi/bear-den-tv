"""Den badges in pixel art (tools/pixelart; guide docs/THEMES.md → Den badges).
One original medal per badge in internal/achievements (Badges): a round
medallion with a gold rim, a pair of bear ears on top and two ribbon tails,
in the badge's own colours, holding a motif:

    first-night-in  a striped popcorn bucket
    movie-night     a film clapperboard
    couch-explorer  a little sofa
    night-owl       an owl's face under a crescent moon
    early-cub       the sun rising over a hill
    rainy-day       an umbrella in the rain
    snow-day        a snowflake
    thunder-buddy   a cloud with a lightning bolt
    all-seasons     a tree, one quarter per season
    style-switcher  a brush painting half a pixel, half a curve
    theme-tourist   a suitcase with travel stickers
    family-den      three bear heads, big to small
    good-host       a guest ticket
    sleepy-bear     a sleeping bear's head with a "z"
    parade-spotter  a drum with two sticks
    loyal-den       a calendar page with a heart

Every medal also gets a silhouette (the same shape in one slate colour) for
badges not earned yet. All share one 32×32 grid, light from the top left and
a dark 1-pixel outline, like the app icons (appicons.py). Writes
apps/tv-shell/assets/pixel/badge-<id>.png and badge-<id>-locked.png, and the
phone remote's copies in apps/remote-web/static/art/pixel/. The Classic twins
are drawn by tools/classicart/badges.py with the same palettes.
"""

import math
import os
import shutil

from px import Img, mix, rgb

S = 32
OUTLINE = '#120E0C'
INK = '#1A1210'
CREAM = '#F6EAD0'
CREAM_D = '#D8C6A2'
GOLD = '#E8B84A'
GOLD_L = '#FFE08A'
GOLD_D = '#9A6A1A'
LOCKED = '#4A5166'
LOCKED_RIM = '#5F6880'
LOCKED_LINE = '#20232C'

# The medallion's colours per badge: top and bottom of its face, the ears and
# the ribbons (the Classic twin reads the same table).
FACE = {
    'first-night-in': ('#C0463C', '#6E1E1A', '#8A2A22'),
    'movie-night': ('#3A3F58', '#15182A', '#C0463C'),
    'couch-explorer': ('#5A8A5A', '#25462A', '#3A6A3E'),
    'night-owl': ('#2E3A78', '#0E1234', '#4A3A8A'),
    'early-cub': ('#F2A65A', '#C0567A', '#D86A4A'),
    'rainy-day': ('#6A8AA8', '#2A3E58', '#3A5A7A'),
    'snow-day': ('#A8C8E8', '#4A6A9A', '#5A7AAA'),
    'thunder-buddy': ('#5A5070', '#1E1A2E', '#E8B84A'),
    'all-seasons': ('#7AA86A', '#3A5A3A', '#C0703A'),
    'style-switcher': ('#8A5AB8', '#3A1E5A', '#5AA8C8'),
    'theme-tourist': ('#4AA8A0', '#1A5A5A', '#C0463C'),
    'family-den': ('#8AC8A8', '#2E6A58', '#6A4A2A'),
    'good-host': ('#D8A04A', '#8A4A1A', '#4AA8A0'),
    'sleepy-bear': ('#4A5A9A', '#1A2046', '#8A9AD8'),
    'parade-spotter': ('#D8584A', '#7A1E2A', '#4A7AD8'),
    'loyal-den': ('#D86A7A', '#7A2A46', '#A83A5A'),
}

BROWN, BROWN_D, BROWN_L = '#8A5A34', '#5A3A20', '#B8844E'


CURRENT = FACE['first-night-in']  # the face being drawn (badge() sets it)


def face_at(y):
    """The medal face's colour on row y (for cut-outs in a motif)."""
    top, bottom, _ = CURRENT
    return mix(top, bottom, (y - 6) / 20)


def medal(img, face):
    """The shared frame: ribbons, ears, a gold rim and the face."""
    top, bottom, ribbon = face
    rib_d = mix(ribbon, '#000000', 0.35)
    img.poly([(8, 22), (13, 24), (10, 31), (8, 29), (5, 31)], ribbon)     # ribbon tails
    img.poly([(23, 22), (18, 24), (21, 31), (23, 29), (26, 31)], ribbon)
    img.line(8, 23, 7, 30, rib_d)
    img.line(23, 23, 24, 30, rib_d)
    for cx in (8, 23):                                                     # bear ears
        img.disc(cx, 6, 3.2, BROWN)
        img.disc(cx, 6, 1.4, BROWN_L)
    img.disc(15.5, 16, 12, GOLD_D)                                         # the rim
    img.disc(15.5, 15.5, 11.4, GOLD)
    for x in range(4, 28):                                                 # rim light (top left)
        for y in range(4, 28):
            d = math.hypot(x - 15.5, y - 15.5)
            if 10.2 < d <= 11.9 and (x - 15.5) + (y - 15.5) < -6 and img.get(x, y)[3]:
                img.put(x, y, GOLD_L)
    for y in range(6, 27):                                                 # the face
        t = (y - 6) / 20
        col = mix(top, bottom, t)
        for x in range(5, 27):
            if math.hypot(x - 15.5, y - 16) <= 9.6:
                img.put(x, y, col)
    for x in range(8, 24):                                                 # a sheen on the face
        for y in range(7, 11):
            if abs(math.hypot(x - 15.5, y - 16) - 9.0) < 0.6 and x < 18:
                img.put(x, y, '#FFFFFF', 0.25)


# --- motifs (inside x 8..23, y 9..24) ----------------------------------------

def popcorn(img):
    red, white = '#E23A2E', '#F6F0E6'
    img.poly([(9, 14), (22, 14), (20, 24), (11, 24)], white)
    for x0 in (10, 14, 18):
        img.poly([(x0, 14), (x0 + 2, 14), (x0 + 1.6, 24), (x0 + 0.4, 24)], red)
    img.hline(9, 22, 14, '#FFFFFF')
    for (x, y) in ((10, 12), (13, 11), (16, 12), (19, 11), (21, 13), (12, 13), (15, 10), (18, 13)):
        img.disc(x, y, 1.6, '#FFF2C0')
        img.put(x - 1, y - 1, '#FFFFFF')
    img.put(14, 12, '#F2C14E')
    img.put(19, 12, '#F2C14E')


def clapper(img):
    img.rect(9, 15, 14, 9, '#20242E')
    img.rect(9, 15, 14, 1, '#3A4050')
    for x in (11, 15, 19):
        img.rect(x, 18, 2, 1, CREAM_D)
    img.rect(10, 21, 11, 1, CREAM_D)
    img.poly([(9, 11), (22, 9), (23, 12), (10, 14)], '#20242E')             # the open clapper
    for i, x in enumerate((11, 15, 19)):
        img.poly([(x, 10.6 - i * 0.7), (x + 2, 10.2 - i * 0.7), (x + 2.6, 12.9 - i * 0.7), (x + 0.6, 13.3 - i * 0.7)], '#F6F0E6')
    img.put(9, 12, GOLD_L)


def sofa(img):
    body, body_d, body_l = '#E0A04A', '#9A6A2A', '#F2C47A'
    img.rect(10, 11, 12, 6, body_d)                                        # back
    img.rect(11, 12, 10, 4, body)
    img.rect(8, 15, 16, 6, body)                                           # seat
    img.hline(8, 23, 15, body_l)
    img.rect(8, 14, 3, 7, body_d)                                          # arms
    img.rect(21, 14, 3, 7, body_d)
    img.hline(8, 10, 14, body_l)
    img.hline(21, 23, 14, body_l)
    img.rect(9, 21, 2, 2, INK)                                             # feet
    img.rect(21, 21, 2, 2, INK)
    img.rect(14, 12, 4, 3, '#C0463C')                                      # a cushion
    img.hline(14, 17, 12, '#E86A5A')


def owl(img):
    pale = '#FFF6D8'
    for x in range(17, 24):                                                # crescent moon
        for y in range(8, 15):
            if math.hypot(x - 20, y - 11) <= 3.2 and math.hypot(x - 21.6, y - 10) > 2.6:
                img.put(x, y, pale)
    img.put(10, 9, pale)
    img.put(13, 11, pale)
    body, body_d = '#8A6A4A', '#5A3E28'
    img.ellipse(14, 19, 5, 5, body)                                        # the owl
    img.poly([(9, 14), (11, 12), (12, 15)], body_d)                        # ear tufts
    img.poly([(19, 14), (17, 12), (16, 15)], body_d)
    for cx in (12, 16):
        img.disc(cx, 17, 2, '#FFF2C0')
        img.disc(cx, 17, 0.9, INK)
    img.put(11, 16, '#FFFFFF')
    img.put(15, 16, '#FFFFFF')
    img.poly([(13, 19), (15, 19), (14, 21)], '#F2A63A')                    # beak
    img.hline(11, 17, 23, body_d)


def sunrise(img):
    img.disc(15.5, 19, 5.5, '#FFD86A')                                     # the sun
    img.disc(15.5, 19, 4, '#FFF2A8')
    for k in range(7):
        a = math.pi + k * math.pi / 6
        x0, y0 = 15.5 + 7 * math.cos(a), 19 + 7 * math.sin(a)
        x1, y1 = 15.5 + 9 * math.cos(a), 19 + 9 * math.sin(a)
        img.line(x0, y0, x1, y1, '#FFE08A')
    for x in range(6, 26):                                                 # the hill
        h = 20 + 1.6 * math.sin((x - 6) / 5.0)
        for y in range(int(h), 26):
            if math.hypot(x - 15.5, y - 16) <= 9.6:
                img.put(x, y, '#3A6A3A' if y > h + 1 else '#5A8A4A')


def umbrella(img):
    for (x, y) in ((9, 10), (12, 13), (21, 11), (22, 15), (10, 18), (20, 20)):
        img.vline(x, y, y + 1, '#A8D8FF')
    red, red_d = '#E24A3A', '#9A2A1E'
    for x in range(8, 24):                                                 # the canopy
        for y in range(9, 17):
            if math.hypot((x - 15.5) / 7.5, (y - 16) / 6.5) <= 1 and y <= 15:
                img.put(x, y, red)
    for x in (10, 13, 16, 19, 22):
        img.put(x, 15, red_d)
    img.hline(10, 18, 10, '#FF7A6A')
    img.vline(15, 15, 22, '#E6E6EC')                                       # the handle
    img.put(14, 23, '#E6E6EC')
    img.put(13, 22, '#E6E6EC')


def snowflake(img):
    white, blue = '#FFFFFF', '#CFE6FF'
    cx, cy = 15.5, 16
    for k in range(6):
        a = k * math.pi / 3 + math.pi / 2
        ex, ey = cx + 7 * math.cos(a), cy + 7 * math.sin(a)
        img.line(cx, cy, ex, ey, white)
        mx, my = cx + 4.2 * math.cos(a), cy + 4.2 * math.sin(a)
        for s in (-1, 1):
            b = a + s * math.pi / 4
            img.line(mx, my, mx + 2.2 * math.cos(b), my + 2.2 * math.sin(b), blue)
    img.disc(cx, cy, 1.4, white)


def storm(img):
    cloud, cloud_d = '#C8C8D8', '#8A8AA0'
    for (x, y, r) in ((12, 13, 3.4), (16, 11, 4), (20, 13, 3.2), (15, 14, 3.5)):
        img.disc(x, y, r, cloud)
    img.hline(9, 22, 16, cloud_d)
    img.hline(10, 21, 17, cloud_d)
    img.poly([(16, 16), (19, 16), (16.5, 19.5), (19, 19.5), (13, 25), (15, 20.5), (12.5, 20.5)], '#FFD84A')
    img.put(17, 17, '#FFF6C0')


def seasons(img):
    cols = {'tl': '#8AD86A', 'tr': '#3A9A3A', 'bl': '#E8A03A', 'br': '#FFFFFF'}
    cx, cy = 15.5, 14
    for x in range(8, 24):                                                 # the crown, one quarter each
        for y in range(7, 21):
            if math.hypot((x - cx) / 7, (y - cy) / 6) <= 1:
                q = ('t' if y < cy else 'b') + ('l' if x < cx else 'r')
                img.put(x, y, cols[q])
    img.put(11, 11, '#FF9AC8')                                             # spring blossoms
    img.put(13, 9, '#FF9AC8')
    img.put(10, 13, '#FF9AC8')
    img.put(19, 18, '#CFE6FF')                                             # winter's snow edge
    img.rect(14, 18, 3, 6, BROWN)                                          # the trunk
    img.vline(14, 18, 23, BROWN_D)
    img.hline(10, 21, 24, '#3A5A3A')


def brush(img):
    # Left half: stepped pixels; right half: a smooth arc. A brush between.
    for i, (x, y) in enumerate(((8, 20), (9, 19), (10, 18), (11, 17), (12, 17))):
        img.rect(x, y, 1, 1, '#FFD84A')
        img.rect(x, y + 1, 1, 2, '#FF8A4A')
    for x in range(15, 24):
        y = 20 - 3.2 * math.sin((x - 15) / 8 * math.pi)
        img.put(x, round(y), '#7AE0FF')
        img.put(x, round(y) + 1, '#3AA0D8')
    img.line(13, 20, 20, 10, '#C08A58')                                    # the handle
    img.line(14, 20, 21, 10, '#9A6438')
    img.rect(11, 20, 3, 2, '#C9C9D0')                                      # ferrule and tip
    img.rect(10, 22, 3, 2, INK)


def suitcase(img):
    body, body_d = '#C87A3A', '#8A4A1E'
    img.hline(12, 19, 10, body_d)                                          # the handle
    img.vline(12, 10, 12, body_d)
    img.vline(19, 10, 12, body_d)
    img.rect(8, 13, 16, 10, body)
    img.hline(8, 23, 13, '#E8A060')
    img.rect(8, 22, 16, 1, body_d)
    img.vline(12, 13, 22, body_d)                                          # straps
    img.vline(19, 13, 22, body_d)
    img.rect(14, 15, 4, 3, '#4AD8C8')                                      # stickers
    img.rect(9, 18, 2, 2, '#FFD84A')
    img.rect(20, 15, 3, 3, '#E86A8A')
    img.put(15, 16, '#FFFFFF')


def family(img):
    def head(cx, cy, r, fur, ear, line):
        for (ex, ey) in ((cx - r * 0.72, cy - r * 0.72), (cx + r * 0.72, cy - r * 0.72)):
            img.disc(ex, ey, r * 0.42, line)
            img.disc(ex, ey, r * 0.3, ear)
        img.disc(cx, cy, r + 0.6, line)
        img.disc(cx, cy, r, fur)
        img.ellipse(cx, cy + r * 0.38, r * 0.42, r * 0.26, CREAM)
        img.put(round(cx), round(cy + r * 0.22), INK)
        img.put(round(cx - r * 0.42), round(cy - r * 0.18), INK)
        img.put(round(cx + r * 0.42), round(cy - r * 0.18), INK)
    head(11, 14, 3.4, '#7A4A28', '#4A2A14', '#2A1810')                    # dad
    head(20.5, 14.5, 3.0, '#C08A58', '#8A5A30', '#4A2A14')                # mama
    head(15.5, 20.5, 2.5, '#E8C08A', '#C08A58', '#6A4424')                # the cub


def ticket(img):
    paper, paper_d = '#FFF2C8', '#D8B878'
    img.poly([(8, 13), (23, 11), (24, 20), (9, 22)], paper)
    for (x, y) in ((8, 17), (8, 18), (9, 17), (9, 18), (23, 15), (24, 15), (23, 16), (24, 16)):
        img.put(x, y, face_at(y))                                          # the notches show the face
    for y in range(13, 22, 2):                                             # the tear line
        img.put(19, y - 1, paper_d)
    img.hline(11, 17, 15, '#C0463C')                                       # "GUEST"
    img.hline(11, 16, 17, paper_d)
    img.hline(11, 15, 19, paper_d)
    img.disc(21.5, 16, 1, '#C0463C')


def sleepy(img):
    head(img, 14, 18, 5.5)
    img.hline(11, 13, 17, INK)                                             # closed eyes
    img.hline(15, 17, 17, INK)
    img.rect(18, 9, 4, 1, '#FFFFFF')                                       # z
    img.line(21, 10, 18, 12, '#FFFFFF')
    img.rect(18, 12, 4, 1, '#FFFFFF')
    img.rect(22, 13, 2, 1, '#CFE6FF')
    img.put(23, 14, '#CFE6FF')
    img.rect(22, 15, 2, 1, '#CFE6FF')


def head(img, cx, cy, r):
    img.disc(cx - r * 0.72, cy - r * 0.72, r * 0.36, BROWN_D)
    img.disc(cx + r * 0.72, cy - r * 0.72, r * 0.36, BROWN_D)
    img.disc(cx, cy, r, BROWN)
    img.ellipse(cx, cy + r * 0.42, r * 0.45, r * 0.3, CREAM)
    img.put(round(cx), round(cy + r * 0.25), INK)


def drum(img):
    img.line(9, 9, 14, 14, '#E6D2A8')                                      # sticks
    img.line(22, 9, 17, 14, '#E6D2A8')
    img.disc(9, 9, 1, '#FFFFFF')
    img.disc(22, 9, 1, '#FFFFFF')
    img.ellipse(15.5, 15, 7, 2, '#F6F0E6')                                 # the head
    img.rect(9, 15, 14, 7, '#4A7AD8')                                      # the shell
    img.ellipse(15.5, 22, 7, 2, '#2A4A9A')
    img.hline(9, 22, 15, '#FFD84A')
    for i, x in enumerate(range(9, 23, 3)):                                # the cords
        img.line(x, 16, x + 1.5, 21, '#FFD84A')
    img.ellipse(15.5, 15, 6, 1.2, '#FFFFFF')


def calendar(img):
    img.rect(9, 10, 14, 13, '#FFF6E8')
    img.rect(9, 10, 14, 4, '#C0463C')
    img.rect(11, 9, 1, 3, INK)
    img.rect(20, 9, 1, 3, INK)
    for x in range(10, 22, 3):                                             # days
        for y in (15, 18, 21):
            img.put(x, y, CREAM_D)
    heart = [(14, 16), (15, 16), (17, 16), (18, 16), (13, 17), (14, 17), (15, 17), (16, 17), (17, 17), (18, 17), (19, 17),
             (14, 18), (15, 18), (16, 18), (17, 18), (18, 18), (15, 19), (16, 19), (17, 19), (16, 20)]
    for (x, y) in heart:
        img.put(x, y, '#E24A6A')
    img.put(14, 16, '#FF8AA8')


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


def badge(bid):
    global CURRENT
    CURRENT = FACE[bid]
    img = Img(S, S)
    medal(img, FACE[bid])
    MOTIFS[bid](img)
    shape = Img(S, S)
    for i, p in enumerate(img.px):
        if p[3]:
            shape.px[i] = p[:3] + (255,)
    shape.outline(OUTLINE)
    return shape


def silhouette(img):
    """The same medal in slate: the shape, the rim and nothing else."""
    out = Img(S, S)
    for i, p in enumerate(img.px):
        if not p[3]:
            continue
        x, y = i % S, i // S
        if p[:3] == (0x12, 0x0E, 0x0C):
            out.px[i] = rgb(LOCKED_LINE)
        elif 10.3 < math.hypot(x - 15.5, y - 15.8) <= 12.2:
            out.px[i] = rgb(LOCKED_RIM)
        else:
            out.px[i] = rgb(LOCKED)
    for (x, y) in QMARK:                                                   # a question mark
        out.px[(y + 12) * S + x + 13] = rgb(LOCKED_RIM)
    return out


QMARK = [(1, 0), (2, 0), (3, 0), (0, 1), (4, 1), (4, 2), (3, 3), (2, 4), (2, 5), (2, 7)]


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    phone = os.path.join(root, 'apps', 'remote-web', 'static', 'art', 'pixel')
    os.makedirs(out, exist_ok=True)
    os.makedirs(phone, exist_ok=True)
    made = []
    for bid in MOTIFS:
        img = badge(bid)
        for name, pic in ((f'badge-{bid}.png', img), (f'badge-{bid}-locked.png', silhouette(img))):
            path = os.path.join(out, name)
            pic.save(path)
            shutil.copyfile(path, os.path.join(phone, name))
        made.append(img)
    if preview:
        os.makedirs(preview, exist_ok=True)
        cols = 8
        rows = (len(made) + cols - 1) // cols + 1
        board = Img((S + 4) * cols, (S + 4) * rows, '#2A2F3A')
        for i, img in enumerate(made):
            board.blit(img, (i % cols) * (S + 4) + 2, (i // cols) * (S + 4) + 2)
        board.blit(silhouette(made[0]), 2, (rows - 1) * (S + 4) + 2)
        board.scaled(6).save(os.path.join(preview, 'badges.png'))
    print('badges: ' + ', '.join(MOTIFS))
