// Settings: phone remote, pairing, devices, Now playing on phones
// (remote.now_playing over IPC), appearance (sent to the coordinator as a
// layout update), weather, playback, diagnostics, and a maintenance exit.

import QtQuick
import BearDen

Item {
    id: root
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
    // Installed themes (built-in and the owner's), in display order.
    readonly property var backgrounds: Themes.list.map(t => t.id)
    readonly property var margins: [2, 3, 4, 5, 6]

    readonly property var rows: [
        { id: "remote", kind: "link", label: qsTr("Phone remote"),
          description: remote.listening ? qsTr("Phones on your network can control this TV after pairing") : qsTr("Off — nothing listens on your network"),
          value: remote.listening ? qsTr("On") : qsTr("Off") },
        { id: "pairing", kind: "link", label: qsTr("Pair a phone"), description: qsTr("Show a QR code and six-digit code"), value: "" },
        { id: "devices", kind: "link", label: qsTr("Paired phones"), description: "", value: String(Session.devices.length) },
        // state.remote.now_playing mirrors config remote.now_playing; missing
        // (an older coordinator) means on, its default.
        { id: "now-playing", kind: "toggle", label: qsTr("Now playing on phones"),
          description: qsTr("Paired phones show the title of what is playing (never while locked)"),
          value: remote.now_playing === false ? "off" : "on" },
        { id: "text", kind: "choice", label: qsTr("Text size"), description: "", value: Math.round((ui.text_scale || 1) * 100) + "%" },
        { id: "density", kind: "choice", label: qsTr("Tile size"), description: "", value: ui.tile_density === "large" ? qsTr("Large") : qsTr("Comfortable") },
        { id: "background", kind: "choice", label: qsTr("Theme"), description: qsTr("The world Bear Den lives in, on the TV and the phone"), value: Themes.get(ui.background).name || qsTr("Den") },
        { id: "style", kind: "choice", label: qsTr("Style"), description: ui.theme === "performance" ? qsTr("No bears, decorations or animations: the lightest on the TV") : qsTr("Bear Den adds the world's decorations and bears; Plain keeps it simple"), value: ui.theme === "performance" ? qsTr("Performance") : ui.theme === "plain-dark" ? qsTr("Plain") : qsTr("Bear Den") },
        { id: "art", kind: "choice", label: qsTr("Art style"), description: ui.art_style === "classic" ? qsTr("Smooth drawings and pictures") : qsTr("Everything drawn in pixel art"), value: ui.art_style === "classic" ? qsTr("Classic") : qsTr("Pixel") },
        { id: "margin", kind: "choice", label: qsTr("Screen edge margin"), description: qsTr("Increase if the edges are cut off on your TV"), value: (ui.safe_margin_percent || 3) + "%" },
        { id: "motion", kind: "toggle", label: qsTr("Reduce motion"), description: "", value: ui.reduced_motion ? "on" : "off" },
        { id: "contrast", kind: "toggle", label: qsTr("High-contrast focus"), description: "", value: ui.high_contrast_focus ? "on" : "off" },
        { id: "hero", kind: "toggle", label: qsTr("Featured panel on Home"), description: "", value: ui.hero_enabled ? "on" : "off" },
        { id: "clock", kind: "toggle", label: qsTr("Clock"), description: "", value: ui.clock_enabled ? "on" : "off" },
        { id: "weather", kind: "link", label: qsTr("Weather"), description: qsTr("Your town's weather by the clock, and in the scene"),
          value: Session.weather.status === undefined || Session.weather.status === "disabled" ? qsTr("Off") : (Session.weather.place || qsTr("On")) },
        { id: "playback", kind: "link", label: qsTr("Playback"), description: qsTr("What this TV can play smoothly, and the best settings for each app"), value: playbackValue },
        { id: "advanced-playback", kind: "link", label: qsTr("Advanced playback"), description: qsTr("Adjust each app's playback settings by hand"), value: manualValue },
        { id: "diagnostics", kind: "link", label: qsTr("Diagnostics"), description: qsTr("Connection, desktop, and app status"), value: "" },
        { id: "exit", kind: "danger", label: qsTr("Exit Bear Den TV"), description: qsTr("For maintenance: returns to the desktop until the next start"), value: "" }
    ]

    function enter() { Themes.reload(); report() }
    function report() { Nav.reportFocus("settings", rows[focusIndex].id, 0) }

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
        case "background": editUi(u => { u.background = cycle(backgrounds, Themes.canonical(u.background), delta); u.accent = World.accentFor(u.background) }); return true
        case "style": editUi(u => u.theme = cycle(["den-dark", "plain-dark", "performance"], u.theme, delta)); return true
        case "art": editUi(u => u.art_style = cycle(["pixel", "classic"], u.art_style || "pixel", delta)); return true
        case "margin": editUi(u => u.safe_margin_percent = cycle(margins, u.safe_margin_percent, delta)); return true
        }
        return false
    }
    function activate(row) {
        switch (row.id) {
        case "remote": openScreen("remote-setup"); break
        case "pairing": openScreen("pairing"); break
        case "devices": openScreen("devices"); break
        case "now-playing": Shell.setNowPlaying(remote.now_playing === false); break
        case "weather": openScreen("weather"); break
        case "playback": openScreen("playback"); break
        case "advanced-playback": openScreen("advanced-playback"); break
        case "diagnostics": openScreen("diagnostics"); break
        case "motion": editUi(u => u.reduced_motion = !u.reduced_motion); break
        case "contrast": editUi(u => u.high_contrast_focus = !u.high_contrast_focus); break
        case "hero": editUi(u => u.hero_enabled = !u.hero_enabled); break
        case "clock": editUi(u => u.clock_enabled = !u.clock_enabled); break
        case "exit":
            confirm(qsTr("Exit Bear Den TV?"), qsTr("The TV returns to the desktop. Bear Den starts again with the next session."),
                    qsTr("Exit"), () => Shell.exitShell())
            break
        default: change(row, 1)
        }
    }

    function navigate(action) {
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.down": focusIndex = Math.min(rows.length - 1, focusIndex + 1); report(); return true
        case "nav.left": change(rows[focusIndex], -1); return true
        case "nav.right": change(rows[focusIndex], 1); return true
        case "select": activate(rows[focusIndex]); return true
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Settings")
        subtitle: Session.previewActive ? qsTr("Previewing changes from a phone") : qsTr("Changes apply right away")
        hints: [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Change")], ["OK", qsTr("Open")], ["Back", qsTr("Home")]]

        ListView {
            id: list
            anchors.fill: parent
            anchors.leftMargin: 8 * Theme.scale
            anchors.rightMargin: parent.width * 0.3
            spacing: 14 * Theme.scale
            model: root.rows
            currentIndex: root.focusIndex
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
                    label: slot.modelData.label
                    description: slot.modelData.description
                    value: slot.modelData.value
                    kind: slot.modelData.kind
                    focused: slot.index === root.focusIndex
                }
            }
        }
    }
}
