// Bottom-of-screen reminder of what the remote buttons do here, on a dark
// backing so it reads over any world (Winter's snow included).

import QtQuick
import BearDen

Item {
    id: root
    property var hints: [["◀ ▶ ▲ ▼", qsTr("Move")], ["OK", qsTr("Select")], ["Back", qsTr("Back")]]
    implicitWidth: row.implicitWidth + 28 * Theme.scale
    implicitHeight: row.implicitHeight + 14 * Theme.scale
    PixelBox {
        anchors.fill: parent
        radius: height / 2
        color: Theme.alpha("#000000", 0.5)
    }
    Row {
    id: row
    anchors.centerIn: parent
    spacing: 36 * Theme.scale
    Repeater {
        model: root.hints
        Row {
            required property var modelData
            spacing: 10 * Theme.scale
            PixelBox {
                implicitWidth: keyLabel.implicitWidth + 20 * Theme.scale
                implicitHeight: keyLabel.implicitHeight + 8 * Theme.scale
                radius: 8 * Theme.scale
                color: Theme.surfaceRaised
                borderColor: Theme.surfaceBorder
                Text {
                    id: keyLabel
                    anchors.centerIn: parent
                    text: modelData[0]
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 18 * Theme.fontUnit
                    font.weight: Font.DemiBold
                }
            }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: modelData[1]
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 20 * Theme.fontUnit
            }
        }
    }
    }
}
