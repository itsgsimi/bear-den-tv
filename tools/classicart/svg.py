"""A tiny SVG writer for tools/classicart (standard library only).

Everything the Classic art uses stays inside what Qt's SVG renderer draws
(SVG Tiny 1.2): paths, circles, ellipses, rects, linear and radial gradients,
fill-opacity and stop-opacity. No filters, masks, clip paths or blur: soft
edges and shadows are radial gradients that fade to a zero stop-opacity.

Numbers are written with at most two decimals, so the output is deterministic.
"""

import math


def n(v):
    """A number as short, stable text."""
    s = f'{v:.2f}'.rstrip('0').rstrip('.')
    return '0' if s in ('-0', '') else s


def pts(points):
    return ' '.join(f'{n(x)},{n(y)}' for x, y in points)


class Rng:
    """A small deterministic generator (an LCG), same everywhere."""

    def __init__(self, seed):
        self.s = (seed * 2654435761 + 12345) & 0xFFFFFFFF

    def next(self):
        self.s = (1103515245 * self.s + 12345) & 0x7FFFFFFF
        return self.s / 0x7FFFFFFF

    def uniform(self, a, b):
        return a + (b - a) * self.next()

    def int(self, a, b):
        return a + int(self.next() * (b - a + 1)) % (b - a + 1)


class Svg:
    def __init__(self, w, h, label, note='', vw=None, vh=None, vx=0, vy=0):
        self.w, self.h = w, h
        self.vx, self.vy = vx, vy
        self.vw, self.vh = (vw or w), (vh or h)
        self.label, self.note = label, note
        self.defs = []
        self.body = []
        self._ids = 0

    # --- paint servers --------------------------------------------------------
    def _id(self, kind):
        self._ids += 1
        return f'{kind}{self._ids}'

    @staticmethod
    def _stops(stops):
        out = []
        for s in stops:
            off, col = s[0], s[1]
            op = s[2] if len(s) > 2 else 1
            extra = '' if op == 1 else f' stop-opacity="{n(op)}"'
            out.append(f'<stop offset="{n(off)}" stop-color="{col}"{extra}/>')
        return ''.join(out)

    def linear(self, stops, x1=0, y1=0, x2=0, y2=1):
        """A gradient across the shape's box (0..1), top to bottom by default."""
        i = self._id('l')
        self.defs.append(f'<linearGradient id="{i}" x1="{n(x1)}" y1="{n(y1)}" x2="{n(x2)}" y2="{n(y2)}">'
                         + self._stops(stops) + '</linearGradient>')
        return f'url(#{i})'

    def linear_abs(self, stops, x1, y1, x2, y2):
        """A gradient in user coordinates."""
        i = self._id('l')
        self.defs.append(f'<linearGradient id="{i}" gradientUnits="userSpaceOnUse" x1="{n(x1)}" y1="{n(y1)}" '
                         f'x2="{n(x2)}" y2="{n(y2)}">' + self._stops(stops) + '</linearGradient>')
        return f'url(#{i})'

    def radial(self, stops, cx=0.5, cy=0.5, r=0.5, fx=None, fy=None):
        i = self._id('r')
        f = '' if fx is None else f' fx="{n(fx)}" fy="{n(fy)}"'
        self.defs.append(f'<radialGradient id="{i}" cx="{n(cx)}" cy="{n(cy)}" r="{n(r)}"{f}>'
                         + self._stops(stops) + '</radialGradient>')
        return f'url(#{i})'

    # --- shapes ---------------------------------------------------------------
    @staticmethod
    def _attrs(fill, opacity, stroke, sw, extra):
        a = f' fill="{fill}"'
        if opacity != 1:
            a += f' fill-opacity="{n(opacity)}"'
        if stroke:
            a += f' stroke="{stroke}" stroke-width="{n(sw)}" stroke-linecap="round" stroke-linejoin="round"'
        return a + extra

    def rect(self, x, y, w, h, fill, rx=0, opacity=1, stroke=None, sw=1, extra=''):
        r = f' rx="{n(rx)}"' if rx else ''
        self.body.append(f'<rect x="{n(x)}" y="{n(y)}" width="{n(w)}" height="{n(h)}"{r}'
                         + self._attrs(fill, opacity, stroke, sw, extra) + '/>')

    def circle(self, cx, cy, r, fill, opacity=1, stroke=None, sw=1, extra=''):
        self.body.append(f'<circle cx="{n(cx)}" cy="{n(cy)}" r="{n(r)}"'
                         + self._attrs(fill, opacity, stroke, sw, extra) + '/>')

    def ellipse(self, cx, cy, rx, ry, fill, opacity=1, stroke=None, sw=1, extra=''):
        self.body.append(f'<ellipse cx="{n(cx)}" cy="{n(cy)}" rx="{n(rx)}" ry="{n(ry)}"'
                         + self._attrs(fill, opacity, stroke, sw, extra) + '/>')

    def path(self, d, fill, opacity=1, stroke=None, sw=1, extra=''):
        self.body.append(f'<path d="{d}"' + self._attrs(fill, opacity, stroke, sw, extra) + '/>')

    def poly(self, points, fill, opacity=1, stroke=None, sw=1):
        self.path('M' + ' L'.join(f'{n(x)} {n(y)}' for x, y in points) + ' Z', fill, opacity, stroke, sw)

    def line(self, x1, y1, x2, y2, stroke, sw=1, opacity=1):
        o = '' if opacity == 1 else f' stroke-opacity="{n(opacity)}"'
        self.body.append(f'<path d="M{n(x1)} {n(y1)} L{n(x2)} {n(y2)}" fill="none" stroke="{stroke}" '
                         f'stroke-width="{n(sw)}" stroke-linecap="round"{o}/>')

    def glow(self, cx, cy, rx, ry, col, strength):
        """A soft light: a radial gradient fading to nothing."""
        g = self.radial([(0, col, strength), (0.45, col, strength * 0.45), (1, col, 0)])
        self.ellipse(cx, cy, rx, ry, g)

    def raw(self, text):
        self.body.append(text)

    def text(self):
        head = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{n(self.w)}" height="{n(self.h)}" '
                f'viewBox="{n(self.vx)} {n(self.vy)} {n(self.vw)} {n(self.vh)}" role="img" aria-label="{self.label}">\n')
        note = f'  <!-- Generated by tools/classicart — do not edit by hand. {self.note} -->\n'
        defs = '  <defs>\n' + ''.join(f'    {d}\n' for d in self.defs) + '  </defs>\n' if self.defs else ''
        return head + note + defs + ''.join(f'  {b}\n' for b in self.body) + '</svg>\n'

    def crop(self, w, h, vx, vy, vw, vh, label):
        """The same drawing, framed differently (for example a phone crop)."""
        c = Svg(w, h, label, self.note, vw, vh, vx, vy)
        c.defs, c.body = self.defs, self.body
        return c

    def save(self, path):
        with open(path, 'w', newline='\n') as f:
            f.write(self.text())


def smooth_ridge(points):
    """A closed-bottom path through (x, y) points with quadratic smoothing."""
    d = f'M{n(points[0][0])} {n(points[0][1])}'
    for i in range(1, len(points) - 1):
        x0, y0 = points[i]
        x1, y1 = points[i + 1]
        d += f' Q{n(x0)} {n(y0)} {n((x0 + x1) / 2)} {n((y0 + y1) / 2)}'
    d += f' L{n(points[-1][0])} {n(points[-1][1])}'
    return d


def pine(s, x, base, h, body, snow=None, shade=None):
    """A soft pine: three stacked rounded tiers, optional snow on each."""
    w = h * 0.46
    for k in range(3):
        top = base - h + k * h * 0.26
        bot = base - h * 0.08 - (2 - k) * h * 0.2
        hw = w * (0.55 + 0.22 * k)
        s.path(f'M{n(x)} {n(top)} Q{n(x + hw * 0.35)} {n(top + (bot - top) * 0.45)} {n(x + hw)} {n(bot)} '
               f'Q{n(x)} {n(bot + h * 0.06)} {n(x - hw)} {n(bot)} '
               f'Q{n(x - hw * 0.35)} {n(top + (bot - top) * 0.45)} {n(x)} {n(top)} Z', body)
        if shade:
            s.path(f'M{n(x)} {n(top)} Q{n(x + hw * 0.35)} {n(top + (bot - top) * 0.45)} {n(x + hw)} {n(bot)} '
                   f'Q{n(x + hw * 0.4)} {n(bot + h * 0.03)} {n(x)} {n(bot + h * 0.02)} Z', shade)
        if snow:
            sy = top + (bot - top) * 0.38
            s.path(f'M{n(x)} {n(top + 0.5)} Q{n(x + hw * 0.3)} {n(sy - (bot - top) * 0.1)} {n(x + hw * 0.45)} {n(sy)} '
                   f'Q{n(x + hw * 0.2)} {n(sy - (bot - top) * 0.12)} {n(x)} {n(sy - (bot - top) * 0.02)} '
                   f'Q{n(x - hw * 0.2)} {n(sy - (bot - top) * 0.12)} {n(x - hw * 0.45)} {n(sy)} '
                   f'Q{n(x - hw * 0.3)} {n(sy - (bot - top) * 0.1)} {n(x)} {n(top + 0.5)} Z', snow)


def star(s, cx, cy, r, fill, opacity=1):
    """A four-point twinkle."""
    k = r * 0.28
    s.path(f'M{n(cx)} {n(cy - r)} Q{n(cx + k)} {n(cy - k)} {n(cx + r)} {n(cy)} Q{n(cx + k)} {n(cy + k)} {n(cx)} {n(cy + r)} '
           f'Q{n(cx - k)} {n(cy + k)} {n(cx - r)} {n(cy)} Q{n(cx - k)} {n(cy - k)} {n(cx)} {n(cy - r)} Z', fill, opacity)


def wave(x0, x1, y, amp, period, phase=0.0, step=None):
    """Points along a gentle sine."""
    step = step or period / 8
    out, x = [], x0
    while x <= x1 + 1e-6:
        out.append((x, y + amp * math.sin((x / period) * 2 * math.pi + phase)))
        x += step
    return out
