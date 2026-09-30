// Application tile in the app's own colours: its icon (Bear Den's own, or the
// owner's brand icon; AppIcon), a faint oversized copy of that icon as texture, the name, what the app is for, and a
// status pill (Running / Starting / Not installed / Installing 42% / Launch
// failed) so state is never conveyed by colour alone. While its install runs
// (state.applications[].install) a bar along the bottom shows how far it is,
// moved only by state pushes.

import QtQuick
import BearDen

Item {
    id: root
    property var item: ({})
    property bool focused: false
    readonly property var app: item.appId ? Session.application(item.appId) : ({})
    readonly property string adapter: app.adapter || ""
    readonly property var brand: Apps.brand(adapter)
    readonly property color tint: item.tint || Theme.accent
    // Artwork: the owner's logo and photo (brand folder), empty when absent.
    readonly property var art: Shell.appArt(adapter, World.classic, Theme.appIcons)
    readonly property bool hasLogo: art.logo.length > 0
    readonly property bool hasPhoto: art.background.length > 0
    readonly property bool installing: item.installState === "preparing" || item.installState === "downloading" || item.installState === "installing"
    readonly property var status: {
        if (installing) return { text: qsTr("Installing %1%").arg(item.installProgress || 0), color: Theme.accent }
        if (item.installed === false && item.installState === "failed") return { text: qsTr("Install failed"), color: Theme.danger }
        if (item.installed === false) return { text: qsTr("Not installed"), color: Theme.warning }
        if (item.launchState === "launching") return { text: qsTr("Starting…"), color: Theme.accent }
        if (item.launchState === "failed" || item.launchState === "crashed") return { text: qsTr("Launch failed"), color: Theme.danger }
        if (item.running) return { text: app.foreground ? qsTr("On screen") : qsTr("Running"), color: Theme.success }
        // The streaming sites' first-run note (Apps.note): until first opened.
        if (item.launchState === "idle" && Apps.note(adapter).length > 0) return { text: Apps.note(adapter), color: Theme.accent }
        return null
    }
    width: Theme.tileWidth
    height: Theme.tileHeight
    z: focused ? 2 : 0

    // Peekaboo: once focus has rested on the tile for a moment, a cub peeks
    // over its top edge holding something that fits the app, blinks now and
    // then, and ducks back down when focus moves on. Never while Bear Den rests
    // or with reduced motion.
    readonly property bool canPeek: focused && World.decorated && !Theme.reducedMotion && item.installed !== false
    property bool peeking: false
    onCanPeekChanged: if (!canPeek) peeking = false
    Timer {
        interval: 1400
        running: root.canPeek && !root.peeking && !Theme.resting && Session.target.kind === "shell"
        onTriggered: root.peeking = true
    }
    property bool blink: false
    Timer {
        interval: 3600
        repeat: true
        running: root.peeking && !Theme.resting && Session.target.kind === "shell"
        onTriggered: { root.blink = true; blinkOff.restart(); interval = 2800 + Math.random() * 3600 }
    }
    Timer { id: blinkOff; interval: 150; onTriggered: root.blink = false }
    readonly property string prop: Apps.stage(adapter).prop
    readonly property real cubSize: 72 * Theme.scale
    // Where the cub peeks, as a share of the tile's width (UX-11).
    readonly property real cubAt: 0.78

    // Behind the tile body: the cub's head rises above the top edge.
    Item {
        anchors.fill: parent
        scale: tile.scale
        BearHead {
            kind: "cub"
            blink: root.blink
            width: root.cubSize
            // Pixel: on the grid, never rotated. Classic: smooth, with a tilt
            // that settles elastically.
            // Over the right part of the tile, clear of the rail heading at the
            // left and the featured panel's tagline (UX-11).
            x: World.classic ? parent.width * root.cubAt - width / 2 : World.snap(parent.width * root.cubAt - width / 2)
            y: World.classic ? (root.peeking ? -height * 0.74 : height * 0.2)
                             : root.peeking ? World.snap(-height * 0.74) : World.snap(height * 0.2)
            Behavior on y { NumberAnimation { id: peekAnim; duration: 460; easing.type: Easing.OutBack } }
            rotation: World.classic && root.peeking ? -7 : 0
            Behavior on rotation { enabled: World.classic; NumberAnimation { duration: 700; easing.type: Easing.OutElastic } }
            visible: root.peeking || peekAnim.running
        }
    }

    // A not-installed tile is dimmed over an opaque base, so a bright scene
    // (Winter's snow) never shows through it (UX-04).
    PixelBox {
        objectName: "tileBase"
        anchors.fill: parent
        scale: tile.scale
        visible: tile.opacity < 1
        radius: Theme.radius
        color: Theme.surfaceRaised
        borderColor: "transparent"
    }
    Item {
        id: tile
        anchors.fill: parent
        scale: root.focused ? Theme.focusScale : 1
        Behavior on scale { NumberAnimation { duration: Theme.duration; easing.type: Easing.OutCubic } }
        // Dimmed while not installed, but full strength when focused (UX-20).
        opacity: root.item.installed === false && !root.installing && !root.focused ? 0.6 : 1

        BrandBackdrop {
            anchors.fill: parent
            topColor: root.brand ? root.brand.top : Qt.darker(root.tint, 1.7)
            bottomColor: root.brand ? root.brand.bottom : Qt.darker(root.tint, 3.4)
            glow: root.brand ? root.brand.glow : root.tint
            glowX: 0.14
            glowY: 0.26
            glowRadius: 0.62
            glowStrength: 0.4
        }
        // Owner-provided backdrop photo (brand folder), when there is one.
        RoundedImage {
            anchors.fill: parent
            visible: root.hasPhoto
            radius: Theme.radius
            coverage: 1
            fade: 0
            source: root.art.background
        }
        // The app's icon, oversized and faint, as the tile's texture.
        Item {
            anchors.fill: parent
            clip: true
            visible: !root.hasPhoto && !root.hasLogo
            // Kept clear of the rounded corners (the clip is rectangular).
            AppIcon {
                size: parent.height * 0.7
                anchors { right: parent.right; rightMargin: -size * 0.16; verticalCenter: parent.verticalCenter }
                adapter: root.adapter
                installed: root.item.installed !== false
                label: root.item.title || ""
                tint: root.tint
                opacity: 0.12
            }
        }
        // Soft top sheen and a bottom shade that keeps the name readable.
        PixelBox {
            anchors.fill: parent
            radius: Theme.radius
            gradient: Gradient {
                GradientStop { position: 0; color: Theme.alpha("#ffffff", 0.07) }
                GradientStop { position: 0.4; color: "transparent" }
                GradientStop { position: 0.55; color: "transparent" }
                GradientStop { position: 1; color: Theme.alpha("#000000", 0.45) }
            }
        }
        PixelBox {
            anchors.fill: parent
            radius: Theme.radius
            color: "transparent"
            borderColor: Theme.alpha("#ffffff", 0.12)
            borderWidth: 1
        }

        // The owner's wordmark (brand folder), when there is one.
        Image {
            visible: root.hasLogo
            anchors.centerIn: parent
            anchors.verticalCenterOffset: -10 * Theme.scale
            width: parent.width * 0.66
            height: parent.height * 0.4
            source: root.art.logo
            sourceSize: Qt.size(width * 2, height * 2)
            fillMode: Image.PreserveAspectFit
            smooth: true
        }
        AppIcon {
            visible: !root.hasLogo
            anchors { left: parent.left; top: parent.top; leftMargin: 22 * Theme.scale; topMargin: 20 * Theme.scale }
            size: 64 * Theme.scale
            adapter: root.adapter
            installed: root.item.installed !== false
            label: root.item.title || ""
            tint: root.tint
        }
        PixelBox {
            visible: root.status !== null
            anchors { right: parent.right; top: parent.top; margins: 16 * Theme.scale }
            implicitWidth: pill.implicitWidth + 30 * Theme.scale
            implicitHeight: 34 * Theme.scale
            radius: height / 2
            color: Theme.alpha("#000000", 0.55)
            Row {
                id: pill
                anchors.centerIn: parent
                spacing: 8 * Theme.scale
                StatusDot {
                    color: root.status ? root.status.color : "transparent"
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    text: root.status ? root.status.text : ""
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 17 * Theme.fontUnit
                    font.weight: Font.DemiBold
                    anchors.verticalCenter: parent.verticalCenter
                }
            }
        }
        ProgressBar {
            objectName: "tileInstallProgress"
            visible: root.installing
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: 22 * Theme.scale; rightMargin: 22 * Theme.scale; bottomMargin: 8 * Theme.scale }
            height: 8 * Theme.scale
            value: (root.item.installProgress || 0) / 100
        }
        Column {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: 22 * Theme.scale; rightMargin: 22 * Theme.scale; bottomMargin: 18 * Theme.scale }
            spacing: 2 * Theme.scale
            Text {
                visible: !root.hasLogo
                width: parent.width
                text: root.item.title || ""
                color: "#ffffff"
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 32 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                width: parent.width
                text: Apps.tagline(root.adapter)
                visible: text.length > 0
                color: Theme.alpha("#ffffff", 0.8)
                elide: Text.ElideRight
                font.family: Theme.fontFamily
                font.pixelSize: 18 * Theme.fontUnit
            }
        }
    }
    Item {
        anchors.fill: tile
        scale: tile.scale
        FocusFrame { shown: root.focused; glint: true }
        FocusDecor { anchors.fill: parent; shown: root.focused }
    }
    // In front of the tile: the cub's paws on the edge, and what it holds.
    Item {
        anchors.fill: tile
        scale: tile.scale
        opacity: root.peeking ? 1 : 0
        visible: opacity > 0
        Behavior on opacity { NumberAnimation { duration: 180 } }
        readonly property real cx: width * root.cubAt
        Repeater {
            model: 2
            Ornament {
                required property int index
                name: "paw-grip"
                width: root.cubSize * 0.3
                height: width * 44 / 64
                x: parent.cx + (index === 0 ? -1 : 1) * root.cubSize * 0.33 - width / 2
                y: -height * 0.7
            }
        }
        Ornament {
            visible: root.prop.length > 0
            name: root.prop
            readonly property var fit: root.prop === "popcorn" ? { w: 0.4, aspect: 80 / 64, dx: 0.62, lift: 0.9, turn: 8 }
                                     : root.prop === "remote" ? { w: 0.2, aspect: 90 / 40, dx: -0.6, lift: 0.88, turn: -20 }
                                     : root.prop === "heart" ? { w: 0.3, aspect: 8 / 9, dx: 0.6, lift: 0.9, turn: 10 }
                                     : { w: 0.64, aspect: 64 / 100, dx: 0, lift: 0.78, turn: 0 }
            width: root.cubSize * fit.w
            height: width * fit.aspect
            x: parent.cx + root.cubSize * fit.dx - width / 2
            y: -height * fit.lift
            rotation: fit.turn
        }
    }
}
