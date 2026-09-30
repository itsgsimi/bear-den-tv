// Header navigation pill: an optional Bear Den icon (UiIcon) and a label.
// `focused` (the D-pad selection) is the loudest: a solid light fill, the
// focus ring and a scale-up. `current` (the page on screen) is quieter: a
// raised surface with an accent bar under the label, so the two never look
// alike.

import QtQuick
import BearDen

PixelBox {
    id: root
    property string text
    property string icon: ""
    property bool current: false
    property bool focused: false
    // Room left and right of the label, together (the top bar narrows it
    // when large text leaves no room).
    property real padding: 48 * Theme.scale
    implicitWidth: content.implicitWidth + padding
    implicitHeight: 60 * Theme.scale
    radius: height / 2
    color: focused ? Theme.pillActiveBg : (current ? Theme.surfaceRaised : "transparent")
    scale: focused ? Theme.focusScale : 1
    Behavior on scale { NumberAnimation { duration: Theme.durationFast } }
    Row {
        id: content
        anchors.centerIn: parent
        spacing: 10 * Theme.scale
        UiIcon {
            visible: root.icon.length > 0
            name: root.icon
            size: 40 * Theme.scale
            anchors.verticalCenter: parent.verticalCenter
        }
        Text {
            id: label
            anchors.verticalCenter: parent.verticalCenter
            text: root.text
            color: root.focused ? Theme.pillActiveText : Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
            font.weight: root.current || root.focused ? Font.DemiBold : Font.Normal
        }
    }
    PixelBox {
        objectName: "currentBar"
        visible: root.current
        anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: 6 * Theme.scale }
        width: Math.min(label.width, 44 * Theme.scale)
        height: 5 * Theme.scale
        radius: height / 2
        color: root.focused ? Theme.pillActiveText : Theme.accent
    }
    FocusFrame { shown: root.focused; cornerRadius: root.radius }
}
