// Read-only status for troubleshooting: Bear Den's helper (the
// coordinator), the desktop, what is in front, the phone remote, the apps
// and why each remote control is or is not available. Labels in plain
// words (UX-22); raw codes stay in the values.

import QtQuick
import BearDen

Item {
    id: root
    property int scrollStep: 0
    function enter() { scrollStep = 0; Nav.reportFocus("diagnostics", "top", 0) }
    function navigate(action) {
        switch (action) {
        case "nav.down": view.contentY = Math.min(Math.max(0, view.contentHeight - view.height), view.contentY + 240 * Theme.scale); return true
        case "nav.up": view.contentY = Math.max(0, view.contentY - 240 * Theme.scale); return true
        case "nav.left": case "nav.right": case "select": return true
        }
        return false
    }

    readonly property var s: Session.session
    readonly property var t: Session.target
    readonly property var r: Session.remote
    readonly property var groups: [
        { title: qsTr("Bear Den"), rows: [
            [qsTr("Bear Den's helper"), Shell.connected ? qsTr("Connected") : (Shell.offline ? qsTr("Offline preview") : qsTr("Not connected (%1)").arg(Shell.connectionState))],
            [qsTr("TV screen version"), Shell.version],
            [qsTr("Updates received"), String(Session.contextEpoch)],
            [qsTr("Settings saved"), String(Session.configRevision)],
            [qsTr("Development mode"), Session.devMode ? qsTr("Yes (DEMO data)") : qsTr("No")]] },
        { title: qsTr("Desktop"), rows: [
            [qsTr("Desktop"), (s.display_session || "?").toUpperCase() + " (" + (s.desktop_adapter || "?") + ")"],
            [qsTr("Locked"), s.locked ? qsTr("Yes") : qsTr("No")],
            [qsTr("TV screen"), s.shell_state || "?"],
            [qsTr("In front"), (t.label || "?") + " (" + (t.kind || "?") + (t.observed ? qsTr(", verified") : qsTr(", not verified")) + ")"]] },
        { title: qsTr("Phone remote"), rows: [
            [qsTr("Enabled"), r.enabled ? qsTr("Yes") : qsTr("No")],
            [qsTr("Listening"), r.listening ? (r.addresses || []).join("  ") : qsTr("No")],
            [qsTr("Connection"), r.transport === "https" ? qsTr("Encrypted") : r.transport === "trusted-lan-http" ? qsTr("Home network, not encrypted") : (r.transport || "?")],
            [qsTr("Paired phones"), String(r.paired_device_count || 0)]] },
        { title: qsTr("Applications"), rows: Session.applications.map(a => [a.label,
            (a.installed ? qsTr("Installed") + (a.version ? " " + a.version : "") + " (" + a.installation + ")" : qsTr("Not installed")) + " · " + a.launch_state
            + (a.last_error ? " · " + a.last_error : "")]) },
        { title: qsTr("Remote controls right now"), rows: Object.keys(Session.capabilities).sort().map(k => {
            const c = Session.capabilities[k]
            return [k, c.available ? qsTr("Available") + (c.backend ? " · " + c.backend : "") : (c.reason || qsTr("Unavailable"))] }) }
    ]

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Diagnostics")
        panel: true
        subtitle: qsTr("For troubleshooting; no private titles or addresses leave this TV")
        hints: [["▲ ▼", qsTr("Scroll")], ["Back", qsTr("Back")]]

        Flickable {
            id: view
            anchors.fill: parent
            contentHeight: col.height
            interactive: false
            clip: true
            Behavior on contentY { enabled: !Theme.reducedMotion; NumberAnimation { duration: Theme.duration } }
            Column {
                id: col
                width: view.width
                spacing: 28 * Theme.scale
                Repeater {
                    model: root.groups
                    Column {
                        required property var modelData
                        width: col.width
                        spacing: 8 * Theme.scale
                        Text {
                            text: modelData.title
                            color: Theme.accent
                            font.family: Theme.fontFamily
                            font.pixelSize: 28 * Theme.fontUnit
                            font.weight: Font.DemiBold
                        }
                        Repeater {
                            model: modelData.rows
                            Row {
                                required property var modelData
                                spacing: 24 * Theme.scale
                                Text {
                                    width: col.width * 0.28
                                    text: modelData[0]
                                    elide: Text.ElideRight
                                    color: Theme.textSecondary
                                    font.family: Theme.fontFamily
                                    font.pixelSize: 22 * Theme.fontUnit
                                }
                                Text {
                                    width: col.width * 0.68
                                    text: modelData[1]
                                    wrapMode: Text.WordWrap
                                    color: Theme.textPrimary
                                    font.family: Theme.fontFamily
                                    font.pixelSize: 22 * Theme.fontUnit
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
