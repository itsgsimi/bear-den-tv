// The Themes page (header pill Themes): how Bear Den looks.
//   Theme        large live-preview cards (ThemePicker): ◀ ▶ tries a world
//                on the whole TV, OK uses it; moving away or Back returns to
//                the theme in use, so nothing is lost or half-applied.
//   Art style    Pixel or Classic (layout ui.art_style)
//   Style        Bear Den, Plain or Performance (ui.theme)
//   App icons    App's own or Bear Den style (ui.app_icons)
//   Den badges   opens the badge shelf (BadgesScreen)
// Every choice but the theme applies at once, as a layout update the
// coordinator validates and stores (like Settings did before). Each row's
// one-line help is in Help.qml. Reports focus as section `themes`.

import QtQuick
import BearDen

Item {
    id: root
    objectName: "themesScreen"
    property bool active: false
    property int line: 0
    signal openScreen(string name)

    // The persisted look (Session.layout is read so this follows every snapshot).
    readonly property var ui: Session.layout && (Session.layoutForEdit().ui || ({}))
    readonly property var rows: [
        { id: "art", kind: "choice", icon: "", label: qsTr("Art style"),
          description: ui.art_style === "classic" ? qsTr("Smooth drawings and pictures") : qsTr("Everything drawn in pixel art"),
          value: ui.art_style === "classic" ? qsTr("Classic") : qsTr("Pixel") },
        { id: "style", kind: "choice", icon: "", label: qsTr("Style"),
          description: ui.theme === "performance" ? qsTr("No bears, decorations or animations: the lightest on the TV") : qsTr("Bear Den adds the world's decorations and bears; Plain keeps it simple"),
          value: ui.theme === "performance" ? qsTr("Performance") : ui.theme === "plain-dark" ? qsTr("Plain") : qsTr("Bear Den") },
        // layout.ui.app_icons: missing means app (the app's own icon).
        { id: "app-icons", kind: "choice", icon: "", label: qsTr("App icons"),
          description: ui.app_icons === "bear_den" ? qsTr("Bear Den's own drawing for every app") : qsTr("Each installed app's own icon; Bear Den's for the rest"),
          value: ui.app_icons === "bear_den" ? qsTr("Bear Den style") : qsTr("App's own") },
        { id: "badges", kind: "link", icon: "medal", label: qsTr("Den badges"), description: qsTr("Playful milestones, counted on this TV only"),
          value: Session.achievements.enabled === false ? qsTr("Off") : qsTr("%1 of %2").arg((Session.achievements.earned || []).length).arg((Session.achievements.progress || []).length) }
    ]
    // Line 0 is the theme strip; then the rows.
    readonly property string focusedId: line === 0 ? "background" : rows[line - 1].id

    function opened() { line = 0; picker.reset() }
    function enter() { Themes.reload(); if (line === 0) picker.reset(); report() }
    function leave() { picker.cancel() }
    function report() { Nav.reportFocus("themes", line === 0 ? "theme:" + picker.focusedId : rows[line - 1].id, 0) }

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
    function change(id, delta) {
        switch (id) {
        case "art": editUi(u => u.art_style = cycle(["pixel", "classic"], u.art_style || "pixel", delta)); return
        case "style": editUi(u => u.theme = cycle(["den-dark", "plain-dark", "performance"], u.theme, delta)); return
        case "app-icons": editUi(u => u.app_icons = cycle(["app", "bear_den"], u.app_icons || "app", delta)); return
        }
    }
    function setLine(l) {
        const next = Math.max(0, Math.min(rows.length, l))
        if (next === line) return
        // Leaving the strip without OK: back to the theme in use.
        if (line === 0) { picker.cancel(); picker.reset() }
        line = next
        report()
    }
    function navigate(action) {
        switch (action) {
        case "nav.up": setLine(line - 1); return true
        case "nav.down": setLine(line + 1); return true
        case "nav.left":
        case "nav.right": {
            const d = action === "nav.left" ? -1 : 1
            if (line === 0) { picker.move(d); report() }
            else change(rows[line - 1].id, d)
            return true
        }
        case "select":
            if (line === 0) picker.apply()
            else if (rows[line - 1].id === "badges") openScreen("badges")
            else change(rows[line - 1].id, 1)
            return true
        case "back":
            picker.cancel()
            return false   // ShellRoot pops back Home
        }
        return false
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Themes")
        subtitle: Session.uiPreviewActive ? qsTr("Trying on %1: OK to use it, Back to keep %2")
                                              .arg((Themes.get(picker.focusedId).name || picker.focusedId))
                                              .arg((Themes.get(picker.applied).name || picker.applied))
                                          : qsTr("◀ ▶ tries a theme on the whole TV · OK uses it")
        hints: [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Try / change")], ["OK", qsTr("Use")], ["Back", qsTr("Home")]]

        Text {
            id: themeHeading
            text: qsTr("Theme")
            color: Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 32 * Theme.fontUnit
            font.weight: Font.DemiBold
            x: 20 * Theme.scale
        }
        ThemePicker {
            id: picker
            anchors { top: themeHeading.bottom; topMargin: 8 * Theme.scale; left: parent.left; right: parent.right }
            active: root.line === 0
        }
        Column {
            id: list
            anchors { top: picker.bottom; topMargin: 24 * Theme.scale; left: parent.left; leftMargin: 20 * Theme.scale }
            width: parent.width * 0.6
            spacing: 14 * Theme.scale
            Repeater {
                model: root.rows
                SettingsRow {
                    required property var modelData
                    required property int index
                    objectName: "themesRow"
                    width: list.width
                    kind: modelData.kind
                    icon: modelData.icon
                    label: modelData.label
                    description: modelData.description
                    value: modelData.value
                    focused: root.line === index + 1
                }
            }
        }
        HelpPanel {
            objectName: "themesHelp"
            anchors { left: list.right; leftMargin: 32 * Theme.scale; right: parent.right; rightMargin: 12 * Theme.scale; top: list.top }
            icon: "themes"
            title: root.line === 0 ? (Themes.get(picker.focusedId).name || qsTr("Theme")) : root.rows[root.line - 1].label
            help: root.line === 0 ? (Themes.get(picker.focusedId).description || Help.text("background")) : Help.text(root.focusedId)
        }
    }
}
