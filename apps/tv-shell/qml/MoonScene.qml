// Midnight world's corner scene, in pixel art (assets/pixel/scene-moon*,
// tools/pixelart/scenes.py): a mobile of a crescent moon and stars on threads,
// the cub asleep in the crescent, "z"s drifting up. The mobile rises and
// settles by a pixel as it breathes (pixel art never rotates). On World's
// heartbeat; still while Bear Den rests. Laid out in art pixels (110×82).
// Local weather (SceneWeather.qml, docs/THEMES.md → Weather in the corner
// scene): rain drips from the crescent and the stars, snow lies on the
// crescent's rim, fog fades the mobile, thunder startles the cub, a clear
// night adds stars.

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
        SceneWeather {
            side: "back"
            unit: root.p
            anchors.fill: parent
            reactions: ({
                night: { stars: [ { x: 20, y: 10 }, { x: 32, y: 3 }, { x: 70, y: 8 }, { x: 84, y: 20 }, { x: 104, y: 6 }, { x: 24, y: 46 } ] }
            })
        }
        PixelSprite { name: "scene-moon"; opacity: 1 - weatherFront.fade }
        BearPuppet {
            kind: "cub"; size: 35 * root.p; facing: -1; sitting: true; asleep: true
            carry: ""
            x: 52 * root.p; y: 73 * root.p - height - (weatherFront.startled ? 2 * root.p : 0)
        }
        SceneWeather {
            id: weatherFront
            side: "front"
            unit: root.p
            anchors.fill: parent
            reactions: ({
                wet: { drips: [ { x: 50, y: 76 }, { x: 62, y: 76 }, { x: 14, y: 37 }, { x: 98, y: 45 } ] },
                storm: { startle: [ { x: 64, y: 30 } ] },
                snow: { caps: [ { x: 33, y: 33, w: 3 }, { x: 36, y: 29, w: 4 }, { x: 40, y: 26, w: 4 }, { x: 44, y: 24, w: 4 }, { x: 48, y: 23, w: 4 } ] },
                fog: { fade: 0.4, mist: [ { y: 44, h: 3 }, { y: 66, h: 4 } ] }
            })
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
