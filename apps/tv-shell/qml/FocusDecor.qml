// The world's decoration around a focused card: two CornerDecors on the
// top-right and bottom-left corners (clear of the app icon at the top left and
// the rail heading above). It grows slowly when focus arrives, keeps adding
// blooms, stars or sparks the longer focus stays, and fades out (rather than
// shrinking) when focus leaves. It keeps living, more slowly, while Bear Den
// rests; it stops for the screensaver, behind apps, and with reduced motion
// (then it appears fully grown and still). Nothing in the Plain style.

import QtQuick
import BearDen

Item {
    id: root
    property bool shown: false
    property real cornerRadius: Theme.radius
    readonly property var decor: World.focus
    readonly property string style: decor.style || ""
    readonly property bool motion: !Theme.reducedMotion
    // Nothing ticks without a style to draw (Plain) or while hidden.
    readonly property bool alive: shown && motion && style.length > 0 && !Theme.screensaver && Session.target.kind === "shell"
    visible: style.length > 0 && fade > 0

    property real progress: 0
    property real fade: 0
    property int extras: 0
    NumberAnimation { id: grow; target: root; property: "progress"; to: 1; duration: 2300; easing.type: Easing.OutQuad }
    NumberAnimation { id: fadeIn; target: root; property: "fade"; to: 1; duration: 220 }
    NumberAnimation {
        id: fadeOut; target: root; property: "fade"; to: 0; duration: 340; easing.type: Easing.InQuad
        onFinished: if (!root.shown) { root.progress = 0; root.extras = 0 }
    }
    function start() {
        fadeOut.stop()
        if (!motion) { fade = 1; progress = 1; return }
        fadeIn.restart()
        grow.from = progress
        grow.restart()
    }
    onShownChanged: {
        if (shown) { start(); return }
        grow.stop(); fadeIn.stop()
        if (motion) fadeOut.restart()
        else { fade = 0; progress = 0; extras = 0 }
    }
    Component.onCompleted: if (shown) start()

    // More blooms (stars, sparks) while focus stays: quickly at first, then slower.
    Timer {
        interval: root.extras < 3 ? 3000 : 6000
        repeat: true
        running: root.alive && root.progress > 0.99 && root.extras < 6
        onTriggered: root.extras++
    }
    // Sway / twinkle / flicker on World's heartbeat (20 fps awake, 4 resting).
    property real phase: 0
    Connections {
        target: World
        enabled: root.alive && root.progress > 0
        function onBeat(dt) { root.phase = (root.phase + Math.PI * 2 * dt / 5.2) % (Math.PI * 20) }
    }

    CornerDecor {
        id: topRight
        style: root.style
        decor: root.decor
        reach: Math.min(root.width * 0.36, 140 * Theme.scale)
        sideReach: root.height * 0.46
        cornerRadius: root.cornerRadius
        variant: 0
        x: root.width - width + inset
        y: -inset
        transform: Scale { origin.x: topRight.width / 2; xScale: -1 }
        progress: root.progress
        phase: root.phase
        extras: root.extras
        visible: root.progress > 0
    }
    CornerDecor {
        id: bottomLeft
        style: root.style
        decor: root.decor
        reach: Math.min(root.width * 0.32, 124 * Theme.scale)
        sideReach: root.height * 0.4
        cornerRadius: root.cornerRadius
        variant: 1
        upright: true
        x: -inset
        y: root.height - height + inset
        transform: Scale { origin.y: bottomLeft.height / 2; yScale: -1 }
        progress: root.progress
        phase: root.phase + 1.7
        extras: root.extras
        visible: root.progress > 0
    }

    // Midnight: now and then a shooting star streaks over the card.
    PixelBox {
        id: shootingStar
        visible: root.style === "stars" && opacity > 0
        width: 90 * Theme.scale
        height: 2.5 * Theme.scale
        radius: height / 2
        rotation: -16
        opacity: 0
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0; color: "#FFFFFF" }
            GradientStop { position: 1; color: "transparent" }
        }
        ParallelAnimation {
            id: shoot
            NumberAnimation { target: shootingStar; property: "x"; from: root.width * 0.95; to: root.width * 0.15; duration: 900; easing.type: Easing.InQuad }
            NumberAnimation { target: shootingStar; property: "y"; from: -64 * Theme.scale; to: -20 * Theme.scale; duration: 900; easing.type: Easing.InQuad }
            SequentialAnimation {
                NumberAnimation { target: shootingStar; property: "opacity"; to: 1; duration: 200 }
                PauseAnimation { duration: 400 }
                NumberAnimation { target: shootingStar; property: "opacity"; to: 0; duration: 300 }
            }
        }
        Timer {
            interval: 5000
            repeat: true
            running: root.style === "stars" && root.alive && root.progress > 0.99 && !Theme.resting
            onTriggered: { shoot.restart(); interval = 6000 + Math.random() * 6000 }
        }
    }
}
