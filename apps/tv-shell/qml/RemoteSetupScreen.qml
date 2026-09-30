// Phone-remote onboarding. Nothing listens on the LAN until the owner picks
// one network interface here and accepts the exposure (contracts/http.md).

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    property var interfaces: []
    signal openScreen(string name)
    readonly property bool listening: Session.remote.listening === true
    readonly property bool remoteOn: Session.remote.enabled === true
    readonly property var actions: remoteOn
        ? [{ id: "pair", text: qsTr("Pair a phone"), primary: true }, { id: "off", text: qsTr("Turn off phone remote"), danger: true }]
        : interfaces.map(i => ({ id: "on:" + i.name, text: qsTr("Allow on %1").arg(i.label), detail: i.addresses + " · " + i.name, primary: true }))
                    .concat([{ id: "cancel", text: qsTr("Not now") }])

    function enter() {
        interfaces = Shell.lanInterfaces()
        focusIndex = 0
        report()
    }
    function report() { if (actions.length > 0) Nav.reportFocus("remote-setup", actions[focusIndex].id, 0) }
    function navigate(action) {
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.down": focusIndex = Math.min(actions.length - 1, focusIndex + 1); report(); return true
        case "nav.left": case "nav.right": return true
        case "select": {
            const a = actions[focusIndex]
            if (!a) return true
            if (a.id === "pair") openScreen("pairing")
            else if (a.id === "off") Shell.configureRemote(false, "")
            else if (a.id === "cancel") return false
            else if (a.id.startsWith("on:")) Shell.configureRemote(true, a.id.slice(3))
            focusIndex = 0
            return true
        }
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Phone remote")
        subtitle: root.listening ? qsTr("On — phones on your network can pair") : qsTr("Control Bear Den from your phone's browser")

        Row {
            anchors.fill: parent
            spacing: 64 * Theme.scale
            Column {
                width: parent.width * 0.48
                spacing: 22 * Theme.scale
                Text {
                    width: parent.width
                    wrapMode: Text.WordWrap
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 30 * Theme.fontUnit
                    text: root.remoteOn
                          ? qsTr("The phone remote is listening at:")
                          : qsTr("Turning this on makes Bear Den reachable from devices on your home network.")
                }
                Repeater {
                    model: root.remoteOn ? (Session.remote.addresses || []) : []
                    Text {
                        required property string modelData
                        text: modelData
                        color: Theme.accent
                        font.family: Theme.monoFamily
                        font.pixelSize: 30 * Theme.fontUnit
                    }
                }
                Text {
                    visible: root.remoteOn && !root.listening
                    width: parent.width
                    wrapMode: Text.WordWrap
                    color: Theme.warning
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                    text: qsTr("Enabled, but not listening yet. Check Diagnostics if this persists.")
                }
                Text {
                    width: parent.width
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                    lineHeight: 1.2
                    text: qsTr("• Only phones you pair on this TV can control it.\n• It uses plain HTTP on a trusted home network: pairing stops casual control but traffic is not encrypted.\n• It never listens on public, VPN, or container interfaces.\n• You can revoke phones or turn this off at any time.")
                }
                Text {
                    visible: !root.remoteOn && root.interfaces.length === 0
                    width: parent.width
                    wrapMode: Text.WordWrap
                    color: Theme.warning
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                    text: qsTr("No home network connection was found. Connect the TV to your network and come back.")
                }
            }
            Column {
                spacing: 18 * Theme.scale
                width: parent.width * 0.4
                Repeater {
                    model: root.actions
                    FocusButton {
                        required property var modelData
                        required property int index
                        width: parent.width
                        text: modelData.text
                        detail: modelData.detail || ""
                        primary: modelData.primary === true
                        danger: modelData.danger === true
                        focused: index === root.focusIndex
                    }
                }
            }
        }
    }
}
