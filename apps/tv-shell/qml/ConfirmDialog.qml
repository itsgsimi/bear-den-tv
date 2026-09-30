// Modal two-button dialog. Used for destructive confirmations (focus starts on
// the safe choice; `danger` draws the confirm button red, UX-31), for timed
// layout confirmations from the coordinator, where Back or the timeout
// reverts, and for a choice between two actions (a running app: Close or
// Switch), where `dismissOnBack` makes Back just close it.

import QtQuick
import BearDen

Rectangle {
    id: root
    property string title
    property string body
    property string confirmLabel: qsTr("OK")
    property string cancelLabel: qsTr("Cancel")
    property int countdown: 0            // seconds; 0 = untimed
    property int focusIndex: 1           // 0 = confirm, 1 = cancel
    property var onAccept: null
    property var onReject: null
    property bool timedOutAccepts: false
    property bool danger: false
    property bool dismissOnBack: false
    visible: false
    color: Theme.scrim

    function open(opts) {
        title = opts.title || ""
        body = opts.body || ""
        confirmLabel = opts.confirmLabel || qsTr("OK")
        cancelLabel = opts.cancelLabel || qsTr("Cancel")
        countdown = opts.countdown || 0
        focusIndex = opts.focusConfirm ? 0 : 1
        onAccept = opts.onAccept || null
        onReject = opts.onReject || null
        danger = opts.danger === true
        dismissOnBack = opts.dismissOnBack === true
        visible = true
        report()
    }
    function report() { Nav.reportFocus("dialog", focusIndex === 0 ? "confirm" : "cancel", 0) }
    function finish(accepted) {
        visible = false
        const cb = accepted ? onAccept : onReject
        onAccept = null; onReject = null
        if (cb) cb()
    }
    function navigate(action) {
        switch (action) {
        case "nav.left": focusIndex = 0; report(); return true
        case "nav.right": focusIndex = 1; report(); return true
        case "nav.up": case "nav.down": return true
        case "select": finish(focusIndex === 0); return true
        case "back":
            if (dismissOnBack) { visible = false; onAccept = null; onReject = null }
            else finish(false)
            return true
        }
        return false
    }
    Timer {
        running: root.visible && root.countdown > 0
        interval: 1000; repeat: true
        onTriggered: { root.countdown -= 1; if (root.countdown <= 0) root.finish(root.timedOutAccepts) }
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
                text: root.body + (root.countdown > 0 ? "\n" + qsTr("Reverting in %1 s unless you keep it.").arg(root.countdown) : "")
                wrapMode: Text.WordWrap
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                lineHeight: 1.15
            }
            Row {
                spacing: 20 * Theme.scale
                topPadding: 12 * Theme.scale
                FocusButton { objectName: "confirmButton"; text: root.confirmLabel; primary: !root.danger; danger: root.danger; focused: root.focusIndex === 0 }
                FocusButton { text: root.cancelLabel; focused: root.focusIndex === 1 }
            }
        }
    }
}
