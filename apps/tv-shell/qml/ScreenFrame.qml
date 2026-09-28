// Common frame for secondary screens: brand, title, subtitle, content, and
// key hints, all inside the safe margins applied by ShellRoot.

import QtQuick
import BearDen

Item {
    id: root
    property string title
    property string subtitle
    property var hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("Select")], ["Back", qsTr("Back")]]
    default property alias content: body.data

    Row {
        id: top
        spacing: 24 * Theme.scale
        BearMark { size: 64 * Theme.scale; anchors.verticalCenter: parent.verticalCenter }
        Column {
            anchors.verticalCenter: parent.verticalCenter
            Text {
                text: root.title
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 48 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                visible: root.subtitle.length > 0
                text: root.subtitle
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
        }
    }
    DemoBadge { visible: Session.devMode; anchors { right: parent.right; verticalCenter: top.verticalCenter } }
    Ornament {
        visible: World.decorated
        name: "divider"
        width: 260 * Theme.scale; height: 26 * Theme.scale
        anchors { left: top.left; top: top.bottom; topMargin: 10 * Theme.scale }
        opacity: 0.8
    }

    Item {
        id: body
        anchors { top: top.bottom; topMargin: 44 * Theme.scale; left: parent.left; right: parent.right; bottom: hintsRow.top; bottomMargin: 28 * Theme.scale }
    }
    KeyHints {
        id: hintsRow
        anchors { left: parent.left; bottom: parent.bottom }
        hints: root.hints
    }
}
