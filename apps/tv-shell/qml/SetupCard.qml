// Honest empty state for a section with no content (design §3.2): explains
// what is missing and leads to Settings instead of showing fake items.

import QtQuick
import BearDen

Item {
    id: root
    property var item: ({})
    property bool focused: false
    width: Theme.cardWidth * 1.4
    height: Theme.cardHeight
    z: focused ? 2 : 0
    PixelBox {
        anchors.fill: parent
        radius: Theme.radius
        color: Theme.surface
        borderColor: root.focused ? Theme.focusColor : Theme.surfaceBorder
        borderWidth: root.focused ? Theme.focusWidth : 2
        scale: root.focused ? Theme.focusScale : 1
        Behavior on scale { NumberAnimation { duration: Theme.duration } }
        Column {
            anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 28 * Theme.scale }
            spacing: 10 * Theme.scale
            Text {
                text: root.item.title || qsTr("Nothing here yet")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                width: parent.width
                text: root.item.subtitle || ""
                wrapMode: Text.WordWrap
                maximumLineCount: 3
                elide: Text.ElideRight
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 20 * Theme.fontUnit
            }
            Text {
                text: qsTr("Press OK to open Settings")
                color: Theme.accent
                font.family: Theme.fontFamily
                font.pixelSize: 20 * Theme.fontUnit
                opacity: root.focused ? 1 : 0.6
            }
        }
    }
}
