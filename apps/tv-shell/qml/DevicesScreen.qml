// Paired phones with connection state; OK revokes one (after confirmation),
// the last row revokes all.

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    signal confirm(string title, string body, string confirmLabel, var onAccept)
    readonly property var devices: Session.devices
    readonly property int rowCount: devices.length + (devices.length > 0 ? 1 : 0)

    function enter() { focusIndex = 0; report() }
    function report() {
        const id = focusIndex < devices.length ? devices[focusIndex].id : (devices.length > 0 ? "revoke-all" : "")
        Nav.reportFocus("devices", id, 0)
    }
    function describe(d) {
        const role = d.permissions.indexOf("owner") >= 0 ? qsTr("Owner") : (d.permissions.indexOf("layout_editor") >= 0 ? qsTr("Can edit layout") : qsTr("Remote control"))
        return (d.connected ? qsTr("Connected now") : qsTr("Not connected")) + " · " + role
    }
    function navigate(action) {
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.down": focusIndex = Math.min(Math.max(0, rowCount - 1), focusIndex + 1); report(); return true
        case "nav.left": case "nav.right": return true
        case "select":
            if (focusIndex < devices.length) {
                const d = devices[focusIndex]
                confirm(qsTr("Remove “%1”?").arg(d.name), qsTr("This phone will stop controlling the TV immediately and must pair again."),
                        qsTr("Remove"), () => Shell.revokeDevice(d.id))
            } else if (devices.length > 0) {
                confirm(qsTr("Remove all phones?"), qsTr("Every paired phone stops controlling the TV immediately."),
                        qsTr("Remove all"), () => Shell.revokeDevice("*"))
            }
            return true
        }
        return false
    }
    onRowCountChanged: focusIndex = Math.min(focusIndex, Math.max(0, rowCount - 1))

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Paired phones")
        subtitle: root.devices.length === 0 ? qsTr("No phones are paired") : qsTr("%n phone(s) can control this TV", "", root.devices.length)
        hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("Remove")], ["Back", qsTr("Back")]]

        Column {
            anchors { left: parent.left; right: parent.right; rightMargin: parent.width * 0.3; leftMargin: 8 * Theme.scale }
            spacing: 14 * Theme.scale
            Repeater {
                model: root.devices
                SettingsRow {
                    required property var modelData
                    required property int index
                    width: parent.width
                    label: modelData.name
                    description: root.describe(modelData)
                    value: qsTr("Remove")
                    focused: index === root.focusIndex
                }
            }
            SettingsRow {
                visible: root.devices.length > 0
                width: parent.width
                kind: "danger"
                label: qsTr("Remove all phones")
                focused: root.focusIndex === root.devices.length
            }
            Text {
                visible: root.devices.length === 0
                text: qsTr("Pair a phone from Settings → Pair a phone.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
            }
        }
    }
}
