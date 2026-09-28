// Campfire world's corner scene, in pixel art (assets/pixel/scene-campfire*,
// tools/pixelart/scenes.py): dad and the cub sit on logs either side of a
// crackling campfire, toasting marshmallows. The fire runs through its frames
// and sparks rise on World's heartbeat; still while Bear Den rests. Laid out in
// art pixels (110×82) at World.px each.

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

    PixelSprite { name: "scene-campfire" }
    PixelSprite { name: "scene-campfire-fire"; frames: 6; frame: Math.floor(root.t * 8); x: 31 * root.p; y: 32 * root.p }
    BearPuppet {
        kind: "dad"; size: 46 * root.p; facing: 1; sitting: true; stickLength: 44
        carry: "stick"; hat: ""
        reach: root.wave(3.1, 0.2) > -0.7 ? 1 : 0
        blink: root.wave(4.7, 0) > 0.985
        x: -6 * root.p; y: 73 * root.p - height
    }
    BearPuppet {
        kind: "cub"; size: 35 * root.p; facing: -1; sitting: true; stickLength: 52
        carry: "stick"; hat: ""
        reach: root.wave(2.6, 0.6) > -0.7 ? 1 : 0
        blink: root.wave(5.3, 0.4) > 0.985
        x: 80 * root.p; y: 74 * root.p - height
    }
    // Sparks: single pixels rising from the flame and fading.
    Repeater {
        model: 5
        Rectangle {
            required property int index
            readonly property real q: { const x = root.t / (1.8 + index * 0.15) + index * 0.23; return x - Math.floor(x) }
            width: root.p; height: root.p
            color: index % 2 ? "#FFD490" : "#FFB45E"
            x: Math.round(52 + (index % 3) * 3 + 2 * Math.sin(q * 6 + index)) * root.p
            y: Math.round(40 - 34 * (1 - (1 - q) * (1 - q))) * root.p
            opacity: q < 0.8 ? 1 : 0.5
            visible: root.alive
        }
    }
}
