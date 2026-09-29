// Settings → Streaming sites: turn each web app (Netflix, Disney+, Hulu and
// the Browser; state.applications[] entries that carry `enabled`) on or off,
// and choose the browsers they run in (state.apps.browsers, a table from the
// coordinator): one for the Browser tile (apps.browser) and one for the
// streaming sites (apps.streaming_browser). OK toggles a site through IPC
// app.enable; OK, ◀ or ▶ on a browser row sends apps.browser
// (contracts/ipc.md); the coordinator stores both in config.json and a new
// snapshot follows. A browser marked streaming_unverified says so on the
// streaming row. A web app that is off has no tile on Home. The rows come
// from the snapshot, never from display names: an entry is listed because
// it has `enabled`. Turning a site on while its browser is missing opens the
// install card for it (ADR 0011). Spec: docs/decisions/0013-brave-as-a-browser-choice.md.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    // Set by ShellRoot: this screen is the one in front.
    property bool active: false

    readonly property var sites: Session.applications.filter(a => a.enabled !== undefined)
    // The browser table and the two choices (older coordinators send none:
    // then there are no browser rows).
    readonly property var browsers: (Session.apps && Session.apps.browsers) || []
    readonly property string browserId: (Session.apps && Session.apps.browser) || "chromium"
    readonly property string streamingId: (Session.apps && Session.apps.streaming_browser) || "chromium"
    readonly property var choices: browsers.length > 1 ? ["browser", "streaming"] : []
    readonly property int count: sites.length + choices.length
    signal openInstall(string appId)

    function browserEntry(id) {
        for (const b of browsers)
            if (b.id === id) return b
        return { id: id, label: id, streaming_unverified: false }
    }
    function enter() { focusIndex = Math.min(focusIndex, Math.max(0, count - 1)); report() }
    function report() {
        const id = focusIndex < sites.length ? sites[focusIndex].id
                 : focusIndex - sites.length < choices.length ? "browser-" + choices[focusIndex - sites.length] : "none"
        Nav.reportFocus("streaming", id, 0)
    }
    function move(to) {
        focusIndex = Math.max(0, Math.min(count - 1, to))
        report()
    }
    function describe(app) {
        const i = app.install || {}
        const browser = Apps.browserLabel(app.adapter)
        if (!app.installed && (i.state === "preparing" || i.state === "downloading" || i.state === "installing"))
            return qsTr("Installing %1… %2%").arg(browser).arg(i.progress)
        if (!app.installed)
            return qsTr("Needs %1 from Flathub: OK to install it").arg(browser)
        if (i.drm === "preparing" || (app.enabled === true && i.drm === "pending"))
            return qsTr("Still setting up playback support")
        return Apps.hint(app.adapter) || Apps.tagline(app.adapter)
    }
    function choiceRow(which) {
        const b = browserEntry(which === "browser" ? browserId : streamingId)
        if (which === "browser")
            return { label: qsTr("Browser tile uses"), value: b.label,
                     description: qsTr("The Browser tile opens in %1").arg(b.label), note: false }
        if (b.streaming_unverified)
            return { label: qsTr("Streaming sites use"), value: b.label,
                     description: qsTr("Unverified for streaming: the sites may not play in %1 (Widevine)").arg(b.label), note: true }
        return { label: qsTr("Streaming sites use"), value: b.label,
                 description: qsTr("Netflix, Disney+ and Hulu open in %1").arg(b.label), note: false }
    }
    // Next browser in the table's order for a choice row; both are sent.
    function cycle(which, delta) {
        const ids = browsers.map(b => b.id)
        const cur = which === "browser" ? browserId : streamingId
        let i = ids.indexOf(cur)
        if (i < 0) i = 0
        const next = ids[(i + delta + ids.length) % ids.length]
        if (which === "browser") Shell.setBrowsers(next, streamingId)
        else Shell.setBrowsers(browserId, next)
    }
    function navigate(action) {
        const choice = focusIndex >= sites.length ? choices[focusIndex - sites.length] : ""
        switch (action) {
        case "nav.up": move(focusIndex - 1); return true
        case "nav.down": move(focusIndex + 1); return true
        case "nav.left":
            if (choice) cycle(choice, -1)
            return true
        case "nav.right":
            if (choice) cycle(choice, 1)
            return true
        case "select": {
            if (choice) { cycle(choice, 1); return true }
            if (sites.length === 0) return true
            const site = sites[focusIndex]
            const on = site.enabled !== true
            Shell.setAppEnabled(site.id, on)
            // Its browser missing: the same install card as a Home tile.
            if (on && site.installed === false) openInstall(site.id)
            return true
        }
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Streaming sites")
        subtitle: qsTr("Websites in a browser, driven by your remote. Off: no tile on Home.")
        hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("On / off")], ["◀ ▶", qsTr("Browser")], ["Back", qsTr("Back")]]

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
            Repeater {
                model: root.choices
                SettingsRow {
                    required property var modelData
                    required property int index
                    readonly property var row: root.choiceRow(modelData)
                    objectName: "browserRow"
                    width: list.width
                    kind: "choice"
                    label: row.label
                    description: row.description
                    value: row.value
                    focused: root.focusIndex === root.sites.length + index
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
            text: qsTr("These services have no Linux apps. In a Linux browser they play at up to 720p, and they need the browser's Widevine module.\n\nSign in once inside each site with a keyboard, or with the Touchpad on your phone. Each site keeps its own sign-in on this TV.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 24 * Theme.fontUnit
        }
    }
}
