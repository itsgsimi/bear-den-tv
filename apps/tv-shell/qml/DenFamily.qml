// Den world's corner scene, in pixel art (assets/pixel/scene-den*,
// tools/pixelart/scenes.py): the bear family (dad, mama, the cub) peeks out
// of their lantern-lit nook, paws on the rim. They blink now and then and the
// cub waves every 11–18 s; from 22:00 to 06:00 they sleep and "z"s drift up.
// Shown only while a single rail leaves room. Laid out in art pixels (110×82)
// at World.px each; all motion stops while resting, behind apps and with
// reduced motion.
// Local weather (SceneWeather.qml, docs/THEMES.md → Weather in the corner
// scene): rain drips off the arch into puddles, snow lies along its top, fog
// fades the rock and drifts mist past, thunder startles dad and mama, a clear
// night has stars.

import QtQuick
import BearDen

Item {
    id: root
    readonly property int p: World.px
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && Session.target.kind === "shell"
    property bool night: false
    function updateNight() { const h = new Date().getHours(); night = h >= 22 || h < 6 }
    Component.onCompleted: updateNight()
    Timer { interval: 60000; repeat: true; running: root.visible; onTriggered: root.updateNight() }
    width: 110 * p
    height: 82 * p

    // The hollow (behind), the family, then the rock arch in front.
    PixelSprite { name: "scene-den-back"; opacity: 1 - weatherFront.fade }
    SceneWeather {
        side: "back"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { puddles: [ { x: 1, y: 80, w: 8 }, { x: 99, y: 80, w: 9 } ] },
            night: { stars: [ { x: 4, y: 6 }, { x: 14, y: 16 }, { x: 30, y: 4 }, { x: 78, y: 4 }, { x: 96, y: 9 }, { x: 104, y: 20 } ] }
        })
    }
    Item {
        anchors.fill: parent
        anchors.topMargin: -weatherFront.hop
        clip: true
        BearHead { alive: root.alive; night: root.night; kind: "dad"; width: 26 * root.p; x: 20 * root.p; y: 83 * root.p - height }
        BearHead { alive: root.alive; night: root.night; kind: "mama"; width: 25 * root.p; x: 62 * root.p; y: 84 * root.p - height }
        BearHead { alive: root.alive; night: root.night; kind: "cub"; width: 21 * root.p; x: 43 * root.p; y: 86 * root.p - height }
    }
    PixelSprite { name: "scene-den"; opacity: 1 - weatherFront.fade }
    Repeater {
        model: [ 24, 38, 60, 76 ]
        Ornament {
            required property var modelData
            name: "paw-grip"
            width: 9 * root.p; height: 5 * root.p
            x: modelData * root.p; y: 77 * root.p
        }
    }
    // The cub's wave: a paw pops up beside it and rocks a pixel either way.
    Ornament {
        id: paw
        name: "paw"
        width: 7 * root.p; height: 7 * root.p
        property int step: -1
        visible: step >= 0
        x: (58 + (step % 2 ? 1 : -1)) * root.p
        y: 62 * root.p
        Timer {
            id: waving
            interval: 180
            repeat: true
            onTriggered: { paw.step += 1; if (paw.step > 6) { paw.step = -1; stop() } }
        }
        Timer {
            interval: 4000
            repeat: true
            running: root.alive && !root.night
            onTriggered: { paw.step = 0; waving.restart(); interval = 11000 + Math.random() * 7000 }
        }
    }
    SceneWeather {
        id: weatherFront
        readonly property int hop: startled ? 2 * root.p : 0
        side: "front"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { drips: [ { x: 5, y: 56 }, { x: 104, y: 56 } ] },
            storm: { startle: [ { x: 30, y: 49 }, { x: 72, y: 51 } ] },
            snow: { caps: [ { x: 13, y: 33, w: 3 }, { x: 16, y: 29, w: 3 }, { x: 19, y: 25, w: 3 }, { x: 22, y: 23, w: 3 },
                            { x: 25, y: 20, w: 3 }, { x: 28, y: 18, w: 3 }, { x: 30, y: 15, w: 5 }, { x: 35, y: 13, w: 5 },
                            { x: 40, y: 11, w: 5 }, { x: 45, y: 10, w: 5 }, { x: 50, y: 10, w: 5 }, { x: 55, y: 10, w: 5 },
                            { x: 60, y: 10, w: 5 }, { x: 65, y: 11, w: 5 }, { x: 70, y: 13, w: 5 }, { x: 75, y: 15, w: 5 },
                            { x: 80, y: 18, w: 3 }, { x: 83, y: 20, w: 3 }, { x: 86, y: 23, w: 3 }, { x: 89, y: 25, w: 3 },
                            { x: 92, y: 29, w: 3 }, { x: 95, y: 33, w: 3 } ] },
            fog: { fade: 0.4, mist: [ { y: 50, h: 3 }, { y: 70, h: 4 } ] }
        })
    }
    Repeater {
        model: root.night ? 3 : 0
        PixelSprite {
            required property int index
            name: "scene-moon-z"
            unit: root.p * (index === 2 ? 2 : 1)
            x: (86 + index * 6) * root.p
            y: (30 - index * 8) * root.p
            opacity: 0.8 - index * 0.2
        }
    }
}
