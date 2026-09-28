"""The featured panel's scenes in pixel art (tools/pixelart): one little room
per kind of app, drawn behind the app's official icon on the panel's right
side. 176×92 art pixels (704×368 on a 1080p TV). Writes
apps/tv-shell/assets/pixel/hero-<scene>.png (frames side by side; the cabin has
one row per time of day: night, dawn, day, dusk) and qml/HeroRig.js with each
scene's frame size, frame count, speed and the "screen" rectangle the icon
sits in. Which scene an app gets is data in qml/Apps.qml.
"""

import json
import math
import os

from px import Img, Rng, dither, mix
import common as c

W, H = 176, 92
OUT = '#120E0C'
TIMES = ['night', 'dawn', 'day', 'dusk']


def noise_screen(img, x, y, w, h, seed, strength=1.0):
    rng = Rng(seed)
    for yy in range(h):
        for xx in range(w):
            v = rng.next()
            if v < 0.5 * strength:
                img.put(x + xx, y + yy, mix('#2A2F3A', '#E8ECF4', rng.next()))


# --- Cinema (Plex) -------------------------------------------------------------
CINEMA_SCREEN = (50, 10, 76, 42)


def cinema(frame, frames):
    img = Img(W, H, '#150E10')
    img.vgradient(0, 0, W, H, [(0, '#1C1216'), (1, '#0C0809')])
    sx, sy, sw, sh = CINEMA_SCREEN
    # Screen frame and its glow on the room.
    img.glow(sx + sw / 2, sy + sh / 2, 70, '#E8D9B0', 0.18 + 0.03 * (frame % 2), ry=40, bands=4)
    img.rect(sx - 2, sy - 2, sw + 4, sh + 4, '#2A1E1A')
    img.rect(sx, sy, sw, sh, '#E9E2D2')
    # Curtains, gathered with pleats.
    for side in (0, 1):
        for x in range(34):
            xx = x if side == 0 else W - 1 - x
            fold = (x // 3) % 2
            top_drop = int(6 * math.sin(x / 34 * math.pi / 2))
            for y in range(0, H - 22 + top_drop):
                col = '#8E2230' if fold else '#6E1823'
                if x > 30 - y // 12:
                    continue
                img.put(xx, y, col)
            img.put(xx, 0, '#C9A34A')
    img.rect(0, 0, W, 4, '#6E1823')
    for x in range(0, W, 6):
        img.rect(x, 3, 3, 2, '#8E2230')
    # The projector beam from the booth, top right, with dust that moves.
    bx, by = W - 8, 4
    for y in range(by, sy + sh):
        t = (y - by) / (sy + sh - by)
        x0 = bx - t * (bx - (sx + sw - 6))
        x1 = bx - t * (bx - (sx + 8))
        for x in range(int(min(x0, x1)), int(max(x0, x1))):
            if dither(x, y, 0.18 + 0.06 * (frame % 2)):
                img.put(x, y, '#FFF2C8', 0.35)
    rng = Rng(7)
    for i in range(10):
        p = (rng.next() + frame / frames) % 1.0
        x = bx - p * (bx - sx - 30) + rng.uniform(-4, 4)
        y = by + p * (sy + sh - by) * 0.8
        img.put(x, y, '#FFF8E0', 0.8)
    img.rect(bx - 3, by - 2, 6, 4, '#2A2A2E')
    img.put(bx - 2, by, '#FFF2C8')
    # Rows of seats in front.
    for row, (y0, col, hi) in enumerate(((66, '#4A1620', '#6E2230'), (74, '#3A1119', '#5A1C28'), (83, '#2A0C12', '#44151E'))):
        off = 4 if row % 2 else 0
        for x in range(-8 + off, W, 16):
            img.rect(x, y0, 13, 10, col)
            img.hline(x + 1, x + 11, y0, hi)
            img.rect(x + 1, y0 - 2, 11, 2, col)
    return img


# --- Cabin (YouTube) -----------------------------------------------------------
CABIN_SCREEN = (70, 26, 40, 30)
SKIES = {
    'night': [(0, '#0A1024'), (1, '#1E2C52')],
    'dawn': [(0, '#3A4C7A'), (1, '#F0B08A')],
    'day': [(0, '#6FA8DC'), (1, '#CDE6F4')],
    'dusk': [(0, '#2A2448'), (1, '#E08A5A')],
}


def cabin(frame, frames, tod):
    img = Img(W, H)
    dark = tod == 'night'
    wall, wall_d = ('#5A3E2A', '#4A3222') if not dark else ('#3A281C', '#2E2016')
    img.rect(0, 0, W, H, wall)
    for y in range(0, H, 6):
        img.hline(0, W - 1, y, wall_d)
        for x in range((y * 7) % 23, W, 23):
            img.put(x, y + 3, wall_d)
    # A window on the left showing the time of day.
    wx, wy, ww, wh = 12, 12, 34, 30
    img.rect(wx - 2, wy - 2, ww + 4, wh + 4, '#2A1C14')
    img.vgradient(wx, wy, ww, wh, [(p, col) for p, col in SKIES[tod]])
    if tod == 'night':
        rng = Rng(3)
        for _ in range(12):
            img.put(rng.int(wx, wx + ww - 1), rng.int(wy, wy + wh - 10), rng.choice(['#E8EEFF', '#9DB2DD']))
        img.disc(wx + 24, wy + 8, 3, '#EEF2FF')
    elif tod == 'day':
        img.disc(wx + 8, wy + 8, 3, '#FFF4C8')
    else:
        img.disc(wx + 17, wy + wh - 6, 4, '#FFD08A')
    for k in range(4):
        c.pine(img, wx + 4 + k * 9, wy + wh, 10 + (k % 2) * 4, '#1E3A2A')
    img.vline(wx + ww // 2, wy, wy + wh - 1, '#2A1C14')
    img.hline(wx, wx + ww - 1, wy + wh // 2, '#2A1C14')
    # Floor, rug, table, and the television.
    img.rect(0, 76, W, 16, '#3A2618' if not dark else '#2A1C12')
    img.ellipse(90, 84, 44, 5, '#8A3A2E')
    img.ellipse(90, 84, 36, 3, '#C9A65A')
    img.rect(60, 62, 60, 4, '#6B4A30')
    img.rect(64, 66, 3, 14, '#553A26')
    img.rect(113, 66, 3, 14, '#553A26')
    sx, sy, sw, sh = CABIN_SCREEN
    img.rect(sx - 6, sy - 5, sw + 12, sh + 12, '#3A3A40')
    img.rect(sx - 5, sy - 4, sw + 10, sh + 10, '#4E4E56')
    img.rect(sx + sw + 1, sy + sh + 1, 3, 2, '#C9A34A')        # knob
    img.line(sx + 8, sy - 5, sx + 2, sy - 14, '#8A8A94')       # rabbit ears
    img.line(sx + sw - 8, sy - 5, sx + sw - 2, sy - 13, '#8A8A94')
    img.rect(sx, sy, sw, sh, '#10141A')
    if frame < frames - 1:                                       # static, then the picture
        noise_screen(img, sx, sy, sw, sh, 11 + frame, 1.0 - frame / frames)
    # A lamp on the right, lit in the evening and at night.
    lx = 150
    img.rect(lx, 50, 2, 26, '#2A2A2E')
    img.poly([(lx - 7, 50), (lx - 3, 38), (lx + 5, 38), (lx + 9, 50)], '#E0C28E' if tod in ('night', 'dusk') else '#B8A078')
    if tod in ('night', 'dusk'):
        img.glow(lx + 1, 52, 30, '#FFC46E', 0.3, ry=24, bands=4)
    img.glow(sx + sw / 2, sy + sh / 2, 34, '#9FC3FF', 0.16, bands=3)
    return img


# --- Arcade (Moonlight) ---------------------------------------------------------
ARCADE_SCREEN = (68, 18, 40, 30)


def arcade(frame, frames):
    img = Img(W, H, '#0B0A18')
    img.vgradient(0, 0, W, H, [(0, '#120F2A'), (1, '#07060F')])
    # Neon floor grid.
    for y in range(70, H, 4):
        img.hline(0, W - 1, y, '#3A1E6A', 0.6)
    for x in range(-40, W + 40, 12):
        img.line(88 + (x - 88) * 0.35, 70, x, H - 1, '#3A1E6A')
    # Neighbouring cabinets in silhouette.
    for x0 in (8, 136):
        img.rect(x0, 20, 30, 56, '#1A1830')
        img.rect(x0 + 4, 28, 22, 16, '#2A2850')
        img.rect(x0 + 2, 16, 26, 6, ['#FF4FA0', '#4FD8FF'][(x0 // 8 + frame) % 2])
    # The main cabinet.
    sx, sy, sw, sh = ARCADE_SCREEN
    img.rect(sx - 10, 4, sw + 20, 76, '#2A2248')
    img.rect(sx - 8, 6, sw + 16, 8, ['#FFD34F', '#FF4FA0', '#4FD8FF', '#FF8A3D'][frame % 4])
    for x in range(sx - 6, sx + sw + 6, 4):                      # marquee bulbs chasing
        img.put(x, 5, '#FFF4C8' if (x // 4 + frame) % 3 == 0 else '#6A5A2A')
    img.rect(sx - 2, sy - 2, sw + 4, sh + 4, '#111018')
    img.rect(sx, sy, sw, sh, '#05060A')
    img.rect(sx - 6, sy + sh + 4, sw + 12, 8, '#3A3060')          # control panel
    img.disc(sx + 6, sy + sh + 8, 1, '#E8E8F0')
    img.rect(sx + 5, sy + sh + 5, 2, 3, '#E8E8F0')
    for i, col in enumerate(('#FF4F4F', '#4FD86A', '#4F8CFF')):
        img.put(sx + 22 + i * 5, sy + sh + 8, col if (i + frame) % 3 else '#FFFFFF')
    img.glow(sx + sw / 2, sy + sh / 2, 40, '#7F9CFF', 0.16 + 0.04 * (frame % 2), bands=3)
    return img


SCENES = {
    'cinema': dict(screen=CINEMA_SCREEN, frames=4, fps=4, times=False, draw=lambda f, n, t: cinema(f, n)),
    'cabin': dict(screen=CABIN_SCREEN, frames=4, fps=6, times=True, intro=True, draw=cabin),
    'arcade': dict(screen=ARCADE_SCREEN, frames=4, fps=3, times=False, draw=lambda f, n, t: arcade(f, n)),
}


def build(root, preview):
    out = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    os.makedirs(out, exist_ok=True)
    rig = {}
    board_rows = []
    for name, s in SCENES.items():
        rows = TIMES if s['times'] else [None]
        sheet = Img(W * s['frames'], H * len(rows))
        for r, tod in enumerate(rows):
            for f in range(s['frames']):
                sheet.blit(s['draw'](f, s['frames'], tod), f * W, r * H)
        sheet.save(os.path.join(out, f'hero-{name}.png'))
        rig[name] = {'size': [W, H], 'frames': s['frames'], 'fps': s['fps'], 'screen': list(s['screen']),
                     'times': TIMES if s['times'] else [], 'intro': bool(s.get('intro'))}
        board_rows.append(sheet)
    # Snow resting on the panel's top edge in December: a tile repeated along it.
    cap = Img(32, 5)
    tops = [2, 1, 1, 0, 1, 1, 2, 2, 1, 0, 0, 1, 2, 3, 2, 1, 1, 1, 0, 1, 2, 2, 1, 1, 0, 0, 1, 1, 2, 2, 1, 1]
    for x, top in enumerate(tops):
        for y in range(top, 4):
            cap.put(x, y, '#F4FAFF' if y > top else '#FFFFFF')
        cap.put(x, 4, '#B4C6DC')
    cap.save(os.path.join(out, 'snowcap.png'))
    js = os.path.join(root, 'apps', 'tv-shell', 'qml', 'HeroRig.js')
    with open(js, 'w') as f:
        f.write('.pragma library\n')
        f.write('// Generated by tools/pixelart/hero.py — do not edit by hand.\n')
        f.write('// The featured panel\'s scenes (assets/pixel/hero-<scene>.png): frame size,\n')
        f.write('// frames per row, speed, the screen rectangle the app icon sits in (art\n')
        f.write('// pixels), the rows by time of day (if any) and whether the first frames\n')
        f.write('// play once as an intro (static before the picture).\n\n')
        f.write('var scenes = ' + json.dumps(rig, indent=1) + ';\n')
    if preview:
        os.makedirs(preview, exist_ok=True)
        bw = max(s.w for s in board_rows)
        board = Img(bw, sum(s.h + 2 for s in board_rows), '#2A2F3A')
        y = 0
        for s in board_rows:
            board.blit(s, 0, y)
            y += s.h + 2
        board.scaled(2).save(os.path.join(preview, 'hero.png'))
    print('hero: ' + ', '.join(SCENES))
