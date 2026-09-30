// Small status indicator: a colored dot plus a label.

import QtQuick
import BearDen

PixelBox {
    id: root
    property string text
    property color dot: Theme.success
    implicitWidth: row.implicitWidth + 28 * Theme.scale
    implicitHeight: 44 * Theme.scale
    radius: height / 2
    color: Theme.surface
    borderColor: Theme.surfaceBorder
    borderWidth: 1
    Row {
        id: row
        anchors.centerIn: parent
        spacing: 10 * Theme.scale
        StatusDot {
            width: 12 * Theme.scale
            color: root.dot
            anchors.verticalCenter: parent.verticalCenter
        }
        Text {
            text: root.text
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 20 * Theme.fontUnit
            anchors.verticalCenter: parent.verticalCenter
        }
    }
}
