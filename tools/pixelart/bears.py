"""The bear family in pixel art (tools/pixelart): dad (the Bear Den mark),
mama (lashes and a daisy) and the cub. Writes into apps/tv-shell/assets/pixel/:

  head-<kind>.png   3 frames: awake, blink, asleep            (scenes, peeks)
  bear-<kind>.png   whole bears, one column per pose (POSES), row 0 eyes
                    open, row 1 blinking                       (visitors, scenes)
  bear-mark.png     the small mark for the header and panels (16×16)

(the mark and each head, one file per face, are also copied to
apps/remote-web/static/art/pixel/ for the phone remote)

and qml/BearRig.js with each pose's frame size, the paw (for what the bear
carries) and the top of the head (for hats), which BearPuppet.qml and BearHead.qml read.
Palette: sage fur, cream muzzle and belly, charcoal ink, amber glints (the
colours of the original vector mark).
"""

import math
import os

from px import Img, from_ascii

FUR, FUR_D, FUR_L = '#79A889', '#5E8C6E', '#98C7A5'
CREAM, CREAM_D = '#EDE3D1', '#CDBFA6'
INK, INK_L = '#16191C', '#3E464C'
AMBER, BLUSH = '#E3B35C', '#E0978A'
OUT = '#15201B'
PETAL, PETAL_D, HEART = '#F7F0E2', '#D9CBB4', '#F2C14E'

POSES = ['stand', 'walk0', 'walk1', 'walk2', 'walk3', 'wave0', 'wave1', 'crouch', 'jump', 'sit', 'reach', 'sleep']

KINDS = {
    #        head w, h   torso rx, ry  leg len, thick  arm len, thick  frame w, h
    'dad':  dict(hw=24, hh=21, trx=7.5, try_=8, leg=7, lt=4, arm=9, at=3, fw=38, fh=46),
    'mama': dict(hw=23, hh=20, trx=7, try_=7.5, leg=6, lt=4, arm=8, at=3, fw=36, fh=43),
    'cub':  dict(hw=19, hh=17, trx=5.5, try_=5.5, leg=5, lt=3, arm=6, at=3, fw=30, fh=35),
}


# --- Heads -----------------------------------------------------------------
def head(kind, eyes):
    """eyes: 'awake', 'blink' or 'asleep'."""
    k = KINDS[kind]
    w, h = k['hw'], k['hh']
    img = Img(w + 2, h + 2)
    o = 1                                        # room for the outline
    cx = o + w / 2 - 0.5
    cy = o + h - 8.5 if kind != 'cub' else o + h - 7
    ry = 8.5 if kind != 'cub' else 7
    rx = w / 2 - 0.6
    er = 4 if kind != 'cub' else 3.2
    ex = rx - er + 1.2
    ey = o + er
    for side in (-1, 1):
        img.disc(cx + side * ex, ey, er, FUR)
        img.disc(cx + side * (ex - 0.8), ey + 0.8, er - 2, CREAM_D)
    if kind == 'dad':                            # the notched right ear
        img.clear(int(cx + ex + er - 1), int(ey - er + 1))
        img.clear(int(cx + ex + er - 2), int(ey - er))
    img.ellipse(cx, cy, rx, ry, FUR)
    # Light from the top left (a solid crescent), shade along the jaw.
    for y in range(img.h):
        for x in range(img.w):
            if img.get(x, y)[:3] != (0x79, 0xA8, 0x89):
                continue
            nx, ny = (x - cx) / rx, (y - cy) / ry
            r = math.hypot(nx, ny)
            if ny > 0.72 and r > 0.8:
                img.put(x, y, FUR_D)
            elif 0.62 < r < 0.86 and nx < -0.25 and ny < -0.25:
                img.put(x, y, FUR_L)
    mz = 5 if kind != 'cub' else 4
    muzzle_y = cy + (3 if kind != 'cub' else 2.5)
    img.ellipse(cx, muzzle_y, mz, 3.2 if kind != 'cub' else 2.6, CREAM)
    img.hline(cx - mz + 2, cx + mz - 2, int(muzzle_y + 3), CREAM_D)
    # Nose and mouth.
    ny = int(muzzle_y - 1.5)
    img.rect(int(cx - 1), ny, 3 if w % 2 else 2, 2, INK)
    img.rect(int(cx - 2), ny, 1, 1, INK)
    img.rect(int(cx + 2), ny, 1, 1, INK)
    img.put(int(cx - 1), ny, INK_L)
    img.put(int(cx) if w % 2 else int(cx) + 0, ny + 2, INK)
    if w % 2 == 0:
        img.put(int(cx) + 1, ny + 2, INK)
    img.put(int(cx - 2), ny + 3, INK)
    img.put(int(cx + 2) + (1 if w % 2 == 0 else 0), ny + 3, INK)
    # Eyes (and brows, lashes).
    d = 4.5 if kind != 'cub' else 4
    ey2 = int(cy - (1.5 if kind != 'cub' else 1))
    for side in (-1, 1):
        ex2 = int(round(cx + side * d))
        if eyes == 'awake' and kind == 'dad':
            img.hline(ex2 - 1, ex2 + 1, ey2 - 1, INK)  # happy closed eyes: ∩
            img.put(ex2 - 2, ey2, INK)
            img.put(ex2 + 2, ey2, INK)
            img.put(ex2, ey2, AMBER)
        elif eyes == 'awake':
            img.rect(ex2 - (1 if side < 0 else 0), ey2 - 1, 2, 2, INK)
            img.put(ex2 - (1 if side < 0 else 0), ey2 - 1, '#F4F1EA')
            if kind == 'mama':
                img.put(ex2 + side * 2 - (1 if side < 0 else 0) + (1 if side < 0 else 0), ey2 - 2, INK)
        else:
            img.hline(ex2 - 1, ex2 + 1, ey2, INK)    # blink / asleep: a line
            if eyes == 'asleep':
                img.put(ex2 - side * 1, ey2 + 1, INK)
        if kind == 'dad':                          # brows, rising outward
            img.hline(ex2 - 1, ex2 + 1, ey2 - 4, INK)
            img.put(ex2 + side * 2, ey2 - 5, INK)
        img.put(int(round(cx + side * (d + 2))), ey2 + 2, BLUSH)
    if kind == 'mama':                           # a daisy on her left ear
        fx, fy = int(cx - ex - 1), int(ey - 1)
        for dx, dy in ((0, -1), (-1, 0), (1, 0), (0, 1)):
            img.put(fx + dx, fy + dy, PETAL)
        img.put(fx - 1, fy - 1, PETAL_D); img.put(fx + 1, fy + 1, PETAL_D)
        img.put(fx, fy, HEART)
    img.outline(OUT)
    return img


# --- Whole bears ---------------------------------------------------------------
def capsule(img, x0, y0, x1, y1, thick, color):
    steps = max(1, int(math.hypot(x1 - x0, y1 - y0) * 2))
    for i in range(steps + 1):
        t = i / steps
        img.disc(x0 + (x1 - x0) * t, y0 + (y1 - y0) * t, (thick - 1) / 2, color)


def limb_end(x, y, length, angle_deg):
    a = math.radians(angle_deg)
    return x + math.sin(a) * length, y + math.cos(a) * length


# Per pose: (legs [far, near] degrees from straight down, positive = towards
# where the bear faces; arms [left, right]; body drop; leg length factor).
POSE = {
    'stand':  ([-4, 4], [-22, 22], 0, 1.0),
    'walk0':  ([-26, 22], [24, -18], 0, 1.0),
    'walk1':  ([-6, 6], [4, -4], -1, 1.0),
    'walk2':  ([22, -26], [-18, 24], 0, 1.0),
    'walk3':  ([6, -6], [-4, 4], -1, 1.0),
    'wave0':  ([-4, 4], [-18, 150], 0, 1.0),
    'wave1':  ([-4, 4], [-18, 118], 0, 1.0),
    'crouch': ([-28, 28], [-50, 50], 2, 0.6),
    'jump':   ([-38, 38], [-140, 140], -2, 0.7),
    'sit':    ([78, 86], [-14, 26], None, 1.0),
    'reach':  ([78, 86], [-14, 82], None, 1.0),
    'sleep':  ([78, 86], [38, 52], None, 1.0),
}


def bear(kind, pose, blink):
    k = KINDS[kind]
    legs, arms, drop, legf = POSE[pose]
    img = Img(k['fw'], k['fh'])
    cx = k['fw'] / 2 - 0.5
    sitting = drop is None
    leg = k['leg'] * legf
    foot_y = k['fh'] - 2                          # feet on the bottom (outline below)
    if sitting:
        hip_y = foot_y - k['lt'] / 2 + 0.5
    else:
        hip_y = foot_y - leg - (drop or 0)
    torso_cy = hip_y - k['try_'] + 2.5
    head_img = head(kind, 'asleep' if pose == 'sleep' else ('blink' if blink else 'awake'))
    head_x = int(round(cx - (head_img.w - 1) / 2))
    head_y = int(round(torso_cy - k['try_'] - head_img.h + 6))
    if pose == 'sleep':
        head_y += 1
    body = Img(k['fw'], k['fh'])
    # Legs (far one first), then the torso, arms, head.
    for i, ang in enumerate(legs):
        hx = cx + (-1 if i == 0 else 1) * (k['trx'] - 3)
        x1, y1 = limb_end(hx, hip_y, leg, ang)
        capsule(body, hx, hip_y, x1, y1, k['lt'], FUR_D if i == 0 else FUR)
        body.ellipse(x1 + (0.8 if sitting else 0), y1 + (0 if sitting else 0.5), k['lt'] / 2, 1, INK_L if i == 0 else INK)
    body.ellipse(cx, torso_cy, k['trx'], k['try_'], FUR)
    body.ellipse(cx, torso_cy + 1.5, k['trx'] - 3, k['try_'] - 3, CREAM)
    for y in range(body.h):                       # shade the torso's lower edge
        for x in range(body.w):
            if body.get(x, y)[:3] == (0x79, 0xA8, 0x89) and y > torso_cy + k['try_'] * 0.45 and (x + y) % 2 == 0:
                body.put(x, y, FUR_D)
    arm_layers = []
    paws = []
    for i, ang in enumerate(arms):
        side = -1 if i == 0 else 1
        sx, sy = cx + side * (k['trx'] - 1.5), torso_cy - k['try_'] + 3.5
        x1, y1 = limb_end(sx, sy, k['arm'], ang)
        layer = Img(k['fw'], k['fh'])
        capsule(layer, sx, sy, x1, y1, k['at'], FUR if i == 1 else FUR_D)
        layer.put(int(round(x1)), int(round(y1)), CREAM_D)
        arm_layers.append(layer.outline(OUT))
        paws.append((x1, y1))
    body.outline(OUT)
    img.blit(arm_layers[0], 0, 0)                 # the far arm, behind
    img.blit(body, 0, 0)
    img.blit(arm_layers[1], 0, 0)                 # the near arm, in front
    img.blit(head_img, head_x, head_y)
    return img, {
        'paw': [round(paws[1][0], 1), round(paws[1][1], 1)],
        'head': [head_x + head_img.w / 2, head_y + 1],
        'headWidth': head_img.w,
    }


MARK = [
    '..............',
    '.GGG......G.G.',
    'GGCGG....GGCGG',
    'GGCGGGGGGGGCGG',
    '.GGGGGGGGGGGG.',
    'GGKKGGGGGGKKGG',
    'GGGGGGGGGGGGGG',
    'GGGKGGGGGGKGGG',
    'GGKAKGGGGKAKGG',
    'GGGGGCCCCGGGGG',
    'GGGGCCKKCCGGGG',
    '.GGCCCKKCCCGG.',
    '.GGCKCCCCKCGG.',
    '..GGCKKKKCGG..',
    '...GGGGGGGG...',
]


def mark():
    img = from_ascii(MARK, {'G': FUR, 'C': CREAM, 'K': INK, 'A': AMBER})
    out = Img(img.w + 2, img.h + 2)
    out.blit(img, 1, 1)
    return out.outline(OUT)


def build(root, preview):
    out_dir = os.path.join(root, 'apps', 'tv-shell', 'assets', 'pixel')
    os.makedirs(out_dir, exist_ok=True)
    rig = {}
    previews = []
    for kind in KINDS:
        heads = [head(kind, e) for e in ('awake', 'blink', 'asleep')]
        hs = Img(heads[0].w * 3, heads[0].h)
        for i, hd in enumerate(heads):
            hs.blit(hd, i * hd.w, 0)
        hs.save(os.path.join(out_dir, f'head-{kind}.png'))
        k = KINDS[kind]
        sheet = Img(k['fw'] * len(POSES), k['fh'] * 2)
        anchors = {}
        for i, pose in enumerate(POSES):
            for row in (0, 1):
                frame, a = bear(kind, pose, row == 1)
                sheet.blit(frame, i * k['fw'], row * k['fh'])
                anchors[pose] = a
        sheet.save(os.path.join(out_dir, f'bear-{kind}.png'))
        rig[kind] = {'frame': [k['fw'], k['fh']], 'head': [heads[0].w, heads[0].h], 'poses': anchors}
        previews.append((hs, sheet))
    mark().save(os.path.join(out_dir, 'bear-mark.png'))
    # The phone remote serves its own copies (apps/remote-web/static/art/pixel).
    phone_dir = os.path.join(root, 'apps', 'remote-web', 'static', 'art', 'pixel')
    os.makedirs(phone_dir, exist_ok=True)
    mark().save(os.path.join(phone_dir, 'bear-mark.png'))
    for kind in KINDS:
        heads = [head(kind, e) for e in ('awake', 'blink', 'asleep')]
        for name, hd in zip(('', '-blink', '-sleep'), heads):
            hd.save(os.path.join(phone_dir, f'head-{kind}{name}.png'))
    js = os.path.join(root, 'apps', 'tv-shell', 'qml', 'BearRig.js')
    import json
    with open(js, 'w') as f:
        f.write('.pragma library\n')
        f.write('// Generated by tools/pixelart/bears.py — do not edit by hand.\n')
        f.write('// The pixel bears\' sheets (assets/pixel/bear-<kind>.png): frame size, the\n')
        f.write('// order of the poses (one column each; row 0 eyes open, row 1 blinking) and,\n')
        f.write('// per pose, the right paw and the top-centre of the head in art pixels.\n')
        f.write('\n')
        f.write('var poses = ' + json.dumps(POSES) + ';\n\n')
        f.write('var kinds = ' + json.dumps(rig, indent=1) + ';\n')
    if preview:
        os.makedirs(preview, exist_ok=True)
        total_w = max(s.w for _, s in previews)
        total_h = sum(s.h + h.h + 4 for h, s in previews)
        board = Img(total_w + 4, total_h + 4, '#2A2F3A')
        y = 2
        for hs, sheet in previews:
            board.blit(hs, 2, y)
            y += hs.h + 2
            board.blit(sheet, 2, y)
            y += sheet.h + 2
        board.blit(mark(), total_w - 16, 2)
        board.scaled(3).save(os.path.join(preview, 'bears.png'))
    print('bears: heads, sheets, mark, BearRig.js')
