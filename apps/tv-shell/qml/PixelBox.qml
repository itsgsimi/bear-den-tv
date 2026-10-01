// A box in pixel art: the shell's stand-in for a rounded Rectangle. Same
// `color`, `gradient` and `radius` (screen pixels), plus `borderColor` and
// `borderWidth`; drawn on the world's pixel grid (World.px), so the corners are
// stairs of whole art pixels and the border is whole art pixels thick. Painted
// with Canvas (QPainter, no shaders) at 1/World.px of the size without
// antialiasing and shown World.px times larger unsmoothed; it repaints only
// when its size or colours change. Children draw on top, like a Rectangle's.
// In the Classic art style (World.classic) it is a plain antialiased rounded
// Rectangle with the same properties instead, and the Canvas is not created
// at all (the shell has hundreds of boxes; an idle Canvas still costs an item,
// a 2D context and a texture each).
// Guide: docs/THEMES.md → Pixel art.

pragma ComponentBehavior: Bound

import QtQuick
import BearDen

Item {
    id: box
    property color color: "white"
    property Gradient gradient: null
    property real radius: 0
    property color borderColor: "transparent"
    property real borderWidth: 0

    onColorChanged: box.repaint()
    onGradientChanged: box.repaint()
    onRadiusChanged: box.repaint()
    onBorderColorChanged: box.repaint()
    onBorderWidthChanged: box.repaint()
    Connections {
        target: box.gradient
        ignoreUnknownSignals: true
        function onUpdated() { box.repaint() }
    }

    function repaint() {
        const canvas = pixelCanvas.itemAt(0) as Canvas
        if (canvas)
            canvas.requestPaint()
    }

    // How far row y is cut in at a corner of radius r (art pixels): straight
    // stairs for small radii, a pixel circle for larger ones.
    function insets(h, r) {
        const out = new Array(h).fill(0)
        for (let y = 0; y < r; ++y) {
            const d = r - y - 0.5
            const v = r <= 3 ? r - y : Math.round(r - Math.sqrt(Math.max(0, r * r - d * d)))
            out[y] = Math.max(out[y], v)
            out[h - 1 - y] = Math.max(out[h - 1 - y], v)
        }
        return out
    }

    Rectangle {
        visible: World.classic
        anchors.fill: parent
        color: box.color
        gradient: box.gradient
        radius: box.radius
        border.color: box.borderColor
        border.width: box.borderWidth
        antialiasing: true
    }

    // The pixel Canvas exists only in the Pixel art style (a Repeater of 0 or
    // 1 keeps it a direct child, under the box's own children).
    Repeater {
        id: pixelCanvas
        model: World.pixel ? 1 : 0
        delegate: Canvas {
            width: Math.max(1, Math.round(box.width / World.px))
            height: Math.max(1, Math.round(box.height / World.px))
            scale: World.px
            transformOrigin: Item.TopLeft
            smooth: false
            antialiasing: false
            renderStrategy: Canvas.Immediate
            onWidthChanged: requestPaint()
            onHeightChanged: requestPaint()
            Component.onCompleted: requestPaint()
            onPaint: {
                const ctx = getContext("2d")
                ctx.reset()
                const w = width, h = height
                const r = Math.min(Math.round(box.radius / World.px), Math.floor(Math.min(w, h) / 2))
                const inset = box.insets(h, r)
                const b = box.borderWidth > 0 && box.borderColor.a > 0 ? Math.max(1, Math.round(box.borderWidth / World.px)) : 0
                // Fill: one span per row.
                let fill = box.color
                if (box.gradient && box.gradient.stops.length > 0) {
                    const horizontal = box.gradient.orientation === Gradient.Horizontal
                    const g = horizontal ? ctx.createLinearGradient(0, 0, w, 0) : ctx.createLinearGradient(0, 0, 0, h)
                    for (let i = 0; i < box.gradient.stops.length; ++i)
                        g.addColorStop(box.gradient.stops[i].position, box.gradient.stops[i].color)
                    fill = g
                }
                ctx.fillStyle = fill
                for (let y = 0; y < h; ++y)
                    ctx.fillRect(inset[y], y, w - 2 * inset[y], 1)
                if (b === 0)
                    return
                // Border: the top and bottom b rows whole, then on each row from
                // its edge to the widest of the b rows above and below it (so the
                // stairs are outlined too).
                ctx.fillStyle = box.borderColor
                for (let y = 0; y < h; ++y) {
                    const a = inset[y], span = w - 2 * a
                    if (span <= 0)
                        continue
                    if (y < b || y >= h - b) {
                        ctx.fillRect(a, y, span, 1)
                        continue
                    }
                    let edge = a + b
                    for (let k = 1; k <= b; ++k)
                        edge = Math.max(edge, inset[y - k], inset[y + k])
                    const run = Math.min(edge - a, Math.ceil(span / 2))
                    ctx.fillRect(a, y, run, 1)
                    ctx.fillRect(w - a - run, y, run, 1)
                }
            }
        }
    }
}
