// Settings → Streaming sites: turn each web app (Netflix, Disney+, Hulu and
// the Browser; state.applications[] entries that carry `enabled`) on or off.
// OK toggles through IPC app.enable (contracts/ipc.md); the coordinator
// stores it in config.json and a new snapshot follows. A web app that is off
// has no tile on Home. The rows come from the snapshot, never from display
// names: an entry is listed because it has `enabled`. Turning a site on
// while Chromium is missing opens the install card for it (ADR 0011).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    // Set by ShellRoot: this screen is the one in front.
    property bool active: false

    readonly property var sites: Session.applications.filter(a => a.enabled !== undefined)
    signal openInstall(string appId)

    function enter() { focusIndex = Math.min(focusIndex, Math.max(0, sites.length - 1)); report() }
    function report() { Nav.reportFocus("streaming", sites.length > 0 ? sites[focusIndex].id : "none", 0) }
    function move(to) {
        focusIndex = Math.max(0, Math.min(sites.length - 1, to))
        report()
    }
    function describe(app) {
        const i = app.install || {}
        if (!app.installed && (i.state === "preparing" || i.state === "downloading" || i.state === "installing"))
            return qsTr("Installing Chromium… %1%").arg(i.progress)
        if (!app.installed)
            return qsTr("Needs Chromium from Flathub: OK to install it")
        if (i.drm === "preparing" || (app.enabled === true && i.drm === "pending"))
            return qsTr("Still setting up playback support")
        return Apps.hint(app.adapter) || Apps.tagline(app.adapter)
    }
    function navigate(action) {
        switch (action) {
        case "nav.up": move(focusIndex - 1); return true
        case "nav.down": move(focusIndex + 1); return true
        case "nav.left":
        case "nav.right": return true
        case "select": {
            if (sites.length === 0) return true
            const site = sites[focusIndex]
            const on = site.enabled !== true
            Shell.setAppEnabled(site.id, on)
            // Chromium missing: the same install card as a Home tile.
            if (on && site.installed === false) openInstall(site.id)
            return true
        }
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Streaming sites")
        subtitle: qsTr("Websites in Chromium, driven by your remote. Off: no tile on Home.")
        hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("On / off")], ["Back", qsTr("Back")]]

        Column {
            id: list
            width: parent.width * 0.62
            x: 20 * Theme.scale
            spacing: 14 * Theme.scale
            Repeater {
                model: root.sites
                SettingsRow {
                    required property var modelData
                    required property int index
                    objectName: "streamingRow"
                    width: list.width
                    kind: "toggle"
                    label: modelData.label
                    description: root.describe(modelData)
                    value: modelData.enabled === true ? "on" : "off"
                    focused: root.focusIndex === index
                }
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
            text: qsTr("These services have no Linux apps. In a Linux browser they play at up to 720p, and they need Chromium's Widevine module.\n\nSign in once inside each site with a keyboard, or with the Touchpad on your phone. Each site keeps its own sign-in on this TV.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 24 * Theme.fontUnit
        }
    }
}
