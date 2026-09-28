// Bottom banner when the shell has state but lost the coordinator: the screen
// stays usable to look at, and actions will fail until it reconnects.

import QtQuick
import BearDen

PixelBox {
    visible: Session.loaded && !Shell.connected && !Shell.offline
    implicitHeight: 64 * Theme.scale
    radius: height / 2
    color: Theme.alpha(Theme.danger, 0.18)
    borderColor: Theme.danger
    implicitWidth: text.implicitWidth + 64 * Theme.scale
    Text {
        id: text
        anchors.centerIn: parent
        text: qsTr("Reconnecting to Bear Den… phone and app controls are paused")
        color: Theme.textPrimary
        font.family: Theme.fontFamily
        font.pixelSize: 22 * Theme.fontUnit
    }
}
