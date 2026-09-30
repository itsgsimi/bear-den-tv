// The remove card: OK on a card of Apps → Remove apps opens it (TV test
// 2026-09-29 item 9; contracts/actions.md "App removal"). It names what is
// removed (for a web app, its browser), the size it frees
// (state.applications[].install.installed_bytes: the app itself, its shared
// runtime stays), which other apps it turns off (the apps that run in the
// same Flatpak, from the browser table: Shell.flatpakIdFor), and that the
// app's sign-ins and settings stay unless its data goes too. Buttons:
// Cancel (focused), Remove, Remove and delete its data; each Remove sends
// IPC app.uninstall (Shell.uninstallApp). While flatpak works the card says
// "Removing …" (install.state removing); once the app is gone it closes and
// ShellRoot says so with what it freed (the app and the shared parts
// nothing else uses, measured by the coordinator). A system-wide install is refused plainly with OK only;
// the coordinator's refusals and failures (a running app, flatpak's error)
// show in the card. Nothing is removed without one of the Remove presses.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Rectangle {
    id: root
    objectName: "removeCard"
    property string appId: ""
    property int focusIndex: 0
    property string replyError: ""
    // Remove was pressed: watch for the app to go.
    property bool pressed: false
    visible: false
    color: Theme.scrim

    // message: what the removal freed ("Removed. It freed about 2.5 GB.").
    signal removed(string name, string message)

    readonly property var app: Session.applications && appId.length > 0 ? Session.application(appId) : ({})
    readonly property var inst: app.install || ({})
    readonly property string adapter: app.adapter || ""
    readonly property string name: Apps.installName(adapter) || app.label || ""
    readonly property bool system: app.installation === "system"
    readonly property bool removing: inst.state === "removing"
    // The other apps that run in the same Flatpak and are on: removing it
    // turns them off (the streaming sites share their browser).
    readonly property var alsoOff: {
        const fid = Shell.flatpakIdFor(adapter)
        if (!fid) return []
        return (Session.applications || []).filter(a => (a.id !== appId || name !== app.label)
            && Shell.flatpakIdFor(a.adapter) === fid && a.enabled !== false && a.hidden !== true)
            .map(a => a.label)
    }
    readonly property var buttons: {
        if (removing) return [["hide", qsTr("Hide")]]
        if (system || (pressed && replyError.length > 0)) return [["close", qsTr("OK")]]
        return [["close", qsTr("Cancel")], ["remove", qsTr("Remove")], ["remove-data", qsTr("Remove and delete its data")]]
    }

    function openFor(id) {
        appId = id
        replyError = ""
        pressed = false
        focusIndex = 0
        visible = true
        report()
    }
    function report() { Nav.reportFocus("dialog", "remove-" + buttons[Math.min(focusIndex, buttons.length - 1)][0], 0) }
    function close() { visible = false }
    function join(list) {
        if (list.length <= 1) return list.join("")
        return qsTr("%1 and %2").arg(list.slice(0, -1).join(", ")).arg(list[list.length - 1])
    }
    function size(b) {
        if (b >= 1e9) return qsTr("%1 GB").arg((b / 1e9).toFixed(1))
        return qsTr("%1 MB").arg(Math.max(1, Math.round(b / 1e6)))
    }
    function navigate(action) {
        switch (action) {
        case "nav.left": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.right": focusIndex = Math.min(buttons.length - 1, focusIndex + 1); report(); return true
        case "nav.up": case "nav.down": return true
        case "back": close(); return true   // a removal carries on
        case "select": {
            const b = buttons[Math.min(focusIndex, buttons.length - 1)][0]
            if (b === "remove" || b === "remove-data") {
                replyError = ""
                pressed = true
                Shell.uninstallApp(appId, b === "remove-data")
                focusIndex = 0
                report()
            } else {
                close()
            }
            return true
        }
        }
        return false
    }
    onButtonsChanged: if (visible) { focusIndex = Math.min(focusIndex, buttons.length - 1); report() }

    Connections {
        target: Shell
        function onInstallReplied(type, id, ok, error, data) {
            if (type !== "app.uninstall" || id !== root.appId) return
            root.replyError = ok ? "" : error
        }
    }
    // Gone: close and say so.
    Connections {
        target: Session
        function onSnapshotChanged() {
            if (root.visible && root.pressed && root.app.installed === false) {
                root.pressed = false
                root.close()
                root.removed(root.name, root.inst.message || "")
            }
        }
    }

    PixelBox {
        anchors.centerIn: parent
        width: Math.min(parent.width * 0.6, 1100 * Theme.scale)
        height: content.height + 96 * Theme.scale
        radius: Theme.radius * 1.4
        color: Theme.surfaceRaised
        borderColor: Theme.surfaceBorder
        Column {
            id: content
            anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 56 * Theme.scale }
            spacing: 22 * Theme.scale
            Row {
                spacing: 28 * Theme.scale
                AppIcon {
                    adapter: root.adapter
                    label: root.app.label || ""
                    size: 112 * Theme.scale
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    objectName: "removeCardTitle"
                    anchors.verticalCenter: parent.verticalCenter
                    width: content.width - 140 * Theme.scale
                    wrapMode: Text.WordWrap
                    text: root.removing ? qsTr("Removing %1…").arg(root.name)
                        : root.system ? qsTr("%1 can't be removed here").arg(root.name)
                        : qsTr("Remove %1?").arg(root.name)
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 44 * Theme.fontUnit
                    font.weight: Font.Bold
                }
            }
            Text {
                objectName: "removeCardBody"
                width: parent.width
                wrapMode: Text.WordWrap
                text: {
                    if (root.system)
                        return qsTr("It is installed for everyone on this PC, so only the PC's own software tool can remove it.")
                    const lines = []
                    if ((root.inst.installed_bytes || 0) > 0)
                        lines.push(qsTr("Frees about %1 on this box, and more if no other app uses its shared parts.").arg(root.size(root.inst.installed_bytes)))
                    if (root.alsoOff.length === 1)
                        lines.push(qsTr("%1 uses %2, so removing it turns %1 off.").arg(root.alsoOff[0]).arg(root.name))
                    else if (root.alsoOff.length > 1)
                        lines.push(qsTr("%1 use %2, so removing it turns them off.").arg(root.join(root.alsoOff)).arg(root.name))
                    if (!root.removing)
                        lines.push(qsTr("Your sign-ins and settings in %1 stay on this box unless you also delete its data. You can install it again from Add apps.").arg(root.name))
                    return lines.join("\n")
                }
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                lineHeight: 1.15
            }
            Text {
                objectName: "removeCardError"
                width: parent.width
                wrapMode: Text.WordWrap
                visible: text.length > 0
                text: root.replyError.length > 0 ? root.replyError : (!root.removing && root.pressed ? (root.inst.message || "") : "")
                color: Theme.danger
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Row {
                spacing: 20 * Theme.scale
                topPadding: 8 * Theme.scale
                Repeater {
                    model: root.buttons
                    FocusButton {
                        required property var modelData
                        required property int index
                        text: modelData[1]
                        danger: modelData[0] === "remove" || modelData[0] === "remove-data"
                        focused: root.focusIndex === index
                    }
                }
            }
        }
    }
}
