// Modal notice with a single OK; Back also dismisses it.

import QtQuick
import BearDen

Rectangle {
    id: root
    property string title
    property string body
    property string detail
    visible: false
    color: Theme.scrim

    function open(t, b, d) {
        title = t; body = b; detail = d || ""
        visible = true
        Nav.reportFocus("dialog", "ok", 0)
    }
    function navigate(action) {
        if (action === "select" || action === "back") { visible = false; return true }
        return true   // swallow movement while modal
    }

    PixelBox {
        anchors.centerIn: parent
        scale: root.visible ? 1 : 0.92
        Behavior on scale { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutBack } }
        width: Math.min(parent.width * 0.6, 1100 * Theme.scale)
        height: content.height + 96 * Theme.scale
        radius: Theme.radius * 1.4
        color: Theme.surfaceRaised
        borderColor: Theme.surfaceBorder
        Column {
            id: content
            anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 56 * Theme.scale }
            spacing: 24 * Theme.scale
            Ornament { visible: World.decorated; name: "sprig"; width: 72 * Theme.scale; height: 36 * Theme.scale }
            Text {
                width: parent.width
                text: root.title
                wrapMode: Text.WordWrap
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 40 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                width: parent.width
                text: root.body
                wrapMode: Text.WordWrap
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                lineHeight: 1.15
            }
            PixelBox {
                visible: root.detail.length > 0
                width: parent.width
                height: detailText.implicitHeight + 32 * Theme.scale
                radius: 14 * Theme.scale
                color: Theme.alpha("#000000", 0.35)
                Text {
                    id: detailText
                    anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 20 * Theme.scale }
                    text: root.detail
                    wrapMode: Text.WrapAnywhere
                    color: Theme.accent
                    font.family: Theme.monoFamily
                    font.pixelSize: 22 * Theme.fontUnit
                }
            }
            FocusButton { text: qsTr("OK"); primary: true; focused: true }
        }
    }
}
