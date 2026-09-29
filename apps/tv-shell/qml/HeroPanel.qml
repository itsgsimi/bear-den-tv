// Large panel describing the focused item: artwork backdrop for content; for
// apps, the app's own colours with its icon (Bear Den's own, or the owner's
// brand icon or wordmark)
// and its state. It follows focus and never auto-advances (design §3.3).
// In the Bear Den style it also has life, all pixel art on World's heartbeat
// (docs/THEMES.md → Pixel art, "The featured panel"):
//  - the app's icon sits in a little room (HeroScene: cinema, cabin TV,
//    arcade; Apps.stage) that shifts a pixel or two as focus moves along the
//    rail;
//  - the title types itself in and the rest drops in row by row when focus
//    moves to another item;
//  - bears visit and doze on its edge (HeroLife); the cub peeks from behind
//    the OK button if focus stays;
//  - a pumpkin in October, snow on its top edge in December.
// Classic draws the same life smooth (the rooms, the snow, the pumpkin
// ornament's SVG).
// Plain and Performance keep the plain panel; reduced motion shows everything
// at once.

import QtQuick
import BearDen

PixelBox {
    id: root
    property var item: ({})
    property string sectionTitle: ""
    readonly property bool isApp: item.kind === "app"
    readonly property var app: isApp && item.appId ? Session.application(item.appId) : ({})
    readonly property var art: isApp ? Shell.appArt(app.adapter || "", World.classic) : ({ icon: "", logo: "", background: "" })
    readonly property string backdrop: isApp ? art.background : (item.artwork || "")
    readonly property bool hasBackdrop: backdropImage.ready
    readonly property color tint: item && item.tint ? item.tint : Theme.tintFor(item && item.itemId ? item.itemId : "bear")
    readonly property var brand: isApp ? Apps.brand(app.adapter || "") : null
    readonly property var stage: isApp ? Apps.stage(app.adapter || "") : null
    property int focusIndex: 0                 // position in the rail, for parallax
    readonly property bool lively: World.decorated && !World.performance
    readonly property bool staged: lively && isApp && !hasBackdrop && art.logo.length === 0
    // The cub behind the OK button: after focus has stayed 7 s on one item.
    property bool cubPeeking: false
    Timer {
        id: cubTimer
        interval: 7000
        running: root.lively && !Theme.reducedMotion && !Theme.resting && Session.target.kind === "shell"
        onTriggered: root.cubPeeking = true
    }

    radius: Theme.radius * 1.4
    color: Theme.surface
    borderColor: Theme.surfaceBorder
    borderWidth: 1

    PixelBox {
        visible: !root.isApp
        anchors.fill: parent
        radius: parent.radius
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0; color: Theme.alpha(Qt.darker(root.tint, 2.8), 0.95) }
            GradientStop { position: 0.6; color: Theme.alpha(Qt.darker(root.tint, 1.8), 0.7) }
            GradientStop { position: 1; color: Theme.alpha(root.tint, 0.55) }
        }
    }
    BrandBackdrop {
        visible: root.isApp
        anchors.fill: parent
        radius: root.radius
        topColor: root.brand ? root.brand.top : Qt.darker(root.tint, 1.7)
        bottomColor: root.brand ? root.brand.bottom : Qt.darker(root.tint, 3.4)
        glow: root.brand ? root.brand.glow : root.tint
        glowX: 0.8
        glowY: 0.5
        glowRadius: 0.3
        glowStrength: 0.38
    }
    // Artwork on the right, rounded and fading into the text side.
    RoundedImage {
        id: backdropImage
        anchors.fill: parent
        radius: root.radius
        coverage: 0.68
        fade: 0.55
        source: root.backdrop
    }
    PixelBox {
        anchors.fill: parent
        radius: root.radius
        color: "transparent"
        borderColor: Theme.surfaceBorder
        borderWidth: 1
    }
    // The world's decoration hugging the panel's corners.
    HeroDecor { anchors.fill: parent; radius: root.radius }
    // Content fades in whenever focus moves to another item.
    property string _shownId: ""
    onItemChanged: {
        const id = item && item.itemId ? item.itemId : ""
        if (id !== _shownId) { _shownId = id; if (!Theme.reducedMotion && !lively) fadeIn.restart() }
    }
    NumberAnimation { id: fadeIn; target: content; property: "opacity"; from: 0.25; to: 1; duration: 260; easing.type: Easing.OutCubic }
    // Typing and the row-by-row drop, on World's heartbeat (fadeIn instead in
    // Plain/Performance; everything at once with reduced motion).
    readonly property string fullTitle: item.title || qsTr("Welcome to Bear Den")
    property int typed: 1e6
    property real revealed: 1
    property real cursorFor: 0
    readonly property bool typing: typed < fullTitle.length || revealed < 1 || cursorFor > 0
    onFullTitleChanged: {
        cubPeeking = false
        cubTimer.restart()
        if (lively && !Theme.reducedMotion && !Theme.resting) { typed = 0; revealed = 0; cursorFor = 1.4 }
        else { typed = 1e6; revealed = 1; cursorFor = 0 }
    }
    Connections {
        target: World
        enabled: root.typing && root.lively
        function onBeat(dt) {
            root.typed = Math.min(root.fullTitle.length, root.typed + Math.max(1, Math.round(root.fullTitle.length / 8)))
            if (root.typed >= root.fullTitle.length) {
                root.revealed = Math.min(1, root.revealed + dt / 0.35)
                if (root.revealed >= 1) root.cursorFor = Math.max(0, root.cursorFor - dt)
            }
        }
    }
    // App identity on the right when there is no backdrop photo.
    HeroScene {
        id: room
        visible: root.staged
        scene: root.stage ? root.stage.scene : ""
        itemId: root.item.itemId || ""
        shift: (root.focusIndex % 5) - 2
        anchors { right: parent.right; rightMargin: 12 * World.px; verticalCenter: parent.verticalCenter }
        AppIcon {
            visible: !room.introPlaying
            readonly property real side: Math.min(room.screenRect.width, room.screenRect.height) * 0.86
            size: side
            x: room.screenRect.x + (room.screenRect.width - side) / 2
            y: room.screenRect.y + (room.screenRect.height - side) / 2
            adapter: root.app.adapter || ""
            label: root.item.title || ""
            tint: root.tint
        }
    }
    Item {
        visible: root.isApp && !root.hasBackdrop && !root.staged
        anchors { right: parent.right; top: parent.top; bottom: parent.bottom }
        width: parent.width * 0.4
        Image {
            visible: root.art.logo.length > 0
            anchors.centerIn: parent
            width: parent.width
            height: parent.height * 0.5
            source: root.art.logo
            sourceSize: Qt.size(width * 2, height * 2)
            fillMode: Image.PreserveAspectFit
        }
        AppIcon {
            visible: root.art.logo.length === 0
            anchors.centerIn: parent
            size: 210 * Theme.scale
            adapter: root.app.adapter || ""
            label: root.item.title || ""
            tint: root.tint
        }
    }
    // Bears visiting and dozing on the panel's bottom edge.
    HeroLife {
        anchors.fill: parent
        visible: root.lively
        item: root.item
        stage: root.staged ? root.stage : null
        standX: room.x + room.width * 0.12
    }
    // The seasons: a pumpkin patch in October, snow on the top edge in December.
    readonly property int month: Shell.monthOverride > 0 ? Shell.monthOverride : new Date().getMonth() + 1
    Row {
        visible: root.lively && root.month === 10
        x: 3 * World.px
        y: parent.height - height + 2 * World.px
        spacing: World.px
        Repeater {
            model: [11, 9, 11]
            Ornament { required property int modelData; name: "pumpkin"; width: modelData * World.px; height: 10 * World.px }
        }
    }
    Item {
        visible: root.lively && root.month === 12
        x: root.radius
        y: -3 * World.px
        width: parent.width - 2 * root.radius
        height: 5 * World.px
        Image {
            visible: World.pixel
            width: Math.ceil(parent.width / World.px)
            height: 5
            scale: World.px
            transformOrigin: Item.TopLeft
            source: World.pixel ? "qrc:/qt/qml/BearDen/assets/pixel/snowcap.png" : ""
            fillMode: Image.Tile
            smooth: false
        }
        // Classic: the smooth tile (tools/classicart/extras.py), same size.
        Image {
            objectName: "heroSnowcapClassic"
            visible: World.classic
            anchors.fill: parent
            source: World.classic ? "qrc:/qt/qml/BearDen/assets/classic/snowcap.svg" : ""
            sourceSize: Qt.size(32 * World.px, 5 * World.px)
            fillMode: Image.Tile
            smooth: true
        }
    }
    BearHead {
        visible: !root.isApp && !root.hasBackdrop && !World.performance
        anchors { right: parent.right; verticalCenter: parent.verticalCenter; rightMargin: 60 * Theme.scale }
        width: parent.height * 0.9
        kind: "dad"
        opacity: 0.16
    }

    Column {
        id: content
        anchors { left: parent.left; leftMargin: 104 * Theme.scale; right: parent.right; rightMargin: parent.width * 0.38; verticalCenter: parent.verticalCenter }
        spacing: 12 * Theme.scale
        Row {
            visible: !root.isApp
            spacing: 14 * Theme.scale
            Text {
                text: root.item.kind === "setup" ? qsTr("SET UP") : root.sectionTitle.toUpperCase()
                color: Theme.accent
                font.family: Theme.fontFamily
                font.pixelSize: 20 * Theme.fontUnit
                font.weight: Font.Bold
                font.letterSpacing: 4 * Theme.scale
                anchors.verticalCenter: parent.verticalCenter
            }
            DemoBadge { visible: root.item.demo === true; anchors.verticalCenter: parent.verticalCenter }
        }
        Text {
            id: title
            width: parent.width
            text: root.fullTitle.substring(0, root.typed)
            color: Theme.textPrimary
            elide: Text.ElideRight
            font.family: Theme.fontFamily
            font.pixelSize: 72 * Theme.fontUnit
            font.weight: Font.Bold
            // A block cursor at the end while the title types, blinking a
            // moment after.
            Rectangle {
                visible: root.cursorFor > 0 && (root.typed < root.fullTitle.length || Math.floor(root.cursorFor * 3) % 2 === 0)
                x: Math.round((title.contentWidth + 2 * World.px) / World.px) * World.px
                y: Math.round(title.height * 0.22 / World.px) * World.px
                width: 3 * World.px
                height: Math.round(title.height * 0.62 / World.px) * World.px
                color: Theme.accent
            }
        }
        Text {
            width: parent.width
            text: root.isApp ? Apps.about(root.app.adapter) : (root.item.subtitle || "")
            visible: text.length > 0
            opacity: root.revealed >= 0.2 ? 1 : 0
            color: Theme.textSecondary
            wrapMode: Text.WordWrap
            maximumLineCount: 2
            elide: Text.ElideRight
            font.family: Theme.fontFamily
            font.pixelSize: 26 * Theme.fontUnit
        }
        // App state, only when there is something to say (install details
        // and versions live in Settings → Diagnostics).
        Row {
            visible: root.isApp && facts.count > 0
            opacity: root.revealed >= 0.45 ? 1 : 0
            spacing: 12 * Theme.scale
            Repeater {
                id: facts
                model: {
                    if (!root.isApp) return []
                    const a = root.app, out = []
                    if (!a.installed) out.push({ text: qsTr("Not installed"), color: Theme.warning })
                    if (a.foreground) out.push({ text: qsTr("On screen"), color: Theme.success })
                    else if (a.running) out.push({ text: qsTr("Running"), color: Theme.success })
                    if (a.last_error) out.push({ text: a.last_error, color: Theme.danger })
                    return out
                }
                PixelBox {
                    required property var modelData
                    implicitWidth: factRow.implicitWidth + 32 * Theme.scale
                    implicitHeight: 40 * Theme.scale
                    radius: height / 2
                    color: Theme.alpha("#000000", 0.4)
                    borderColor: Theme.alpha("#ffffff", 0.14)
                    Row {
                        id: factRow
                        anchors.centerIn: parent
                        spacing: 10 * Theme.scale
                        PixelBox {
                            width: 10 * Theme.scale; height: width; radius: width / 2
                            color: modelData.color
                            anchors.verticalCenter: parent.verticalCenter
                        }
                        Text {
                            text: modelData.text
                            color: Theme.textPrimary
                            font.family: Theme.fontFamily
                            font.pixelSize: 20 * Theme.fontUnit
                            anchors.verticalCenter: parent.verticalCenter
                        }
                    }
                }
            }
        }
        ProgressBar {
            visible: !root.isApp && root.item.progress !== undefined && root.item.progress >= 0
            width: parent.width * 0.6
            value: root.item.progress || 0
            opacity: root.revealed >= 0.6 ? 1 : 0
        }
        Row {
            spacing: 16 * Theme.scale
            topPadding: 8 * Theme.scale
            opacity: root.revealed >= 0.8 ? 1 : 0
            // Primary action, with the remote's OK key shown as a keycap.
            PixelBox {
                id: okButton
                implicitWidth: okRow.implicitWidth + 44 * Theme.scale
                implicitHeight: 64 * Theme.scale
                radius: height / 2
                color: Theme.pillActiveBg
                // The cub sneaks up behind the button when focus stays a while,
                // and ducks back down as soon as focus moves (drawn behind it).
                BearHead {
                    z: -1
                    kind: "cub"
                    width: 21 * World.px
                    alive: root.cubPeeking
                    x: okButton.width - width - 3 * World.px
                    visible: root.cubPeeking || cubRise.running
                    y: root.cubPeeking ? -height * 0.62 : 0
                    Behavior on y { enabled: !Theme.reducedMotion; NumberAnimation { id: cubRise; duration: 380; easing.type: Easing.OutBack } }
                }
                Row {
                    id: okRow
                    anchors.centerIn: parent
                    spacing: 14 * Theme.scale
                    PixelBox {
                        anchors.verticalCenter: parent.verticalCenter
                        implicitWidth: keyText.implicitWidth + 16 * Theme.scale
                        implicitHeight: 32 * Theme.scale
                        radius: 8 * Theme.scale
                        color: "transparent"
                        borderColor: Theme.alpha(Theme.pillActiveText, 0.55)
                        borderWidth: 2 * Theme.scale
                        Text {
                            id: keyText
                            anchors.centerIn: parent
                            text: qsTr("OK")
                            color: Theme.pillActiveText
                            font.family: Theme.fontFamily
                            font.pixelSize: 16 * Theme.fontUnit
                            font.weight: Font.Bold
                        }
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.isApp
                              ? (root.item.installed === false ? qsTr("How to install")
                                 : (root.item.running ? qsTr("Switch to %1").arg(root.item.title) : qsTr("Open %1").arg(root.item.title)))
                              : (root.item.kind === "setup" ? qsTr("Open Settings")
                                 : (root.item.progress !== undefined && root.item.progress > 0 ? qsTr("Resume in Plex") : qsTr("Open in Plex")))
                        color: Theme.pillActiveText
                        font.family: Theme.fontFamily
                        font.pixelSize: 25 * Theme.fontUnit
                        font.weight: Font.Bold
                    }
                }
            }
        }
    }
}
