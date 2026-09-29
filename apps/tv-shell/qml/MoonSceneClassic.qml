// Midnight world's corner scene: a mobile hangs in the corner — the cub fast
// asleep on a crescent moon, stars dangling on threads — swaying gently while
// "z"s drift up and the stars twinkle. The cub is a whole bear (BearPuppet,
// seated, asleep) curled in the crescent. Everything moves on World's
// heartbeat. Still while Bear Den rests.
// This is the Classic art style's version (World.classic; ADR 0006): smooth
// SVG art laid out in 300 design units across `size`. MoonScene.qml is the
// pixel one; HomeScreen picks between them.
// Local weather (SceneWeather.qml, docs/THEMES.md → Weather in the corner
// scene), as in the pixel scene: drips from the crescent and the stars in
// the rain, snow on the crescent's rim, fog fading the mobile, the cub
// startled by lightning, more stars on a clear night.

import QtQuick
import BearDen

Item {
    id: root
    property real size: 300 * Theme.scale
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && Session.target.kind === "shell"
    width: size
    height: size * 0.75
    readonly property real u: width / 300

    property real t: 0
    Connections {
        target: World
        enabled: root.alive
        function onBeat(dt) { root.t += dt }
    }
    function wave(period, phase) { return Math.sin(2 * Math.PI * (t / period + phase)) }
    function cycle(period, phase) { const x = t / period + phase; return x - Math.floor(x) }

    Item {
        id: mobile
        anchors.fill: parent
        transform: Rotation {
            id: swing
            origin.x: 160 * root.u
            origin.y: -40 * root.u
            angle: 2.5 * root.wave(10.4, 0)
        }
        SceneWeather {
            side: "back"
            unit: root.u
            anchors.fill: parent
            reactions: ({
                night: { stars: [ { x: 90, y: 14 }, { x: 214, y: 20 }, { x: 290, y: 150 }, { x: 236, y: 196 }, { x: 60, y: 196 }, { x: 110, y: 204 } ] }
            })
        }
        BrandBackdrop {
            x: 40 * root.u; y: 30 * root.u
            width: 220 * root.u; height: 190 * root.u
            topColor: "transparent"; bottomColor: "transparent"
            glow: "#BFD2FF"; glowX: 0.55; glowY: 0.5; glowRadius: 0.45; glowStrength: 0.28
            radius: 0
        }
        // Threads.
        Rectangle { x: 163 * root.u; y: -40 * root.u; width: 1.2 * root.u; height: 96 * root.u; color: "#9DB2DD"; opacity: 0.7 }
        Rectangle { x: 52 * root.u; y: -40 * root.u; width: 1 * root.u; height: 74 * root.u; color: "#9DB2DD"; opacity: 0.6 }
        Rectangle { x: 262 * root.u; y: -40 * root.u; width: 1 * root.u; height: 112 * root.u; color: "#9DB2DD"; opacity: 0.6 }
        Rectangle { x: 26 * root.u; y: -40 * root.u; width: 1 * root.u; height: 170 * root.u; color: "#9DB2DD"; opacity: 0.5 }
        Ornament { name: "moon"; width: 150 * root.u; height: 150 * root.u; x: 70 * root.u; y: 40 * root.u; opacity: 1 - weatherFront.fade }
        BearPuppet {
            kind: "cub"; size: 78 * root.u; facing: -1; sitting: true; asleep: true
            carry: ""
            x: 136 * root.u; y: 150 * root.u - height - (weatherFront.startled ? 5 * root.u : 0)
            transformOrigin: Item.Bottom
            rotation: -14 + 1.5 * root.wave(5.2, 0.3)   // leaning back into the moon, breathing
        }
        Repeater {
            model: [ { x: 38, y: 30, w: 30 }, { x: 249, y: 68, w: 26 }, { x: 16, y: 126, w: 20 } ]
            Ornament {
                required property var modelData
                required property int index
                name: "star"
                width: modelData.w * root.u; height: width
                x: modelData.x * root.u; y: modelData.y * root.u
                scale: 1.09 + 0.09 * root.wave(1.8, index * 0.39)
                opacity: 1 - weatherFront.fade
            }
        }
        SceneWeather {
            id: weatherFront
            side: "front"
            unit: root.u
            anchors.fill: parent
            reactions: ({
                wet: { drips: [ { x: 118, y: 176 }, { x: 150, y: 186 }, { x: 52, y: 58 }, { x: 262, y: 92 } ] },
                storm: { startle: [ { x: 160, y: 50 } ] },
                snow: { caps: [ { x: 98, y: 60, w: 11, a: -32 }, { x: 108, y: 54, w: 11, a: -18 }, { x: 118, y: 52, w: 11, a: -11 },
                                { x: 128, y: 50, w: 11, a: -6 }, { x: 138, y: 49, w: 11, a: 0 }, { x: 148, y: 50, w: 11, a: 13 } ] },
                fog: { fade: 0.4, mist: [ { y: 100, h: 10 }, { y: 160, h: 12 } ] }
            })
        }
    }
    // Sleep "z"s.
    Repeater {
        model: 3
        Text {
            required property int index
            text: "z"
            color: "#EDE3D1"
            font.family: Theme.fontFamily
            font.pixelSize: (16 + index * 6) * root.u
            font.weight: Font.Bold
            x: (206 + index * 14) * root.u
            readonly property real p: root.cycle(2.7, -index / 3)
            y: root.alive ? (72 - 64 * Math.sin(p * Math.PI / 2)) * root.u : 60 * root.u
            opacity: root.alive ? (p < 0.22 ? p / 0.22 * 0.9 : 0.9 * (1 - p) / 0.78) : 0.7 - index * 0.2
        }
    }
}
