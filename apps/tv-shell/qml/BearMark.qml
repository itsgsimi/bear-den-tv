// The Bear Den mark on a raised tile: the dad bear in pixel art
// (assets/pixel/bear-mark.png, 16×16, tools/pixelart/bears.py), drawn
// unsmoothed at a whole number of World.px per art pixel. Classic art style
// (World.classic): the smooth SVG mark (assets/bear-mark.svg).

import QtQuick
import BearDen

PixelBox {
    id: root
    property real size: 56 * Theme.scale
    width: size
    height: size
    radius: size * 0.24
    color: Theme.surfaceRaised
    borderColor: Theme.surfaceBorder
    borderWidth: 1
    Image {
        objectName: "bearMarkClassic"
        visible: World.classic
        anchors.centerIn: parent
        width: root.size * 0.82
        height: width
        source: World.classic ? "qrc:/qt/qml/BearDen/assets/bear-mark.svg" : ""
        sourceSize: Qt.size(width * 2, height * 2)
        smooth: true
    }
    Image {
        visible: World.pixel
        anchors.centerIn: parent
        readonly property real unit: Math.max(1, Math.floor(root.size * 0.86 / (16 * World.px))) * World.px
        width: 16 * unit
        height: width
        source: "qrc:/qt/qml/BearDen/assets/pixel/bear-mark.png"
        smooth: false
    }
}
