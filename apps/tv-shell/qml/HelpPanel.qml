// The panel beside a list of settings: the focused entry's name, its one
// plain sentence of help (Help.qml, by row id) and, for an app, what is good
// to know about it (state.applications[].notes: the coordinator's honest
// caveats, e.g. "Up to 720p in a Linux browser"). Used by Settings, the
// Themes and Apps pages and the first-run setup.

import QtQuick
import BearDen

PixelBox {
    id: root
    property string title: ""
    property string help: ""
    property var notes: []
    property string icon: ""
    implicitHeight: column.implicitHeight + 56 * Theme.scale
    radius: 18 * Theme.scale
    color: Theme.surface
    borderColor: Theme.surfaceBorder
    borderWidth: 1
    visible: title.length > 0 || help.length > 0

    Column {
        id: column
        anchors { left: parent.left; right: parent.right; top: parent.top; margins: 28 * Theme.scale }
        spacing: 14 * Theme.scale
        Row {
            spacing: 16 * Theme.scale
            UiIcon {
                visible: root.icon.length > 0
                name: root.icon
                size: 56 * Theme.scale
                anchors.verticalCenter: parent.verticalCenter
            }
            Text {
                objectName: "helpTitle"
                width: column.width - (root.icon.length > 0 ? 72 * Theme.scale : 0)
                anchors.verticalCenter: parent.verticalCenter
                text: root.title
                color: Theme.textPrimary
                wrapMode: Text.WordWrap
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
        }
        Text {
            objectName: "helpText"
            width: column.width
            visible: text.length > 0
            text: root.help
            color: Theme.textSecondary
            wrapMode: Text.WordWrap
            lineHeight: 1.1
            font.family: Theme.fontFamily
            font.pixelSize: 24 * Theme.fontUnit
        }
        Text {
            visible: (root.notes || []).length > 0
            text: qsTr("Good to know")
            color: Theme.accent
            font.family: Theme.fontFamily
            font.pixelSize: 20 * Theme.fontUnit
            font.weight: Font.Bold
            font.letterSpacing: 2 * Theme.scale
        }
        Repeater {
            model: root.notes || []
            Text {
                required property string modelData
                objectName: "helpNote"
                width: column.width
                text: "• " + modelData
                color: Theme.textPrimary
                wrapMode: Text.WordWrap
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
            }
        }
    }
}
