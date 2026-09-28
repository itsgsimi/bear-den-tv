// Header navigation pill. `current` marks the visible screen; `focused` the
// D-pad selection (outline + scale, not color alone).

import QtQuick
import BearDen

PixelBox {
    id: root
    property string text
    property bool current: false
    property bool focused: false
    implicitWidth: label.implicitWidth + 56 * Theme.scale
    implicitHeight: 60 * Theme.scale
    radius: height / 2
    color: current ? Theme.pillActiveBg : (focused ? Theme.surfaceRaised : "transparent")
    scale: focused ? Theme.focusScale : 1
    Behavior on scale { NumberAnimation { duration: Theme.durationFast } }
    Text {
        id: label
        anchors.centerIn: parent
        text: root.text
        color: root.current ? Theme.pillActiveText : Theme.textPrimary
        font.family: Theme.fontFamily
        font.pixelSize: 26 * Theme.fontUnit
        font.weight: root.current ? Font.DemiBold : Font.Normal
    }
    FocusFrame { shown: root.focused; cornerRadius: root.radius }
}
