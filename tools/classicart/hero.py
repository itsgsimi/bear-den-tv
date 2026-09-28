"""The featured panel's rooms in the Classic art style (tools/classicart): the
smooth counterparts of tools/pixelart/hero.py, drawn on the same 176×92 grid
so the same rig (qml/HeroRig.js: size, the icon's `screen` rectangle, times of
day, intro) places them. Writes apps/tv-shell/assets/classic/:

- hero-cinema.svg, hero-arcade.svg, hero-cabin-<night|dawn|day|dusk>.svg: the
  rooms (the cabin's window shows the time of day);
- hero-<scene>-glow.svg: each room's light (projector beam, TV glow, neon),
  which HeroScene pulses on World's heartbeat instead of pixel frames;
- hero-static-0.svg, hero-static-1.svg: the cabin TV's intro static, over the
  screen rectangle (40×30), alternated while the intro plays.
"""

import math
import os

from svg import Svg, Rng, n, pine

W, H = 176, 92
SCALE = 4                      # the SVG's nominal size: 704×368, as the pixel room on a 1080p TV
TIMES = ['night', 'dawn', 'day', 'dusk']
CINEMA_SCREEN = (50, 10, 76, 42)
CABIN_SCREEN = (70, 26, 40, 30)
ARCADE_SCREEN = (68, 18, 40, 30)


def canvas(label, note):
    return Svg(W * SCALE, H * SCALE, label, note, W, H)


# --- Cinema (Plex) ---------------------------------------------------------------
def curtain(s, left):
    """A draped curtain with soft folds, gathered at a gold tieback."""
    x0, x1 = (0, 36) if left else (W, W - 36)
    sign = 1 if left else -1
    stops = []
    folds = 7
    for k in range(folds * 2 + 1):
        stops.append((k / (folds * 2), '#9A2634' if k % 2 else '#5E141E'))
    fill = s.linear_abs(stops, x0, 0, x1, 0)
    inner = x1
    d = (f'M{n(x0)} 0 L{n(inner)} 0 Q{n(inner - sign * 4)} 22 {n(x0 + sign * 16)} 46 '
         f'Q{n(x0 + sign * 26)} 60 {n(x0 + sign * 30)} 74 L{n(x0)} 74 Z')
    s.path(d, fill)
    s.path(d, s.linear_abs([(0, '#000000', 0), (1, '#000000', 0.35)], x1, 0, x0, 0))
    # Tieback.
    s.ellipse(x0 + sign * 14, 46, 5, 2.2, s.linear([(0, '#F4D27A'), (1, '#A87A2A')]))


def cinema(s):
    s.rect(0, 0, W, H, s.linear([(0, '#24141A'), (1, '#0C0809')]))
    sx, sy, sw, sh = CINEMA_SCREEN
    # Screen light on the room.
    s.glow(sx + sw / 2, sy + sh / 2, 80, 48, '#E8D9B0', 0.22)
    s.rect(sx - 2.5, sy - 2.5, sw + 5, sh + 5, s.linear([(0, '#3A2A24'), (1, '#1E1512')]), rx=1.5)
    s.rect(sx, sy, sw, sh, s.radial([(0, '#F6F0E2'), (0.8, '#E6DDC8'), (1, '#CFC3A8')]), rx=0.6)
    curtain(s, True)
    curtain(s, False)
    # Valance with scallops and gold trim.
    s.rect(0, 0, W, 5, s.linear([(0, '#8E2230'), (1, '#6E1823')]))
    for x in range(0, W + 1, 8):
        s.ellipse(x + 4, 5, 4.2, 2.6, '#7A1C28')
    s.rect(0, 0, W, 1.2, '#D9B35A')
    # Projector booth window, top right.
    s.rect(W - 13, 1.5, 9, 5, '#2A2A2E', rx=1)
    s.circle(W - 8.5, 4, 1.5, s.radial([(0, '#FFFBE8'), (1, '#E8C870')]))
    # Rows of seats: rounded backs catching the screen light.
    for row, (y0, col, hi) in enumerate(((66, '#5A1C28', '#8A3040'), (74, '#46141E', '#6E2432'),
                                         (83, '#300C14', '#50182A'))):
        off = 4 if row % 2 else 0
        g = s.linear([(0, hi), (0.35, col), (1, '#140608')])
        for x in range(-8 + off, W, 16):
            s.rect(x + 0.5, y0 - 2, 12, 12.5, g, rx=3.5)
            s.rect(x + 2, y0 - 1.2, 9, 1.2, '#FFFFFF', rx=0.6, opacity=0.08)


def cinema_glow(s):
    sx, sy, sw, sh = CINEMA_SCREEN
    bx, by = W - 8.5, 4
    beam = s.linear_abs([(0, '#FFF6D8', 0.55), (1, '#FFF6D8', 0.04)], bx, by, sx + sw / 2, sy + sh)
    s.poly([(bx, by - 1), (bx, by + 1), (sx + 10, sy + sh - 4), (sx + sw - 8, sy + 2)], beam)
    rng = Rng(7)
    for _ in range(14):
        p = rng.next()
        x = bx - p * (bx - sx - 30) + rng.uniform(-4, 4)
        y = by + p * (sy + sh - by) * 0.75
        s.circle(x, y, rng.uniform(0.25, 0.55), '#FFF8E0', rng.uniform(0.4, 0.9))
    s.glow(sx + sw / 2, sy + sh / 2, 56, 34, '#FFF1C8', 0.16)


# --- Cabin (YouTube) --------------------------------------------------------------
SKIES = {
    'night': ('#0A1024', '#1E2C52'),
    'dawn': ('#3A4C7A', '#F0B08A'),
    'day': ('#6FA8DC', '#CDE6F4'),
    'dusk': ('#2A2448', '#E08A5A'),
}


def cabin(s, tod):
    dark = tod == 'night'
    wall = ('#6A4A32', '#4A3222') if not dark else ('#44301F', '#2C1E14')
    s.rect(0, 0, W, H, s.linear([(0, wall[0]), (1, wall[1])]))
    # Log planks.
    for y in range(6, 76, 7):
        s.line(0, y, W, y, '#000000', 0.8, 0.22)
        s.line(0, y + 0.8, W, y + 0.8, '#FFFFFF', 0.4, 0.06)
    # Window with the time of day.
    wx, wy, ww, wh = 12, 12, 34, 30
    s.rect(wx - 2.5, wy - 2.5, ww + 5, wh + 5, s.linear([(0, '#3A281C'), (1, '#24180F')]), rx=1.5)
    top, bottom = SKIES[tod]
    s.rect(wx, wy, ww, wh, s.linear([(0, top), (1, bottom)]))
    if tod == 'night':
        rng = Rng(3)
        for _ in range(14):
            s.circle(rng.uniform(wx + 1, wx + ww - 1), rng.uniform(wy + 1, wy + wh - 11), rng.uniform(0.2, 0.45), '#E8EEFF')
        s.glow(wx + 24, wy + 8, 7, 7, '#DCE6FF', 0.4)
        s.circle(wx + 24, wy + 8, 2.8, s.radial([(0, '#FFFFFF'), (1, '#CCD6EE')]))
    elif tod == 'day':
        s.glow(wx + 8, wy + 8, 9, 9, '#FFF4C8', 0.6)
        s.circle(wx + 8, wy + 8, 3, '#FFF6D0')
        s.ellipse(wx + 24, wy + 10, 5, 1.8, '#FFFFFF', 0.8)
        s.ellipse(wx + 27, wy + 9, 3, 1.6, '#FFFFFF', 0.8)
    else:
        s.glow(wx + 17, wy + wh - 6, 14, 9, '#FFD08A', 0.6)
        s.circle(wx + 17, wy + wh - 6, 3.8, '#FFD8A0')
    hill = {'night': '#2A3E5A', 'dawn': '#6A7A9A', 'day': '#9CC0A8', 'dusk': '#5A4A6A'}[tod]
    s.path(f'M{wx} {wy + wh - 5} Q{wx + 12} {wy + wh - 10} {wx + ww} {wy + wh - 6} L{wx + ww} {wy + wh} L{wx} {wy + wh} Z', hill)
    trees = {'night': '#12241A', 'dawn': '#1E3A2A', 'day': '#2E5A3A', 'dusk': '#1E2A2A'}[tod]
    for k in range(4):
        pine(s, wx + 4 + k * 9, wy + wh + 1, 10 + (k % 2) * 4, trees)
    # Muntins, sill and curtains.
    s.rect(wx + ww / 2 - 0.8, wy, 1.6, wh, '#3A281C')
    s.rect(wx, wy + wh / 2 - 0.8, ww, 1.6, '#3A281C')
    s.rect(wx - 4, wy + wh + 2, ww + 8, 2.5, '#7A5638', rx=1)
    for cx in (wx - 3, wx + ww + 3):
        s.path(f'M{cx - 3} {wy - 4} L{cx + 3} {wy - 4} Q{cx + 1} {wy + 14} {cx + 2.5} {wy + wh + 1} L{cx - 2.5} {wy + wh + 1} '
               f'Q{cx - 1} {wy + 14} {cx - 3} {wy - 4} Z', s.linear([(0, '#B84A3A'), (1, '#7A2A22')]))
    s.rect(wx - 8, wy - 5.5, ww + 16, 1.6, '#2A1C14', rx=0.8)
    # Floor, rug, table.
    s.rect(0, 76, W, 16, s.linear([(0, '#4A3220' if not dark else '#30200F'), (1, '#24170D')]))
    s.ellipse(90, 84, 44, 5.5, s.radial([(0, '#A84A3A'), (1, '#6E2A22')]))
    s.ellipse(90, 84, 34, 3.6, 'none', stroke='#D9B35A', sw=0.8)
    s.rect(60, 61.5, 60, 4.5, s.linear([(0, '#8A6242'), (1, '#5A3E28')]), rx=1.2)
    s.rect(64, 66, 3, 14, '#553A26', rx=0.8)
    s.rect(113, 66, 3, 14, '#553A26', rx=0.8)
    # The television.
    sx, sy, sw, sh = CABIN_SCREEN
    s.ellipse(90, 61.5, 30, 1.8, '#000000', 0.25)
    s.line(sx + 9, sy - 5, sx + 2, sy - 15, '#9A9AA4', 0.9)
    s.line(sx + sw - 9, sy - 5, sx + sw - 2, sy - 14, '#9A9AA4', 0.9)
    s.circle(sx + 2, sy - 15, 0.9, '#C8C8D0')
    s.circle(sx + sw - 2, sy - 14, 0.9, '#C8C8D0')
    s.rect(sx - 6.5, sy - 5.5, sw + 13, sh + 12, s.linear([(0, '#5A5A64'), (1, '#2E2E36')]), rx=4)
    s.rect(sx - 2.5, sy - 2.5, sw + 5, sh + 5, '#1A1A20', rx=3)
    s.rect(sx, sy, sw, sh, s.radial([(0, '#1C2230'), (1, '#0A0D12')]), rx=2)
    s.circle(sx + sw + 3.5, sy + sh + 3, 1.3, '#C9A34A')
    s.circle(sx + sw + 3.5, sy + sh - 1, 1, '#8A8A94')
    # A lamp on the right, lit in the evening and at night.
    lx = 150
    lit = tod in ('night', 'dusk')
    if lit:
        s.glow(lx + 1, 48, 32, 26, '#FFC46E', 0.42)
    s.rect(lx - 0.8, 50, 1.8, 26, '#2A2A2E')
    s.ellipse(lx, 76, 6, 1.4, '#2A2A2E')
    shade = s.linear([(0, '#FFE8B8'), (1, '#E0B070')]) if lit else s.linear([(0, '#C8B08A'), (1, '#9A8060')])
    s.path(f'M{lx - 7} 50 L{lx - 3.5} 38 L{lx + 4.5} 38 L{lx + 8} 50 Z', shade)


def cabin_glow(s):
    sx, sy, sw, sh = CABIN_SCREEN
    s.glow(sx + sw / 2, sy + sh / 2, 44, 34, '#9FC3FF', 0.26)


def static(s, seed):
    rng = Rng(seed)
    s.rect(0, 0, 40, 30, '#2A2F3A', rx=2)
    for y in range(30):
        x = 0.0
        while x < 40:
            w = rng.uniform(0.6, 3.2)
            v = rng.next()
            if v > 0.35:
                shade = int(0x40 + v * 0xA8)
                s.rect(x, y, w, 1, f'#{shade:02X}{shade:02X}{min(255, shade + 12):02X}', opacity=0.9)
            x += w
    s.rect(0, 11 + seed % 7, 40, 2.5, '#FFFFFF', opacity=0.18)


# --- Arcade (Moonlight) -----------------------------------------------------------
def arcade(s):
    s.rect(0, 0, W, H, s.linear([(0, '#16123A'), (1, '#07060F')]))
    s.glow(88, 76, 110, 26, '#7A3AD8', 0.28)
    # Neon floor grid, fading into the distance.
    fade = s.linear_abs([(0, '#8A4AFF', 0.15), (1, '#8A4AFF', 0.7)], 0, 70, 0, H)
    for y in (70, 72.5, 75.5, 79, 83.5, 89):
        s.line(0, y, W, y, fade, 0.5)
    for x in range(-40, W + 41, 12):
        s.line(88 + (x - 88) * 0.35, 70, x, H, fade, 0.5)
    # Neighbouring cabinets.
    for x0, col in ((8, '#FF4FA0'), (136, '#4FD8FF')):
        s.rect(x0, 20, 30, 56, s.linear([(0, '#24204A'), (1, '#12102A')]), rx=2)
        s.rect(x0 + 4, 28, 22, 16, s.radial([(0, '#3A3870'), (1, '#1A1838')]), rx=1.5)
        s.rect(x0 + 2, 15.5, 26, 6, col, rx=1.5, opacity=0.8)
    # The main cabinet.
    sx, sy, sw, sh = ARCADE_SCREEN
    s.path(f'M{sx - 10} 8 Q{sx - 10} 4 {sx - 6} 4 L{sx + sw + 6} 4 Q{sx + sw + 10} 4 {sx + sw + 10} 8 '
           f'L{sx + sw + 10} 80 L{sx - 10} 80 Z', s.linear([(0, '#3A2E66'), (1, '#1E1838')]))
    s.rect(sx - 10, 4, 2, 76, '#FF4FA0', opacity=0.5)
    s.rect(sx + sw + 8, 4, 2, 76, '#4FD8FF', opacity=0.5)
    s.rect(sx - 8, 6, sw + 16, 8, s.linear([(0, '#FFE27A'), (0.5, '#FF8A3D'), (1, '#FF4FA0')], x2=1, y2=0), rx=2)
    s.rect(sx - 3, sy - 3, sw + 6, sh + 6, '#0E0C18', rx=2.5)
    s.rect(sx, sy, sw, sh, s.radial([(0, '#141828'), (1, '#05060A')]), rx=1.5)
    # Control panel, joystick and buttons.
    s.path(f'M{sx - 8} {sy + sh + 4} L{sx + sw + 8} {sy + sh + 4} L{sx + sw + 11} {sy + sh + 12} L{sx - 11} {sy + sh + 12} Z',
           s.linear([(0, '#4A3E7A'), (1, '#2A2248')]))
    s.line(sx + 6, sy + sh + 9, sx + 6, sy + sh + 5.5, '#C8C8D8', 0.9)
    s.circle(sx + 6, sy + sh + 5.2, 1.6, s.radial([(0, '#FF8A8A'), (1, '#C8283A')], fx=0.35, fy=0.3))
    for i, col in enumerate(('#FF4F4F', '#4FD86A', '#4F8CFF')):
        s.circle(sx + 22 + i * 5, sy + sh + 8, 1.5, s.radial([(0, '#FFFFFF'), (0.4, col), (1, col)], fx=0.35, fy=0.3))


def arcade_glow(s):
    sx, sy, sw, sh = ARCADE_SCREEN
    s.glow(sx + sw / 2, 10, 40, 10, '#FFD34F', 0.45)
    s.glow(23, 18.5, 20, 8, '#FF4FA0', 0.5)
    s.glow(151, 18.5, 20, 8, '#4FD8FF', 0.5)
    s.glow(sx + sw / 2, sy + sh / 2, 44, 34, '#7F9CFF', 0.24)
    for x in range(sx - 6, sx + sw + 7, 4):
        s.circle(x, 5, 0.7, '#FFF4C8', 0.9)


def build(root):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'classic')
    os.makedirs(out, exist_ok=True)
    files = []

    def save(name, draw, label):
        s = canvas(label, f'Featured-panel room "{name}", Classic: tools/classicart/hero.py')
        draw(s)
        path = os.path.join(out, f'{name}.svg')
        s.save(path)
        files.append(path)

    save('hero-cinema', cinema, 'cinema')
    save('hero-cinema-glow', cinema_glow, 'cinema light')
    for tod in TIMES:
        save(f'hero-cabin-{tod}', lambda s, t=tod: cabin(s, t), f'cabin, {tod}')
    save('hero-cabin-glow', cabin_glow, 'cabin TV light')
    save('hero-arcade', arcade, 'arcade')
    save('hero-arcade-glow', arcade_glow, 'arcade neon')
    for k in range(2):
        s = Svg(160, 120, 'static', 'The cabin TV\'s intro static, Classic: tools/classicart/hero.py', 40, 30)
        static(s, 11 + k)
        path = os.path.join(out, f'hero-static-{k}.svg')
        s.save(path)
        files.append(path)
    return files
