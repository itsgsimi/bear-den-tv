// Forest world's corner scene: mama and the cub peek out of a tent between the
// pines, a lantern glows on its post and a few fireflies drift by. They blink
// now and then; the lantern flickers. Still while Bear Den rests.
// This is the Classic art style's version (World.classic; ADR 0006): smooth
// SVG art laid out in 300 design units across `size`. CampScene.qml is the
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

    Ornament { name: "pine"; width: 46 * root.u; height: 58 * root.u; x: 196 * root.u; y: 50 * root.u }
    // The tent's lit interior, seen through the open door.
    Canvas {
        x: 94 * root.u; y: 100 * root.u
        width: 112 * root.u; height: 114 * root.u
        antialiasing: true
        onWidthChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d"), w = width, h = height
            ctx.reset()
            ctx.beginPath()
            ctx.moveTo(w * 0.5, h * 0.02); ctx.lineTo(w, h); ctx.lineTo(0, h); ctx.closePath()
            ctx.fillStyle = "#2A2118"
            ctx.fill()
            const g = ctx.createRadialGradient(w * 0.5, h * 0.8, 0, w * 0.5, h * 0.8, h * 0.7)
            g.addColorStop(0, "rgba(255, 196, 110, 0.55)")
            g.addColorStop(1, "rgba(255, 196, 110, 0)")
            ctx.fillStyle = g
            ctx.fill()
        }
    }
    BearHead { alive: root.alive; kind: "mama"; width: 56 * root.u; x: 112 * root.u; y: 150 * root.u }
    BearHead { alive: root.alive; kind: "cub"; width: 46 * root.u; x: 148 * root.u; y: 168 * root.u }
    Ornament { name: "tent"; width: 220 * root.u; height: 158.4 * root.u; x: 40 * root.u; y: 63 * root.u }
    Ornament { name: "paw-grip"; width: 16 * root.u; height: 11 * root.u; x: 150 * root.u; y: 204 * root.u }
    Ornament { name: "paw-grip"; width: 16 * root.u; height: 11 * root.u; x: 178 * root.u; y: 204 * root.u }
    Ornament { name: "pine"; width: 64 * root.u; height: 80 * root.u; x: -8 * root.u; y: 140 * root.u }
    Ornament { name: "pine"; width: 50 * root.u; height: 62 * root.u; x: 250 * root.u; y: 160 * root.u }

    // Lantern on its post.
    Rectangle { x: 270 * root.u; y: 112 * root.u; width: 4 * root.u; height: 110 * root.u; radius: width / 2; color: "#6B4A32" }
    Rectangle { x: 244 * root.u; y: 110 * root.u; width: 30 * root.u; height: 4 * root.u; radius: height / 2; color: "#6B4A32" }
    BrandBackdrop {
        id: lanternGlow
        x: 206 * root.u; y: 90 * root.u
        width: 100 * root.u; height: 100 * root.u
        topColor: "transparent"; bottomColor: "transparent"
        glow: "#FFC46E"; glowX: 0.5; glowY: 0.5; glowRadius: 0.5; glowStrength: 0.5
        radius: 0
        SequentialAnimation on opacity {
            running: root.alive
            loops: Animation.Infinite
            NumberAnimation { to: 0.75; duration: 900; easing.type: Easing.InOutSine }
            NumberAnimation { to: 1; duration: 700; easing.type: Easing.InOutSine }
        }
    }
    Rectangle { x: 249 * root.u; y: 114 * root.u; width: 1.5 * root.u; height: 12 * root.u; color: "#3B424A" }
    Ornament { name: "lantern"; width: 34 * root.u; height: 34 * root.u; x: 233 * root.u; y: 122 * root.u }

    // Fireflies drifting by the tent.
    Repeater {
        model: 3
        Rectangle {
            required property int index
            width: 5 * root.u; height: width; radius: width / 2
            color: "#F4E08A"
            x: (40 + index * 70) * root.u
            y: (40 + (index % 2) * 30) * root.u
            opacity: 0.8
            Rectangle { anchors.centerIn: parent; width: parent.width * 4; height: width; radius: width / 2; color: "#33F4E08A" }
            SequentialAnimation on y {
                running: root.alive
                loops: Animation.Infinite
                NumberAnimation { to: (28 + (index % 2) * 30) * root.u; duration: 2200 + index * 500; easing.type: Easing.InOutSine }
                NumberAnimation { to: (46 + (index % 2) * 30) * root.u; duration: 2400 + index * 400; easing.type: Easing.InOutSine }
            }
            SequentialAnimation on opacity {
                running: root.alive
                loops: Animation.Infinite
                NumberAnimation { to: 0.15; duration: 1400 + index * 300; easing.type: Easing.InOutSine }
                NumberAnimation { to: 0.9; duration: 1200 + index * 200; easing.type: Easing.InOutSine }
            }
        }
    }
}
