// Selection ring just outside a focused element, separated from it by a thin
// gap: one art pixel thick with stair-step corners (PixelBox), on the world's
// pixel grid. Focus is never shown by colour alone: the owner also scales up.
// `cornerRadius` is the owner's corner radius; the ring follows it.
// `glint` (tiles): a spark in the theme's bloom colour — a bright pixel with a
// short fading tail — races once round the ring when focus lands, then drifts
// slowly round it, on World's heartbeat. Stops while resting; none in Plain or
// reduced motion.
// Classic art style (World.classic): a smooth rounded ring Theme.focusWidth
// thick, and the glint is an antialiased spark with a soft halo and a fading
// tail that follows the rounded corners (Canvas, repainted on each beat only
// while it glints).

import QtQuick
import BearDen

PixelBox {
    id: ring
    property bool shown: false
    property real cornerRadius: Theme.radius
    property bool glint: false
    readonly property int p: World.px
    readonly property real gap: World.classic ? 3 * Theme.scale : p
    readonly property real thickness: World.classic ? Theme.focusWidth : Math.max(p, Math.round(Theme.focusWidth / p) * p)
    anchors.fill: parent
    anchors.margins: -(gap + thickness)
    radius: cornerRadius + gap + thickness
    color: "transparent"
    borderColor: Theme.focusColor
    borderWidth: thickness
    opacity: shown ? 1 : 0
    visible: opacity > 0
    Behavior on opacity { NumberAnimation { duration: Theme.durationFast } }

    readonly property bool glinting: glint && shown && World.decorated && !Theme.reducedMotion
                                     && !Theme.screensaver && Session.target.kind === "shell"
    // Position round the ring, 0..1 per lap: a fast first lap, then slow.
    property real pos: 0
    property real speed: 0
    onGlintingChanged: if (glinting) { pos = 0.08; speed = 1.4; if (World.classic) spark.requestPaint() }
    Component.onCompleted: if (glinting) { pos = 0.08; speed = 1.4 }
    Connections {
        target: World
        enabled: ring.glinting && !Theme.resting
        function onBeat(dt) {
            ring.pos = (ring.pos + ring.speed * dt) % 1
            ring.speed = Math.max(0.12, ring.speed - 1.3 * dt)   // ease from a sprint to a stroll
            if (World.classic)
                spark.requestPaint()
        }
    }
    // A point `d` art pixels along the ring's border (clockwise from the top
    // left), on its straight edges; corners are skipped over.
    function pointAt(d) {
        const W = Math.round(width / p), H = Math.round(height / p)
        const r = Math.min(Math.round(radius / p), Math.floor(Math.min(W, H) / 2))
        const top = W - 2 * r, side = H - 2 * r
        const P = 2 * (top + side)
        d = ((Math.round(d) % P) + P) % P
        if (d < top) return [r + d, 0]
        d -= top
        if (d < side) return [W - 1, r + d]
        d -= side
        if (d < top) return [W - 1 - r - d, H - 1]
        d -= top
        return [0, H - 1 - r - d]
    }
    readonly property real perimeter: 2 * (Math.round(width / p) + Math.round(height / p))
    Repeater {
        model: ring.glinting && World.pixel ? 6 : 0
        Rectangle {
            required property int index
            readonly property var at: ring.pointAt(ring.pos * ring.perimeter - index * 2)
            x: at[0] * ring.p
            y: at[1] * ring.p
            width: ring.p
            height: ring.p
            color: index === 0 ? "#FFFFFF" : (World.palette.bloom || "#FFFFFF")
            opacity: 1 - index / 6
        }
    }

    // Classic: the smooth spark.
    Canvas {
        id: spark
        objectName: "focusSpark"
        anchors.fill: parent
        visible: ring.glinting && World.classic
        renderStrategy: Canvas.Immediate
        antialiasing: true
        onVisibleChanged: if (visible) requestPaint()
        // A point `d` along the ring's centre line (clockwise from the top-left
        // corner's end), for a rounded rectangle x, y, W, H with radius r.
        function pointAt(d, x, y, W, H, r) {
            const sx = W - 2 * r, sy = H - 2 * r, q = Math.PI * r / 2
            const legs = [sx, q, sy, q, sx, q, sy, q]
            const P = legs.reduce((a, b) => a + b, 0)
            d = ((d % P) + P) % P
            let i = 0
            while (i < 7 && d > legs[i]) { d -= legs[i]; i++ }
            const arc = (cx, cy, a0) => { const a = a0 + d / r; return [cx + r * Math.cos(a), cy + r * Math.sin(a)] }
            switch (i) {
            case 0: return [x + r + d, y]
            case 1: return arc(x + W - r, y + r, -Math.PI / 2)
            case 2: return [x + W, y + r + d]
            case 3: return arc(x + W - r, y + H - r, 0)
            case 4: return [x + W - r - d, y + H]
            case 5: return arc(x + r, y + H - r, Math.PI / 2)
            case 6: return [x, y + H - r - d]
            default: return arc(x + r, y + r, Math.PI)
            }
        }
        onPaint: {
            const ctx = getContext("2d")
            ctx.reset()
            if (!visible)
                return
            const w = ring.thickness
            const x = w / 2, y = w / 2, W = width - w, H = height - w
            const r = Math.max(0.5, Math.min(ring.radius - w / 2, W / 2, H / 2))
            const P = 2 * (W + H) - (8 - 2 * Math.PI) * r
            const head = ring.pos * P
            const tail = P * 0.22
            ctx.lineCap = "butt"
            ctx.strokeStyle = World.palette.bloom || "#FFFFFF"
            ctx.lineWidth = w * 1.7
            // The tail: short strokes fading in towards the head.
            const n = 24
            for (let k = 0; k < n; k++) {
                const a = pointAt(head - tail + tail * k / n, x, y, W, H, r)
                const b = pointAt(head - tail + tail * (k + 1) / n, x, y, W, H, r)
                ctx.globalAlpha = Math.pow((k + 1) / n, 1.4) * 0.9
                ctx.beginPath(); ctx.moveTo(a[0], a[1]); ctx.lineTo(b[0], b[1]); ctx.stroke()
            }
            // The spark: a bright dot with a soft halo.
            const p = pointAt(head, x, y, W, H, r)
            ctx.globalAlpha = 0.35
            ctx.fillStyle = World.palette.bloom || "#FFFFFF"
            ctx.beginPath(); ctx.arc(p[0], p[1], w * 4, 0, 2 * Math.PI); ctx.fill()
            ctx.globalAlpha = 1
            ctx.fillStyle = "#FFFFFF"
            ctx.beginPath(); ctx.arc(p[0], p[1], w * 1.6, 0, 2 * Math.PI); ctx.fill()
        }
    }
}
