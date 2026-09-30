#!/usr/bin/env python3
"""Regenerate Bear Den's pixel art (guide: docs/THEMES.md → Pixel art).

    python3 tools/pixelart/build.py                 every world, bear, ornament and weather icon
    python3 tools/pixelart/build.py campfire bears  only these
    python3 tools/pixelart/build.py --preview campfire   also write previews to
                                                          build/pixel-preview/

Worlds write their backdrop (wallpaper.png), phone backdrop and sprite sheets
into themes/<id>/ and rewrite that theme.json's "wallpaper" block so sprite
positions always match the art. Output is deterministic: running it twice
changes nothing. Pure Python standard library.
"""

import importlib
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, 'worlds'))

WORLDS = ['den', 'forest', 'midnight', 'campfire', 'winter']
PREVIEW = os.path.join(ROOT, 'build', 'pixel-preview')


def build_world(name, preview):
    mod = importlib.import_module(name)
    img, sprites = mod.build()
    folder = os.path.join(ROOT, 'themes', mod.ID)
    img.save(os.path.join(folder, 'wallpaper.png'))
    written, entries, sheets = set(), [], {}
    for (file, sheet, fps, x, y) in sprites:
        if sheet is not None:
            sheets[file] = sheet
        sheet = sheets[file]
        if file not in written:
            sheet.save(os.path.join(folder, file))
            written.add(file)
        frames = sheet.frames
        entries.append({'sheet': file, 'frames': frames, 'fps': fps, 'x': x, 'y': y})
    manifest_path = os.path.join(folder, 'theme.json')
    with open(manifest_path) as f:
        manifest = json.load(f)
    wp = manifest.get('wallpaper', {})
    wp['image'] = 'wallpaper.png'
    wp['pixel'] = True
    wp['sprites'] = entries
    manifest['wallpaper'] = wp
    if hasattr(mod, 'phone'):
        phone_img = mod.phone(img)
        phone_img.save(os.path.join(folder, 'backdrop.png'))
        manifest.setdefault('phone', {})['backdrop'] = 'backdrop.png'
    with open(manifest_path, 'w') as f:
        json.dump(manifest, f, indent=2, ensure_ascii=False)
        f.write('\n')
    if preview:
        os.makedirs(PREVIEW, exist_ok=True)
        for fr in range(2):
            comp = img.crop(0, 0, img.w, img.h)
            for e in entries:
                s = sheets[e['sheet']]
                fw = s.w // e['frames']
                comp.blit(s.crop((fr % e['frames']) * fw, 0, fw, s.h), e['x'], e['y'])
            comp.scaled(2).save(os.path.join(PREVIEW, f'{mod.ID}-f{fr}.png'))
    print(f'{mod.ID}: wallpaper + {len(written)} sheet(s)')



def main(argv):
    preview = '--preview' in argv
    names = [a for a in argv if not a.startswith('--')] or WORLDS + ['bears', 'ornaments', 'scenes', 'hero', 'weather', 'weatherprops', 'appicons', 'badges', 'uiicons']
    for n in names:
        if n in WORLDS:
            build_world(n, preview)
        else:
            mod = importlib.import_module(n)
            mod.build(ROOT, PREVIEW if preview else None)


if __name__ == '__main__':
    main(sys.argv[1:])
