// A little pixel-art room behind the featured app's icon (assets/pixel/
// hero-<scene>.png, generated with HeroRig.js by tools/pixelart/hero.py): a
// cinema, a cabin with a TV, an arcade. Which app gets which room is data in
// Apps.stage(). The icon sits in the room's `screenRect`.
//  - Frames loop on World's heartbeat; a scene with an intro (the cabin TV's
//    static) plays it once when `item` changes, then holds the picture.
//  - A scene with times of day shows the row for the TV's clock (the cabin's
//    window: night, dawn, day, dusk).
//  - `shift` (art pixels) nudges the room sideways for parallax as focus moves
//    along the rail.
// Still while Bear Den rests, behind apps and with reduced motion.
// Classic (World.classic) draws the same rooms smooth, on the same rig, from
// assets/classic/hero-<scene>[-<time>].svg (tools/classicart/hero.py): the
// room's light (hero-<scene>-glow.svg) pulses instead of pixel frames, the
// cabin's intro static alternates hero-static-0/1.svg over the screen, and
// the parallax slides continuously instead of in whole art pixels.

import QtQuick
import BearDen
import "HeroRig.js" as Rig

Item {
    id: root
    property string scene: ""
    property string itemId: ""
    property int shift: 0               // -2..2 art pixels
    // The room is 4 art pixels narrower than its art, so shifting never shows
    // an edge; the slide eases over a few frames and snaps to whole pixels.
    property real slide: shift
    Behavior on slide { enabled: !Theme.reducedMotion; NumberAnimation { duration: 240; easing.type: Easing.OutCubic } }
    readonly property real offset: World.classic ? Math.max(-2, Math.min(2, slide)) - 2
                                                 : Math.round(Math.max(-2, Math.min(2, slide))) - 2
    readonly property var rig: Rig.scenes[scene] || null
    readonly property int p: World.px
    readonly property bool alive: visible && !Theme.reducedMotion && !Theme.resting && !Theme.screensaver
                                  && (Session.target.kind === "shell" || !Session.loaded)
    width: rig ? (rig.size[0] - 4) * p : 0
    height: rig ? rig.size[1] * p : 0
    // The icon's place, in this item's coordinates.
    readonly property rect screenRect: rig ? Qt.rect((rig.screen[0] + offset) * p, rig.screen[1] * p, rig.screen[2] * p, rig.screen[3] * p)
                                           : Qt.rect(0, 0, 0, 0)
    // The intro is showing (the icon waits for the picture).
    readonly property bool introPlaying: rig !== null && rig.intro && frame < rig.frames - 1

    property real t: 0
    property int frame: 0
    onItemIdChanged: { t = 0; frame = rig && rig.intro && !Theme.reducedMotion ? 0 : (rig ? rig.frames - 1 : 0) }
    Connections {
        target: World
        enabled: root.alive && root.rig !== null
        function onBeat(dt) {
            root.t += dt
            const f = Math.floor(root.t * root.rig.fps)
            root.frame = root.rig.intro ? Math.min(f, root.rig.frames - 1) : f % root.rig.frames
        }
    }
    function timeRow() {
        if (!rig || rig.times.length === 0) return 0
        const h = new Date().getHours()
        const name = h >= 21 || h < 5 ? "night" : h < 8 ? "dawn" : h < 18 ? "day" : "dusk"
        return Math.max(0, rig.times.indexOf(name))
    }
    property int row: timeRow()
    Timer { interval: 60000; repeat: true; running: root.visible; onTriggered: root.row = root.timeRow() }

    clip: true
    Image {
        visible: root.rig !== null && World.pixel
        source: root.rig && World.pixel ? "qrc:/qt/qml/BearDen/assets/pixel/hero-" + root.scene + ".png" : ""
        smooth: false
        x: root.offset * root.p
        width: root.rig ? root.rig.size[0] * root.p : 0
        height: root.height
        sourceClipRect: root.rig ? Qt.rect(root.frame % root.rig.frames * root.rig.size[0], root.row * root.rig.size[1],
                                           root.rig.size[0], root.rig.size[1])
                                 : Qt.rect(0, 0, 0, 0)
    }
    // Classic: the smooth room, its pulsing light and the intro static.
    readonly property string classicDir: "qrc:/qt/qml/BearDen/assets/classic/"
    Item {
        id: classicRoom
        objectName: "heroClassicRoom"
        visible: root.rig !== null && World.classic
        x: root.offset * root.p
        width: root.rig ? root.rig.size[0] * root.p : 0
        height: root.height
        Image {
            objectName: "heroClassicImage"
            anchors.fill: parent
            source: root.rig && World.classic
                    ? root.classicDir + "hero-" + root.scene + (root.rig.times.length > 0 ? "-" + root.rig.times[root.row] : "") + ".svg" : ""
            sourceSize: Qt.size(width, height)
            smooth: true
        }
        Image {
            anchors.fill: parent
            source: root.rig && World.classic ? root.classicDir + "hero-" + root.scene + "-glow.svg" : ""
            sourceSize: Qt.size(width, height)
            smooth: true
            // One slow breath per loop of the pixel frames.
            opacity: 0.75 + 0.25 * Math.sin(root.t * 2 * Math.PI * (root.rig ? root.rig.fps / root.rig.frames : 1))
        }
        Image {
            visible: root.introPlaying
            x: root.rig ? root.rig.screen[0] * root.p : 0
            y: root.rig ? root.rig.screen[1] * root.p : 0
            width: root.rig ? root.rig.screen[2] * root.p : 0
            height: root.rig ? root.rig.screen[3] * root.p : 0
            source: World.classic && root.rig && root.rig.intro ? root.classicDir + "hero-static-" + (root.frame % 2) + ".svg" : ""
            sourceSize: Qt.size(width, height)
            opacity: root.rig ? 1 - 0.5 * root.frame / root.rig.frames : 1
        }
    }
    // A frame round the room, like a window into it.
    PixelBox {
        anchors.fill: parent
        color: "transparent"
        borderColor: Theme.alpha("#000000", 0.55)
        borderWidth: World.px
    }
}
