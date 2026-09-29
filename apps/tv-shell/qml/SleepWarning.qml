// The sleep timer's last minute (state.power.warning; contracts/actions.md
// "Sleep, screen off and wake"): a gentle card near the bottom, "Going to
// sleep in 1 minute — press any key to stay awake", with the cub dozing
// (BearHead asleep, breathing only while motion is allowed). ShellRoot
// swallows the key that answers it and sends IPC power.activity; the
// coordinator cancels the timer and the card goes. Shown only while the
// shell is in front: over an app the TV cannot show it, and the coordinator
// notices TV input from the X idle counter instead.

import QtQuick
import BearDen

PixelBox {
    id: root
    objectName: "sleepWarning"
    readonly property bool shown: Session.loaded && !Session.locked && Session.power.warning === true
                                  && Session.power.display !== "off"
    readonly property bool alive: shown && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver
                                  && Session.target.kind === "shell"
    visible: opacity > 0
    opacity: shown ? 1 : 0
    Behavior on opacity { NumberAnimation { duration: Theme.duration } }
    implicitWidth: row.implicitWidth + 64 * Theme.scale
    implicitHeight: row.implicitHeight + 40 * Theme.scale
    radius: 22 * Theme.scale
    color: Theme.surfaceRaised
    borderColor: Theme.warning
    borderWidth: 2

    Row {
        id: row
        anchors.centerIn: parent
        spacing: 28 * Theme.scale
        BearHead {
            kind: "cub"
            night: true
            alive: root.alive
            width: 84 * Theme.scale
            height: width
            anchors.verticalCenter: parent.verticalCenter
        }
        Column {
            anchors.verticalCenter: parent.verticalCenter
            spacing: 6 * Theme.scale
            Text {
                objectName: "sleepWarningTitle"
                text: qsTr("Going to sleep in 1 minute")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 34 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                text: qsTr("Press any key to stay awake")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
            }
        }
    }
}
