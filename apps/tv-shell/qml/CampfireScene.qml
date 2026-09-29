// Campfire world's corner scene, in pixel art (assets/pixel/scene-campfire*,
// tools/pixelart/scenes.py): dad and the cub sit on logs either side of a
// crackling campfire, toasting marshmallows. The fire runs through its frames
// and sparks rise on World's heartbeat; still while Bear Den rests. Laid out in
// art pixels (110×82) at World.px each.
// Local weather (SceneWeather.qml, docs/THEMES.md → Weather in the corner
// scene): in the rain a tarp goes up over the fire, which smoulders (dimmer,
// slower, one spark) while puddles glint; snow lies on the log ends and the
// ground; fog fades the props and drifts mist past; thunder startles both bears; a clear night has
// stars.

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
    readonly property bool smoulder: weatherBack.wet
    readonly property int hop: weatherFront.startled ? 2 * p : 0

    PixelSprite { name: "scene-campfire"; opacity: 1 - weatherFront.fade }
    PixelSprite {
        objectName: "campfireFlame"
        name: "scene-campfire-fire"; frames: 6; frame: Math.floor(root.t * (root.smoulder ? 4 : 8))
        x: 31 * root.p; y: 32 * root.p
        opacity: root.smoulder ? 0.55 : 1
    }
    SceneWeather {
        id: weatherBack
        side: "back"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { shelter: { x: 30, y: 23 }, puddles: [ { x: 8, y: 80, w: 10 }, { x: 48, y: 80, w: 12 }, { x: 88, y: 80, w: 9 } ] },
            night: { stars: [ { x: 8, y: 6 }, { x: 24, y: 14 }, { x: 47, y: 4 }, { x: 70, y: 12 }, { x: 90, y: 3 }, { x: 103, y: 16 } ] }
        })
    }
    BearPuppet {
        kind: "dad"; size: 46 * root.p; facing: 1; sitting: true; stickLength: 44
        carry: "stick"; hat: ""
        reach: root.wave(3.1, 0.2) > -0.7 ? 1 : 0
        blink: root.wave(4.7, 0) > 0.985
        x: -6 * root.p; y: 73 * root.p - height - root.hop
    }
    BearPuppet {
        kind: "cub"; size: 35 * root.p; facing: -1; sitting: true; stickLength: 52
        carry: "stick"; hat: ""
        reach: root.wave(2.6, 0.6) > -0.7 ? 1 : 0
        blink: root.wave(5.3, 0.4) > 0.985
        x: 80 * root.p; y: 74 * root.p - height - root.hop
    }
    // Sparks: single pixels rising from the flame and fading (one, low, in the rain).
    Repeater {
        model: root.smoulder ? 1 : 5
        Rectangle {
            required property int index
            readonly property real q: { const x = root.t / (1.8 + index * 0.15) + index * 0.23; return x - Math.floor(x) }
            width: root.p; height: root.p
            color: index % 2 ? "#FFD490" : "#FFB45E"
            x: Math.round(52 + (index % 3) * 3 + 2 * Math.sin(q * 6 + index)) * root.p
            y: Math.round(40 - 34 * (1 - (1 - q) * (1 - q))) * root.p
            opacity: q < 0.8 ? 1 : 0.5
            visible: root.alive && (!root.smoulder || q < 0.35)
        }
    }
    SceneWeather {
        id: weatherFront
        side: "front"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { drips: [ { x: 42, y: 31 }, { x: 55, y: 32 }, { x: 68, y: 31 } ] },
            storm: { startle: [ { x: 10, y: 28 }, { x: 92, y: 38 } ] },
            snow: { caps: [ { x: 25, y: 69, w: 7 }, { x: 78, y: 70, w: 7 }, { x: 2, y: 79, w: 16 }, { x: 36, y: 80, w: 8 }, { x: 72, y: 80, w: 12 }, { x: 94, y: 79, w: 14 } ] },
            fog: { fade: 0.45, mist: [ { y: 58, h: 3 }, { y: 72, h: 4 } ] }
        })
    }
}
