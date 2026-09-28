"""Pixel-art weather icons for the Home header chip and Settings → Weather
(tools/pixelart; guide docs/THEMES.md → Pixel art, spec: the `weather` block of
contracts/state.schema.json). Writes apps/tv-shell/assets/pixel/weather-<name>.png
for every icon the shell picks from `state.weather.current`:

    sun, moon, sun-cloud, moon-cloud, cloud, fog, drizzle, rain, snow, thunder

Each icon is composed from a few character-grid parts (sun, moon, cloud,
drops, flakes, bolt) on a 14×14 canvas and gets the same dark 1-pixel outline
as the ornaments, so all ten are 16×16 and read over any wallpaper.
"""

import os

from px import Img, from_ascii

PAL = {
    'L': '#FFF4C8', 'Y': '#F2C14E', 'y': '#C98A2E',
    'M': '#EEF2FF', 'm': '#B7C3DF',
    'W': '#F7F3EA', 'w': '#C9CFDA', 'S': '#9AA3B5', 's': '#6E778A',
    'U': '#7FB3F0', 'u': '#4A7FC4', 'F': '#FFE27A',
}
OUTLINE = '#17130F'
SIZE = 14

SUN = [
    '....L....',
    '.L.....L.',
    '...YYY...',
    '..YLLLY..',
    'L.YLLLY.L',
    '..YLLLy..',
    '...yyy...',
    '.L.....L.',
    '....L....',
]
BIG_SUN = [
    '......L......',
    '..L...L...L..',
    '...L.....L...',
    '.....YYY.....',
    '....YLLLY....',
    '...YLLLLLY...',
    'LL.YLLLLLY.LL',
    '...YLLLLLy...',
    '....yLLLy....',
    '.....yyy.....',
    '...L.....L...',
    '..L...L...L..',
    '......L......',
]
MOON = [
    '...MMMM..',
    '..MMMm...',
    '.MMMm....',
    '.MMm.....',
    '.MMm.....',
    '.MMMm....',
    '..MMMMm..',
    '...MMMMm.',
    '.....m...',
]
CLOUD = [
    '.....WWW....',
    '...WWWWWW...',
    '..WWWWWWWWW.',
    '.WWWWWWWWWWW',
    'WWWWWWWWWWWW',
    'wWWWWWWWWWWw',
    '.wwwwwwwwww.',
]
DARK = {'W': PAL['S'], 'w': PAL['s']}
FOG = [
    '.WWWWWWWWW....',
    '..............',
    '...wwwwwwwwwww',
    '..............',
    'WWWWWWWWWWW...',
    '..............',
    '..wwwwwwwwwww.',
]
BOLT = [
    '..FF',
    '.FF.',
    'FFFF',
    '.FF.',
    'FF..',
    'F...',
]


def part(rows, pal=None):
    p = dict(PAL)
    p.update(pal or {})
    return from_ascii(rows, p)


def drops(img, y, spots, c):
    for (x, dy) in spots:
        img.put(x, y + dy, c)
        img.put(x - 1, y + dy + 1, c)


def icon(name):
    img = Img(SIZE, SIZE)
    if name == 'sun':
        img.blit(part(BIG_SUN), 0, 0)
    elif name == 'moon':
        img.blit(part(MOON), 3, 2)
    elif name in ('sun-cloud', 'moon-cloud'):
        img.blit(part(SUN if name == 'sun-cloud' else MOON), 0, 0)
        img.blit(part(CLOUD), 2, 6)
    elif name == 'cloud':
        img.blit(part(CLOUD, DARK), 2, 2)   # a grey cloud behind
        img.blit(part(CLOUD), 0, 5)
    elif name == 'fog':
        img.blit(part(FOG), 0, 3)
    elif name in ('drizzle', 'rain', 'thunder'):
        img.blit(part(CLOUD, DARK if name != 'drizzle' else None), 1, 0)
        if name == 'drizzle':
            for x, dy in ((3, 0), (7, 2), (11, 0)):
                img.put(x, 8 + dy, PAL['U'])
                img.put(x, 10 + dy, PAL['U'])
        elif name == 'rain':
            drops(img, 8, ((4, 0), (8, 1), (12, 0), (6, 3), (10, 3)), PAL['U'])
        else:
            img.blit(part(BOLT), 6, 7)
            drops(img, 8, ((3, 1), (12, 1)), PAL['U'])
    elif name == 'snow':
        img.blit(part(CLOUD), 1, 0)
        for (x, y) in ((3, 9), (8, 8), (12, 10), (5, 12), (10, 12)):
            img.put(x, y, PAL['M'])
            if (x + y) % 2:
                for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                    img.put(x + dx, y + dy, PAL['m'])
    else:
        raise ValueError(name)
    out = Img(SIZE + 2, SIZE + 2)
    out.blit(img, 1, 1)
    return out.outline(OUTLINE)


NAMES = ['sun', 'moon', 'sun-cloud', 'moon-cloud', 'cloud', 'fog', 'drizzle', 'rain', 'snow', 'thunder']


def build(root, preview):
    folder = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    made = []
    for name in NAMES:
        img = icon(name)
        img.save(os.path.join(folder, 'weather-' + name + '.png'))
        made.append(img)
    if preview:
        os.makedirs(preview, exist_ok=True)
        board = Img(sum(m.w + 3 for m in made) + 3, max(m.h for m in made) + 6, '#2A2F3A')
        x = 3
        for m in made:
            board.blit(m, x, 3)
            x += m.w + 3
        board.scaled(8).save(os.path.join(preview, 'weather.png'))
    print(f'weather: {len(made)}')
