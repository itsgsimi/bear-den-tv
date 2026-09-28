// Forest world's corner scene, in pixel art (assets/pixel/scene-camp*,
// tools/pixelart/scenes.py): mama and the cub peek out of a lantern-lit tent
// between pines; the lantern flickers and fireflies drift, on World's
// heartbeat. Laid out in art pixels (110×82) at World.px each.

import QtQuick
import BearDen

Item {
    id: root
    readonly property int p: World.px
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && Session.target.kind === "shell"
    width: 110 * p
    height: 82 * p

    property real t: 0
    Connections {
        target: World
        enabled: root.alive
        function onBeat(dt) { root.t += dt }
    }
    function wave(period, phase) { return Math.sin(2 * Math.PI * (t / period + phase)) }

    // Inside the tent (behind the bears), the bears, then the tent's front.
    PixelSprite { name: "scene-camp-back" }
    Item {
        anchors.fill: parent
        clip: true
        BearHead { alive: root.alive; kind: "mama"; width: 25 * root.p; x: 30 * root.p; y: 81 * root.p - height }
        BearHead { alive: root.alive; kind: "cub"; width: 21 * root.p; x: 46 * root.p; y: 83 * root.p - height }
    }
    PixelSprite { name: "scene-camp" }
    Ornament { name: "paw-grip"; width: 9 * root.p; height: 5 * root.p; x: 37 * root.p; y: 77 * root.p }
    Ornament { name: "paw-grip"; width: 9 * root.p; height: 5 * root.p; x: 53 * root.p; y: 77 * root.p }
    PixelSprite { name: "scene-camp-lantern"; frames: 4; frame: Math.floor(root.t * 3); x: 78 * root.p; y: 35 * root.p }
    // Fireflies.
    Repeater {
        model: 3
        Rectangle {
            required property int index
            width: root.p; height: root.p
            color: "#F4E08A"
            x: Math.round(14 + index * 30 + 3 * root.wave(4.4 + index, index * 0.3)) * root.p
            y: Math.round(10 + (index % 2) * 10 + 4 * root.wave(5.1 + index * 0.7, index * 0.2)) * root.p
            opacity: root.wave(2.6 + index * 0.4, index * 0.33) > -0.2 ? 1 : 0.25
        }
    }
}
