// Midnight world's corner scene, in pixel art (assets/pixel/scene-moon*,
// tools/pixelart/scenes.py): a mobile of a crescent moon and stars on threads,
// the cub asleep in the crescent, "z"s drifting up. The mobile rises and
// settles by a pixel as it breathes (pixel art never rotates). On World's
// heartbeat; still while Bear Den rests. Laid out in art pixels (110×82).

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
    function cycle(period, phase) { const x = t / period + phase; return x - Math.floor(x) }

    Item {
        anchors.fill: parent
        y: root.wave(5.2, 0) > 0 ? -root.p : 0
        PixelSprite { name: "scene-moon" }
        BearPuppet {
            kind: "cub"; size: 35 * root.p; facing: -1; sitting: true; asleep: true
            carry: ""
            x: 52 * root.p; y: 73 * root.p - height
        }
        // The hanging stars twinkle: a bright pixel on and off.
        Repeater {
            model: [ { x: 14, y: 33 }, { x: 98, y: 41 }, { x: 6, y: 61 } ]
            Rectangle {
                required property var modelData
                required property int index
                width: root.p; height: root.p
                color: "#FFFFFF"
                x: modelData.x * root.p; y: modelData.y * root.p
                visible: root.wave(1.8, index * 0.39) > 0.3
            }
        }
    }
    Repeater {
        model: 3
        PixelSprite {
            required property int index
            name: "scene-moon-z"
            unit: root.p * (index === 2 ? 2 : 1)
            readonly property real q: root.cycle(2.7, -index / 3)
            x: (82 + index * 5) * root.p
            y: root.alive ? Math.round(44 - 34 * Math.sin(q * Math.PI / 2)) * root.p : (40 - index * 8) * root.p
            opacity: root.alive ? (q < 0.2 || q > 0.85 ? 0.4 : 0.9) : 0.7 - index * 0.2
        }
    }
}
