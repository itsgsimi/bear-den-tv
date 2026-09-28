// One corner of a theme's decoration (the corner engine), grown around a
// card's rounded corner just outside the focus ring (or just inside a panel's
// edge, with a negative `gap`). The path runs along the top edge, around the
// corner, down the side and hooks outward. A theme picks a drawing `style`
// (manifest focus.style / panel.style; docs/THEMES.md):
//   vine    tapered stem, two-tone leaves, tendrils
//   fern    fronds unfurling in pairs to a fiddlehead curl
//   stars   a constellation traced star by star; more little stars twinkle in over time
//   embers  a glowing, flickering ember trail with coals; sparks rise (more over time)
// and ornaments (`decor`): a tip that blooms at the end, and extras that pop
// open along the path the longer focus stays. Colours come from the theme's
// palette. Drawn with Canvas (QPainter) — no GPU shaders — on the world's pixel
// grid: the canvas is 1/World.px of the item's size, painted without
// antialiasing and shown World.px times larger unsmoothed, so every stem, leaf
// and spark is made of whole art pixels (docs/THEMES.md → Pixel art). Geometry
// stays in screen units (the painter is scaled down). Geometry is for a
// TOP-LEFT corner at (inset, inset); FocusDecor/HeroDecor mirror it for other
// corners and set `upright` when mirrored vertically so flames and sparks rise.
// Classic art style (World.classic): the same drawing on a full-size
// antialiased canvas, the tip blooms with a spin (upright tips flicker by
// scaling) and extras pop open with an overshoot, turned a little each.

import QtQuick
import BearDen

Item {
    id: root
    property string style: "vine"
    // The theme's decoration (World.focus / World.panel): tip, tipUpright,
    // extras, extrasBottom, extrasUpright — ornament URLs and flags.
    property var decor: ({})
    property real progress: 0          // 0 = bare, 1 = fully grown
    property real phase: 0             // sway/twinkle/flicker phase (radians)
    property int extras: 0             // progressive additions shown so far
    property real cornerRadius: 20 * Theme.scale
    property real reach: 120 * Theme.scale      // along the top edge
    property real sideReach: 90 * Theme.scale   // down the side
    property real inset: 46 * Theme.scale       // where the card's corner sits in this item
    property real gap: 13 * Theme.scale         // path distance outside the card edge (negative: inside)
    property real size: 1                       // scale of leaves, stars and ornaments
    property int variant: 0                     // 0 = top corner, 1 = bottom corner (mirrored vertically)
    property bool upright: false                // mirrored vertically: keep flames/sparks rising
    property var palette: World.palette
    readonly property real s: Theme.scale * size
    width: inset + Math.max(reach, sideReach) + 30 * Theme.scale * size
    height: width
    // The pixel canvas and the Classic one; requestPaint() repaints the one shown.
    Canvas {
        id: smoothCanvas
        objectName: "cornerSmooth"
        visible: World.classic
        anchors.fill: parent
        antialiasing: true
        renderStrategy: Canvas.Immediate
        onVisibleChanged: if (visible) requestPaint()
        onPaint: root.paintAll(getContext("2d"))
    }
    Canvas {
        id: canvas
        objectName: "cornerPixel"
        visible: World.pixel
        onVisibleChanged: if (visible) requestPaint()
        width: Math.ceil(root.width / World.px)
        height: Math.ceil(root.height / World.px)
        scale: World.px
        transformOrigin: Item.TopLeft
        smooth: false
        antialiasing: false
        renderStrategy: Canvas.Immediate
        onPaint: root.paintAll(getContext("2d"))
    }
    function requestPaint() { if (World.classic) smoothCanvas.requestPaint(); else canvas.requestPaint() }
    onProgressChanged: requestPaint()
    onPhaseChanged: requestPaint()
    onExtrasChanged: requestPaint()
    onStyleChanged: requestPaint()
    onPaletteChanged: requestPaint()
    onWidthChanged: requestPaint()

    // The tip of the path (for the tip ornament) and how far it has bloomed.
    property point tip: Qt.point(0, 0)
    readonly property real bloom: {
        const b = Math.max(0, Math.min(1, (progress - 0.84) / 0.16))
        const c1 = 1.70158, c3 = c1 + 1            // ease-out-back
        return b <= 0 ? 0 : 1 + c3 * Math.pow(b - 1, 3) + c1 * Math.pow(b - 1, 2)
    }

    // --- Path ------------------------------------------------------------
    // Arc-length parametrisation: top edge (right→left), corner arc, side
    // (top→bottom), then a short hook outward, with a gentle wiggle.
    function pointAt(t) {
        const cr = cornerRadius, g = gap, h = 16 * Theme.scale
        const r = Math.max(4 * Theme.scale, cr + g)
        const la = Math.max(1, reach - cr), lb = Math.PI / 2 * r, lc = Math.max(1, sideReach - cr)
        const hookSweep = Math.PI * 0.42, ld = h * hookSweep
        const total = la + lb + lc + ld
        let d = t * total, x, y, ang
        if (d < la) {
            x = inset + cr + la - d; y = inset - g; ang = Math.PI
        } else if (d < la + lb) {
            const a = -Math.PI / 2 - (d - la) / r
            x = inset + cr + Math.cos(a) * r
            y = inset + cr + Math.sin(a) * r
            ang = a - Math.PI / 2
        } else if (d < la + lb + lc) {
            x = inset - g; y = inset + cr + (d - la - lb); ang = Math.PI / 2
        } else {
            const phi = (d - la - lb - lc) / h
            const cx = inset - g - h, cy = inset + cr + lc
            x = cx + Math.cos(phi) * h; y = cy + Math.sin(phi) * h
            ang = Math.PI / 2 + phi
        }
        const w = Math.sin(t * Math.PI * 3.4 + 0.5) * Math.sin(Math.PI * Math.min(1, t / 0.92)) * 2.6 * Theme.scale
        return { x: x + Math.cos(ang + Math.PI / 2) * w, y: y + Math.sin(ang + Math.PI / 2) * w, ang: ang }
    }
    // A point pushed `d` pixels outward (away from the card) from the path.
    function outward(t, d) {
        const p = pointAt(t)
        return { x: p.x + Math.cos(p.ang + Math.PI / 2) * d, y: p.y + Math.sin(p.ang + Math.PI / 2) * d, ang: p.ang }
    }
    function clamp01(v) { return Math.max(0, Math.min(1, v)) }

    // --- Shared strokes --------------------------------------------------
    function taperedStroke(ctx, upto, steps, w0, w1, color) {
        ctx.lineCap = "round"
        ctx.strokeStyle = color
        let prev = pointAt(0)
        for (let i = 1; i <= upto; ++i) {
            const t = i / steps, p = pointAt(t)
            ctx.beginPath()
            ctx.moveTo(prev.x, prev.y)
            ctx.lineTo(p.x, p.y)
            ctx.lineWidth = (w0 + (w1 - w0) * t) * s
            ctx.stroke()
            prev = p
        }
    }
    function leafShape(ctx, len, wid) {
        ctx.beginPath()
        ctx.moveTo(0, 0)
        ctx.bezierCurveTo(len * 0.25, -wid * 1.05, len * 0.72, -wid * 0.8, len, 0)
        ctx.bezierCurveTo(len * 0.7, wid * 0.75, len * 0.22, wid * 0.9, 0, 0)
    }
    function leaf(ctx, p, angle, len, wid) {
        ctx.save()
        ctx.translate(p.x, p.y)
        ctx.rotate(angle)
        leafShape(ctx, len, wid)
        const fill = ctx.createLinearGradient(0, 0, len, 0)
        fill.addColorStop(0, palette.dark)
        fill.addColorStop(1, palette.light)
        ctx.fillStyle = fill
        ctx.fill()
        ctx.beginPath()
        ctx.moveTo(len * 0.08, 0)
        ctx.quadraticCurveTo(len * 0.5, -wid * 0.12, len * 0.82, 0)
        ctx.strokeStyle = Theme.alpha(palette.stem, 0.8)
        ctx.lineWidth = 1 * Theme.scale
        ctx.stroke()
        ctx.restore()
    }
    function curl(ctx, p, th, grow, turns, step0, width) {
        if (grow <= 0) return
        let x = p.x, y = p.y
        const n = Math.round(26 * turns * grow)
        ctx.beginPath()
        ctx.moveTo(x, y)
        for (let i = 1; i <= n; ++i) {
            th += 0.1 + i * 0.016 / turns
            const step = (step0 - i * 0.075 / turns) * s
            x += Math.cos(th) * step; y += Math.sin(th) * step
            ctx.lineTo(x, y)
        }
        ctx.strokeStyle = palette.stem
        ctx.lineWidth = width * s
        ctx.lineCap = "round"
        ctx.stroke()
    }
    function sparkle(ctx, x, y, r, alpha) {
        ctx.beginPath()
        ctx.arc(x, y, r * 2.2, 0, Math.PI * 2)
        ctx.fillStyle = Theme.alpha(palette.glow, 0.16 * alpha)
        ctx.fill()
        ctx.beginPath()
        ctx.moveTo(x, y - r)
        ctx.quadraticCurveTo(x, y, x + r, y)
        ctx.quadraticCurveTo(x, y, x, y + r)
        ctx.quadraticCurveTo(x, y, x - r, y)
        ctx.quadraticCurveTo(x, y, x, y - r)
        ctx.fillStyle = Theme.alpha(palette.light, alpha)
        ctx.fill()
    }

    // --- Styles ------------------------------------------------------------
    readonly property var vineLeaves: [
        [{ t: 0.04, side: 1, s: 0.7 }, { t: 0.13, side: -1, s: 0.55 }, { t: 0.24, side: 1, s: 1.1 },
         { t: 0.36, side: -1, s: 0.62 }, { t: 0.47, side: 1, s: 1.3 }, { t: 0.56, side: -1, s: 0.66 },
         { t: 0.66, side: 1, s: 1.15 }, { t: 0.77, side: -1, s: 0.5 }, { t: 0.85, side: 1, s: 0.85 }],
        [{ t: 0.05, side: -1, s: 0.5 }, { t: 0.15, side: 1, s: 0.95 }, { t: 0.27, side: -1, s: 0.6 },
         { t: 0.38, side: 1, s: 1.25 }, { t: 0.5, side: -1, s: 0.68 }, { t: 0.6, side: 1, s: 1.2 },
         { t: 0.71, side: -1, s: 0.55 }, { t: 0.8, side: 1, s: 0.95 }]
    ]
    function paintVine(ctx, upto, steps) {
        taperedStroke(ctx, upto, steps, 4.8, 1.6, palette.stem)
        const tendrils = variant ? [0.2, 0.55] : [0.3, 0.72]
        for (const t of tendrils) {
            const p = pointAt(t)
            curl(ctx, p, p.ang + 1.0, clamp01((progress - t) / 0.18), 1, 3.1, 1.4)
        }
        const leaves = vineLeaves[variant % 2]
        for (let k = 0; k < leaves.length; ++k) {
            const L = leaves[k]
            const grow = clamp01((progress - L.t) / 0.14)
            if (grow <= 0) continue
            const open = L.side > 0 ? 0.95 : 0.6
            const p = pointAt(L.t)
            leaf(ctx, p, p.ang + L.side * open * (0.35 + 0.65 * grow) + Math.sin(phase + k * 1.3) * 0.1,
                 26 * s * L.s * grow, 10 * s * L.s * grow)
        }
    }
    function paintFern(ctx, upto, steps) {
        taperedStroke(ctx, upto, steps, 3.6, 1.3, palette.stem)
        const pairs = 17
        for (let k = 0; k < pairs; ++k) {
            const t = 0.04 + k * 0.048
            const grow = clamp01((progress - t) / 0.1)
            if (grow <= 0) break
            const p = pointAt(t)
            const size = (1.05 - t * 0.6) * grow
            const sway = Math.sin(phase + k * 0.7) * 0.07
            leaf(ctx, p, p.ang + 0.9 + sway, 17 * s * size, 4.6 * s * size)           // outward pinna
            leaf(ctx, p, p.ang - 0.55 + sway, 10 * s * size, 3.4 * s * size)          // tucked pinna
        }
        const tipGrow = clamp01((progress - 0.86) / 0.14)                            // fiddlehead
        const end = pointAt(Math.min(progress, 1))
        curl(ctx, end, end.ang + 0.4, tipGrow, 1.3, 2.4, 2.2)
    }
    readonly property var starNodes: [
        [{ t: 0.0, o: 4, r: 4.5 }, { t: 0.16, o: 9, r: 6 }, { t: 0.33, o: 2, r: 4 }, { t: 0.47, o: 11, r: 7.5 },
         { t: 0.62, o: 3, r: 4.5 }, { t: 0.78, o: 8, r: 6 }, { t: 0.92, o: 2, r: 3.8 }],
        [{ t: 0.0, o: 6, r: 4 }, { t: 0.18, o: 1, r: 5.5 }, { t: 0.36, o: 10, r: 6.5 }, { t: 0.52, o: 3, r: 4.2 },
         { t: 0.68, o: 9, r: 7 }, { t: 0.84, o: 2, r: 4.5 }]
    ]
    // Extra little stars that twinkle in over time: position along the path,
    // distance outward, radius.
    readonly property var extraStars: [
        { t: 0.1, o: 26, r: 2.4 }, { t: 0.4, o: 30, r: 3 }, { t: 0.25, o: 20, r: 2 }, { t: 0.7, o: 28, r: 2.6 },
        { t: 0.55, o: 36, r: 2.2 }, { t: 0.85, o: 22, r: 2.8 }, { t: 0.05, o: 38, r: 2 }, { t: 0.62, o: 18, r: 2.2 },
        { t: 0.33, o: 40, r: 2.4 }, { t: 0.95, o: 32, r: 2 }, { t: 0.2, o: 44, r: 1.8 }, { t: 0.76, o: 42, r: 2 }
    ]
    function paintStars(ctx) {
        const nodes = starNodes[variant % 2]
        const pts = nodes.map(n => outward(n.t, n.o * Theme.scale))
        ctx.lineCap = "round"
        ctx.strokeStyle = Theme.alpha(palette.stem, 0.5)
        ctx.lineWidth = 1.3 * Theme.scale
        for (let k = 1; k < nodes.length; ++k) {                   // lines, traced as progress passes
            const a = nodes[k - 1], b = nodes[k]
            const f = clamp01((progress - a.t) / Math.max(0.01, b.t - a.t))
            if (f <= 0) break
            ctx.beginPath()
            ctx.moveTo(pts[k - 1].x, pts[k - 1].y)
            ctx.lineTo(pts[k - 1].x + (pts[k].x - pts[k - 1].x) * f, pts[k - 1].y + (pts[k].y - pts[k - 1].y) * f)
            ctx.stroke()
        }
        for (let k = 0; k < nodes.length; ++k) {                    // stars at the nodes
            const appear = clamp01((progress - nodes[k].t) / 0.08)
            if (appear <= 0) continue
            const tw = 0.8 + 0.25 * Math.sin(phase * 2 + k * 1.7)
            sparkle(ctx, pts[k].x, pts[k].y, nodes[k].r * s * appear * tw, 0.95)
        }
        for (let k = 0; k < Math.min(extras * 2, extraStars.length); ++k) {
            const e = extraStars[k], p = outward(e.t, e.o * Theme.scale)
            const tw = 0.55 + 0.45 * Math.sin(phase * 1.6 + k * 2.3)
            sparkle(ctx, p.x, p.y, e.r * s, tw)
        }
    }
    function paintEmbers(ctx, upto, steps) {
        taperedStroke(ctx, upto, steps, 9, 5, Theme.alpha(palette.glow, 0.16))    // soft glow
        ctx.lineCap = "round"
        let prev = pointAt(0)
        for (let i = 1; i <= upto; ++i) {                                         // flickering core
            const t = i / steps, p = pointAt(t)
            const flick = 0.65 + 0.35 * Math.sin(phase * 3 + t * 19)
            ctx.beginPath()
            ctx.moveTo(prev.x, prev.y)
            ctx.lineTo(p.x, p.y)
            ctx.strokeStyle = Theme.alpha(palette.bloom, flick)
            ctx.lineWidth = (3.2 - 1.8 * t) * s
            ctx.stroke()
            prev = p
        }
        for (let k = 0; k < 6; ++k) {                                              // coals
            const t = 0.08 + k * 0.15
            if (t > progress) break
            const p = pointAt(t)
            const glow = 0.6 + 0.4 * Math.sin(phase * 2.2 + k * 2.1)
            ctx.beginPath()
            ctx.arc(p.x, p.y, 6.5 * s, 0, Math.PI * 2)
            ctx.fillStyle = Theme.alpha(palette.glow, 0.22 * glow)
            ctx.fill()
            ctx.beginPath()
            ctx.arc(p.x, p.y, 2.6 * s, 0, Math.PI * 2)
            ctx.fillStyle = Theme.alpha(palette.light, 0.7 + 0.3 * glow)
            ctx.fill()
        }
        if (progress < 0.99) return
        const sparks = Math.min(4 + extras * 2, 14)                                // rising sparks
        const up = upright ? 1 : -1
        for (let k = 0; k < sparks; ++k) {
            const base = pointAt(0.05 + ((k * 0.37) % 0.85))
            const rise = ((phase / (Math.PI * 2)) * (0.8 + (k % 3) * 0.25) + k * 0.29) % 1
            const x = base.x + Math.sin(rise * 7 + k) * 5 * Theme.scale
            const y = base.y + up * rise * 46 * Theme.scale
            ctx.beginPath()
            ctx.arc(x, y, (1.9 - rise) * s, 0, Math.PI * 2)
            ctx.fillStyle = Theme.alpha(palette.light, (1 - rise) * 0.95)
            ctx.fill()
        }
    }

    function paintAll(ctx) {
        ctx.reset()
        if (progress <= 0 || !palette) return
        if (World.pixel)
            ctx.scale(1 / World.px, 1 / World.px)
        const steps = 90
        const upto = Math.max(1, Math.round(steps * progress))
        switch (style) {
        case "vine": paintVine(ctx, upto, steps); break
        case "fern": paintFern(ctx, upto, steps); break
        case "stars": paintStars(ctx); break
        case "embers": paintEmbers(ctx, upto, steps); break
        }
        const end = pointAt(Math.min(progress, 1))
        tip = Qt.point(end.x, end.y)
    }

    // --- Ornaments -----------------------------------------------------------
    // The tip ornament (a daisy, a crescent moon, a flame, a snowflake, …) blooms
    // with a little spin as the path finishes; upright tips (flames, mushrooms)
    // stay upright on mirrored corners and flicker instead of spinning.
    Ornament {
        readonly property bool upright: root.decor.tipUpright === true
        readonly property real grow: root.bloom
        url: root.decor.tip || ""
        width: (upright ? 26 : 30) * root.s
        height: width
        x: World.classic ? root.tip.x - width / 2 : World.snap(root.tip.x - width / 2)
        y: World.classic ? root.tip.y - height * (upright ? 0.8 : 0.5) : World.snap(root.tip.y - height * (upright ? 0.8 : 0.5))
        visible: grow > 0 && url.toString().length > 0
        flip: root.upright && upright
        // Pixel art is never rotated or scaled in between: it pops open
        // (bloom) and upright tips flicker by hopping a pixel. Classic tips
        // grow with a spin, and upright ones flicker by scaling.
        scale: World.classic ? grow * (upright ? 0.9 + 0.12 * Math.sin(root.phase * 3.3) : 1) : grow >= 0.5 ? 1 : 0
        rotation: World.classic && !upright ? (1 - Math.min(1, grow)) * -120 + Math.sin(root.phase) * 8 : 0
        transformOrigin: World.classic && upright ? Item.Bottom : Item.Center
        transform: Translate { y: World.pixel && upright && Math.sin(root.phase * 3.3) > 0.6 ? -World.px : 0 }
    }
    // Ornaments that pop open along the path while focus stays (the theme's
    // extras; extrasBottom on bottom corners). Upright extras (mushrooms) stand
    // on the top edge; the rest turn a little each.
    readonly property var extraSpots: variant === 0
        ? [{ t: 0.3, o: 14 }, { t: 0.62, o: 16 }, { t: 0.14, o: 18 }, { t: 0.8, o: 14 }, { t: 0.46, o: 20 }, { t: 0.05, o: 12 }]
        : [{ t: 0.38, o: 15 }, { t: 0.66, o: 14 }, { t: 0.2, o: 18 }, { t: 0.52, o: 18 }, { t: 0.82, o: 13 }, { t: 0.08, o: 14 }]
    readonly property var extraUrls: (variant === 0 ? root.decor.extras : root.decor.extrasBottom) || []
    Repeater {
        model: root.extraUrls.length > 0 ? root.extraSpots : []
        Ornament {
            required property var modelData
            required property int index
            readonly property bool open: root.extras > index && root.progress > 0.99
            readonly property bool standing: root.decor.extrasUpright === true && root.variant === 0
            readonly property var at: root.outward(modelData.t, modelData.o * Theme.scale * root.size)
            url: root.extraUrls[index % root.extraUrls.length]
            width: (standing ? 22 : index % 2 ? 20 : 17) * root.s
            height: width
            x: World.classic ? at.x - width / 2 : World.snap(at.x - width / 2)
            y: World.classic ? at.y - height * (standing ? 0.85 : 0.5) : World.snap(at.y - height * (standing ? 0.85 : 0.5))
            mirror: World.pixel && !standing && index % 2 === 1
            // Classic: turned a little each, popping open with an overshoot.
            rotation: World.classic && !standing ? index * 47 : 0
            transformOrigin: standing ? Item.Bottom : Item.Center
            scale: World.pixel || open ? 1 : 0
            visible: World.classic ? scale > 0 : open
            Behavior on scale { enabled: World.classic; NumberAnimation { duration: 650; easing.type: Easing.OutBack } }
        }
    }
}
