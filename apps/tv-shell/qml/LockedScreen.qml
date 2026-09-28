// While the desktop session is locked nothing private is shown and every
// action is refused; the coordinator never unlocks anything.

import QtQuick
import BearDen

Rectangle {
    color: Theme.alpha(Theme.bgBottom, 0.96)
    Column {
        anchors.centerIn: parent
        spacing: 24 * Theme.scale
        DenMascot { asleep: true; size: 380 * Theme.scale; anchors.horizontalCenter: parent.horizontalCenter }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: qsTr("This TV is locked")
            color: Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 48 * Theme.fontUnit
            font.weight: Font.DemiBold
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: qsTr("Unlock it with the keyboard to continue. Phones cannot control a locked TV.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
        }
    }
}
