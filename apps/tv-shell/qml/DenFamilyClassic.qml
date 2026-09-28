// The bear family at home in their den, keeping the quiet corner of the home
// screen company: dad, mama (with her daisy) and the cub peek out with their
// paws on the rim. Now and then someone blinks and the cub waves; at night
// (22:00–06:00) they sleep and "z"s drift up. Nothing moves while Bear Den
// rests, an app is in front, or motion is reduced.
// This is the Classic art style's version (World.classic; ADR 0006): smooth
// SVG art laid out in 300 design units across `size`. DenFamily.qml is the
// pixel one; HomeScreen picks between them.

import QtQuick
import BearDen

Item {
    id: root
    property real size: 300 * Theme.scale
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && Session.target.kind === "shell"
    property bool night: false
    function updateNight() { const h = new Date().getHours(); night = h >= 22 || h < 6 }
    Component.onCompleted: updateNight()
    Timer { interval: 60000; repeat: true; running: root.visible; onTriggered: root.updateNight() }
    width: size
    height: size * 0.75
    readonly property real u: width / 300          // design unit: this scene is laid out at 300 px wide

    Ornament { name: "den"; anchors.fill: parent }
    // A daisy and a sprig growing by the door.
    Ornament { name: "sprig"; width: 62 * root.u; height: 31 * root.u; x: -18 * root.u; y: root.height - 34 * root.u; rotation: -24 }
    Ornament { name: "daisy"; width: 26 * root.u; height: width; x: 8 * root.u; y: root.height - 40 * root.u }

    // Warm lantern light inside the den.
    BrandBackdrop {
        x: 50 * root.u; y: 70 * root.u
        width: 200 * root.u; height: 150 * root.u
        topColor: "transparent"; bottomColor: "transparent"
        glow: "#E3B35C"
        glowX: 0.5; glowY: 0.72; glowRadius: 0.5; glowStrength: 0.32
        radius: 0
    }
    // The family, inside the opening; the den's floor hides their chins.
    Item {
        anchors.fill: parent
        anchors.bottomMargin: root.height * 4 / 150
        clip: true
        BearHead { alive: root.alive; night: root.night; kind: "dad"; width: 104 * root.u; x: 56 * root.u; y: 98 * root.u }
        BearHead { alive: root.alive; night: root.night; kind: "mama"; width: 98 * root.u; x: 142 * root.u; y: 106 * root.u }
        BearHead { alive: root.alive; night: root.night; kind: "cub"; width: 72 * root.u; x: 114 * root.u; y: 150 * root.u }
    }
    // Paws resting on the rim.
    Repeater {
        model: [ { x: 62, w: 30 }, { x: 118, w: 24 }, { x: 160, w: 24 }, { x: 212, w: 30 } ]
        Ornament {
            required property var modelData
            name: "paw-grip"
            width: modelData.w * root.u
            height: width * 44 / 64
            x: modelData.x * root.u
            y: root.height * 146 / 150 - height * 0.72
        }
    }
    // The cub waves every so often.
    Ornament {
        id: wave
        name: "paw"
        width: 26 * root.u
        height: width
        x: 182 * root.u
        y: 150 * root.u
        transformOrigin: Item.Bottom
        opacity: 0
        visible: opacity > 0
        SequentialAnimation {
            id: waveAnim
            NumberAnimation { target: wave; property: "opacity"; to: 1; duration: 160 }
            NumberAnimation { target: wave; property: "rotation"; from: 0; to: 22; duration: 180; easing.type: Easing.OutSine }
            NumberAnimation { target: wave; property: "rotation"; to: -14; duration: 240; easing.type: Easing.InOutSine }
            NumberAnimation { target: wave; property: "rotation"; to: 22; duration: 240; easing.type: Easing.InOutSine }
            NumberAnimation { target: wave; property: "rotation"; to: -14; duration: 240; easing.type: Easing.InOutSine }
            NumberAnimation { target: wave; property: "rotation"; to: 0; duration: 180; easing.type: Easing.OutSine }
            NumberAnimation { target: wave; property: "opacity"; to: 0; duration: 200 }
        }
        Timer {
            interval: 4000
            repeat: true
            running: root.alive && !root.night
            onTriggered: { waveAnim.restart(); interval = 11000 + Math.random() * 7000 }
        }
    }
    // Night: "z"s drift up from the sleeping family.
    Repeater {
        model: root.night ? 3 : 0
        Text {
            required property int index
            text: "z"
            color: "#EDE3D1"
            font.family: Theme.fontFamily
            font.pixelSize: (18 + index * 7) * root.u
            font.weight: Font.Bold
            x: (212 + index * 16) * root.u
            y: 80 * root.u
            opacity: root.alive ? 0 : 0.7 - index * 0.2
            SequentialAnimation on y {
                running: root.alive
                loops: Animation.Infinite
                PauseAnimation { duration: index * 900 }
                NumberAnimation { from: 92 * root.u; to: 18 * root.u; duration: 2700; easing.type: Easing.OutSine }
            }
            SequentialAnimation on opacity {
                running: root.alive
                loops: Animation.Infinite
                PauseAnimation { duration: index * 900 }
                NumberAnimation { from: 0; to: 0.9; duration: 600 }
                NumberAnimation { to: 0; duration: 2100 }
            }
        }
    }
}
