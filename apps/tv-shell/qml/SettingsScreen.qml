// Settings, in two levels: a list of categories (a card with a Bear Den icon
// each), and OK opens one category's rows; Back returns to the category you
// were on. The categories and their rows (ids stable; apps/tv-shell/AGENTS.md →
// Settings rows, with where every former row went):
//   Phones & remote          remote, pairing, devices, now-playing
//                            (remote.now_playing over IPC)
//   Display & accessibility  text, density, margin, motion, contrast
//   Home screen              hero, clock, weather
//   Playback                 playback, advanced-playback, plex
//   Power & TV               sleep, screen-off (the power.sleep_timer and
//                            display.off actions, state.power), cec,
//                            cec-volume (cec.configure, state.cec),
//                            autostart (autostart.configure, state.autostart)
//   About                    version, diagnostics, setup-again (the first-run
//                            setup, OnboardingScreen), exit
// The look (Theme, Style, Art style, App icons, Badges) lives on the Themes
// page (ThemesScreen) and the apps (Add apps, Streaming sites, Keep apps up
// to date) on the Apps page (AppsScreen). Appearance fields are sent to the
// coordinator as a layout update. Each row's one-line help is in Help.qml,
// shown beside the list (HelpPanel).

import QtQuick
import BearDen

Item {
    id: root
    objectName: "settingsScreen"
    // "" = the category list; else the open category's id.
    property string category: ""
    property int categoryIndex: 0
    property int focusIndex: 0
    signal openScreen(string name)
    signal confirm(string title, string body, string confirmLabel, var onAccept)

    readonly property var ui: Session.ui
    readonly property var remote: Session.remote
    readonly property var textScales: [1.0, 1.15, 1.3, 1.5]
    // "3 tuned", "1 waiting", … from the playback test.
    readonly property string playbackValue: {
        const apps = (Session.playback && Session.playback.apps) || []
        if (apps.length === 0) return qsTr("Checking…")
        const done = apps.filter(a => a.status === "tuned" || a.status === "applied").length
        return done === apps.length ? qsTr("Tuned") : qsTr("%1 of %2 tuned").arg(done).arg(apps.length)
    }
    // "Auto", or how many playback settings were chosen by hand.
    readonly property string manualValue: {
        const apps = (Session.playback && Session.playback.apps) || []
        const n = apps.reduce((sum, a) => sum + (a.settings || []).filter(s => s.overridden).length, 0)
        return n === 0 ? qsTr("Auto") : qsTr("%n by hand", "", n)
    }
    // Settings → Plex at a glance (state.plex; empty without a connector).
    readonly property string plexValue: {
        switch (Session.plex.status) {
        case "connected": return Session.plex.server || qsTr("On")
        case "linking": case "choose_server": case "choose_libraries": return qsTr("Signing in…")
        case "error": return qsTr("Needs attention")
        case "signed_out": return qsTr("Off")
        }
        return qsTr("Not available")
    }
    readonly property var margins: [2, 3, 4, 5, 6]
    // Sleep timer choices (contract.SleepChoices; 0 = off) and Screen off.
    readonly property var sleepChoices: [0, 15, 30, 45, 60, 90, 120]
    readonly property var power: Session.power
    readonly property bool sleepRunning: power.sleep_at_ms !== undefined && power.sleep_at_ms !== null
    readonly property var screenOffCap: Session.capabilities["display.off"] || ({})
    // state.cec: TV control over HDMI-CEC (empty from an older coordinator).
    readonly property var cec: Session.cec
    readonly property bool cecOn: cec.enabled === true
    readonly property string cecTarget: cec.volume_target === "tv" ? "tv" : "pc"
    // state.autostart: the user's login entry (absent from an older
    // coordinator: then there is no row).
    readonly property var autostart: Session.autostart || ({})
    readonly property bool hasAutostart: autostart.enabled !== undefined
    function cecDescription() {
        if (cec.available !== true)
            return cec.reason || qsTr("Not available on this TV box")
        if (!cecOn) return qsTr("Turn the TV on and off with Bear Den, and switch it to this input")
        if (cec.tv_power === "standby") return qsTr("The TV is in standby")
        if (cec.tv_power === "on") return qsTr("The TV is on and follows Bear Den")
        return qsTr("The TV did not say whether it is on")
    }
    function sleepLabel(minutes) {
        if (!minutes) return qsTr("Off")
        if (minutes === 60) return qsTr("1 hour")
        if (minutes % 60 === 0) return qsTr("%1 hours").arg(minutes / 60)
        if (minutes > 60) return qsTr("1 h %1 min").arg(minutes - 60)
        return qsTr("%1 min").arg(minutes)
    }

    // Every row, by id. A row that does not apply here (CEC volume without
    // CEC, autostart from an older coordinator) is absent.
    readonly property var allRows: {
        const r = {}
        const add = (row) => { r[row.id] = row }
        add({ id: "remote", kind: "link", label: qsTr("Phone remote"),
              description: remote.listening ? qsTr("Phones on your network can control this TV after pairing") : qsTr("Off — nothing listens on your network"),
              value: remote.listening ? qsTr("On") : qsTr("Off") })
        add({ id: "pairing", kind: "link", label: qsTr("Pair a phone"), description: qsTr("Show a QR code and six-digit code"), value: "" })
        add({ id: "devices", kind: "link", label: qsTr("Paired phones"), description: "", value: String(Session.devices.length) })
        // state.remote.now_playing mirrors config remote.now_playing; missing
        // (an older coordinator) means on, its default.
        add({ id: "now-playing", kind: "toggle", label: qsTr("Now playing on phones"),
              description: qsTr("Paired phones show the title of what is playing (never while locked)"),
              value: remote.now_playing === false ? "off" : "on" })
        add({ id: "text", kind: "choice", label: qsTr("Text size"), description: "", value: Math.round((ui.text_scale || 1) * 100) + "%" })
        add({ id: "density", kind: "choice", label: qsTr("Tile size"), description: "", value: ui.tile_density === "large" ? qsTr("Large") : qsTr("Comfortable") })
        add({ id: "margin", kind: "choice", label: qsTr("Screen edge margin"), description: qsTr("Increase if the edges are cut off on your TV"), value: (ui.safe_margin_percent || 3) + "%" })
        add({ id: "motion", kind: "toggle", label: qsTr("Reduce motion"), description: "", value: ui.reduced_motion ? "on" : "off" })
        add({ id: "contrast", kind: "toggle", label: qsTr("High-contrast focus"), description: "", value: ui.high_contrast_focus ? "on" : "off" })
        add({ id: "hero", kind: "toggle", label: qsTr("Featured panel on Home"), description: "", value: ui.hero_enabled ? "on" : "off" })
        add({ id: "clock", kind: "toggle", label: qsTr("Clock"), description: "", value: ui.clock_enabled ? "on" : "off" })
        add({ id: "weather", kind: "link", label: qsTr("Weather"), description: qsTr("Your town's weather by the clock, and in the scene"),
              value: Session.weather.status === undefined || Session.weather.status === "disabled" ? qsTr("Off") : (Session.weather.place || qsTr("On")) })
        add({ id: "playback", kind: "link", label: qsTr("Playback"), description: qsTr("What this TV can play smoothly, and the best settings for each app"), value: playbackValue })
        add({ id: "advanced-playback", kind: "link", label: qsTr("Advanced playback"), description: qsTr("Adjust each app's playback settings by hand"), value: manualValue })
        add({ id: "plex", kind: "link", label: qsTr("Plex"), description: qsTr("Continue Watching and Recently Added on Home"), value: plexValue })
        add({ id: "sleep", kind: "choice", label: qsTr("Sleep timer"),
              description: sleepRunning ? qsTr("Pauses, goes Home and turns the screen off when it runs out")
                                        : qsTr("Pause, go Home and turn the screen off after a while"),
              value: sleepRunning ? sleepLabel(power.sleep_minutes) : qsTr("Off") })
        add({ id: "screen-off", kind: "link", label: qsTr("Turn the screen off"),
              description: screenOffCap.available ? qsTr("Any key turns it back on") : (screenOffCap.reason || qsTr("Not available here")),
              value: "" })
        add({ id: "cec", kind: "toggle", label: qsTr("TV control over HDMI (CEC)"), description: cecDescription(), value: cecOn ? "on" : "off" })
        if (cecOn && cec.available === true)
            add({ id: "cec-volume", kind: "choice", label: qsTr("Phone volume buttons"),
                  description: cecTarget === "tv" ? qsTr("Change the TV's volume over HDMI") : qsTr("Change this box's volume"),
                  value: cecTarget === "tv" ? qsTr("TV") : qsTr("PC") })
        if (hasAutostart)
            add({ id: "autostart", kind: "toggle", label: qsTr("Start with this PC"),
                  description: autostart.available === false ? (autostart.reason || qsTr("Not available here")) : qsTr("Start Bear Den when you log in"),
                  value: autostart.enabled === true ? "on" : "off" })
        add({ id: "version", kind: "info", label: qsTr("Version"), description: qsTr("Bear Den TV"), value: Shell.version })
        add({ id: "diagnostics", kind: "link", label: qsTr("Diagnostics"), description: qsTr("Connection, desktop, and app status"), value: "" })
        add({ id: "setup-again", kind: "link", label: qsTr("Run setup again"), description: qsTr("Your look, apps, the phone remote and extras"), value: "" })
        add({ id: "exit", kind: "danger", label: qsTr("Exit Bear Den TV"), description: qsTr("For maintenance: returns to the desktop until the next start"), value: "" })
        return r
    }
    readonly property var categories: [
        { id: "phones", icon: "phone", label: qsTr("Phones & remote"), rows: ["remote", "pairing", "devices", "now-playing"],
          help: qsTr("Control this TV from your phone: turn the phone remote on, pair phones and choose what they see.") },
        { id: "display", icon: "display", label: qsTr("Display & accessibility"), rows: ["text", "density", "margin", "motion", "contrast"],
          help: qsTr("Text and tile sizes, the picture's edges, motion and the focus outline.") },
        { id: "home", icon: "home", label: qsTr("Home screen"), rows: ["hero", "clock", "weather"],
          help: qsTr("What Home shows: the featured panel, the clock and the weather.") },
        { id: "playback", icon: "play", label: qsTr("Playback"), rows: ["playback", "advanced-playback", "plex"],
          help: qsTr("How well this PC plays video, each app's playback settings, and your Plex account.") },
        { id: "power", icon: "power", label: qsTr("Power & TV"), rows: ["sleep", "screen-off", "cec", "cec-volume", "autostart"],
          help: qsTr("The sleep timer, the screen, controlling the TV over HDMI, and starting with this PC.") },
        { id: "about", icon: "about", label: qsTr("About"), rows: ["version", "diagnostics", "setup-again", "exit"],
          help: qsTr("The version, diagnostics, the first-run setup and leaving Bear Den.") }
    ]
    // The category a row lives in ("" = none: it moved to Themes or Apps).
    function categoryOf(rowId) {
        for (const c of categories)
            if (c.rows.indexOf(rowId) >= 0) return c.id
        return ""
    }
    readonly property var currentCategory: {
        for (const c of categories)
            if (c.id === category) return c
        return null
    }
    // The rows on screen: the open category's, or the categories themselves.
    readonly property var rows: currentCategory
        ? currentCategory.rows.filter(id => allRows[id] !== undefined).map(id => allRows[id])
        : categories.map(c => ({ id: c.id, kind: "link", icon: c.icon, label: c.label,
                                 description: c.rows.filter(id => allRows[id] !== undefined).map(id => allRows[id].label).join(" · "),
                                 value: "" }))
    readonly property int index: currentCategory ? focusIndex : categoryIndex
    readonly property var focusedRow: rows[Math.max(0, Math.min(rows.length - 1, index))] || ({})

    // Opened from Home (not returning from a page it opened): the category list.
    function opened() { category = ""; focusIndex = 0 }
    function enter() { Themes.reload(); report() }
    function report() {
        if (currentCategory) {
            focusIndex = Math.max(0, Math.min(rows.length - 1, focusIndex))
            Nav.reportFocus("settings", rows[focusIndex].id, 0)
        } else {
            Nav.reportFocus("settings-categories", categories[categoryIndex].id, 0)
        }
    }
    function openCategory(id) {
        for (let i = 0; i < categories.length; ++i)
            if (categories[i].id === id) categoryIndex = i
        category = id
        focusIndex = 0
        report()
    }

    function cycle(list, current, delta) {
        let i = list.indexOf(current)
        if (i < 0) i = 0
        return list[Math.max(0, Math.min(list.length - 1, i + delta))]
    }
    function editUi(mutate) {
        const layout = JSON.parse(JSON.stringify(Session.layoutForEdit()))
        if (!layout.ui) return
        mutate(layout.ui)
        Shell.updateLayout(layout)
    }
    function change(row, delta) {
        switch (row.id) {
        case "text": editUi(u => u.text_scale = cycle(textScales, u.text_scale, delta)); return true
        case "density": editUi(u => u.tile_density = delta > 0 ? "large" : "comfortable"); return true
        case "margin": editUi(u => u.safe_margin_percent = cycle(margins, u.safe_margin_percent, delta)); return true
        case "sleep": Shell.setSleepTimer(cycle(sleepChoices, sleepRunning ? power.sleep_minutes : 0, delta)); return true
        case "cec-volume": Shell.setCEC(true, delta > 0 ? "tv" : "pc"); return true
        }
        return false
    }
    function activate(row) {
        switch (row.id) {
        case "remote": openScreen("remote-setup"); break
        case "pairing": openScreen("pairing"); break
        case "devices": openScreen("devices"); break
        case "now-playing": Shell.setNowPlaying(remote.now_playing === false); break
        // Stored even without an adapter: it takes effect when one appears.
        case "cec": Shell.setCEC(!cecOn, cecTarget); break
        case "cec-volume": Shell.setCEC(true, cecTarget === "tv" ? "pc" : "tv"); break
        case "autostart": if (autostart.available !== false) Shell.setAutostart(autostart.enabled !== true); break
        case "weather": openScreen("weather"); break
        case "playback": openScreen("playback"); break
        case "advanced-playback": openScreen("advanced-playback"); break
        case "diagnostics": openScreen("diagnostics"); break
        case "setup-again": openScreen("onboarding"); break
        // After a short pause, so the OK key's own release does not wake the display.
        case "screen-off": if (screenOffCap.available) screenOffDelay.restart(); break
        case "plex": openScreen("plex"); break
        case "motion": editUi(u => u.reduced_motion = !u.reduced_motion); break
        case "contrast": editUi(u => u.high_contrast_focus = !u.high_contrast_focus); break
        case "hero": editUi(u => u.hero_enabled = !u.hero_enabled); break
        case "clock": editUi(u => u.clock_enabled = !u.clock_enabled); break
        case "version": break
        case "exit":
            confirm(qsTr("Exit Bear Den TV?"), qsTr("The TV returns to the desktop. Bear Den starts again with the next session."),
                    qsTr("Exit"), () => Shell.exitShell())
            break
        default: change(row, 1)
        }
    }

    Timer { id: screenOffDelay; objectName: "screenOffDelay"; interval: 800; onTriggered: Shell.screenOff() }

    function navigate(action) {
        if (!currentCategory) {
            switch (action) {
            case "nav.up": categoryIndex = Math.max(0, categoryIndex - 1); report(); return true
            case "nav.down": categoryIndex = Math.min(categories.length - 1, categoryIndex + 1); report(); return true
            case "nav.left": case "nav.right": return true
            case "select": openCategory(categories[categoryIndex].id); return true
            }
            return false   // Back: Home
        }
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.down": focusIndex = Math.min(rows.length - 1, focusIndex + 1); report(); return true
        case "nav.left": change(rows[focusIndex], -1); return true
        case "nav.right": change(rows[focusIndex], 1); return true
        case "select": activate(rows[focusIndex]); return true
        case "back": category = ""; report(); return true   // back to the category it was on
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: root.currentCategory ? root.currentCategory.label : qsTr("Settings")
        subtitle: Session.previewActive ? qsTr("Previewing changes from a phone")
                : root.currentCategory ? qsTr("Settings · changes apply right away") : qsTr("Choose a category")
        hints: root.currentCategory
               ? [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Change")], ["OK", qsTr("Open")], ["Back", qsTr("Categories")]]
               : [["▲ ▼", qsTr("Move")], ["OK", qsTr("Open")], ["Back", qsTr("Home")]]

        ListView {
            id: list
            objectName: "settingsList"
            anchors.fill: parent
            anchors.leftMargin: 8 * Theme.scale
            anchors.rightMargin: parent.width * 0.36
            spacing: 14 * Theme.scale
            model: root.rows
            currentIndex: root.index
            // `rows` is rebuilt on every snapshot, and a new model sends the
            // view back to the top: keep the focused row (the sleep timer
            // row's value changes as it is set) in view.
            onModelChanged: Qt.callLater(() => { currentIndex = Qt.binding(() => root.index); positionViewAtIndex(root.index, ListView.Contain) })
            interactive: false
            clip: true
            highlightMoveDuration: Theme.duration
            preferredHighlightBegin: height * 0.3
            preferredHighlightEnd: height * 0.7
            highlightRangeMode: ListView.ApplyRange
            topMargin: 8 * Theme.scale
            bottomMargin: 8 * Theme.scale
            // ListView puts every delegate at x = 0, so the row sits inside a
            // full-width slot; the inset keeps its corners and focus frame clear
            // of the clip.
            delegate: Item {
                id: slot
                required property var modelData
                required property int index
                width: list.width
                height: row.height
                SettingsRow {
                    id: row
                    objectName: "settingsRow"
                    width: parent.width - 40 * Theme.scale
                    x: 20 * Theme.scale
                    icon: slot.modelData.icon || ""
                    label: slot.modelData.label
                    description: slot.modelData.description
                    value: slot.modelData.value
                    kind: slot.modelData.kind
                    focused: slot.index === root.index
                }
            }
        }
        HelpPanel {
            objectName: "settingsHelp"
            anchors { left: list.right; leftMargin: 24 * Theme.scale; right: parent.right; rightMargin: 12 * Theme.scale; top: parent.top; topMargin: 8 * Theme.scale }
            icon: root.currentCategory ? "" : (root.focusedRow.icon || "")
            title: root.focusedRow.label || ""
            help: root.currentCategory ? Help.text(root.focusedRow.id || "")
                                       : (root.categories[root.categoryIndex] ? root.categories[root.categoryIndex].help : "")
        }
    }
}
