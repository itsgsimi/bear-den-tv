// The Apps page (header pill Apps): Bear Den's little app store, one page from
// top to bottom:
//   On this TV       the installed apps that have a tile (a strip of cards;
//                    OK opens one)
//   Add apps         every app Bear Den knows that is not installed and
//                    could be (state.applications[].install), one card per
//                    Flatpak, so the web apps are one card per browser;
//                    OK opens the install card (InstallCard.qml), which shows
//                    the honest size and installs only on its Install press;
//                    cards show progress from state pushes. ADR 0011.
//   Streaming sites  one toggle per web app (the entries that carry
//                    `enabled`; OK sends Shell.setAppEnabled → IPC
//                    app.enable; turning a site on while its browser is
//                    missing opens the install card), then "Browser tile
//                    uses" / "Streaming sites use" from Session.apps.browsers
//                    (◀ ▶ or OK send Shell.setBrowsers → IPC apps.browser; a
//                    browser with streaming_unverified says so; the help
//                    panel shows the focused browser's notes). ADR 0013,
//                    ADR 0014.
//   Updates          Keep apps up to date (Shell.setAutoUpdate → IPC
//                    apps.configure).
// Beside the list, the focused entry's help (Help.qml) and, for an app, its
// notes (state.applications[].notes). Everything comes from the snapshot,
// never from display names. `focusSection("add-apps" | "streaming")` is how
// Home's Add apps tile and the old screen names land on a section.
// Reports focus as sections `apps-installed`, `add-apps`, `streaming`, `apps`.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    objectName: "appsScreen"
    property bool active: false
    property int line: 0
    property int installedIndex: 0
    property int addIndex: 0
    // forApp: opened for that app itself (its Streaming sites row), not a
    // shared browser card (InstallCard.forApp).
    signal openInstall(string appId, bool forApp)

    // Installed apps with a tile on Home.
    readonly property var installed: Session.applications.filter(a => a.installed === true && a.hidden !== true)
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
    // Why there is nothing to add: everything is installed, or installs are
    // not possible here (and then some app may still be missing).
    readonly property string noAddReason: {
        if (cap.available === false && cap.reason) return cap.reason
        if (Session.applications.some(a => a.installed !== true && a.hidden !== true))
            return qsTr("Bear Den can't install apps on this TV, so apps that are not installed are left to you.")
        return qsTr("Every app Bear Den knows is installed.")
    }
    readonly property var sites: Session.applications.filter(a => a.enabled !== undefined)
    // The browser table and the two choices (older coordinators send none:
    // then there are no browser rows).
    readonly property var browsers: (Session.apps && Session.apps.browsers) || []
    readonly property string browserId: (Session.apps && Session.apps.browser) || "brave"
    readonly property string streamingId: (Session.apps && Session.apps.streaming_browser) || "chrome"
    readonly property var choices: browsers.length > 1 ? ["browser", "streaming"] : []
    readonly property bool hasApps: Session.apps !== undefined && Session.apps.auto_update !== undefined

    // Every focusable line, top to bottom.
    readonly property var lines: {
        const out = []
        if (installed.length > 0) out.push({ kind: "strip", id: "installed" })
        if (missing.length > 0) out.push({ kind: "strip", id: "add" })
        for (const s of sites) out.push({ kind: "site", id: s.id, app: s })
        for (const c of choices) out.push({ kind: "browser", id: "browser-" + c, which: c })
        if (hasApps) out.push({ kind: "toggle", id: "auto-update" })
        return out
    }
    readonly property var current: lines[Math.max(0, Math.min(lines.length - 1, line))] || ({ kind: "none", id: "none" })
    function lineOf(id) {
        for (let i = 0; i < lines.length; ++i)
            if (lines[i].id === id) return i
        return -1
    }

    // Opened from Home's top bar: from the top.
    function opened() { line = 0; installedIndex = 0; addIndex = 0 }
    function enter() {
        line = Math.max(0, Math.min(line, lines.length - 1))
        installedIndex = Math.max(0, Math.min(installedIndex, installed.length - 1))
        addIndex = Math.max(0, Math.min(addIndex, missing.length - 1))
        report()
    }
    // Land on a section: "add-apps" (Home's Add apps tile), "streaming".
    function focusSection(name) {
        let i = -1
        if (name === "add-apps") i = lineOf("add")
        else if (name === "streaming" && sites.length > 0) i = lineOf(sites[0].id)
        if (i >= 0) line = i
        report()
    }
    function report() {
        const c = current
        switch (c.kind) {
        case "strip": {
            const a = c.id === "installed" ? installed[installedIndex] : missing[addIndex]
            Nav.reportFocus(c.id === "installed" ? "apps-installed" : "add-apps", a ? a.id : "none", 0)
            return
        }
        case "site": case "browser": Nav.reportFocus("streaming", c.id, 0); return
        case "toggle": Nav.reportFocus("apps", c.id, 0); return
        }
        Nav.reportFocus("apps", "none", 0)
    }
    function moveLine(delta) {
        line = Math.max(0, Math.min(lines.length - 1, line + delta))
        report()
    }
    onLinesChanged: if (active && line >= lines.length) moveLine(0)
    onMissingChanged: if (active && addIndex >= missing.length) { addIndex = Math.max(0, missing.length - 1); report() }

    function installLabel(app) { return Apps.installName(app.adapter) || app.label }
    function installDescribe(app) {
        const i = app.install
        if (i.state === "failed") return i.message || qsTr("The last install stopped.")
        if (i.state === "none" && i.message) return i.message
        return Apps.installWhy(app.adapter) || Apps.tagline(app.adapter)
    }
    function installValue(app) {
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
    function browserEntry(id) {
        for (const b of browsers)
            if (b.id === id) return b
        return { id: id, label: id, streaming_unverified: false, notes: [] }
    }
    function describeSite(app) {
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
                     description: qsTr("The Browser tile opens in %1").arg(b.label) }
        if (b.streaming_unverified)
            return { label: qsTr("Streaming sites use"), value: b.label,
                     description: qsTr("Unverified for streaming: the sites may not play in %1 (Widevine)").arg(b.label) }
        return { label: qsTr("Streaming sites use"), value: b.label,
                 description: qsTr("Netflix, Disney+ and Hulu open in %1").arg(b.label) }
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
        const c = current
        switch (action) {
        case "nav.up": moveLine(-1); return true
        case "nav.down": moveLine(1); return true
        case "nav.left":
        case "nav.right": {
            const d = action === "nav.left" ? -1 : 1
            if (c.kind === "strip" && c.id === "installed") installedIndex = Math.max(0, Math.min(installed.length - 1, installedIndex + d))
            else if (c.kind === "strip") addIndex = Math.max(0, Math.min(missing.length - 1, addIndex + d))
            else if (c.kind === "browser") cycle(c.which, d)
            report()
            return true
        }
        case "select":
            switch (c.kind) {
            case "strip":
                if (c.id === "installed") Shell.launchApp(installed[installedIndex].id)
                else openInstall(missing[addIndex].id, false)
                return true
            case "site": {
                const on = c.app.enabled !== true
                Shell.setAppEnabled(c.app.id, on)
                // Its browser missing: the same install card as a Home tile.
                if (on && c.app.installed === false) openInstall(c.app.id, true)
                return true
            }
            case "browser": cycle(c.which, 1); return true
            case "toggle": Shell.setAutoUpdate(Session.apps.auto_update === false); return true
            }
            return true
        }
        return false
    }

    // What the panel beside the list says about the focused entry.
    readonly property var focusedApp: current.kind === "strip" ? (current.id === "installed" ? installed[installedIndex] : missing[addIndex])
                                    : current.kind === "site" ? current.app : null
    readonly property string helpId: current.kind === "strip" ? (current.id === "installed" ? "installed" : "add-apps")
                                   : current.kind === "site" ? "streaming" : current.id

    ScreenFrame {
        id: frame
        anchors.fill: parent
        title: qsTr("Apps")
        subtitle: qsTr("What's on this TV, and what Bear Den can add")
        hints: [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Choose")], ["OK", qsTr("Open")], ["Back", qsTr("Home")]]

        Flickable {
            id: view
            anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
            width: parent.width * 0.64
            clip: true
            interactive: false
            contentHeight: column.height + 40 * Theme.scale
            property Item focusedItem: null
            contentY: {
                const f = focusedItem
                if (!f) return 0
                const y = f.mapToItem(column, 0, 0).y
                const maxY = Math.max(0, contentHeight - height)
                return Math.min(maxY, Math.max(0, y - height * 0.3))
            }
            Behavior on contentY { enabled: !Theme.reducedMotion; NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }

            Column {
                id: column
                x: 20 * Theme.scale
                y: 16 * Theme.scale
                width: view.width - 40 * Theme.scale
                spacing: 16 * Theme.scale

                SectionHeading { text: qsTr("On this TV"); visible: root.installed.length > 0 }
                CardStrip {
                    visible: root.installed.length > 0
                    model: root.installed
                    active: root.current.kind === "strip" && root.current.id === "installed"
                    index: root.installedIndex
                    onFocusedItemChanged: if (focusedItem) view.focusedItem = focusedItem
                    delegate: AppCard {
                        required property var modelData
                        required property int index
                        objectName: "installedCard"
                        app: modelData
                        label: modelData.label
                        line: modelData.running ? qsTr("Running") : Apps.tagline(modelData.adapter)
                        focused: ListView.view !== null && ListView.view.active && index === root.installedIndex
                    }
                }

                SectionHeading { text: qsTr("Add apps") }
                PixelBox {
                    visible: root.missing.length === 0
                    width: column.width
                    height: emptyText.implicitHeight + 40 * Theme.scale
                    radius: 18 * Theme.scale
                    color: Theme.surface
                    borderColor: Theme.surfaceBorder
                    borderWidth: 1
                    Text {
                        id: emptyText
                        objectName: "addAppsEmpty"
                        anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 28 * Theme.scale }
                        wrapMode: Text.WordWrap
                        text: root.noAddReason
                        color: Theme.textSecondary
                        font.family: Theme.fontFamily
                        font.pixelSize: 26 * Theme.fontUnit
                    }
                }
                CardStrip {
                    visible: root.missing.length > 0
                    model: root.missing
                    active: root.current.kind === "strip" && root.current.id === "add"
                    index: root.addIndex
                    onFocusedItemChanged: if (focusedItem) view.focusedItem = focusedItem
                    delegate: AppCard {
                        required property var modelData
                        required property int index
                        objectName: "addAppsCard"
                        app: modelData
                        installed: false
                        label: root.installLabel(modelData)
                        line: root.installValue(modelData)
                        progress: modelData.install.state === "downloading" || modelData.install.state === "preparing" || modelData.install.state === "installing"
                                  ? (modelData.install.progress || 0) / 100 : -1
                        focused: ListView.view !== null && ListView.view.active && index === root.addIndex
                    }
                }

                SectionHeading { text: qsTr("Streaming sites"); visible: root.sites.length > 0 }
                Repeater {
                    model: root.sites
                    SettingsRow {
                        id: siteRow
                        required property var modelData
                        required property int index
                        objectName: "streamingRow"
                        width: column.width
                        kind: "toggle"
                        label: modelData.label
                        description: root.describeSite(modelData)
                        value: modelData.enabled === true ? "on" : "off"
                        focused: root.current.kind === "site" && root.current.id === modelData.id
                        onFocusedChanged: if (focused) view.focusedItem = siteRow
                    }
                }
                Repeater {
                    model: root.choices
                    SettingsRow {
                        id: choiceRow
                        required property var modelData
                        required property int index
                        readonly property var row: root.choiceRow(modelData)
                        objectName: "browserRow"
                        width: column.width
                        kind: "choice"
                        label: row.label
                        description: row.description
                        value: row.value
                        focused: root.current.kind === "browser" && root.current.which === modelData
                        onFocusedChanged: if (focused) view.focusedItem = choiceRow
                    }
                }

                SectionHeading { text: qsTr("Updates"); visible: root.hasApps }
                SettingsRow {
                    id: updateRow
                    objectName: "autoUpdateRow"
                    visible: root.hasApps
                    width: column.width
                    kind: "toggle"
                    icon: "refresh"
                    label: qsTr("Keep apps up to date")
                    description: qsTr("Update the apps installed here while the TV is idle")
                    value: Session.apps && Session.apps.auto_update === false ? "off" : "on"
                    focused: root.current.id === "auto-update"
                    onFocusedChanged: if (focused) view.focusedItem = updateRow
                }
            }
        }
        HelpPanel {
            objectName: "appsHelp"
            anchors { left: view.right; leftMargin: 28 * Theme.scale; right: parent.right; rightMargin: 12 * Theme.scale; top: parent.top; topMargin: 16 * Theme.scale }
            title: root.focusedApp ? (root.current.id === "add" ? root.installLabel(root.focusedApp) : root.focusedApp.label)
                 : root.current.kind === "browser" ? root.choiceRow(root.current.which).label
                 : root.current.id === "auto-update" ? qsTr("Keep apps up to date") : ""
            help: root.current.id === "add" && root.focusedApp ? root.installDescribe(root.focusedApp) + ". " + Help.text("add-apps")
                : root.focusedApp && root.current.id === "installed" ? Apps.about(root.focusedApp.adapter)
                : Help.text(root.helpId)
            // An app's notes, or on a browser row that browser's (who makes
            // it, what it shares, its limits: state.apps.browsers[].notes).
            notes: root.focusedApp ? (root.focusedApp.notes || [])
                 : root.current.kind === "browser" ? (root.browserEntry(root.current.which === "browser" ? root.browserId : root.streamingId).notes || [])
                 : []
        }
    }

    // A section's title on the page.
    component SectionHeading: Text {
        color: Theme.textPrimary
        font.family: Theme.fontFamily
        font.pixelSize: 32 * Theme.fontUnit
        font.weight: Font.DemiBold
        topPadding: 8 * Theme.scale
    }
    // A horizontal strip of cards; the focused card scrolls into view.
    component CardStrip: ListView {
        id: strip
        property bool active: false
        property int index: 0
        readonly property Item focusedItem: active ? itemAtIndex(index) : null
        width: column.width
        height: 190 * Theme.scale
        orientation: ListView.Horizontal
        spacing: 22 * Theme.scale
        interactive: false
        clip: false
        currentIndex: index
        highlightFollowsCurrentItem: false
        onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
        leftMargin: 8 * Theme.scale
        rightMargin: 8 * Theme.scale
    }
    // One app as a card: icon, name, a status line, an install bar.
    component AppCard: PixelBox {
        id: card
        property var app: ({})
        property bool installed: true
        property string label: ""
        property string line: ""
        property real progress: -1
        property bool focused: false
        width: 300 * Theme.scale
        height: 180 * Theme.scale
        radius: 18 * Theme.scale
        color: focused ? Theme.surfaceRaised : Theme.surface
        borderColor: Theme.surfaceBorder
        borderWidth: 1
        scale: focused ? 1.04 : 1
        Behavior on scale { NumberAnimation { duration: Theme.durationFast } }
        AppIcon {
            anchors { left: parent.left; top: parent.top; margins: 20 * Theme.scale }
            size: 72 * Theme.scale
            adapter: card.app.adapter || ""
            installed: card.installed
            label: card.label
            tint: Theme.tintFor(card.app.id || "")
        }
        Column {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: 20 * Theme.scale }
            spacing: 2 * Theme.scale
            Text {
                width: parent.width
                text: card.label
                elide: Text.ElideRight
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                width: parent.width
                text: card.line
                visible: text.length > 0
                elide: Text.ElideRight
                color: card.installed ? Theme.textSecondary : Theme.accent
                font.family: Theme.fontFamily
                font.pixelSize: 20 * Theme.fontUnit
                font.weight: card.installed ? Font.Normal : Font.DemiBold
            }
        }
        ProgressBar {
            visible: card.progress >= 0
            anchors { left: parent.left; right: parent.right; top: parent.top; margins: 20 * Theme.scale; topMargin: 100 * Theme.scale }
            value: Math.max(0, card.progress)
        }
        FocusFrame { shown: card.focused; cornerRadius: card.radius }
    }
}
