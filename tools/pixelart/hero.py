"""The featured panel's scenes in pixel art (tools/pixelart): one little room
per kind of app, drawn behind the app's icon on the panel's right side:
cinema (Plex), cabin (YouTube), arcade (Moonlight), a music nook with a
record player and a bobbing bear (Spotify), a home theatre in the woods
(Jellyfin) and a cosy retro corner with a chunky CRT and a joystick
(RetroArch). 176×92 art pixels (704×368 on a 1080p TV). Writes
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


# --- Music nook (Spotify) --------------------------------------------------------
NOOK_SCREEN = (74, 12, 40, 30)


def nook(frame, frames):
    img = Img(W, H, '#23382C')
    for x in range(0, W, 8):                                     # striped wallpaper
        img.rect(x, 0, 4, 74, '#2A4234')
    for y in range(8, 74, 16):
        for x in range((y // 16) % 2 * 4 + 2, W, 8):
            img.put(x, y, '#3E5A46')
    img.glow(150, 30, 46, '#FFC46E', 0.22 + 0.02 * (frame % 2), ry=38, bands=4)
    img.rect(0, 74, W, 18, '#5A3A22')                            # floor boards
    for y in range(76, H, 5):
        img.hline(0, W - 1, y, '#4A2E1A')
    img.ellipse(112, 84, 46, 5, '#2E6A48')                        # a green rug
    img.ellipse(112, 84, 38, 3, '#3E8A5E')
    # The frame on the wall the icon sits in, like an album sleeve.
    sx, sy, sw, sh = NOOK_SCREEN
    img.rect(sx - 4, sy - 4, sw + 8, sh + 8, '#6A4424')
    img.rect(sx - 3, sy - 3, sw + 6, sh + 6, '#8A5A30')
    img.rect(sx - 1, sy - 1, sw + 2, sh + 2, '#1A120C')
    img.rect(sx, sy, sw, sh, '#1C2A22')
    # Record cabinet with a record player on top, records inside.
    img.rect(10, 50, 52, 26, '#6A4424')
    img.hline(10, 61, 50, '#9A6A3E')
    img.rect(13, 54, 46, 19, '#2A1C12')
    cols = ['#C84A3A', '#E0B040', '#3A7AC8', '#E8E0D0', '#8A4AA8', '#3AA868', '#D87A3A']
    for i, x in enumerate(range(14, 58, 3)):
        img.rect(x, 56 + (i % 3 == 0), 2, 17 - (i % 3 == 0), cols[i % len(cols)])
    img.rect(12, 76, 3, 2, '#2A1C12')
    img.rect(57, 76, 3, 2, '#2A1C12')
    img.rect(16, 43, 40, 7, '#8A5A34')                           # the player
    img.hline(16, 55, 43, '#B07A4A')
    img.ellipse(32, 44, 11, 3, '#161616')
    img.ellipse(32, 44, 7, 2, '#2A2A2A')
    img.rect(31, 44, 3, 1, '#F2C14E')
    glint = [(24, 43), (28, 42), (38, 42), (41, 44)][frame % 4]  # the record turns
    img.put(glint[0], glint[1], '#7A7A7A')
    img.line(51, 41, 47, 45, '#D8D8E0')                          # tonearm
    img.rect(50, 40, 3, 2, '#9A9AA4')
    # Notes rising from the player.
    for k in range(3):
        p = ((frame + k * 4 / 3) % frames) / frames
        nx = 40 + k * 7 + int(3 * math.sin(p * 6.3 + k))
        ny = int(40 - p * 26)
        col = ['#FFE6A0', '#A8E8C0', '#FFFFFF'][k]
        img.rect(nx, ny, 2, 2, col)
        img.vline(nx + 1, ny - 4, ny, col)
        img.put(nx + 2, ny - 4, col)
    # A bear by the player, bobbing to the music with headphones on.
    bob = 1 if frame % 2 else 0
    bx, by = 136, 50 + bob
    fur, fur_d, muzzle = '#8A5A34', '#6A4226', '#D8B080'
    img.ellipse(bx, 66, 13, 10, fur)                             # body, sitting
    img.ellipse(bx, 68, 7, 6, '#B08058')
    img.disc(bx - 9, by - 7, 3, fur_d)                           # ears
    img.disc(bx + 9, by - 7, 3, fur_d)
    img.disc(bx, by, 9, fur)                                     # head
    img.ellipse(bx, by + 4, 4, 3, muzzle)
    img.rect(bx - 1, by + 2, 3, 2, '#1A120C')                    # nose
    img.rect(bx - 4, by - 2, 2, 2 - bob, '#1A120C')              # eyes, happy squint
    img.rect(bx + 3, by - 2, 2, 2 - bob, '#1A120C')
    img.line(bx - 9, by - 3, bx - 6, by - 10, '#2A2A2E')          # headphones
    img.line(bx + 9, by - 3, bx + 6, by - 10, '#2A2A2E')
    img.hline(bx - 6, bx + 6, by - 11, '#2A2A2E')
    img.rect(bx - 11, by - 3, 3, 6, '#2FCF6A')
    img.rect(bx + 9, by - 3, 3, 6, '#2FCF6A')
    img.rect(bx - 16, 70, 6, 5, fur_d)                           # feet
    img.rect(bx + 10, 70, 6, 5, fur_d)
    # A lamp on the right.
    img.rect(163, 34, 2, 40, '#2A2A2E')
    img.poly([(157, 34), (160, 24), (168, 24), (171, 34)], '#F0D49A')
    return img


# --- Home theatre in the woods (Jellyfin) ----------------------------------------
THEATRE_SCREEN = (56, 10, 64, 36)


def theatre(frame, frames):
    img = Img(W, H)
    img.vgradient(0, 0, W, 70, [(0, '#0C0A22'), (0.7, '#2A1E4A'), (1, '#3A2A5A')])
    rng = Rng(21)
    for _ in range(26):
        x, y = rng.int(0, W - 1), rng.int(0, 40)
        img.put(x, y, '#E8E4FF' if rng.next() > 0.5 else '#9A8ACA')
    img.disc(150, 12, 4, '#F4EEDA')                               # the moon
    img.disc(152, 11, 4, '#1A1638')
    for k, x in enumerate(range(-6, W + 10, 13)):                 # the woods
        if 40 < x < 136:
            continue
        c.pine(img, x, 76, 34 + (k * 7) % 14, '#10201A')
    img.rect(0, 70, W, 22, '#18281E')                             # clearing
    img.ellipse(88, 86, 60, 6, '#22382A')
    sx, sy, sw, sh = THEATRE_SCREEN
    img.glow(sx + sw / 2, sy + sh / 2 + 10, 74, '#C8B8FF', 0.2 + 0.03 * (frame % 2), ry=46, bands=4)
    img.rect(sx - 5, sy - 3, 2, 64, '#3A2A1A')                    # posts
    img.rect(sx + sw + 3, sy - 3, 2, 64, '#3A2A1A')
    img.rect(sx - 3, sy - 3, sw + 6, 2, '#5A3A22')
    img.rect(sx - 2, sy - 1, sw + 4, sh + 2, '#D8D0E8')
    img.rect(sx, sy, sw, sh, '#10121E')
    # String lights along the top, twinkling in turn.
    for i, x in enumerate(range(8, W - 6, 9)):
        y = 4 + int(3 * math.sin(x / 176 * math.pi * 3))
        img.put(x, y - 1, '#3A2A1A')
        on = (i + frame) % 3 != 0
        col = ['#FFD34F', '#FF8A8A', '#8AD8FF'][i % 3]
        img.rect(x, y, 2, 2, col if on else '#4A4050')
    # Logs to sit on, a projector on a stump.
    for lx in (36, 100):
        img.rect(lx, 76, 32, 5, '#6A4424')
        img.hline(lx, lx + 31, 76, '#8A5A34')
        img.disc(lx, 78, 2, '#C8A070')
    img.rect(140, 70, 14, 10, '#5A3A22')
    img.rect(141, 64, 12, 6, '#2A2A30')
    img.put(141, 66, '#FFF2C8')
    for y in range(sy + 6, 66):                                   # the beam
        t = (y - (sy + 6)) / (66 - sy - 6)
        x0 = sx + sw - 4 + t * (141 - (sx + sw - 4))
        for x in range(int(x0) - 2, int(x0) + 1):
            if dither(x, y, 0.2):
                img.put(x, y, '#FFF2C8', 0.3)
    rng = Rng(5 + frame)                                           # fireflies
    for _ in range(6):
        img.put(rng.int(4, W - 4), rng.int(48, 72), '#D8FF8A')
    return img


# --- Retro corner (RetroArch) ------------------------------------------------------
RETRO_SCREEN = (68, 22, 40, 30)


def retro(frame, frames):
    img = Img(W, H, '#3A2A4A')
    for y in range(0, 70, 10):                                    # wallpaper diamonds
        for x in range((y // 10) % 2 * 5, W, 10):
            img.put(x, y + 4, '#4A3A5E')
            img.put(x + 1, y + 5, '#4A3A5E')
    img.glow(34, 26, 40, '#FFB86E', 0.2, ry=34, bands=4)
    img.rect(0, 68, W, 24, '#4A2E22')                              # floor, rug
    img.ellipse(92, 84, 50, 6, '#6A2A4A')
    img.ellipse(92, 84, 42, 4, '#8A3A5A')
    # A shelf of books and a plant, top left.
    img.rect(8, 22, 44, 3, '#7A5234')
    for i, x in enumerate(range(10, 40, 4)):
        h = 10 + (i * 3) % 5
        img.rect(x, 22 - h, 3, h, ['#C84A3A', '#3A7AC8', '#E0B040', '#3AA868', '#8A4AA8'][i % 5])
    img.rect(42, 16, 7, 6, '#B06A3A')
    img.disc(45, 13, 4, '#3A8A4A')
    # A lava lamp on the left, its blobs rising.
    img.poly([(20, 50), (24, 36), (28, 36), (32, 50)], '#E0506A')
    for k in range(2):
        yy = 48 - ((frame + k * 2) % frames) * 3
        img.rect(24 + k * 2, yy - k * 4, 3, 3, '#FFD06A')
    img.rect(19, 50, 14, 4, '#2A2A30')
    img.rect(22, 33, 8, 3, '#2A2A30')
    # The desk.
    img.rect(40, 60, 108, 5, '#8A5A34')
    img.hline(40, 147, 60, '#B07A4A')
    img.rect(44, 65, 4, 13, '#6A4424')
    img.rect(140, 65, 4, 13, '#6A4424')
    # A chunky CRT television.
    sx, sy, sw, sh = RETRO_SCREEN
    img.rect(sx - 8, sy - 7, sw + 22, sh + 15, '#C8BFA8')
    img.hline(sx - 8, sx + sw + 13, sy - 7, '#E6DEC8')
    img.rect(sx - 8, sy + sh + 6, sw + 22, 2, '#8A826E')
    img.rect(sx - 3, sy - 3, sw + 6, sh + 6, '#2A2A30')
    img.rect(sx, sy, sw, sh, '#0A0C14')
    for k in range(3):                                             # speaker grille, dials
        img.hline(sx + sw + 5, sx + sw + 11, sy + 2 + k * 2, '#8A826E')
    img.rect(sx + sw + 6, sy + 12, 4, 4, '#5A5448')
    img.rect(sx + sw + 6, sy + 19, 4, 4, '#5A5448')
    img.rect(sx + sw + 7, sy + 26, 2, 2, '#4FD86A' if frame % 2 else '#1E5A2A')   # power light
    img.glow(sx + sw / 2, sy + sh / 2, 34, '#8AB0FF', 0.14 + 0.04 * (frame % 2), bands=3)
    # The joystick on the desk, wire curling to the TV.
    img.rect(122, 55, 14, 5, '#1E1E24')
    img.hline(122, 135, 55, '#3A3A44')
    img.vline(128, 47, 54, '#8A8A94')
    img.rect(126, 44, 5, 4, '#E0404A')
    img.put(127, 44, '#FF9A9A')
    img.rect(133, 56, 2, 1, '#FFD34F')
    img.line(122, 58, 116, 59, '#1E1E24')
    # A poster of the stars, right.
    img.rect(152, 12, 18, 24, '#1A1A3A')
    img.rect(152, 12, 18, 1, '#E0B040')
    for (px_, py_) in ((156, 18), (164, 16), (160, 24), (166, 30), (155, 30)):
        img.put(px_, py_, '#FFF4C8')
    return img


SCENES = {
    'cinema': dict(screen=CINEMA_SCREEN, frames=4, fps=4, times=False, draw=lambda f, n, t: cinema(f, n)),
    'cabin': dict(screen=CABIN_SCREEN, frames=4, fps=6, times=True, intro=True, draw=cabin),
    'arcade': dict(screen=ARCADE_SCREEN, frames=4, fps=3, times=False, draw=lambda f, n, t: arcade(f, n)),
    'nook': dict(screen=NOOK_SCREEN, frames=4, fps=4, times=False, draw=lambda f, n, t: nook(f, n)),
    'theatre': dict(screen=THEATRE_SCREEN, frames=4, fps=3, times=False, draw=lambda f, n, t: theatre(f, n)),
    'retro': dict(screen=RETRO_SCREEN, frames=4, fps=3, times=False, draw=lambda f, n, t: retro(f, n)),
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
