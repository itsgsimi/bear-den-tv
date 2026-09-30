// D-pad button used on screens and dialogs. Screens own focus bookkeeping and
// set `focused`; `primary` fills with the accent when focused, with dark text
// on it (the accents are mid-light colours, so light text would not read).

import QtQuick
import BearDen

PixelBox {
    id: root
    property string text
    property string detail
    property bool focused: false
    property bool primary: false
    property bool danger: false
    property bool available: true
    implicitWidth: Math.max(220 * Theme.scale, column.implicitWidth + 64 * Theme.scale)
    implicitHeight: detail.length > 0 ? 96 * Theme.scale : 72 * Theme.scale
    radius: 18 * Theme.scale
    // Only the focused button is filled, so an unfocused primary action never
    // looks selected; primary is marked by accent text instead.
    color: focused ? (primary ? Theme.accent : Theme.pillActiveBg) : Theme.surface
    borderColor: danger ? Theme.danger : Theme.surfaceBorder
    borderWidth: danger ? 2 : 1
    opacity: available ? 1 : 0.45
    scale: focused ? 1.03 : 1
    Behavior on scale { NumberAnimation { duration: Theme.durationFast } }
    Column {
        id: column
        anchors.centerIn: parent
        spacing: 4 * Theme.scale
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: root.text
            color: root.focused ? Theme.pillActiveText
                                : (root.danger ? Theme.danger : (root.primary ? Theme.accent : Theme.textPrimary))
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
            font.weight: Font.DemiBold
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            visible: root.detail.length > 0
            text: root.detail
            color: root.focused ? Theme.pillActiveText : Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 20 * Theme.fontUnit
        }
    }
    FocusFrame { shown: root.focused; cornerRadius: root.radius }
}
