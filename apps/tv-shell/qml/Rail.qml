// One home section: a title and a horizontal list of tiles/cards. The rail
// remembers its focused item by stable id (FocusMemory) and keeps it across
// content refreshes and reorders.

import QtQuick
import BearDen

Item {
    id: root
    required property string sectionId
    required property string title
    required property var items          // ItemsModel
    required property bool isApplications
    property bool active: false          // this rail holds the D-pad focus
    property int currentIndex: 0
    // _rev re-evaluates these after the items model reconciles in place.
    property int _rev: 0
    readonly property string currentId: _rev >= 0 && items && items.count > currentIndex ? items.idAt(currentIndex) : ""
    readonly property var currentData: _rev >= 0 && items && items.count > currentIndex ? items.get(currentIndex) : ({})
    readonly property real scrollX: list.contentX
    signal activated(var item)

    implicitHeight: heading.height + 52 * Theme.scale + list.height

    function restore() {
        if (!items || items.count === 0) { currentIndex = 0; return }
        let idx = items.indexOfId(FocusMemory.itemFor(sectionId))
        if (idx < 0) idx = Math.min(Math.max(FocusMemory.indexFor(sectionId), 0), items.count - 1)
        currentIndex = idx
        list.positionViewAtIndex(idx, ListView.Contain)
        remember()
    }
    function remember() {
        if (!items || items.count === 0) return
        FocusMemory.remember(sectionId, currentId, currentIndex, list.contentX)
        if (active) Nav.reportFocus(sectionId, currentId, list.contentX)
    }
    function move(delta) {
        if (!items || items.count === 0) return false
        const next = currentIndex + delta
        if (next < 0 || next >= items.count) return false
        currentIndex = next
        remember()
        return true
    }
    // Where the tiles are, in `target`'s coordinates (bear visitors land on them).
    function tileRects(target) {
        const out = []
        if (!items) return out
        for (let i = 0; i < items.count; ++i) {
            const it = list.itemAtIndex(i)
            if (!it) continue
            const p = it.mapToItem(target, 0, 0)
            out.push({ x: p.x, y: p.y, w: it.width, h: it.height, focused: active && i === currentIndex })
        }
        return out
    }
    function activate() {
        if (items && items.count > 0) activated(items.get(currentIndex))
    }

    onActiveChanged: if (active) restore()

    // Keep focus on the same item id when the provider refreshes or reorders.
    property string _trackedId: ""
    onCurrentIdChanged: if (currentId !== "") _trackedId = currentId
    Connections {
        target: root.items
        function onReconciled() {
            root._rev++
            const idx = root.items.indexOfId(root._trackedId)
            if (idx >= 0) root.currentIndex = idx
            else root.currentIndex = Math.min(root.currentIndex, Math.max(0, root.items.count - 1))
        }
    }

    // The theme's little mark beside the heading (manifest "heading").
    Image {
        visible: url.toString().length > 0
        readonly property url url: World.heading
        source: url
        width: 72 * Theme.scale; height: 36 * Theme.scale
        sourceSize: Qt.size(width * 2, height * 2)
        fillMode: Image.PreserveAspectFit
        horizontalAlignment: Image.AlignLeft
        smooth: true
        anchors { left: heading.right; leftMargin: 14 * Theme.scale; verticalCenter: heading.verticalCenter }
        opacity: root.active ? 1 : 0.45
        Behavior on opacity { NumberAnimation { duration: Theme.duration } }
    }
    Text {
        id: heading
        text: root.title
        color: root.active ? Theme.textPrimary : Theme.textSecondary
        font.family: Theme.fontFamily
        font.pixelSize: 34 * Theme.fontUnit
        font.weight: Font.DemiBold
    }

    ListView {
        id: list
        anchors { top: heading.bottom; topMargin: 52 * Theme.scale; left: parent.left; right: parent.right }
        height: root.isApplications ? Theme.tileHeight : Theme.cardHeight
        orientation: ListView.Horizontal
        spacing: Theme.railGap
        model: root.items
        currentIndex: root.currentIndex
        interactive: false
        clip: false
        highlightFollowsCurrentItem: false
        preferredHighlightBegin: 0
        preferredHighlightEnd: width - Theme.tileWidth
        highlightRangeMode: ListView.ApplyRange
        highlightMoveDuration: Theme.duration
        onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
        Behavior on contentX { enabled: !Theme.reducedMotion; NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }
        delegate: Loader {
            id: cell
            required property var model
            required property int index
            readonly property bool isFocused: root.active && index === root.currentIndex
            // Above its neighbours, so the focused tile's vines and cub are not covered.
            z: isFocused ? 2 : 0
            // Neighbours ease aside a little to make room for the decoration.
            readonly property real push: !root.active || !World.decorated || index === root.currentIndex ? 0
                                         : (index < root.currentIndex ? -1 : 1) * 16 * Theme.scale
            transform: Translate {
                x: cell.push
                Behavior on x { enabled: !Theme.reducedMotion; NumberAnimation { duration: 900; easing.type: Easing.OutCubic } }
            }
            sourceComponent: model.kind === "app" ? appTile : (model.kind === "setup" ? setupCard : contentCard)
            Component { id: appTile; AppTile { item: cell.model; focused: cell.isFocused } }
            Component { id: contentCard; ContentCard { item: cell.model; focused: cell.isFocused } }
            Component { id: setupCard; SetupCard { item: cell.model; focused: cell.isFocused } }
        }
    }
}
