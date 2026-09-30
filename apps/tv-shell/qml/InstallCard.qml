// The install card: OK on a "Not installed" tile, a card of Apps → Add apps
// or of the first-run setup, or turning a streaming site on while its
// browser is missing opens it
// (docs/decisions/0011-per-user-flathub-installs.md). It shows the app's
// name and Bear Den's own icon, the size Flathub reports (IPC
// app.install_info, asked when the card opens), "From Flathub", and Install
// (focused) / Not now. Install is the owner's consent (IPC app.install);
// while it runs the card shows the progress from state.applications[].install
// (updated on state pushes, no animation of its own), Back hides the card
// and the install carries on, Cancel install stops it. The shell auto-opens
// the app when it is done (ShellRoot). With installs unavailable the card
// says why (Flatpak missing) and offers only OK. Before Install, it also
// lists what is good to know about the app (state.applications[].notes: the
// coordinator's honest caveats, e.g. the streaming sites' 720p).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Rectangle {
    id: root
    objectName: "installCard"
    property string appId: ""
    property int focusIndex: 0
    // The last reply to app.install_info or app.install for this app.
    property string replyError: ""
    property bool asked: false
    visible: false
    color: Theme.scrim

    // Re-read on every snapshot (Session.applications notifies).
    readonly property var app: Session.applications && appId.length > 0 ? Session.application(appId) : ({})
    readonly property var inst: app.install || ({})
    readonly property string adapter: app.adapter || ""
    readonly property string instState: inst.state || "none"
    readonly property bool running: instState === "preparing" || instState === "downloading" || instState === "installing"
    readonly property var cap: Session.capabilities["app.install"] || ({})
    // Why installs are unavailable, when they are (Flatpak missing, an
    // older coordinator, …); "" when Install would work.
    readonly property string unavailable: {
        if (app.installed === true || running || instState === "done") return ""
        if (instState === "available" || instState === "failed") return ""
        return inst.message || cap.reason || qsTr("Bear Den can't install apps on this box.")
    }
    readonly property string what: Apps.installName(adapter)
    readonly property var buttons: {
        if (running) return [["hide", qsTr("Hide")], ["cancel", qsTr("Cancel install")]]
        if (unavailable.length > 0) return [["close", qsTr("OK")]]
        if (instState === "failed") return [["install", qsTr("Try again")], ["close", qsTr("Not now")]]
        return [["install", qsTr("Install")], ["close", qsTr("Not now")]]
    }

    signal installRequested(string appId)

    // forApp: the card was opened from this app's own tile, card or
    // Streaming sites row (not a shared browser card of Add apps), so its
    // Install also turns this app on once installed (IPC app.install
    // "enable"; the streaming sites share their browser's Flatpak).
    property bool forApp: false
    function openFor(id, own) {
        appId = id
        forApp = own === true
        replyError = ""
        focusIndex = 0
        visible = true
        asked = false
        askSize()
        report()
    }
    function askSize() {
        if (asked || instState !== "available" || (inst.size_bytes || 0) > 0) return
        asked = true
        Shell.installInfo(appId)
    }
    function report() { Nav.reportFocus("dialog", buttons.length > 0 ? "install-" + buttons[Math.min(focusIndex, buttons.length - 1)][0] : "install", 0) }
    function close() { visible = false }
    function navigate(action) {
        switch (action) {
        case "nav.left": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.right": focusIndex = Math.min(buttons.length - 1, focusIndex + 1); report(); return true
        case "nav.up": case "nav.down": return true
        case "back": close(); return true   // an install carries on
        case "select": {
            const b = buttons[Math.min(focusIndex, buttons.length - 1)][0]
            if (b === "install") {
                replyError = ""
                Shell.installApp(appId, forApp)
                installRequested(appId)
                focusIndex = 0
            } else if (b === "cancel") {
                Shell.cancelInstall(appId)
                focusIndex = 0
            } else {
                close()
            }
            report()
            return true
        }
        }
        return false
    }
    onButtonsChanged: if (visible) { focusIndex = Math.min(focusIndex, buttons.length - 1); report() }
    onInstStateChanged: if (visible) askSize()

    Connections {
        target: Shell
        function onInstallReplied(type, id, ok, error, data) {
            if (id !== root.appId) return
            root.replyError = ok ? "" : error
        }
    }

    function size(b) {
        if (b >= 1e9) return qsTr("%1 GB").arg((b / 1e9).toFixed(1))
        return qsTr("%1 MB").arg(Math.max(1, Math.round(b / 1e6)))
    }
    readonly property string sizeLine: {
        if ((inst.size_bytes || 0) > 0)
            return qsTr("About %1 to download, %2 on this box").arg(size(inst.size_bytes)).arg(size(inst.disk_bytes || inst.size_bytes))
        if (instState === "available" && replyError.length === 0) return qsTr("Checking the size…")
        return ""
    }
    readonly property string phaseLine: {
        switch (instState) {
        case "preparing": return qsTr("Getting ready…")
        case "downloading": return inst.phase === "app" ? qsTr("Getting %1…").arg(what || app.label || "") : qsTr("Getting shared parts…")
        case "installing": return qsTr("Finishing…")
        case "done": return inst.drm === "pending" ? qsTr("Installed. Still setting up playback support.") : qsTr("Ready")
        }
        return ""
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
                Column {
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 6 * Theme.scale
                    Text {
                        text: root.what.length > 0 ? qsTr("%1 needs %2").arg(root.app.label || "").arg(root.what) : (root.app.label || "")
                        color: Theme.textPrimary
                        font.family: Theme.fontFamily
                        font.pixelSize: 44 * Theme.fontUnit
                        font.weight: Font.Bold
                    }
                    Text {
                        text: root.what.length > 0 ? root.what + " · " + Apps.installWhy(root.adapter) + " · " + qsTr("From Flathub") : qsTr("From Flathub")
                        color: Theme.accent
                        font.family: Theme.fontFamily
                        font.pixelSize: 24 * Theme.fontUnit
                        font.weight: Font.DemiBold
                    }
                }
            }
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                visible: text.length > 0
                text: root.unavailable.length > 0 ? root.unavailable
                     : root.running || root.instState === "done" ? ""
                     : Apps.about(root.adapter)
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                lineHeight: 1.15
            }
            // Good to know, before the owner decides.
            Column {
                objectName: "installCardNotes"
                width: parent.width
                spacing: 6 * Theme.scale
                visible: !root.running && root.instState !== "done" && (root.app.notes || []).length > 0
                Text {
                    text: qsTr("Good to know")
                    color: Theme.accent
                    font.family: Theme.fontFamily
                    font.pixelSize: 20 * Theme.fontUnit
                    font.weight: Font.Bold
                    font.letterSpacing: 2 * Theme.scale
                }
                Repeater {
                    model: root.app.notes || []
                    Text {
                        required property string modelData
                        objectName: "installCardNote"
                        width: parent.width
                        wrapMode: Text.WordWrap
                        text: "• " + modelData
                        color: Theme.textPrimary
                        font.family: Theme.fontFamily
                        font.pixelSize: 22 * Theme.fontUnit
                    }
                }
            }
            // Progress: from the state pushes only.
            Column {
                width: parent.width
                spacing: 12 * Theme.scale
                visible: root.running || root.instState === "done"
                Row {
                    width: parent.width
                    Text {
                        width: parent.width - pct.width
                        text: root.phaseLine
                        color: Theme.textPrimary
                        font.family: Theme.fontFamily
                        font.pixelSize: 26 * Theme.fontUnit
                    }
                    Text {
                        id: pct
                        text: (root.instState === "done" ? 100 : (root.inst.progress || 0)) + "%"
                        color: Theme.accent
                        font.family: Theme.fontFamily
                        font.pixelSize: 26 * Theme.fontUnit
                        font.weight: Font.DemiBold
                    }
                }
                ProgressBar {
                    objectName: "installCardProgress"
                    width: parent.width
                    height: 14 * Theme.scale
                    value: root.instState === "done" ? 1 : (root.inst.progress || 0) / 100
                }
            }
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                visible: text.length > 0
                text: root.sizeLine
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Text {
                objectName: "installCardError"
                width: parent.width
                wrapMode: Text.WordWrap
                visible: text.length > 0
                text: root.instState === "failed" ? (root.inst.message || qsTr("The install stopped.")) : root.replyError
                color: Theme.danger
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                visible: root.running
                text: qsTr("Back hides this; the install carries on and the tile shows how far it is.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
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
                        primary: index === 0 && modelData[0] !== "close"
                        danger: modelData[0] === "cancel"
                        focused: root.focusIndex === index
                    }
                }
            }
        }
    }
}
