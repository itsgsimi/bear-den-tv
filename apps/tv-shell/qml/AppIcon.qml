// An application's icon (Shell.appArt, docs/THEMES.md → App icons), in order:
// the owner's brand folder, Bear Den's own icon for the adapter (pixel art
// drawn unsmoothed at a whole-number scale, or the Classic SVG), the icon its
// installed Flatpak exports, otherwise a monogram on the app tint.

import QtQuick
import BearDen

Item {
    id: root
    property string adapter
    property string label
    property color tint: Theme.accent
    property real size: 64 * Theme.scale
    readonly property var art: Shell.appArt(adapter, World.classic)
    readonly property string source: art.icon
    // Bear Den's pixel icons are 32×32 art pixels: shown at the largest whole
    // multiple that fits, never smoothed or scaled in between.
    readonly property bool pixelArt: art.iconSource === "bundled" && World.pixel
    readonly property int grid: 32
    readonly property real drawn: pixelArt ? Math.max(1, Math.floor(size / grid)) * grid : size
    width: size
    height: size
    Image {
        objectName: "appIconImage"
        visible: root.source.length > 0
        anchors.centerIn: parent
        width: root.drawn
        height: root.drawn
        source: root.source
        sourceSize: root.pixelArt ? Qt.size(root.grid, root.grid) : Qt.size(width * 2, height * 2)
        fillMode: Image.PreserveAspectFit
        smooth: !root.pixelArt
        asynchronous: true
    }
    PixelBox {
        visible: root.source.length === 0
        anchors.fill: parent
        radius: root.size * 0.24
        color: Qt.lighter(root.tint, 1.25)
        borderColor: Theme.alpha("#ffffff", 0.35)
        borderWidth: 1
        Text {
            anchors.centerIn: parent
            text: root.label.length > 0 ? root.label.charAt(0).toUpperCase() : "?"
            color: "#ffffff"
            font.family: Theme.fontFamily
            font.pixelSize: root.size * 0.52
            font.weight: Font.Black
        }
    }
}
