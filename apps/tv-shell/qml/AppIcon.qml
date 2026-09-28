// An application's icon: its official icon as-is (owner brand folder, else the
// icon its installed Flatpak exports), otherwise a monogram on the app tint.

import QtQuick
import BearDen

Item {
    id: root
    property string adapter
    property string label
    property color tint: Theme.accent
    property real size: 64 * Theme.scale
    readonly property string source: Shell.appArt(adapter).icon
    width: size
    height: size
    Image {
        visible: root.source.length > 0
        anchors.fill: parent
        source: root.source
        sourceSize: Qt.size(width * 2, height * 2)
        fillMode: Image.PreserveAspectFit
        smooth: true
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
