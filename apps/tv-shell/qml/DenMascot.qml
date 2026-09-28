// The cub peeking out of its den (or asleep inside). The cub rises into view
// once, then breathes gently; asleep, "z"s float up. Static when motion is
// reduced.

import QtQuick
import BearDen

Item {
    id: root
    property bool asleep: false
    // `visible` is the effective visibility: a hidden mascot (lock screen while
    // unlocked, connecting screen once loaded) must not keep Qt rendering frames.
    property bool animate: !Theme.reducedMotion && !Theme.resting && visible
    property real size: 360 * Theme.scale
    width: size
    height: size * 0.75
    clip: true

    Ornament {
        id: den
        name: "den"
        anchors.fill: parent
    }
    // Cub head inside the opening; the den's lower edge hides its chin.
    Item {
        id: cub
        width: root.width * 0.44
        height: width
        anchors.horizontalCenter: parent.horizontalCenter
        y: root.height * 0.36 + rise
        property real rise: root.animate ? root.height * 0.5 : 0
        Component.onCompleted: if (root.animate) riseAnim.start()
        NumberAnimation on rise { id: riseAnim; running: false; to: 0; duration: 900; easing.type: Easing.OutBack }
        // Classic: the awake cub breathes gently (BearHead breathes asleep).
        transformOrigin: Item.Bottom
        SequentialAnimation on scale {
            running: World.classic && root.animate && !root.asleep
            loops: Animation.Infinite
            NumberAnimation { from: 1; to: 1.035; duration: 1600; easing.type: Easing.InOutSine }
            NumberAnimation { from: 1.035; to: 1; duration: 1600; easing.type: Easing.InOutSine }
        }
        BearHead {
            anchors.fill: parent
            kind: root.asleep ? "dad" : "cub"
            night: root.asleep
            alive: root.animate
        }
    }
    // Paws resting on the den's lip.
    Repeater {
        model: 2
        PixelBox {
            required property int index
            width: root.width * 0.1
            height: width * 0.7
            radius: height / 2
            color: "#79A889"
            borderColor: "#5E8C6E"
            borderWidth: 2
            x: root.width * (index === 0 ? 0.34 : 0.56)
            y: root.height - height * 0.9
            visible: !root.asleep
        }
    }
    // Sleep "z"s.
    Repeater {
        model: root.asleep ? 3 : 0
        Text {
            required property int index
            text: "z"
            color: "#EDE3D1"
            font.family: Theme.fontFamily
            font.pixelSize: (22 + index * 8) * Theme.scale
            font.weight: Font.Bold
            x: root.width * 0.62 + index * 18 * Theme.scale
            y: root.height * 0.3
            opacity: root.animate ? 0 : 0.7 - index * 0.2
            SequentialAnimation on y {
                running: root.animate
                loops: Animation.Infinite
                PauseAnimation { duration: index * 900 }
                NumberAnimation { from: root.height * 0.34; to: root.height * 0.02; duration: 2700; easing.type: Easing.OutSine }
            }
            SequentialAnimation on opacity {
                running: root.animate
                loops: Animation.Infinite
                PauseAnimation { duration: index * 900 }
                NumberAnimation { from: 0; to: 0.9; duration: 600 }
                NumberAnimation { to: 0; duration: 2100 }
            }
        }
    }
}
