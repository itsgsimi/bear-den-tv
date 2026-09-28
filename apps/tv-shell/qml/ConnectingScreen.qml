// Shown until the first snapshot arrives from the coordinator.

import QtQuick
import BearDen

Item {
    id: root
    Column {
        anchors.centerIn: parent
        spacing: 28 * Theme.scale
        DenMascot { size: 380 * Theme.scale; anchors.horizontalCenter: parent.horizontalCenter }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Shell.rejectReason.length > 0 ? qsTr("Bear Den could not start") : qsTr("Starting Bear Den…")
            color: Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 44 * Theme.fontUnit
            font.weight: Font.DemiBold
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Shell.rejectReason.length > 0
                  ? qsTr("The coordinator refused this shell: %1").arg(Shell.rejectReason)
                  : (Shell.attempt > 1 ? qsTr("Waiting for the coordinator (attempt %1)").arg(Shell.attempt) : qsTr("Connecting to the coordinator"))
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
        }
        Row {
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: 14 * Theme.scale
            visible: Shell.rejectReason.length === 0
            Repeater {
                model: 3
                PixelBox {
                    required property int index
                    width: 16 * Theme.scale; height: width; radius: width / 2
                    color: Theme.accent
                    SequentialAnimation on opacity {
                        running: !Theme.reducedMotion && root.visible
                        loops: Animation.Infinite
                        PauseAnimation { duration: index * 160 }
                        NumberAnimation { from: 0.25; to: 1; duration: 320 }
                        NumberAnimation { from: 1; to: 0.25; duration: 320 }
                        PauseAnimation { duration: (2 - index) * 160 }
                    }
                }
            }
        }
    }
}
