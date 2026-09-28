// Provider item (Plex continue watching / recently added). Missing artwork
// falls back to a tinted panel; DEMO items are labeled.

import QtQuick
import BearDen

Item {
    id: root
    property var item: ({})
    property bool focused: false
    width: Theme.cardWidth
    height: Theme.cardHeight
    z: focused ? 2 : 0

    PixelBox {
        id: card
        anchors.fill: parent
        radius: Theme.radius
        color: Theme.surfaceRaised
        clip: true
        scale: root.focused ? Theme.focusScale : 1
        Behavior on scale { NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }
        PixelBox {
            anchors.fill: parent
            radius: parent.radius
            gradient: Gradient {
                GradientStop { position: 0; color: Qt.darker(Theme.tintFor(root.item.itemId || root.item.title || ""), 1.6) }
                GradientStop { position: 1; color: Qt.darker(Theme.tintFor(root.item.itemId || root.item.title || ""), 3.2) }
            }
        }
        Image {
            anchors.fill: parent
            visible: status === Image.Ready
            source: root.item.artwork ? root.item.artwork : ""
            fillMode: Image.PreserveAspectCrop
            asynchronous: true
        }
        Rectangle {
            anchors.fill: parent
            gradient: Gradient {
                GradientStop { position: 0.35; color: "transparent" }
                GradientStop { position: 1; color: Theme.alpha("#000000", 0.8) }
            }
        }
        DemoBadge {
            visible: root.item.demo === true
            anchors { top: parent.top; left: parent.left; margins: 14 * Theme.scale }
        }
        Column {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: 18 * Theme.scale }
            spacing: 6 * Theme.scale
            Text {
                width: parent.width
                text: root.item.title || ""
                color: Theme.textPrimary
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                width: parent.width
                text: root.item.subtitle || ""
                color: Theme.textSecondary
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 19 * Theme.fontUnit
            }
            ProgressBar {
                visible: root.item.progress !== undefined && root.item.progress >= 0
                width: parent.width
                value: root.item.progress || 0
            }
        }
    }
    Item {
        anchors.fill: card
        scale: card.scale
        FocusFrame { shown: root.focused; glint: true }
        FocusDecor { anchors.fill: parent; shown: root.focused }
    }
}
