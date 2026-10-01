// An app-coloured background: a vertical gradient in the app's brand colours
// with a glow where its icon sits. Pixel art like the rest of the world: painted
// at 1/World.px of the size without antialiasing (so the glow falls off in
// visible bands and the corners are the same stairs as PixelBox) and shown
// World.px times larger unsmoothed. Painted once per size or colour change
// (Canvas/QPainter, no shaders), so it costs nothing while idle.
// Classic art style (World.classic): painted at full size with antialiasing, a
// smooth radial glow in a smoothly rounded rectangle.

pragma ComponentBehavior: Bound

import QtQuick
import BearDen

Item {
    id: root
    property color topColor: "#2B2F33"
    property color bottomColor: "#121416"
    property color glow: Theme.accent
    property real glowX: 0.2          // glow centre, as fractions of the size
    property real glowY: 0.35
    property real glowRadius: 0.8     // fraction of the larger side
    property real glowStrength: 0.45  // alpha at the centre
    property real radius: Theme.radius

    function requestPaint() {
        const c = canvas.item as Canvas
        if (c)
            c.requestPaint()
    }
    onTopColorChanged: requestPaint()
    onBottomColorChanged: requestPaint()
    onGlowChanged: requestPaint()
    onRadiusChanged: requestPaint()

    // Only the current style's Canvas exists (every app tile has a backdrop).
    // No size of its own: each Canvas sizes itself (the pixel one at 1/World.px,
    // scaled up).
    Loader {
        id: canvas
        sourceComponent: World.classic ? smoothCanvas : pixelCanvas
    }
    Component {
        id: smoothCanvas
        Canvas {
            objectName: "brandBackdropSmooth"
            width: root.width
            height: root.height
            antialiasing: true
            onWidthChanged: requestPaint()
            onHeightChanged: requestPaint()
            Component.onCompleted: requestPaint()
            onPaint: {
                const ctx = getContext("2d")
                ctx.reset()
                if (width <= 0 || height <= 0)
                    return
                ctx.beginPath()
                ctx.roundedRect(0, 0, width, height, root.radius, root.radius)
                const base = ctx.createLinearGradient(0, 0, 0, height)
                base.addColorStop(0, root.topColor)
                base.addColorStop(1, root.bottomColor)
                ctx.fillStyle = base
                ctx.fill()
                const cx = width * root.glowX, cy = height * root.glowY
                const r = Math.max(width, height) * root.glowRadius
                const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, r)
                g.addColorStop(0, Theme.alpha(root.glow, root.glowStrength))
                g.addColorStop(0.4, Theme.alpha(root.glow, root.glowStrength * 0.3))
                g.addColorStop(1, Theme.alpha(root.glow, 0))
                ctx.fillStyle = g
                ctx.fill()
            }
        }
    }
    Component {
        id: pixelCanvas
        Canvas {
            width: Math.max(1, Math.round(root.width / World.px))
            height: Math.max(1, Math.round(root.height / World.px))
            scale: World.px
            transformOrigin: Item.TopLeft
            smooth: false
            antialiasing: false
            onWidthChanged: requestPaint()
            onHeightChanged: requestPaint()
            Component.onCompleted: requestPaint()
            onPaint: {
                const ctx = getContext("2d")
                ctx.reset()
                const w = width, h = height
                if (w <= 1 || h <= 1)
                    return
                // The stair-cornered outline (the same stairs as PixelBox).
                const r = Math.min(Math.round(root.radius / World.px), Math.floor(Math.min(w, h) / 2))
                const inset = new Array(h).fill(0)
                for (let y = 0; y < r; ++y) {
                    const d = r - y - 0.5
                    const v = r <= 3 ? r - y : Math.round(r - Math.sqrt(Math.max(0, r * r - d * d)))
                    inset[y] = Math.max(inset[y], v)
                    inset[h - 1 - y] = Math.max(inset[h - 1 - y], v)
                }
                const base = ctx.createLinearGradient(0, 0, 0, h)
                base.addColorStop(0, root.topColor)
                base.addColorStop(1, root.bottomColor)
                const cx = w * root.glowX, cy = h * root.glowY
                const rad = Math.max(w, h) * root.glowRadius
                const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, rad)
                g.addColorStop(0, Theme.alpha(root.glow, root.glowStrength))
                g.addColorStop(0.4, Theme.alpha(root.glow, root.glowStrength * 0.3))
                g.addColorStop(1, Theme.alpha(root.glow, 0))
                for (const fill of [base, g]) {
                    ctx.fillStyle = fill
                    for (let y = 0; y < h; ++y)
                        ctx.fillRect(inset[y], y, w - 2 * inset[y], 1)
                }
            }
        }
    }
}
