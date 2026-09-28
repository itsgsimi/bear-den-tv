// Full-screen backdrop: the theme's wallpaper (a smooth picture, or a pixel-art
// world, each with optional animated layers; manifest `wallpaper`, contracts/theme.schema.json,
// docs/THEMES.md), local weather (WeatherSky, and Ambient's weather particles)
// plus scrims so the header and rails stay readable.

import QtQuick
import BearDen

Item {
    id: root
    readonly property bool pixel: World.wallpaper.pixel === true
    Rectangle {
        anchors.fill: parent
        gradient: Gradient {
            GradientStop { position: 0; color: Theme.bgTop }
            GradientStop { position: 1; color: Theme.bgBottom }
        }
    }
    Image {
        id: wall
        visible: !root.pixel
        anchors.fill: parent
        source: root.pixel ? "" : Theme.wallpaperSource
        sourceSize: Qt.size(root.width * 1.1, root.height * 1.1)
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        // Very slow "breathing" drift so the den feels alive; only while
        // Bear Den itself is in front (not behind apps, the lock screen, or
        // another window) and motion is not reduced.
        readonly property bool drifting: !root.pixel && !Theme.reducedMotion && !Theme.resting && (Session.target.kind === "shell" || !Session.loaded)
        transformOrigin: Item.TopLeft
        // Stepped on World's heartbeat rather than animated every frame: the
        // drift is 6% over 45 s, so each step is a fraction of a pixel, and it
        // lands in frames the other decorations draw anyway.
        property real driftT: 0
        scale: 1.0 + 0.06 * (1 - Math.cos(driftT)) / 2
        Connections {
            target: World
            enabled: wall.drifting || wall.animating
            function onBeat(dt) {
                if (wall.drifting)
                    wall.driftT = (wall.driftT + Math.PI * dt / 45) % (Math.PI * 2)
                if (wall.animating)
                    wall.t += dt
            }
        }
        // Animated layers over a smooth picture (the Classic art style's
        // classic.wallpaper.sprites): placed in the picture's own pixels
        // (wallpaper width/height, read by ThemeRegistry) and scaled and
        // cropped with it, drift included; drawn smooth. Frames advance on the
        // heartbeat and stop like the pixel world's.
        readonly property bool animating: !root.pixel && (World.wallpaper.sprites || []).length > 0 && !Theme.reducedMotion
                                          && !Theme.resting && !Theme.screensaver && (Session.target.kind === "shell" || !Session.loaded)
        property real t: 0
        Item {
            id: layers
            readonly property real natW: World.wallpaper.width || 0
            readonly property real natH: World.wallpaper.height || 0
            readonly property real k: natW > 0 ? Math.max(wall.width / natW, wall.height / natH) : 1
            visible: !root.pixel && natW > 0
            width: natW
            height: natH
            x: (wall.width - natW * k) / 2
            y: (wall.height - natH * k) / 2
            scale: k
            transformOrigin: Item.TopLeft
            Repeater {
                model: layers.visible ? (World.wallpaper.sprites || []) : []
                Image {
                    required property var modelData
                    readonly property int frames: Math.max(1, modelData.frames)
                    property size sheet: Qt.size(0, 0)
                    readonly property int frameWidth: Math.floor(sheet.width / frames)
                    source: modelData.sheet
                    smooth: true
                    x: modelData.x
                    y: modelData.y
                    width: frameWidth
                    height: sheet.height
                    sourceClipRect: Qt.rect((Math.floor(wall.t * modelData.fps) % frames) * frameWidth, 0, frameWidth, sheet.height)
                    onStatusChanged: if (status === Image.Ready && sheet.width === 0) sheet = sourceSize
                }
            }
        }
    }
    // A pixel-art world (manifest wallpaper.pixel): the picture at a
    // whole-number scale, unsmoothed, covering the screen, with its animated
    // layers (wallpaper.sprites: sheets of frames side by side) on top at the
    // same scale. Frames advance on World's heartbeat, so they land in frames
    // drawn anyway, and stop while resting, behind apps and with reduced motion.
    Item {
        id: art
        visible: root.pixel
        readonly property real k: Math.max(1, Math.ceil(Math.max(root.width / Math.max(1, artImage.implicitWidth),
                                                                 root.height / Math.max(1, artImage.implicitHeight))))
        width: artImage.implicitWidth
        height: artImage.implicitHeight
        x: Math.floor((root.width - width * k) / 2)
        y: Math.floor((root.height - height * k) / 2)
        scale: k
        transformOrigin: Item.TopLeft
        readonly property bool animating: root.pixel && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver
                                          && (Session.target.kind === "shell" || !Session.loaded)
        property real t: 0
        Connections {
            target: World
            enabled: art.animating
            function onBeat(dt) { art.t += dt }
        }
        Image {
            id: artImage
            source: root.pixel ? Theme.wallpaperSource : ""
            smooth: false
            mipmap: false
        }
        Repeater {
            model: root.pixel ? (World.wallpaper.sprites || []) : []
            Image {
                required property var modelData
                readonly property int frames: Math.max(1, modelData.frames)
                // The sheet's own size, taken once it loads (sourceSize would
                // be re-read whenever the clip changes).
                property size sheet: Qt.size(0, 0)
                readonly property int frameWidth: Math.floor(sheet.width / frames)
                source: modelData.sheet
                smooth: false
                mipmap: false
                x: modelData.x
                y: modelData.y
                width: frameWidth
                height: sheet.height
                sourceClipRect: Qt.rect((Math.floor(art.t * modelData.fps) % frames) * frameWidth, 0, frameWidth, sheet.height)
                onStatusChanged: if (status === Image.Ready && sheet.width === 0) sheet = sourceSize
            }
        }
    }
    WeatherSky { anchors.fill: parent }
    Ambient { anchors.fill: parent }
    // Top scrim: keeps the header legible over bright wallpaper detail.
    Rectangle {
        anchors { left: parent.left; right: parent.right; top: parent.top }
        height: parent.height * 0.24
        gradient: Gradient {
            GradientStop { position: 0; color: Theme.alpha("#000000", 0.7) }
            GradientStop { position: 1; color: "transparent" }
        }
    }
    Rectangle {
        anchors.fill: parent
        gradient: Gradient {
            GradientStop { position: 0.55; color: "transparent" }
            GradientStop { position: 1; color: Theme.alpha("#000000", 0.45) }
        }
    }
}
