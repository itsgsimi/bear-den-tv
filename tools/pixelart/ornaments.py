"""The built-in ornaments as pixel-art icons (tools/pixelart): small sprites
for the focus decorations, rail headings, what bears carry and wear, and
dialog flourishes. Writes apps/tv-shell/assets/ornaments/<name>.png (and the
Winter theme's own snowflake). Drawn as character grids; most get a dark
1-pixel outline so they read over any wallpaper, glowing ones (stars, flames,
the moon) don't. Scenes use their own larger sprites (scenes.py).
"""

import os

from px import Img, from_ascii

PAL = {
    'W': '#F7F0E2', 'w': '#D9CBB4', 'Y': '#F2C14E', 'y': '#C98A2E',
    'R': '#D9483F', 'r': '#9E2F2F', 'P': '#F2A7B8', 'p': '#C9728A',
    'G': '#7FB56F', 'g': '#4E8458', 'h': '#B5DE9A',
    'B': '#8A6443', 'b': '#5C3F28', 'n': '#C89A68', 'c': '#E3C08A',
    'O': '#FF8A3D', 'o': '#C9552B', 'F': '#FFD490', 'f': '#FFF4C8',
    'K': '#3A3F4A', 'k': '#22262E', 'S': '#8A93A6', 's': '#C4CAD6',
    'M': '#EEF2FF', 'm': '#B7C3DF', 'L': '#F4E3A1', 'U': '#5B8FD6', 'u': '#3A639E',
    'C': '#EDE3D1', 'V': '#79A889', 'v': '#5E8C6E',
}
OUTLINE = '#17130F'

# name: (rows, outlined)
SPRITES = {
    'daisy': ([
        '..W.W..',
        '.WWWWW.',
        'WWYYYWW',
        '.WYyYW.',
        'WWYYYWW',
        '.WWWWW.',
        '..W.W..'], True),
    'blossom': ([
        '..P.P..',
        '.PPpPP.',
        'PPpYpPP',
        '.pYyYp.',
        'PPpYpPP',
        '.PPpPP.',
        '..P.P..'], True),
    'heart': ([
        '.RR.RR.',
        'RPRRRRR',
        'RRRRRRR',
        '.RRRRr.',
        '..RRr..',
        '...r...'], True),
    'star': ([
        '...Y...',
        '...Y...',
        '..YLY..',
        'YYLLLYY',
        '.YLLLY.',
        '.YY.YY.',
        '.Y...Y.'], False),
    'sparkle': ([
        '...L...',
        '...L...',
        '..LWL..',
        'LLWWWLL',
        '..LWL..',
        '...L...',
        '...L...'], False),
    'moon': ([
        '..MMM..',
        '.MMm...',
        'MMm....',
        'MMm....',
        'MMm....',
        '.MMm...',
        '..MMM..'], False),
    'flame': ([
        '...O...',
        '..OO...',
        '..OFO..',
        '.OOFO..',
        '.OFFFO.',
        'OOFfFOO',
        'OFfffFO',
        'oOFfFOo',
        '.oOOOo.'], False),
    'sprig': ([
        '..GG...GG..',
        '.GhGg.GhGg.',
        'ggggggggggg',
        '.gGGG..gGG.',
        '..GG....G..'], True),
    'berries': ([
        '..gGg..',
        '...g...',
        '.RR.RR.',
        'RPRRPRR',
        'RRrRRrR',
        '.rr.rr.'], True),
    'mushroom': ([
        '.RRRRR.',
        'RWRRRWR',
        'RRRWRRR',
        'rrrrrrr',
        '..CCC..',
        '..CwC..',
        '..CCC..'], True),
    'paw': ([
        '.v.v.v.',
        '.v.v.v.',
        '.......',
        '..vvv..',
        '.vvvvv.',
        '.vvvvv.',
        '..v.v..'], False),
    'paw-grip': ([
        '.VVVVV.',
        'VVCVCVV',
        'VVVVVVV'], True),
    'popcorn': ([
        '.W.W.W.',
        'WWYWWWW',
        'WYWWWYW',
        'RWRWRWR',
        'RWRWRWR',
        'RWRWRWR',
        '.RWRWR.'], True),
    'remote': ([
        'KKKKK',
        'KRKKK',
        'KKKKK',
        'KsKsK',
        'KKKKK',
        'KsKsK',
        'KKKKK',
        'KsKsK',
        'KKKKK'], True),
    'controller': ([
        '.KKKKKKK.',
        'KKsKKKRKK',
        'KsssKRKUK',
        'KKsKKKUKK',
        'KKKK.KKKK',
        '.KK...KK.'], True),
    'hat-beanie': ([
        '.......WW.......',
        '......WWWW......',
        '.....RRRRRR.....',
        '...RRRRRRRRRR...',
        '..RRrRRrRRrRRR..',
        '..RRRRRRRRRRRR..',
        '.WWWWWWWWWWWWWW.',
        '.wWwWwWwWwWwWwW.'], True),
    'hat-nightcap': ([
        '............WW..',
        '..........UUWW..',
        '........UUUu....',
        '......UUUUUu....',
        '....UUUUYUUU....',
        '..UUUUUUUUUUU...',
        '.UUUYUUUUUUYUU..',
        '.WWWWWWWWWWWWWW.',
        '.wWwWwWwWwWwWwW.'], True),
    # Bear tips (TipBear.qml): the Themes bear's painter's beret with a dab
    # of paint, the tiny phone a phone tip's bear holds, and the wooden
    # arrow on top of a tip's sign.
    'hat-beret': ([
        '..........kk....',
        '.....rRRRRRr....',
        '...RRRRRRRRRRR..',
        '..RRRYRRRRRRRRR.',
        '.RRRYGRRRRRRRRRR',
        '..rrrrrrrrrrrrr.'], True),
    'phone': ([
        'KKKKK',
        'KmmmK',
        'KUMUK',
        'KMUMK',
        'KmmmK',
        'KKsKK'], True),
    'pointer-up': ([
        '...c...',
        '..cnB..',
        '.cnBBb.',
        'cnBBBbb',
        '..nBb..',
        '..nBb..',
        '..nBb..'], True),
    'pumpkin': ([
        '....gg...',
        '...g.....',
        '.OOoOOoO.',
        'OOOoOOoOO',
        'OOoOOOoOO',
        'OOoOOOoOO',
        'oOOoOOoOo',
        '.ooooooo.'], True),
    'tent': ([
        '....b....',
        '...nBb...',
        '..nnBBb..',
        '.nnnkBBb.',
        'nnnkkkBBb'], True),
    'lantern': ([
        '.kkk.',
        '..k..',
        'kkkkk',
        'kFYFk',
        'kYfYk',
        'kFYFk',
        'kkkkk'], True),
    'logs': ([
        '..BBBBBn.',
        '.BBBBBBnc',
        '.bbbbbbnn',
        'BBBBBBn..',
        'bbbbbbnc.'], True),
    'pine': ([
        '...G...',
        '..GGG..',
        '.GGgGG.',
        '..GGG..',
        '.GGgGG.',
        'GGGgGGG',
        '..GGG..',
        '.GGgGGG',
        'GGGGgGG',
        '...b...'], True),
    'den': ([
        '...ssss...',
        '..sSSSSs..',
        '.sSSkkSSs.',
        'sSSkkkkSSs',
        'sSkkkkkkSs',
        'SSkkkkkkSS'], True),
    'corner': ([
        'gggggggGh',
        'g..G..G..',
        'g.Gh.....',
        'g........',
        'gG.......',
        'gGh......',
        'g........',
        'gG.......',
        'h........'], False),
    'divider': ([
        '.....G...W.W...G.....',
        'ggggghg.WWYWW.ghggggg',
        '.....G...W.W...G.....'], False),
}

EXTRA = {
    # The Winter theme's own ornament (themes/winter/snowflake.png).
    ('themes', 'winter', 'snowflake'): ([
        '...M...',
        '.M.M.M.',
        '..MMM..',
        'MMMmMMM',
        '..MMM..',
        '.M.M.M.',
        '...M...'], False),
}


def sprite(rows, outlined):
    img = from_ascii(rows, PAL)
    if not outlined:
        return img
    out = Img(img.w + 2, img.h + 2)
    out.blit(img, 1, 1)
    return out.outline(OUTLINE)


def build(root, preview):
    folder = os.path.join(root, 'apps', 'tv-shell', 'assets', 'ornaments')
    made = []
    for name, (rows, outlined) in SPRITES.items():
        img = sprite(rows, outlined)
        img.save(os.path.join(folder, name + '.png'))
        made.append(img)
    for path, (rows, outlined) in EXTRA.items():
        img = sprite(rows, outlined)
        img.save(os.path.join(root, *path[:-1], path[-1] + '.png'))
        made.append(img)
    if preview:
        os.makedirs(preview, exist_ok=True)
        board = Img(sum(m.w + 3 for m in made) + 3, max(m.h for m in made) + 6, '#2A2F3A')
        x = 3
        for m in made:
            board.blit(m, x, 3)
            x += m.w + 3
        board.scaled(6).save(os.path.join(preview, 'ornaments.png'))
    print(f'ornaments: {len(made)}')
