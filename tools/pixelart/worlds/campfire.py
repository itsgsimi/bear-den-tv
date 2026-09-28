"""Campfire world backdrop: a cold, rainy night in the woods with warm pockets
of firelight — a far tent with its own little fire, smoke drifting, rain
falling and rings on the puddles. The big fire and the bears are Home's
corner scene (CampfireScene.qml), lit by the glow painted here."""

from px import Img, mix
import common as c

ID = 'campfire'


def build():
    W, H = c.W, c.H
    img = Img(W, H, '#05070F')
    img.vgradient(0, 0, W, 205, [(0, '#04060D'), (0.45, '#0A1020'), (0.8, '#141E36'), (1, '#1E2B4A')])
    # The moon, half behind cloud banks whose tops it silvers.
    img.glow(96, 40, 90, '#34466E', 0.45, ry=60, bands=5)
    img.disc(96, 38, 8, '#B7C3DF')
    img.disc(95, 37, 7, '#D6DEF2')
    img.put(92, 36, '#B7C3DF'); img.put(98, 41, '#B7C3DF'); img.put(97, 35, '#C4CEE6')
    c.clouds(img, 11, 0, 70, '#121A2E', light='#3A4A70', dark='#0B1122', cover=0.5, scale=70, light_from=(96, 38))
    c.clouds(img, 23, 34, 128, '#0F1628', light='#2A3858', dark='#0A0F1C', cover=0.46, scale=90, light_from=(96, 38))
    # Mountains, rim-lit from the moon on the left.
    c.ridge(img, 3, 172, 54, 95, '#172139', rim='#27355A', rim_side=1)
    c.ridge(img, 7, 192, 30, 48, '#10182A', rim='#1A2542', rim_side=1)
    # Far forest, then the tall dark edges of the clearing.
    c.forest(img, 5, 204, 170, 8, 20, '#0B1220', rim='#152038', rim_side=-1)
    img.vgradient(0, 205, W, H - 205, [(0, '#0B0E14'), (1, '#07080B')])
    c.forest(img, 9, 214, 26, 40, 86, '#06090F', x0=0, x1=130, density=lambda x: 1.0 - x / 170)
    c.forest(img, 13, 216, 18, 36, 70, '#06090F', x0=392, x1=480)
    # Warm light from the corner fire, bottom right, and the tent's fire.
    img.glow(440, 268, 150, '#FF7A2F', 0.42, ry=70, bands=5)
    img.glow(292, 222, 46, '#FF8A3D', 0.36, ry=22, bands=4)
    # The far tent: canvas lit from inside, door glowing.
    tent_x, tent_y = 250, 224
    img.poly([(tent_x, tent_y), (tent_x + 13, tent_y - 16), (tent_x + 26, tent_y)], '#3B2A22')
    img.poly([(tent_x + 13, tent_y - 16), (tent_x + 26, tent_y), (tent_x + 18, tent_y)], '#2A1D18')
    img.poly([(tent_x + 9, tent_y), (tent_x + 13, tent_y - 8), (tent_x + 17, tent_y)], '#E89A4A')
    img.line(tent_x + 13, tent_y - 16, tent_x + 13, tent_y - 19, '#2A1D18')
    # Logs by the tent's fire.
    img.rect(284, 226, 14, 2, '#3A2416')
    img.rect(286, 225, 10, 1, '#5A3620')
    # A path of trodden earth from the tent towards the corner fire.
    rng = c.Rng(41)
    for i in range(160):
        t = i / 159
        x = 272 + t * 200 + rng.uniform(-3, 3)
        y = 228 + t * t * 44 + rng.uniform(-1, 1)
        img.put(x, y, '#16120F')
        img.put(x + 1, y, '#1B1510')
    # Grass tufts and stones, warmer where the firelight reaches.
    for i in range(420):
        x, y = rng.int(0, W - 1), rng.int(212, H - 1)
        warm = max(0.0, 1 - (((x - 440) / 170) ** 2 + ((y - 268) / 80) ** 2) ** 0.5)
        warm = max(warm, max(0.0, 1 - (((x - 292) / 40) ** 2 + ((y - 224) / 16) ** 2) ** 0.5))
        blade = c.mix('#141A1C', '#6B4A2A', min(1, warm * 1.3))
        tip = c.mix('#1C2426', '#B8793C', min(1, warm * 1.3))
        if rng.next() < 0.85:
            img.put(x, y, blade)
            img.put(x + rng.choice([-1, 0, 1]), y - 1, tip)
        else:
            img.rect(x, y, 3, 2, c.mix('#15181D', '#5A3E2A', warm))
            img.put(x + 1, y, c.mix('#20242B', '#8A6040', warm))
    # Puddles holding the sky and the firelight.
    for (x, y, rx, ry) in ((196, 242, 18, 3), (330, 236, 12, 2), (372, 254, 20, 3), (120, 258, 16, 2)):
        img.ellipse(x, y, rx, ry, '#1A2440')
        img.ellipse(x + rx // 3, y, rx // 3, max(1, ry - 1), '#2B3A60')
    img.hline(360, 366, 254, '#FF9A4A')
    img.hline(318, 322, 236, '#E88A40')

    sprites = [
        ('fire-small.png', c.sheet(c.flame_frames(9, 12, 4, seed=4)), 8, 286, 214),
        ('smoke.png', c.sheet(c.smoke_frames(28, 60, 8, '#6A7388', seed=5)), 6, 282, 156),
        ('ripples.png', c.sheet(c.ripple_frames(40, 8, 6, '#6E82B0')), 6, 176, 238),
        ('ripples.png', None, 6, 352, 250),
        ('rain.png', c.sheet(c.rain_frames(17, W, H, 6, 420, '#8FA6D0', alpha=(0.16, 0.38))), 12, 0, 0),
    ]
    return img, sprites




def phone(img):
    return img.crop(236, 0, 168, 270)
