// Forest world's corner scene, in pixel art (assets/pixel/scene-camp*,
// tools/pixelart/scenes.py): mama and the cub peek out of a lantern-lit tent
// between pines; the lantern flickers and fireflies drift, on World's
// heartbeat. Laid out in art pixels (110×82) at World.px each.
// Local weather (SceneWeather.qml, docs/THEMES.md → Weather in the corner
// scene): rain drips from the lantern arm and the tent into puddles, snow
// lies on the tent's ridge, the pines and the lantern arm, fog fades the
// camp, thunder startles mama and the cub, a clear night has stars. The
// fireflies stay in out of rain, snow and fog.

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
    SceneWeather {
        side: "back"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { puddles: [ { x: 2, y: 80, w: 8 }, { x: 66, y: 80, w: 10 }, { x: 99, y: 80, w: 8 } ] },
            night: { stars: [ { x: 6, y: 4 }, { x: 30, y: 8 }, { x: 44, y: 3 }, { x: 66, y: 6 }, { x: 78, y: 14 }, { x: 104, y: 4 } ] }
        })
    }
    // In fog the tent and the bears in it fade as one picture (a layer, only
    // then), so the bears never show through the tent.
    Item {
        anchors.fill: parent
        opacity: 1 - weatherFront.fade
        layer.enabled: weatherFront.fade > 0
        PixelSprite { name: "scene-camp-back" }
        Item {
            anchors.fill: parent
            anchors.topMargin: weatherFront.startled ? -2 * root.p : 0
            clip: true
            BearHead { alive: root.alive; kind: "mama"; width: 25 * root.p; x: 30 * root.p; y: 81 * root.p - height }
            BearHead { alive: root.alive; kind: "cub"; width: 21 * root.p; x: 46 * root.p; y: 83 * root.p - height }
        }
        PixelSprite { name: "scene-camp" }
    }
    Ornament { name: "paw-grip"; width: 9 * root.p; height: 5 * root.p; x: 37 * root.p; y: 77 * root.p }
    Ornament { name: "paw-grip"; width: 9 * root.p; height: 5 * root.p; x: 53 * root.p; y: 77 * root.p }
    PixelSprite { name: "scene-camp-lantern"; frames: 4; frame: Math.floor(root.t * 3); x: 78 * root.p; y: 35 * root.p }
    // Fireflies (they stay in out of the rain, snow and fog).
    Repeater {
        model: weatherFront.look === "" || weatherFront.look === "night" ? 3 : 0
        Rectangle {
            required property int index
            width: root.p; height: root.p
            color: "#F4E08A"
            x: Math.round(14 + index * 30 + 3 * root.wave(4.4 + index, index * 0.3)) * root.p
            y: Math.round(10 + (index % 2) * 10 + 4 * root.wave(5.1 + index * 0.7, index * 0.2)) * root.p
            opacity: root.wave(2.6 + index * 0.4, index * 0.33) > -0.2 ? 1 : 0.25
        }
    }
    SceneWeather {
        id: weatherFront
        side: "front"
        unit: root.p
        anchors.fill: parent
        reactions: ({
            wet: { drips: [ { x: 92, y: 37 }, { x: 16, y: 74 } ] },
            storm: { startle: [ { x: 38, y: 49 }, { x: 60, y: 54 } ] },
            snow: { caps: [ { x: 49, y: 18, w: 3 }, { x: 48, y: 20, w: 3 }, { x: 50, y: 20, w: 3 }, { x: 47, y: 22, w: 3 },
                            { x: 51, y: 22, w: 3 }, { x: 46, y: 24, w: 3 }, { x: 52, y: 24, w: 3 }, { x: 45, y: 26, w: 3 },
                            { x: 53, y: 26, w: 3 }, { x: 44, y: 28, w: 3 }, { x: 54, y: 28, w: 3 }, { x: 16, y: 27, w: 5 },
                            { x: 86, y: 33, w: 12 } ] },
            fog: { fade: 0.4, mist: [ { y: 46, h: 3 }, { y: 68, h: 4 } ] }
        })
    }
}
