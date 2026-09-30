// Home: header pills, hero for the focused item, and one rail per enabled
// section. Up/Down moves between rails (and up into the header), Left/Right
// within a rail; each rail restores its item by id. The first apps rail ends
// with an "Add apps" tile while Bear Den could install something
// (SessionModel adds it, kind `add-apps`); OK on it opens the Apps page at
// Add apps.

import QtQuick
import BearDen

Item {
    id: root
    property int row: 0            // rail index; -1 = header pills
    property int headerIndex: 0
    readonly property int railCount: rails.count
    readonly property var activeRail: row >= 0 && rails.count > row ? rails.itemAt(row) : null

    signal openScreen(string name)
    signal appUnavailable(var app)
    signal message(string title, string body)

    function restoreFocus() {
        const idx = Session.sections.indexOfSection(FocusMemory.lastSectionId)
        row = railCount === 0 ? -1 : (idx >= 0 ? idx : 0)
        headerIndex = 0
        if (activeRail) activeRail.restore()
        else reportHeader()
    }
    // The app tiles' rectangles in `target`'s coordinates, from the rail holding
    // focus (or the first one); empty when it is not a row of apps.
    function tileRects(target) {
        const r = activeRail ? activeRail : (rails.count > 0 ? rails.itemAt(0) : null)
        return r && r.isApplications ? r.tileRects(target) : []
    }
    // The app id of the focused tile ("" when focus is not on an app).
    function focusedAppId() {
        const d = activeRail ? activeRail.currentData : null
        return d && d.kind === "app" ? d.appId : ""
    }
    function reportHeader() {
        Nav.reportFocus("header", header.pills[headerIndex][0], 0)
    }
    function setRow(r) {
        row = r
        if (row < 0) reportHeader()
    }

    function navigate(action) {
        switch (action) {
        case "nav.up":
            if (row > 0) setRow(row - 1)
            else if (row === 0) setRow(-1)
            return true
        case "nav.down":
            if (row < 0 && railCount > 0) setRow(0)
            else if (row >= 0 && row < railCount - 1) setRow(row + 1)
            return true
        case "nav.left":
        case "nav.right": {
            const d = action === "nav.left" ? -1 : 1
            if (row < 0) {
                headerIndex = Math.max(0, Math.min(header.pills.length - 1, headerIndex + d))
                reportHeader()
            } else if (activeRail) {
                activeRail.move(d)
            }
            return true
        }
        case "select":
            if (row < 0) {
                const target = header.pills[headerIndex][0]
                if (target === "home") setRow(railCount > 0 ? 0 : -1)
                else openScreen(target)
            } else if (activeRail) {
                activeRail.activate()
            }
            return true
        case "back":
            if (row < 0 && railCount > 0) { setRow(0); return true }
            return false   // at the root: ShellRoot reports at_root, never exits
        }
        return false
    }

    function activateItem(item) {
        if (item.kind === "add-apps") {
            openScreen("add-apps")   // the Apps page, at Add apps
        } else if (item.kind === "app") {
            if (item.installed === false) appUnavailable(Session.application(item.appId))
            else Shell.launchApp(item.appId)
        } else if (item.kind === "setup") {
            // Empty, loading or failing Plex rows: Settings → Plex when this
            // session has the connector (state.plex), else Settings.
            openScreen(Session.plex.status !== undefined ? "plex" : "settings")
        } else if (item.demo) {
            message(qsTr("DEMO item"), qsTr("“%1” is demo content from --dev-fixtures. With a real Plex connection, OK opens it in Plex HTPC.").arg(item.title))
        } else {
            // Provider items come from the Plex connector: open Plex HTPC itself
            // (play_exact deep links are not verified yet, so both open the app).
            Shell.launchApp("plex-htpc")
        }
    }

    Header {
        id: header
        anchors { top: parent.top; left: parent.left; right: parent.right }
        current: "home"
        focusIndex: root.row < 0 ? root.headerIndex : -1
    }

    HeroPanel {
        id: hero
        visible: Theme.heroEnabled
        anchors { top: header.bottom; topMargin: 28 * Theme.scale; left: parent.left; right: parent.right }
        height: visible ? 380 * Theme.scale : 0
        item: root.activeRail ? root.activeRail.currentData : ({})
        focusIndex: root.activeRail ? root.activeRail.currentIndex : 0
        sectionTitle: root.activeRail ? root.activeRail.title : ""
    }

    // The theme's corner scene (manifest "scene": den, camp, moon, campfire), when
    // the rail leaves room for it. None in the Plain style. Pixel scenes are
    // 110×82 art pixels; Classic ones (<Name>Classic.qml) are smooth, drawn
    // at the same width (4:3).
    Loader {
        objectName: "cornerScene"
        readonly property var firstRail: rails.count > 0 ? rails.itemAt(0) : null
        readonly property real classicSize: 110 * World.px   // as wide as the pixel scenes
        width: World.classic ? classicSize : 110 * World.px
        height: World.classic ? classicSize * 0.75 : 82 * World.px
        anchors { right: parent.right; bottom: parent.bottom; rightMargin: 8 * Theme.scale }
        active: World.scene !== "" && Session.loaded && root.railCount === 1 && firstRail !== null && firstRail.items !== null
                && firstRail.items.count * (Theme.tileWidth + Theme.railGap) + width + 60 * Theme.scale < root.width
        sourceComponent: (World.classic
                          ? { den: denClassic, camp: campClassic, moon: moonClassic, campfire: campfireClassic }
                          : { den: denScene, camp: campScene, moon: moonScene, campfire: campfireScene })[World.scene] || null
        Component { id: denScene; DenFamily {} }
        Component { id: campScene; CampScene {} }
        Component { id: moonScene; MoonScene {} }
        Component { id: campfireScene; CampfireScene {} }
        Component { id: denClassic; DenFamilyClassic { size: 110 * World.px } }
        Component { id: campClassic; CampSceneClassic { size: 110 * World.px } }
        Component { id: moonClassic; MoonSceneClassic { size: 110 * World.px } }
        Component { id: campfireClassic; CampfireSceneClassic { size: 110 * World.px } }
    }

    Flickable {
        id: railsView
        anchors { top: hero.visible ? hero.bottom : header.bottom; topMargin: 36 * Theme.scale; left: parent.left; right: parent.right; bottom: parent.bottom }
        // Wider than the page so a focused tile's ring and vines are not clipped.
        anchors.leftMargin: -44 * Theme.scale
        anchors.rightMargin: -44 * Theme.scale
        contentHeight: column.height + 40 * Theme.scale
        interactive: false
        clip: true
        contentY: {
            const r = root.activeRail
            if (!r) return 0
            const maxY = Math.max(0, contentHeight - height)
            return Math.min(maxY, Math.max(0, r.y - 8 * Theme.scale))
        }
        Behavior on contentY { enabled: !Theme.reducedMotion; NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }

        Column {
            id: column
            x: 44 * Theme.scale
            y: 10 * Theme.scale
            width: railsView.width - 88 * Theme.scale
            spacing: 40 * Theme.scale
            Repeater {
                id: rails
                model: Session.sections
                Rail {
                    required property int index
                    width: column.width
                    active: root.row === index
                    onActivated: (item) => root.activateItem(item)
                }
            }
        }
    }

    DenMascot {
        visible: root.railCount === 0 && Session.loaded && !World.performance
        size: 260 * Theme.scale
        anchors { horizontalCenter: railsView.horizontalCenter; bottom: emptyText.top; bottomMargin: 20 * Theme.scale }
    }
    Text {
        id: emptyText
        visible: root.railCount === 0 && Session.loaded
        anchors.centerIn: railsView
        text: qsTr("No sections are enabled. Open Settings to choose what appears here.")
        color: Theme.textSecondary
        font.family: Theme.fontFamily
        font.pixelSize: 28 * Theme.fontUnit
    }

    Connections {
        target: Session.sections
        function onReconciled() {
            if (root.row >= root.railCount) root.setRow(root.railCount - 1)
        }
    }
}
