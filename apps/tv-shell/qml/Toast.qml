// Transient notification in the top-right corner (coordinator `notify`).

import QtQuick
import BearDen

PixelBox {
    id: root
    property string kind: "info"
    property string text
    visible: opacity > 0
    opacity: 0
    implicitWidth: Math.min(720 * Theme.scale, label.implicitWidth + 80 * Theme.scale)
    implicitHeight: label.implicitHeight + 40 * Theme.scale
    radius: 18 * Theme.scale
    color: Theme.surfaceRaised
    borderColor: kind === "error" ? Theme.danger : (kind === "warning" ? Theme.warning : (kind === "success" ? Theme.success : Theme.surfaceBorder))
    borderWidth: 2
    Behavior on opacity { NumberAnimation { duration: Theme.duration } }
    transform: Translate { x: root.opacity > 0.5 ? 0 : 40 * Theme.scale; Behavior on x { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } } }

    function show(k, t) {
        kind = k; text = t
        opacity = 1
        hideTimer.restart()
    }
    Timer { id: hideTimer; interval: 5000; onTriggered: root.opacity = 0 }
    Text {
        id: label
        anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 40 * Theme.scale }
        text: root.text
        wrapMode: Text.WordWrap
        color: Theme.textPrimary
        font.family: Theme.fontFamily
        font.pixelSize: 24 * Theme.fontUnit
    }
}
