// Settings → Add apps: every app Bear Den knows (the adapter table's apps in
// state.applications) that is not installed, core and optional, one row per
// Flatpak, so the four web apps are one row for Chromium ("Browser for
// Netflix, Disney+, Hulu"). OK opens the install card (InstallCard.qml);
// each row shows its install's progress from state.applications[].install.
// Optional apps appear on Home once installed (hide_when_missing).
// Spec: docs/decisions/0011-per-user-flathub-installs.md.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    property bool active: false
    signal openInstall(string appId)

    // One entry per Flatpak id, first app wins (the rows come from the
    // snapshot, never from display names).
    readonly property var missing: {
        const out = []
        const seen = {}
        for (const a of Session.applications) {
            if (a.installed === true || a.install === undefined) continue
            const key = Shell.flatpakIdFor(a.adapter) || a.id
            if (seen[key]) continue
            seen[key] = true
            out.push(a)
        }
        return out
    }
    readonly property var cap: Session.capabilities["app.install"] || ({})

    function enter() { focusIndex = Math.min(focusIndex, Math.max(0, missing.length - 1)); report() }
    function report() { Nav.reportFocus("add-apps", missing.length > 0 ? missing[focusIndex].id : "none", 0) }
    function move(to) {
        focusIndex = Math.max(0, Math.min(missing.length - 1, to))
        report()
    }
    onMissingChanged: if (active && focusIndex >= missing.length) move(missing.length - 1)
    function label(app) { return Apps.installName(app.adapter) || app.label }
    function describe(app) {
        const i = app.install
        if (i.state === "failed") return i.message || qsTr("The last install stopped.")
        if (i.state === "none" && i.message) return i.message
        return Apps.installWhy(app.adapter) || Apps.tagline(app.adapter)
    }
    function value(app) {
        switch (app.install.state) {
        case "preparing": return qsTr("Getting ready…")
        case "downloading": return qsTr("Installing %1%").arg(app.install.progress)
        case "installing": return qsTr("Finishing…")
        case "done": return qsTr("Ready")
        case "failed": return qsTr("Try again")
        case "available": return qsTr("Install")
        }
        return qsTr("Can't install")
    }
    function navigate(action) {
        switch (action) {
        case "nav.up": move(focusIndex - 1); return true
        case "nav.down": move(focusIndex + 1); return true
        case "nav.left":
        case "nav.right": return true
        case "select":
            if (missing.length > 0) openInstall(missing[focusIndex].id)
            return true
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Add apps")
        subtitle: qsTr("Apps Bear Den knows, installed from Flathub for this TV's user. No password needed.")
        hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("Install")], ["Back", qsTr("Back")]]

        Column {
            id: list
            width: parent.width * 0.62
            x: 20 * Theme.scale
            spacing: 14 * Theme.scale
            Repeater {
                model: root.missing
                Item {
                    required property var modelData
                    required property int index
                    readonly property var inst: modelData.install
                    readonly property bool running: inst.state === "preparing" || inst.state === "downloading" || inst.state === "installing"
                    width: list.width
                    height: row.height
                    SettingsRow {
                        id: row
                        objectName: "addAppsRow"
                        width: parent.width
                        kind: "link"
                        label: root.label(parent.modelData)
                        description: root.describe(parent.modelData)
                        value: root.value(parent.modelData)
                        focused: root.focusIndex === parent.index
                    }
                    ProgressBar {
                        visible: parent.running
                        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: 32 * Theme.scale; rightMargin: 32 * Theme.scale; bottomMargin: 10 * Theme.scale }
                        value: (parent.inst.progress || 0) / 100
                    }
                }
            }
            Text {
                visible: root.missing.length === 0
                width: list.width
                wrapMode: Text.WordWrap
                text: root.cap.available === false && root.cap.reason ? root.cap.reason : qsTr("Every app Bear Den knows is installed.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
            }
        }
        PixelBox {
            id: about
            anchors { left: list.right; leftMargin: 40 * Theme.scale; right: parent.right; rightMargin: 20 * Theme.scale; top: list.top }
            height: aboutText.implicitHeight + 56 * Theme.scale
            radius: 18 * Theme.scale
            color: Theme.surface
            borderColor: Theme.surfaceBorder
            borderWidth: 1
        }
        Text {
            id: aboutText
            anchors { left: about.left; right: about.right; top: about.top; margins: 28 * Theme.scale }
            wrapMode: Text.Wrap
            text: qsTr("Installs go to this user's Flatpak folder, from Flathub only. Nothing installs until you press Install.\n\nKeep apps up to date (in Settings) updates them while the TV is idle.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 24 * Theme.fontUnit
        }
    }
}
