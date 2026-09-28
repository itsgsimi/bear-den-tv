// Campfire world's corner scene: dad and the cub sit on logs either side of a
// crackling campfire, toasting marshmallows. Whole bears (BearPuppet, seated,
// reaching out with their sticks); the flame flickers, its glow breathes and
// sparks rise, all on World's heartbeat. Still while Bear Den rests.
// This is the Classic art style's version (World.classic; ADR 0006): smooth
// SVG art laid out in 300 design units across `size`. CampfireScene.qml is the
// pixel one; HomeScreen picks between them.

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

    BrandBackdrop {
        x: 20 * root.u; y: 30 * root.u
        width: 260 * root.u; height: 200 * root.u
        topColor: "transparent"; bottomColor: "transparent"
        glow: "#FF7A2F"; glowX: 0.5; glowY: 0.62; glowRadius: 0.45; glowStrength: 0.34
        radius: 0
        opacity: 0.89 + 0.11 * root.wave(1.16, 0)
    }

    // A sitting log: bark, with the cut end showing its rings.
    component SitLog: Item {
        property real u: 1
        width: 74 * u; height: 20 * u
        Rectangle { anchors.fill: parent; radius: height / 2; color: "#6B4A30"
            Rectangle { x: 8 * parent.parent.u; y: 5 * parent.parent.u; width: parent.width - 30 * parent.parent.u; height: 2.5 * parent.parent.u; radius: height / 2; color: "#825B3B" }
        }
        Rectangle {
            width: parent.height; height: parent.height; radius: width / 2
            x: parent.width - width; color: "#C89A68"; border.color: "#6B4A30"; border.width: 2 * parent.u
            Rectangle { anchors.centerIn: parent; width: parent.width * 0.45; height: width; radius: width / 2; color: "transparent"; border.color: "#A87B50"; border.width: 1.5 * parent.parent.u }
        }
    }
    SitLog { u: root.u; x: -30 * root.u; y: 196 * root.u }
    SitLog { u: root.u; x: 232 * root.u; y: 200 * root.u; transform: Scale { xScale: -1; origin.x: 37 * root.u } }

    // Dad on the left, the cub on the right, both facing the fire.
    BearPuppet {
        kind: "dad"; size: 96 * root.u; facing: 1; sitting: true; stickLength: 70
        carry: "stick"; hat: ""
        reach: 0.82 + 0.06 * root.wave(3.1, 0.2)
        blink: root.wave(4.7, 0) > 0.985
        x: -18 * root.u; y: 200 * root.u - height
    }
    BearPuppet {
        kind: "cub"; size: 74 * root.u; facing: -1; sitting: true; stickLength: 76
        carry: "stick"; hat: ""
        reach: 0.86 + 0.06 * root.wave(2.6, 0.6)
        blink: root.wave(5.3, 0.4) > 0.985
        x: 252 * root.u; y: 204 * root.u - height
    }

    Ornament { name: "logs"; width: 140 * root.u; height: 79 * root.u; x: 80 * root.u; y: 146 * root.u }
    // The flame: two layers flickering out of step.
    Ornament {
        name: "flame"
        width: 92 * root.u; height: width
        x: 104 * root.u; y: 76 * root.u
        transformOrigin: Item.Bottom
        scale: 1.01 + 0.06 * root.wave(0.84, 0)
        rotation: 3 * root.wave(1.5, 0.3)
    }
    Ornament {
        name: "flame"
        width: 54 * root.u; height: width
        x: 134 * root.u; y: 108 * root.u
        opacity: 0.85
        transformOrigin: Item.Bottom
        scale: 1 + 0.1 * root.wave(0.72, 0.5)
    }
    // Sparks rising and fading.
    Repeater {
        model: 5
        Rectangle {
            required property int index
            readonly property real p: { const x = root.t / (1.8 + index * 0.15) + index * 0.23; return x - Math.floor(x) }
            width: 4 * root.u; height: width; radius: width / 2
            color: "#FFD490"
            x: (136 + (index % 3) * 12 - 8) * root.u
            y: (96 - 86 * (1 - (1 - p) * (1 - p))) * root.u
            opacity: 0.95 * (1 - p)
        }
    }
}
