// A strip of large theme cards, one per installed theme (Themes.list): each
// is a small render of that world (its wallpaper in the current art style,
// its accent on a row of little tiles, the bear mark) with its name. Moving
// along the strip tries the theme on the whole TV at once
// (Session.previewUi: nothing is sent or stored); OK applies it (a layout
// update with the theme and its accent, like Settings did); leaving without
// OK (cancel(), Back) returns to the theme in use. Used by the Themes page
// and the first-run setup's "Your look" step. Never branches on a theme id.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

ListView {
    id: root
    objectName: "themePicker"
    property bool active: false
    property int index: 0
    readonly property var ids: Themes.list.map(t => t.id)
    // The theme in use (the persisted layout, not a preview).
    // (Session.layout is read so this follows every new snapshot.)
    readonly property string applied: Session.layout && Themes.canonical(((Session.layoutForEdit() || {}).ui || {}).background || "")
    readonly property string focusedId: ids[Math.max(0, Math.min(ids.length - 1, index))] || ""

    // Start on the theme in use.
    function reset() {
        const i = ids.indexOf(applied)
        index = i >= 0 ? i : 0
        positionViewAtIndex(index, ListView.Contain)
    }
    function previewFocused() {
        const persisted = (Session.layoutForEdit() || {}).ui
        if (!persisted) return
        if (focusedId === applied) { Session.endUiPreview(); return }
        const ui = JSON.parse(JSON.stringify(persisted))
        ui.background = focusedId
        ui.accent = World.accentFor(focusedId)
        Session.previewUi(ui)
    }
    // ◀ ▶: move and try the theme on.
    function move(delta) {
        const next = Math.max(0, Math.min(ids.length - 1, index + delta))
        if (next === index) return false
        index = next
        previewFocused()
        return true
    }
    // OK: use the focused theme (the preview ends when the coordinator's
    // layout says the same).
    function apply() {
        const layout = JSON.parse(JSON.stringify(Session.layoutForEdit()))
        if (!layout.ui) return
        layout.ui.background = focusedId
        layout.ui.accent = World.accentFor(focusedId)
        Shell.updateLayout(layout)
    }
    // Leaving without OK: back to the theme in use.
    function cancel() { Session.endUiPreview() }

    height: 284 * Theme.scale
    orientation: ListView.Horizontal
    spacing: 26 * Theme.scale
    interactive: false
    // Clipped to the page's safe area; the margins leave room for the
    // focused card's scale-up and ring.
    clip: true
    model: Themes.list
    currentIndex: index
    highlightFollowsCurrentItem: false
    onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
    leftMargin: 24 * Theme.scale
    rightMargin: 24 * Theme.scale

    delegate: Item {
        id: card
        required property var modelData
        required property int index
        readonly property bool focused: root.active && index === root.index
        readonly property bool inUse: modelData.id === root.applied
        readonly property var look: Themes.get(modelData.id, World.classic ? "classic" : "pixel")
        readonly property var wall: look.wallpaper || ({})
        readonly property color accent: modelData.accent || Theme.accent
        objectName: "themeCard"
        property string themeId: modelData.id
        width: 360 * Theme.scale
        height: 240 * Theme.scale
        y: 22 * Theme.scale
        scale: focused ? 1.05 : 1
        z: focused ? 2 : 0
        Behavior on scale { NumberAnimation { duration: Theme.durationFast } }

        PixelBox {
            id: frame
            anchors.fill: parent
            radius: 20 * Theme.scale
            color: card.wall.bottom || Theme.surface
            borderColor: card.focused ? card.accent : Theme.surfaceBorder
            borderWidth: card.focused ? 2 * Theme.scale : 1
            // The world itself, cropped to the card.
            RoundedImage {
                anchors.fill: parent
                anchors.margins: 1
                radius: frame.radius
                coverage: 1
                fade: 0
                source: card.wall.image || ""
            }
            // A bottom shade so the name reads on any world.
            PixelBox {
                anchors.fill: parent
                radius: frame.radius
                gradient: Gradient {
                    GradientStop { position: 0.45; color: "transparent" }
                    GradientStop { position: 1; color: Theme.alpha("#000000", 0.75) }
                }
            }
            // A miniature Home: the bear mark and a row of tiles in the accent.
            BearMark { size: 40 * Theme.scale; anchors { left: parent.left; top: parent.top; margins: 16 * Theme.scale } }
            Row {
                anchors { left: parent.left; leftMargin: 18 * Theme.scale; bottom: name.top; bottomMargin: 12 * Theme.scale }
                spacing: 8 * Theme.scale
                Repeater {
                    model: 3
                    PixelBox {
                        required property int index
                        width: 58 * Theme.scale; height: 34 * Theme.scale
                        radius: 7 * Theme.scale
                        color: index === 0 ? card.accent : Theme.alpha(card.accent, 0.4)
                        borderColor: Theme.alpha("#ffffff", index === 0 ? 0.8 : 0.2)
                        borderWidth: index === 0 ? 2 * Theme.scale : 1
                    }
                }
            }
            Text {
                id: name
                anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: 16 * Theme.scale }
                text: card.modelData.name || card.modelData.id
                elide: Text.ElideRight
                color: "#ffffff"
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
                font.weight: Font.Bold
            }
            PixelBox {
                visible: card.inUse
                anchors { right: parent.right; top: parent.top; margins: 14 * Theme.scale }
                implicitWidth: inUseText.implicitWidth + 24 * Theme.scale
                implicitHeight: 34 * Theme.scale
                radius: height / 2
                color: Theme.alpha("#000000", 0.6)
                Text {
                    id: inUseText
                    anchors.centerIn: parent
                    text: qsTr("In use")
                    color: "#ffffff"
                    font.family: Theme.fontFamily
                    font.pixelSize: 18 * Theme.fontUnit
                    font.weight: Font.DemiBold
                }
            }
        }
        FocusFrame { shown: card.focused; cornerRadius: frame.radius }
    }
}
