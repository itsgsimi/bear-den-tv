// Settings → Advanced playback: every setting Bear Den tunes for an app,
// adjustable by hand among the options this box can handle
// (state.playback.apps[].settings; internal/applications/tuning). Left/Right
// picks an option (playback.set), OK returns the row to Auto. The coordinator
// stores the choice in config.json and applies it when the app is closed.

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0

    readonly property var p: Session.playback
    readonly property bool ready: p && p.apps !== undefined
    // One row per setting, app by app; an app's first row carries its heading
    // so the heading scrolls into view with it.
    readonly property var choices: {
        const out = []
        const apps = ready ? p.apps : []
        for (const app of apps) {
            const settings = app.settings || []
            for (let i = 0; i < settings.length; ++i)
                out.push({ app: app, setting: settings[i], first: i === 0, id: app.adapter + "." + settings[i].id })
        }
        return out
    }
    readonly property var focused: choices.length > 0 ? choices[Math.min(focusIndex, choices.length - 1)] : null

    function enter() {
        focusIndex = Math.max(0, Math.min(focusIndex, choices.length - 1))
        report()
    }
    function report() { Nav.reportFocus("playback", focused ? focused.id : "none", 0) }

    function optionOf(setting, value) {
        for (const o of setting.options) if (o.value === value) return o
        return null
    }
    function labelOf(setting, value) {
        const o = optionOf(setting, value)
        return o ? o.label : value
    }
    function change(delta) {
        if (!focused) return
        const s = focused.setting
        const values = s.options.map(o => o.value)
        let i = values.indexOf(s.value)
        if (i < 0) i = values.indexOf(s.auto)
        const next = values[Math.max(0, Math.min(values.length - 1, i + delta))]
        if (next !== undefined && next !== s.value)
            Shell.setPlayback(focused.app.adapter, s.id, next)
    }
    function resetToAuto() {
        if (focused && focused.setting.overridden)
            Shell.setPlayback(focused.app.adapter, focused.setting.id, "")
    }
    function statusText(app) {
        switch (app.status) {
        case "pending": return qsTr("Changes apply when %1 closes").arg(app.label)
        case "off": return qsTr("Automatic tuning is off: choices are saved, not applied")
        case "error": return qsTr("Could not apply the settings")
        case "applied": return qsTr("Settings applied")
        case "tuned": return qsTr("Settings in place")
        }
        return ""
    }

    function navigate(action) {
        switch (action) {
        case "nav.up": focusIndex = Math.max(0, focusIndex - 1); report(); return true
        case "nav.down": focusIndex = Math.max(0, Math.min(choices.length - 1, focusIndex + 1)); report(); return true
        case "nav.left": change(-1); return true
        case "nav.right": change(1); return true
        case "select": resetToAuto(); return true
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Advanced playback")
        subtitle: root.ready ? qsTr("Only the choices this TV can handle are offered") : qsTr("Checking what this TV can play…")
        hints: [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Change")], ["OK", qsTr("Back to Auto")], ["Back", qsTr("Back")]]

        Text {
            visible: root.ready && root.choices.length === 0
            anchors.centerIn: parent
            text: qsTr("No app with adjustable settings is installed.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
        }

        ListView {
            id: list
            anchors.fill: parent
            anchors.leftMargin: 8 * Theme.scale
            anchors.rightMargin: parent.width * 0.22
            spacing: 14 * Theme.scale
            model: root.choices
            currentIndex: Math.min(root.focusIndex, root.choices.length - 1)
            interactive: false
            clip: true
            highlightMoveDuration: Theme.duration
            preferredHighlightBegin: height * 0.3
            preferredHighlightEnd: height * 0.7
            highlightRangeMode: ListView.ApplyRange
            topMargin: 8 * Theme.scale
            bottomMargin: 8 * Theme.scale
            delegate: Column {
                id: entry
                required property var modelData
                required property int index
                readonly property var s: modelData.setting
                readonly property var option: root.optionOf(s, s.value)
                // ListView puts every delegate at x = 0: inset with padding so
                // the rows' corners and focus frame stay clear of the clip.
                width: list.width
                leftPadding: 20 * Theme.scale
                rightPadding: 20 * Theme.scale
                spacing: 12 * Theme.scale

                Column {
                    visible: entry.modelData.first
                    topPadding: entry.index > 0 ? 18 * Theme.scale : 0
                    bottomPadding: 10 * Theme.scale // clear of the focused row's frame
                    spacing: 2 * Theme.scale
                    Text {
                        text: entry.modelData.app.label
                        color: Theme.textPrimary
                        font.family: Theme.fontFamily
                        font.pixelSize: 32 * Theme.fontUnit
                        font.weight: Font.DemiBold
                    }
                    Text {
                        visible: text.length > 0
                        text: root.statusText(entry.modelData.app)
                        color: Theme.textSecondary
                        font.family: Theme.fontFamily
                        font.pixelSize: 19 * Theme.fontUnit
                    }
                }
                SettingsRow {
                    objectName: "settingsRow"
                    width: entry.width - entry.leftPadding - entry.rightPadding
                    kind: "choice"
                    focused: entry.index === list.currentIndex
                    label: entry.s.label
                    value: root.labelOf(entry.s, entry.s.value)
                    badge: !entry.s.overridden && entry.s.value === entry.s.auto ? qsTr("Auto") : ""
                    // The focused option's note; otherwise how the value was chosen.
                    description: focused && entry.option && entry.option.note ? entry.option.note
                               : entry.s.overridden ? qsTr("Chosen by hand · Auto: %1").arg(root.labelOf(entry.s, entry.s.auto))
                               : qsTr("Chosen by Bear Den for this TV")
                }
            }
        }
    }
}
