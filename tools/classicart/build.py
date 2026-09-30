"""Build Bear Den's generated Classic art (smooth SVG), the counterpart of
tools/pixelart for the parts that were only ever drawn as pixels.

    python3 -B tools/classicart/build.py            # everything
    python3 -B tools/classicart/build.py winter     # only these parts

Parts: winter, hero, weather, extras, layers, appicons, badges, uiicons. Deterministic: a second run changes
nothing. See README.md.
"""

import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
ROOT = os.path.normpath(os.path.join(HERE, '..', '..'))

import appicons  # noqa: E402
import badges   # noqa: E402
import extras   # noqa: E402
import hero     # noqa: E402
import layers   # noqa: E402
import uiicons  # noqa: E402
import weather  # noqa: E402
import winter   # noqa: E402

PARTS = {'winter': winter.build, 'hero': hero.build, 'weather': weather.build, 'extras': extras.build,
         'layers': layers.build, 'appicons': appicons.build, 'badges': badges.build,
         'uiicons': uiicons.build}


def main(argv):
    names = argv or list(PARTS)
    for name in names:
        if name not in PARTS:
            sys.exit(f'unknown part {name!r}; parts: {", ".join(PARTS)}')
    for name in names:
        files = PARTS[name](ROOT)
        print(f'{name}: {len(files)} files')


if __name__ == '__main__':
    main(sys.argv[1:])
