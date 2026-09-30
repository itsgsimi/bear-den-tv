// Paired phones with connection state. On a family phone ◀ ▶ choose what OK
// does: Remove, or Make owner (Remote only for an owner), each after a
// confirmation (Cancel focused). Make owner grants owner with layout
// editing (IPC devices.grant: the TV is trusted; UX-05: nothing could make
// a phone the owner, so Add apps and Layout on the phone never showed);
// Remote only takes both back. The last row revokes all. Guest passes
// (devices[].guest) carry a Guest badge and the time they end and have
// left (contracts/http.md#guest-passes), and can only be removed.

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    signal confirm(string title, string body, string confirmLabel, var onAccept, bool danger)
    signal openScreen(string name)
    readonly property var devices: Session.devices
    readonly property int rowCount: devices.length + (devices.length > 0 ? 1 : 0)
    // The action ◀ ▶ picked on the focused row (index into actionsFor).
    property int actionIndex: 0
    function isOwner(d) { return d.permissions.indexOf("owner") >= 0 }
    // What OK can do to a phone: [id, label]; Remove first.
    function actionsFor(d) {
        if (d.guest === true) return [["remove", qsTr("Remove")]]
        return [["remove", qsTr("Remove")], isOwner(d) ? ["remote-only", qsTr("Remote only")] : ["owner", qsTr("Make owner")]]
    }
    function act(d, a) {
        switch (a) {
        case "remove":
            confirm(qsTr("Remove “%1”?").arg(d.name), qsTr("This phone will stop controlling the TV immediately and must pair again."),
                    qsTr("Remove"), () => Shell.revokeDevice(d.id), true)
            return
        case "owner":
            confirm(qsTr("Make “%1” an owner?").arg(d.name),
                    qsTr("It can then install and remove apps, manage paired phones and edit the Home layout, as the TV can. Make only your own phones owners."),
                    qsTr("Make owner"), () => Shell.grantDevice(d.id, ["controller", "layout_editor", "owner"]), false)
            return
        case "remote-only":
            confirm(qsTr("Make “%1” a remote only?").arg(d.name),
                    qsTr("It keeps the remote, but can no longer install apps, manage phones or edit the layout."),
                    qsTr("Remote only"), () => Shell.grantDevice(d.id, ["controller"]), false)
            return
        }
    }

    // Wall-clock now for "time left"; refreshed on entry and every 30 s while
    // shown (text only, no motion).
    property double now: Date.now()
    Timer { interval: 30000; running: root.visible; repeat: true; onTriggered: root.now = Date.now() }

    function enter() { now = Date.now(); focusIndex = 0; actionIndex = 0; report() }
    function timeLeft(ms) {
        const min = Math.max(0, Math.round((ms - root.now) / 60000))
        if (min < 60) return qsTr("%n min left", "", min)
        const h = Math.round(min / 60)
        if (h < 48) return qsTr("%n h left", "", h)
        const days = Math.round(h / 24)
        return days === 1 ? qsTr("1 day left") : qsTr("%1 days left").arg(days)
    }
    // Times in the header clock's format (UX-28: a 24-hour pass end beside
    // a 12-hour clock).
    function clockTime(d) { return Qt.formatTime(d, Qt.locale().timeFormat(Locale.ShortFormat)) }
    function endsAt(ms) {
        const d = new Date(ms)
        return ms - root.now < 20 * 3600 * 1000 ? clockTime(d) : Qt.formatDate(d, "ddd") + " " + clockTime(d)
    }
    function pairedOn(iso) {
        const d = new Date(iso)
        return isNaN(d.getTime()) ? "" : qsTr("paired %1").arg(Qt.formatDate(d, "ddd d MMM") + " " + clockTime(d))
    }
    function report() {
        const id = focusIndex < devices.length ? devices[focusIndex].id : (devices.length > 0 ? "revoke-all" : "pair")
        Nav.reportFocus("devices", id, 0)
    }
    function describe(d) {
        const role = d.guest && d.expires_at_ms ? qsTr("Guest pass · ends %1 (%2)").arg(endsAt(d.expires_at_ms)).arg(timeLeft(d.expires_at_ms))
                   : d.permissions.indexOf("owner") >= 0 ? qsTr("Owner") : (d.permissions.indexOf("layout_editor") >= 0 ? qsTr("Can edit layout") : qsTr("Remote control"))
        const when = d.guest ? "" : pairedOn(d.created_at || "")
        return (d.connected ? qsTr("Connected now") : qsTr("Not connected")) + " · " + role + (when ? " · " + when : "")
    }
    function navigate(action) {
        // Nothing paired: the one thing to do is pair one (UX-17).
        if (devices.length === 0) {
            if (action === "select") { openScreen("pairing"); return true }
            return action !== "back"
        }
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); actionIndex = 0; report(); return true
        case "nav.down": focusIndex = Math.min(Math.max(0, rowCount - 1), focusIndex + 1); actionIndex = 0; report(); return true
        case "nav.left": case "nav.right":
            if (focusIndex < devices.length) {
                const n = actionsFor(devices[focusIndex]).length
                actionIndex = (actionIndex + (action === "nav.left" ? n - 1 : 1)) % n
                report()
            }
            return true
        case "select":
            if (focusIndex < devices.length) {
                const d = devices[focusIndex]
                const acts = actionsFor(d)
                act(d, acts[Math.min(actionIndex, acts.length - 1)][0])
            } else if (devices.length > 0) {
                confirm(qsTr("Remove all phones?"), qsTr("Every paired phone stops controlling the TV immediately."),
                        qsTr("Remove all"), () => Shell.revokeDevice("*"), true)
            }
            return true
        }
        return false
    }
    onRowCountChanged: focusIndex = Math.min(focusIndex, Math.max(0, rowCount - 1))

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Paired phones")
        subtitle: root.devices.length === 0 ? qsTr("No phones are paired")
                  : root.devices.length === 1 ? qsTr("1 phone can control this TV") : qsTr("%1 phones can control this TV").arg(root.devices.length)
        hints: root.devices.length === 0 ? [["OK", qsTr("Pair a phone")], ["Back", qsTr("Back")]]
                                         : [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Choose")], ["OK", qsTr("Do it")], ["Back", qsTr("Back")]]

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
                    readonly property var acts: root.actionsFor(modelData)
                    objectName: "deviceRow"
                    badge: modelData.guest === true ? qsTr("Guest") : ""
                    kind: acts.length > 1 && focused ? "choice" : "link"
                    value: acts[focused ? Math.min(root.actionIndex, acts.length - 1) : 0][1]
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
                text: qsTr("No phone can control this TV yet.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
            }
            FocusButton {
                objectName: "pairFromDevices"
                visible: root.devices.length === 0
                text: qsTr("Pair a phone")
                primary: true
                focused: true
            }
        }
    }
}
