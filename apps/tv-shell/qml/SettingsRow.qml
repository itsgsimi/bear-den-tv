// One settings entry: optional Bear Den icon (UiIcon), label, optional
// description, value, and an affordance showing whether OK opens a page (›),
// Left/Right changes the value (‹ ›), or it is only information (`info`).
// With large text the description wraps to two lines and the row grows
// (UX-24), instead of cutting it off mid-word.

import QtQuick
import BearDen

PixelBox {
    id: root
    property string label
    property string description
    property string value
    property string kind: "link"   // link | choice | toggle | danger | info
    property string icon: ""       // a UiIcon name, drawn before the label
    property bool focused: false
    // Optional small marker before the value (e.g. "Auto" on Advanced playback).
    property string badge: ""
    implicitHeight: Math.max((description.length > 0 ? 104 : 84) * Theme.scale, texts.implicitHeight + 32 * Theme.scale)
    radius: 18 * Theme.scale
    color: focused ? Theme.surfaceRaised : Theme.surface
    borderColor: Theme.surfaceBorder
    borderWidth: 1
    scale: focused ? 1.015 : 1
    Behavior on scale { NumberAnimation { duration: Theme.durationFast } }

    UiIcon {
        id: leadIcon
        visible: root.icon.length > 0
        name: root.icon
        size: 64 * Theme.scale
        anchors { left: parent.left; leftMargin: 24 * Theme.scale; verticalCenter: parent.verticalCenter }
    }
    Column {
        id: texts
        anchors { left: leadIcon.visible ? leadIcon.right : parent.left; leftMargin: leadIcon.visible ? 20 * Theme.scale : 32 * Theme.scale; verticalCenter: parent.verticalCenter; right: badgePill.visible ? badgePill.left : valueText.left; rightMargin: 24 * Theme.scale }
        spacing: 4 * Theme.scale
        Text {
            width: parent.width
            wrapMode: Text.WordWrap
            maximumLineCount: 2
            elide: Text.ElideRight
            text: root.label
            color: root.kind === "danger" ? Theme.danger : Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 28 * Theme.fontUnit
            font.weight: Font.DemiBold
        }
        Text {
            visible: root.description.length > 0
            width: parent.width
            text: root.description
            wrapMode: Text.WordWrap
            maximumLineCount: 2
            elide: Text.ElideRight
            color: Theme.textMuted
            font.family: Theme.fontFamily
            font.pixelSize: 20 * Theme.fontUnit
        }
    }
    PixelBox {
        id: badgePill
        visible: root.badge.length > 0
        anchors { right: valueText.left; rightMargin: 16 * Theme.scale; verticalCenter: parent.verticalCenter }
        implicitWidth: badgeText.implicitWidth + 24 * Theme.scale
        implicitHeight: 32 * Theme.scale
        radius: height / 2
        color: Theme.alpha("#000000", 0.3)
        borderColor: Theme.surfaceBorder
        Text {
            id: badgeText
            anchors.centerIn: parent
            text: root.badge
            color: Theme.textMuted
            font.family: Theme.fontFamily
            font.pixelSize: 17 * Theme.fontUnit
            font.weight: Font.Medium
        }
    }
    Text {
        id: valueText
        anchors { right: parent.right; rightMargin: 32 * Theme.scale; verticalCenter: parent.verticalCenter }
        text: root.kind === "choice" ? "‹  " + root.value + "  ›"
              : root.kind === "toggle" ? (root.value === "on" ? qsTr("On") : qsTr("Off"))
              : root.kind === "info" ? root.value
              : (root.value.length > 0 ? root.value + "   ›" : "›")
        color: root.kind === "toggle" && root.value === "on" ? Theme.success : (root.focused ? Theme.textPrimary : Theme.textSecondary)
        font.family: Theme.fontFamily
        font.pixelSize: 26 * Theme.fontUnit
        font.weight: Font.Medium
    }
    FocusFrame { objectName: "focusFrame"; shown: root.focused; cornerRadius: root.radius }
}
