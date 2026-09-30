// The "Add apps" tile at the end of the first apps rail on Home (kind
// `add-apps`, added by SessionModel while some app Bear Den knows is not
// installed and can be): the same size as an app tile, Bear Den's "+" icon
// (UiIcon `plus`), "Add apps" and a line built from what is actually missing
// ("Spotify, Netflix and more", item.subtitle). OK opens the Apps page at
// Add apps (HomeScreen.activateItem). No third-party logos.

import QtQuick
import BearDen

Item {
    id: root
    objectName: "addAppsTile"
    property var item: ({})
    property bool focused: false
    width: Theme.tileWidth
    height: Theme.tileHeight
    z: focused ? 2 : 0

    Item {
        id: tile
        anchors.fill: parent
        scale: root.focused ? Theme.focusScale : 1
        Behavior on scale { NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }

        BrandBackdrop {
            anchors.fill: parent
            topColor: Qt.darker(Theme.accent, 2.6)
            bottomColor: Qt.darker(Theme.accent, 4.2)
            glow: Theme.accent
            glowX: 0.14
            glowY: 0.26
            glowRadius: 0.62
            glowStrength: 0.35
        }
        // A dashed-looking inner edge: an empty slot waiting for an app.
        PixelBox {
            anchors.fill: parent
            anchors.margins: 10 * Theme.scale
            radius: Theme.radius * 0.7
            color: "transparent"
            borderColor: Theme.alpha("#ffffff", 0.22)
            borderWidth: 2 * Theme.scale
        }
        UiIcon {
            anchors { left: parent.left; top: parent.top; leftMargin: 22 * Theme.scale; topMargin: 20 * Theme.scale }
            name: "plus"
            size: 64 * Theme.scale
        }
        Column {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: 22 * Theme.scale; rightMargin: 22 * Theme.scale; bottomMargin: 18 * Theme.scale }
            spacing: 2 * Theme.scale
            Text {
                width: parent.width
                text: root.item.title || qsTr("Add apps")
                color: "#ffffff"
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 32 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                objectName: "addAppsTileLine"
                width: parent.width
                text: root.item.subtitle || ""
                visible: text.length > 0
                color: Theme.alpha("#ffffff", 0.8)
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 18 * Theme.fontUnit
            }
        }
    }
    Item {
        anchors.fill: tile
        scale: tile.scale
        FocusFrame { shown: root.focused; glint: true }
        FocusDecor { anchors.fill: parent; shown: root.focused }
    }
}
