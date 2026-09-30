// Shown until the first snapshot arrives from the coordinator. Plain words
// (UX-09): "Getting ready…", then what to do if Bear Den's helper (the
// coordinator) never answers; a refusal's code only in small print.

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
            objectName: "connectingMessage"
            // Plain words (UX-09); the code only in the small print below.
            // After about half a minute of tries (attempt 5 with the 1 s → 30 s
            // backoff) it says what to do.
            width: Math.min(implicitWidth, root.width * 0.7)
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WordWrap
            text: Shell.rejectReason.length > 0
                  ? qsTr("This screen and Bear Den's helper don't match. Restart the PC; if it happens again, install Bear Den again.")
                  : (Shell.attempt >= 5 ? qsTr("Bear Den's helper isn't running. Restart the PC, or run bear-den-tv doctor from a keyboard.")
                                        : qsTr("Getting ready…"))
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            visible: Shell.rejectReason.length > 0
            text: qsTr("Details: %1").arg(Shell.rejectReason)
            color: Theme.textMuted
            font.family: Theme.fontFamily
            font.pixelSize: 20 * Theme.fontUnit
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
