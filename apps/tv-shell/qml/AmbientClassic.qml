// Ambient's particles in the Classic art style (World.classic; ADR 0006):
// round motes with soft halos, leaves that turn as they fall, round stars that
// twinkle smoothly, a gradient comet, embers with halos, and rain as soft
// antialiased streaks slanting with the wind. The same kinds, counts and
// colours as the pixel particles in Ambient.qml, and every particle is a
// function of the host's clock `t` (World's heartbeat), so nothing here runs
// on its own. Cheap: rectangles, position/rotation/opacity only.

import QtQuick
import BearDen

Item {
    id: root
    required property Item host
    readonly property string kind: visible ? host.kind : ""
    readonly property int count: host.count
    function color(i, fallback) { return host.color(i, fallback) }
    function rnd(i, k) { return host.rnd(i, k) }
    function wave(period, phase) { return host.wave(period, phase) }
    function cycle(period, phase) { return host.cycle(period, phase) }

    // Fireflies: every third mote glows (colour 0); the rest are dust (colour 1).
    Repeater {
        model: root.kind === "fireflies" ? root.count : 0
        Rectangle {
            id: mote
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            readonly property bool firefly: index % 3 === 0
            readonly property color tint: firefly ? root.color(0, ["#E3B35C"]) : root.color(1, ["#E3B35C", "#EDE3D1"])
            width: (firefly ? 7 : 4) * Theme.scale
            height: width
            radius: width / 2
            antialiasing: true
            color: tint
            x: r1 * root.width + 30 * Theme.scale * root.wave(14 + r2 * 16, r3)
            y: root.height * (0.08 + 0.6 * r2) - 30 * Theme.scale * (1 + root.wave(18 + r1 * 18, r2))
            opacity: (firefly ? 0.85 : 0.35) * Math.pow(Math.max(0, root.wave(7 + r3 * 9, r1)), 1.5)
            Rectangle {
                visible: mote.firefly
                anchors.centerIn: parent
                width: parent.width * 4; height: width; radius: width / 2
                antialiasing: true
                color: Theme.alpha(mote.tint, 0.07)
            }
        }
    }

    // Leaves and snow: something falling, drifting side to side.
    Repeater {
        model: root.kind === "leaves" || root.kind === "snow" ? root.count : 0
        Rectangle {
            required property int index
            readonly property bool snow: root.kind === "snow"
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            width: (snow ? 3 + r2 * 4 : 10 + r2 * 8) * Theme.scale
            height: snow ? width : width * 0.46
            radius: height / 2
            antialiasing: true
            color: snow ? root.color(index, ["#F4FAFF"]) : root.color(index, ["#C9A65A", "#7FAE72", "#5E8F5E"])
            opacity: snow ? 0.5 + r3 * 0.4 : 0.75
            x: r1 * root.width + (snow ? 20 : 40) * Theme.scale * root.wave(6 + r3 * 4, r2)
            y: -20 * Theme.scale + root.cycle((snow ? 20 : 16) + r2 * 12, r3) * (root.height + 40 * Theme.scale)
            rotation: snow ? 0 : 60 * root.wave(5.4 + r1 * 3, r3)
        }
    }

    // Rain: soft streaks, fading in from the top, slanted by a little wind.
    Repeater {
        model: root.kind === "rain" ? root.count : 0
        Rectangle {
            id: drop
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            readonly property real p: root.cycle(1.1 + r2 * 0.7, r3)
            readonly property color tint: root.color(index, ["#B8D0EE", "#E2ECF8"])
            width: 2 * Theme.scale
            height: (r2 > 0.5 ? 30 : 20) * Theme.scale
            radius: width / 2
            antialiasing: true
            gradient: Gradient {
                GradientStop { position: 0; color: "transparent" }
                GradientStop { position: 1; color: drop.tint }
            }
            opacity: 0.45 + r1 * 0.3
            // The fall moves 60 px left per screen height: lean the streak to match.
            rotation: Math.atan2(60 * Theme.scale, root.height + height) * 180 / Math.PI
            x: r1 * (root.width + 60 * Theme.scale) - 60 * Theme.scale * p
            y: -height + p * (root.height + height)
        }
    }

    // Stars twinkling in the upper sky, and now and then a shooting star.
    Repeater {
        model: root.kind === "stars" ? root.count : 0
        Rectangle {
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            width: (2 + r3 * 3) * Theme.scale
            height: width
            radius: width / 2
            antialiasing: true
            color: index % 4 === 0 ? root.color(1, ["#E8EEFF", "#F4E3A1"]) : root.color(0, ["#E8EEFF"])
            x: r1 * root.width
            y: root.height * 0.45 * r2
            opacity: 0.6 + 0.35 * root.wave(2 + r1 * 3.4, r3)
        }
    }
    Rectangle {
        id: comet
        objectName: "classicComet"
        visible: root.kind === "stars" && opacity > 0
        width: 160 * Theme.scale
        height: 3 * Theme.scale
        radius: height / 2
        antialiasing: true
        rotation: -18
        opacity: 0
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0; color: "#FFFFFF" }
            GradientStop { position: 1; color: "transparent" }
        }
        ParallelAnimation {
            id: fly
            NumberAnimation { target: comet; property: "x"; from: root.width * 0.85; to: root.width * 0.35; duration: 1300; easing.type: Easing.InQuad }
            NumberAnimation { target: comet; property: "y"; from: root.height * 0.05; to: root.height * 0.2; duration: 1300; easing.type: Easing.InQuad }
            SequentialAnimation {
                NumberAnimation { target: comet; property: "opacity"; to: 0.9; duration: 250 }
                PauseAnimation { duration: 650 }
                NumberAnimation { target: comet; property: "opacity"; to: 0; duration: 400 }
            }
        }
        Timer {
            interval: 9000
            repeat: true
            running: root.kind === "stars" && root.host.running
            onTriggered: { fly.restart(); interval = 14000 + Math.random() * 14000 }
        }
    }

    // Embers rising from below, flickering as they go.
    Repeater {
        model: root.kind === "embers" ? root.count : 0
        Rectangle {
            id: ember
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            readonly property real p: root.cycle(9 + r2 * 8, r3)
            width: (3 + r2 * 3) * Theme.scale
            height: width
            radius: width / 2
            antialiasing: true
            color: index % 3 === 0 ? root.color(0, ["#FFD490"]) : root.color(1, ["#FFD490", "#FF8A3D"])
            x: r1 * root.width + 25 * Theme.scale * root.wave(5.4 + r3 * 4, r1)
            y: root.height + 10 * Theme.scale - (1 - (1 - p) * (1 - p)) * root.height * (0.75 - 0.3 * r3)
            opacity: Math.max(0, Math.min(1, p * 8) * (1 - p) * (0.8 + 0.2 * root.wave(1.3, r2)))
            Rectangle {
                anchors.centerIn: parent
                width: parent.width * 3.5; height: width; radius: width / 2
                antialiasing: true
                color: Theme.alpha(ember.color, 0.14)
            }
        }
    }
}
