// One of the family's heads in pixel art (assets/pixel/head-<kind>.png: awake,
// blinking, asleep), blinking now and then at a relaxed, uneven pace and
// breathing (a one-pixel rise) while asleep at night. `alive` gates all
// motion (resting, screensaver, apps in front). Drawn unsmoothed at a whole
// number of World.px per art pixel, sitting on the bottom of its box.
// Classic art style (World.classic): the family's SVG heads (assets/bear-cub,
// bear-mama, and bear-mark for dad; their -sleep faces when closed) fill the
// square box, smooth, and breathe by a gentle scale while asleep.

import QtQuick
import BearDen
import "BearRig.js" as Rig

Item {
    id: bear
    property string kind: "cub"          // dad | mama | cub
    property bool alive: false
    property bool night: false
    property bool blink: false
    readonly property bool closed: night || blink
    readonly property var rig: Rig.kinds[kind] || Rig.kinds.cub
    readonly property real unit: Math.max(1, Math.round(width / (rig.head[0] * World.px))) * World.px
    property bool breathIn: false
    height: width
    // Classic: the SVG head.
    readonly property string svg: kind === "dad" ? (closed ? "bear-sleep" : "bear-mark")
                                                 : "bear-" + (kind === "mama" ? "mama" : "cub") + (closed ? "-sleep" : "")
    Image {
        objectName: "bearHeadClassic"
        visible: World.classic
        anchors.fill: parent
        source: World.classic ? "qrc:/qt/qml/BearDen/assets/" + bear.svg + ".svg" : ""
        sourceSize: Qt.size(width * 2, height * 2)
        smooth: true
        antialiasing: true
        transformOrigin: Item.Bottom
        SequentialAnimation on scale {
            running: World.classic && bear.alive && bear.night
            loops: Animation.Infinite
            NumberAnimation { to: 1.03; duration: 2200; easing.type: Easing.InOutSine }
            NumberAnimation { to: 1; duration: 2200; easing.type: Easing.InOutSine }
        }
    }
    Image {
        visible: World.pixel
        source: "qrc:/qt/qml/BearDen/assets/pixel/head-" + bear.kind + ".png"
        width: bear.rig.head[0] * bear.unit
        height: bear.rig.head[1] * bear.unit
        x: Math.round((bear.width - width) / 2 / World.px) * World.px
        y: bear.height - height - (bear.breathIn ? bear.unit : 0)
        smooth: false
        sourceClipRect: Qt.rect((bear.night ? 2 : bear.blink ? 1 : 0) * bear.rig.head[0], 0, bear.rig.head[0], bear.rig.head[1])
    }
    Timer {
        interval: 3000 + Math.random() * 4000
        repeat: true
        running: bear.alive && !bear.night
        onTriggered: { bear.blink = true; unblink.restart(); interval = 3000 + Math.random() * 5000 }
    }
    Timer { id: unblink; interval: 150; onTriggered: bear.blink = false }
    Timer {
        interval: 2200
        repeat: true
        running: World.pixel && bear.alive && bear.night
        onTriggered: bear.breathIn = !bear.breathIn
    }
}
