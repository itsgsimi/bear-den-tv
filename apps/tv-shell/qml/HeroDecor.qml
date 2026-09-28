// The world's decoration wrapped around the featured panel's top-left and
// bottom-right corners, just inside its edge (vines and blossoms, ferns and
// mushrooms, a constellation with the moon, or an ember trail with a flame).
// Fully grown; gently alive while Bear Den is awake. Nothing in Plain.

import QtQuick
import BearDen

Item {
    id: root
    property real radius: Theme.radius
    readonly property var decor: World.panel
    readonly property string style: decor.style || ""
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && Session.target.kind === "shell"
    visible: style.length > 0
    property real phase: 0
    Connections {
        target: World
        enabled: root.alive
        function onBeat(dt) { root.phase = (root.phase + Math.PI * 2 * dt / 6.4) % (Math.PI * 20) }
    }
    CornerDecor {
        style: root.style
        decor: root.decor
        size: 1.2
        gap: -18 * Theme.scale
        cornerRadius: root.radius
        reach: 250 * Theme.scale
        sideReach: 150 * Theme.scale
        x: -inset; y: -inset
        progress: 1
        extras: 6
        phase: root.phase
        variant: 0
    }
    CornerDecor {
        style: root.style
        decor: root.decor
        size: 1.2
        gap: -18 * Theme.scale
        cornerRadius: root.radius
        reach: 230 * Theme.scale
        sideReach: 140 * Theme.scale
        x: root.width - width + inset; y: root.height - height + inset
        rotation: 180
        upright: true
        progress: 1
        extras: 6
        phase: root.phase + 2.1
        variant: 1
    }
}
