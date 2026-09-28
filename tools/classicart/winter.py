"""Winter's Classic world (tools/classicart): a snowy cabin under an aurora at
night, the smooth counterpart of tools/pixelart/worlds/winter.py.

Writes themes/winter/classic-wallpaper.svg (2560×1440) and
classic-backdrop.svg (780×1260, a portrait crop around the cabin). The
`classic` block in themes/winter/theme.json points at them (written by hand;
tools/pixelart only rewrites `wallpaper` and `phone.backdrop`).
"""

import math
import os

from svg import Svg, Rng, n, smooth_ridge, pine, star

W, H = 2560, 1440
CABIN_X, GROUND = 905, 1240


def aurora(s):
    """Curtains of light: wavy bands, bright along their lower edge."""
    bands = [
        # (y of the lower edge, height, amplitude, period, phase, colour, strength)
        (520, 330, 70, 1500, 0.4, '#5CF2B0', 0.34),
        (450, 280, 55, 1100, 2.1, '#6FE3D0', 0.26),
        (600, 250, 45, 1800, 4.0, '#8CF5A0', 0.22),
        (400, 240, 60, 1300, 5.3, '#A58CF0', 0.14),
    ]
    for low, height, amp, period, phase, col, k in bands:
        xs = [x for x in range(-100, W + 201, 100)]
        lower = [(x, low + amp * math.sin(x / period * 2 * math.pi + phase)
                  + 0.35 * amp * math.sin(x / (period * 0.37) * 2 * math.pi + phase * 1.7)) for x in xs]
        upper = [(x, y - height - 0.5 * amp * math.sin(x / (period * 0.6) * 2 * math.pi + phase)) for x, y in lower]
        g = s.linear_abs([(0, col, 0), (0.55, col, k * 0.5), (0.9, col, k), (1, col, 0)],
                         0, low - height - amp, 0, low + amp)
        d = smooth_ridge(lower)[1:]
        back = smooth_ridge(list(reversed(upper)))[1:]
        s.path('M' + d + ' L' + back + ' Z', g)
        # Soft vertical rays: tall ellipses that fade at every edge.
        rng = Rng(int(low))
        ray = s.radial([(0, col, k * 0.9), (0.5, col, k * 0.35), (1, col, 0)], cy=0.75, fy=0.85, fx=0.5)
        for x, y in lower[1:-1]:
            for _ in range(2):
                if rng.next() < 0.55:
                    s.ellipse(x + rng.uniform(-50, 50), y - height * 0.45, rng.uniform(14, 36), height * 0.75, ray)


def hills(s, top, amp, period, phase, fill, seed):
    rng = Rng(seed)
    pts = []
    for x in range(-100, W + 201, 160):
        pts.append((x, top + amp * math.sin(x / period * 2 * math.pi + phase) + rng.uniform(-amp * 0.3, amp * 0.3)))
    s.path(smooth_ridge(pts) + f' L{W + 200} {H} L-100 {H} Z', fill)
    return pts


def ridge_y(pts, x):
    for (x0, y0), (x1, y1) in zip(pts, pts[1:]):
        if x0 <= x <= x1:
            return y0 + (y1 - y0) * (x - x0) / (x1 - x0)
    return pts[-1][1]


def cabin(s, smoke=True):
    x, g = CABIN_X, GROUND
    w, h = 250, 130
    # Warm light spilling on the snow.
    s.glow(x, g + 10, 420, 110, '#FFB85C', 0.35)
    # Shadow under the cabin.
    s.ellipse(x + 10, g + 6, w * 0.7, 18, s.radial([(0, '#0B1A33', 0.55), (1, '#0B1A33', 0)]))
    # Walls: logs.
    s.rect(x - w / 2, g - h, w, h, s.linear([(0, '#7A4E30'), (1, '#4E3120')]), rx=6)
    for k in range(1, 7):
        y = g - h + k * h / 7
        s.line(x - w / 2 + 6, y, x + w / 2 - 6, y, '#3A2416', 3, 0.55)
    for side in (-1, 1):
        for k in range(7):
            s.circle(x + side * (w / 2 - 2), g - h + (k + 0.5) * h / 7, 8, '#8E6040', stroke='#3A2416', sw=2)
    # Chimney.
    s.rect(x + 50, g - h - 110, 34, 80, s.linear([(0, '#6A6470'), (1, '#46404C')]), rx=3)
    s.rect(x + 44, g - h - 118, 46, 14, '#EAF2FA', rx=7)
    # Roof with a thick snow blanket.
    s.path(f'M{x - w / 2 - 34} {g - h + 8} L{x} {g - h - 96} L{x + w / 2 + 34} {g - h + 8} Z',
           s.linear([(0, '#5A3A2A'), (1, '#3A2418')]))
    s.path(f'M{x - w / 2 - 44} {g - h + 14} Q{x - w / 2 - 50} {g - h - 4} {x - w / 2 - 28} {g - h - 8} '
           f'L{x - 10} {g - h - 108} Q{x} {g - h - 114} {x + 10} {g - h - 108} '
           f'L{x + w / 2 + 28} {g - h - 8} Q{x + w / 2 + 50} {g - h - 4} {x + w / 2 + 44} {g - h + 14} '
           f'Q{x + w / 2} {g - h + 4} {x + w / 4} {g - h + 12} Q{x} {g - h + 2} {x - w / 4} {g - h + 12} '
           f'Q{x - w / 2} {g - h + 4} {x - w / 2 - 44} {g - h + 14} Z',
           s.linear([(0, '#FFFFFF'), (0.6, '#E6F0FA'), (1, '#B8CCE4')]))
    # Icicles.
    rng = Rng(5)
    for k in range(12):
        ix = x - w / 2 - 10 + k * (w + 20) / 11
        il = rng.uniform(10, 28)
        s.path(f'M{n(ix - 4)} {g - h + 10} L{n(ix)} {n(g - h + 10 + il)} L{n(ix + 4)} {g - h + 10} Z', '#DCEBFA', 0.85)
    # Windows, glowing.
    for wx in (x - 72, x + 40):
        s.glow(wx + 16, g - 70, 90, 70, '#FFC46E', 0.45)
        s.rect(wx - 4, g - 98, 72 - 32 + 4 + 8, 60, '#3A2416', rx=5)
        s.rect(wx, g - 94, 40, 52, s.radial([(0, '#FFF0B8'), (0.7, '#FFC46E'), (1, '#E08A3A')]), rx=3)
        s.line(wx + 20, g - 94, wx + 20, g - 42, '#5A3A22', 4)
        s.line(wx, g - 68, wx + 40, g - 68, '#5A3A22', 4)
        s.rect(wx - 8, g - 42, 56, 9, '#EEF4FA', rx=4)
    # Door.
    s.rect(x - 16, g - 84, 36, 84, s.linear([(0, '#5E3A22'), (1, '#3E2616')]), rx=10)
    s.circle(x + 12, g - 42, 3.5, '#E3B35C')
    # Snow drift against the wall.
    s.path(f'M{x - w / 2 - 60} {g + 6} Q{x - w / 2} {g - 30} {x - 40} {g - 6} Q{x + 40} {g + 4} {x + w / 2 - 10} {g - 22} '
           f'Q{x + w / 2 + 40} {g - 20} {x + w / 2 + 80} {g + 8} Z', s.linear([(0, '#FFFFFF'), (1, '#D8E6F4')]))
    # Chimney smoke (static puffs; drifting, fading). The wallpaper leaves
    # them out: there the smoke is an animated layer (layers.py).
    for k in range(6 if smoke else 0):
        px = x + 67 + k * 26 + 10 * math.sin(k * 1.3)
        py = g - h - 150 - k * 58
        r = 22 + k * 9
        s.ellipse(px, py, r * 1.2, r, s.radial([(0, '#C9D4E4', 0.34 - k * 0.045), (1, '#C9D4E4', 0)]))


def fence(s):
    y = 1330
    for k in range(10):
        x = 60 + k * 58
        yy = y + 6 * math.sin(k * 0.7)
        s.rect(x, yy - 70, 14, 86, s.linear([(0, '#6A4A34'), (1, '#3E2A1E')]), rx=4)
        s.ellipse(x + 7, yy - 72, 11, 7, '#F4FAFF')
    s.path(f'M60 {y - 48} Q330 {y - 34} 640 {y - 50}', 'none', stroke='#4A3222', sw=8)
    s.path(f'M60 {y - 16} Q330 {y - 4} 640 {y - 20}', 'none', stroke='#4A3222', sw=8)
    s.path(f'M52 {y - 54} Q330 {y - 42} 648 {y - 56}', 'none', stroke='#F4FAFF', sw=5)


def build_scene(smoke=True):
    s = Svg(W, H, 'Winter: a snowy cabin under an aurora',
            'Winter, Classic: tools/classicart/winter.py')
    s.rect(0, 0, W, H, s.linear([(0, '#050B1E'), (0.45, '#10204A'), (0.7, '#23406E'), (1, '#3A5A86')]))
    # Stars: many tiny, some twinkles.
    rng = Rng(42)
    for _ in range(260):
        x, y = rng.uniform(0, W), rng.uniform(0, 820) ** 1.0
        r = rng.uniform(0.9, 2.6)
        s.circle(x, y, r, '#EAF2FF', rng.uniform(0.35, 0.95))
    for _ in range(14):
        star(s, rng.uniform(80, W - 80), rng.uniform(40, 560), rng.uniform(8, 16), '#F4F8FF', 0.9)
    # Moon, top right, with a halo.
    s.glow(2180, 190, 260, 260, '#DCE8FF', 0.28)
    s.circle(2180, 190, 58, s.radial([(0, '#FFFFFF'), (0.8, '#EEF2FF'), (1, '#C9D4EC')], fx=0.4, fy=0.35))
    s.circle(2162, 176, 10, '#D6DEF0', 0.7)
    s.circle(2196, 214, 7, '#D6DEF0', 0.6)
    aurora(s)
    # Distant snowy mountains, pale in the haze.
    far = hills(s, 830, 60, 1400, 0.9, s.linear([(0, '#8FA8C8'), (1, '#4E6890')]), 1)
    s.rect(0, 760, W, 260, s.linear([(0, '#6F8DB8', 0), (1, '#6F8DB8', 0.35)]))
    # Far pines.
    rng = Rng(7)
    mid = hills(s, 960, 30, 900, 2.5, s.linear([(0, '#DCE8F6'), (1, '#9CB4D2')]), 2)
    for k in range(64):
        x = k * 42 + rng.uniform(-14, 14)
        pine(s, x, ridge_y(mid, x) + 18, rng.uniform(70, 120), '#2E4A6E', snow='#C8DAEE')
    s.rect(0, 920, W, 180, s.linear([(0, '#B8CCE4', 0), (1, '#B8CCE4', 0.3)]))
    # The field the cabin stands on.
    near = hills(s, 1150, 26, 1600, 0.2, s.linear([(0, '#F4F8FF'), (0.5, '#D4E2F2'), (1, '#A8BEDA')]), 3)
    for k in range(34):
        x = k * 78 + rng.uniform(-20, 20)
        if abs(x - CABIN_X) < 230:
            continue
        pine(s, x, ridge_y(near, x) + 26, rng.uniform(150, 230), '#1E3656', snow='#EAF2FC', shade='#16294A')
    cabin(s, smoke)
    # Foreground drifts with blue shadows.
    s.path(smooth_ridge([(x, 1310 + 30 * math.sin(x / 700 + 1.2)) for x in range(-100, W + 201, 150)])
           + f' L{W + 200} {H} L-100 {H} Z', s.linear([(0, '#FFFFFF'), (0.4, '#E4EEF8'), (1, '#B4C8E2')]))
    s.ellipse(1900, 1400, 700, 70, s.radial([(0, '#7E9CC6', 0.35), (1, '#7E9CC6', 0)]))
    fence(s)
    # Big pines framing the right edge.
    pine(s, 2440, 1400, 520, '#16294A', snow='#EEF4FC', shade='#0F1E38')
    pine(s, 2250, 1420, 400, '#1A3052', snow='#EEF4FC', shade='#12223E')
    # Sparkles on the snow.
    rng = Rng(9)
    for _ in range(40):
        star(s, rng.uniform(0, W), rng.uniform(1300, 1430), rng.uniform(3, 7), '#FFFFFF', 0.8)
    return s


def build(root):
    out = os.path.join(root, 'themes', 'winter')
    build_scene(smoke=False).save(os.path.join(out, 'classic-wallpaper.svg'))
    # Phone: a portrait crop around the cabin, the full height; it has no
    # animated layers, so it keeps the static smoke.
    s = build_scene()
    vw = H * 780 / 1260
    s.crop(780, 1260, CABIN_X - vw * 0.42, 0, vw, H, 'Winter phone backdrop').save(
        os.path.join(out, 'classic-backdrop.svg'))
    return ['themes/winter/classic-wallpaper.svg', 'themes/winter/classic-backdrop.svg']
