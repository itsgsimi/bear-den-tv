// A whole bear in pixel art (assets/pixel/bear-<kind>.png, generated with the
// rig in BearRig.js by tools/pixelart/bears.py): one frame per pose, picked
// from a handful of numbers the director animates — no animations of its own,
// so it costs nothing while still.
//   walk      walk-cycle phase (radians); steps through four walking frames
//   walking   whether the walk cycle applies
//   wave      0..1 how far the right arm is raised; wavePhase swings it
//   squash    -1 (stretched, mid-jump) .. 1 (squashed, landing/take-off)
//   facing    1 = facing right, -1 = facing left (the sprite mirrors)
//   sitting   seated (on a log, the moon…), legs forward
//   reach     0..1 how far the right arm reaches forward (toasting, pointing)
//   asleep    curled up, eyes shut
//   blink     eyes closed for a moment
// Scenes use it too (CampfireScene, MoonScene): a whole bear wherever the bear
// is out in the open; heads alone (BearHead) where a bear peeks out.
// The theme dresses the bear (manifest "bears"): a hat on its head and
// something in its paw (an ornament, or "stick" for a toasting stick with a
// marshmallow). Mama keeps her paws free (she wears her daisy).
// The sprite is drawn unsmoothed at a whole number of World.px per art pixel;
// `size` is the height asked for, and the bear takes the nearest whole scale.
// Dressed for the weather (`weatherDress`, set by BearVisitors; docs/THEMES.md
// → Weather in the corner scene): with rain in the scene (World.weatherLook
// "wet"/"storm") the bear holds a leaf umbrella (scene-wx-umbrella, the stem
// from its paw) instead of what it carries; with snow it wears the beanie.
// Classic art style (World.classic): BearPuppetClassic draws the same bear,
// posed by the same numbers, from smooth SVG parts (70×100 units of size/100).

import QtQuick
import BearDen
import "BearRig.js" as Rig

Item {
    id: bear
    property string kind: "cub"          // cub | mama | dad
    property real size: 110 * Theme.scale   // standing height
    property real walk: 0
    property bool walking: false
    property real wave: 0
    property real wavePhase: 0
    property real squash: 0
    property int facing: 1
    property bool blink: false
    property bool sitting: false
    property real reach: 0
    property real stickLength: 42           // the toasting stick, in hundredths of `size`
    property bool asleep: false
    property url hat: World.bears.hat || ""
    property string carry: World.bears.carry || ""
    property bool weatherDress: false
    readonly property string dress: !weatherDress ? ""
        : World.weatherLook === "wet" || World.weatherLook === "storm" ? "umbrella"
        : World.weatherLook === "snow" ? "snow" : ""
    readonly property url wornHat: dress === "snow" ? World.ornament("hat-beanie") : hat
    readonly property string held: dress === "umbrella" ? "" : carry

    readonly property var rig: Rig.kinds[kind] || Rig.kinds.cub
    readonly property int frameW: rig.frame[0]
    readonly property int frameH: rig.frame[1]
    // Screen pixels per art pixel: a whole multiple of the world's pixel.
    readonly property real unit: Math.max(1, Math.round(size / (frameH * World.px))) * World.px
    readonly property string pose: {
        if (sitting) return asleep ? "sleep" : reach > 0.5 ? "reach" : "sit"
        if (wave > 0.4) return Math.sin(wavePhase) > 0 ? "wave0" : "wave1"
        if (squash > 0.4) return "crouch"
        if (squash < -0.4) return "jump"
        if (walking) {
            const turn = ((walk % (2 * Math.PI)) + 2 * Math.PI) % (2 * Math.PI)
            return "walk" + Math.floor(turn / (Math.PI / 2)) % 4
        }
        return "stand"
    }
    readonly property var at: rig.poses[pose]
    width: World.classic ? 0.7 * size : frameW * unit
    height: World.classic ? size : frameH * unit

    Loader {
        objectName: "bearPuppetClassic"
        active: World.classic
        sourceComponent: Component { BearPuppetClassic { puppet: bear } }
    }
    Item {
        id: body
        visible: World.pixel
        width: bear.width
        height: bear.height
        transform: Scale { origin.x: body.width / 2; xScale: bear.facing }

        Image {
            source: "qrc:/qt/qml/BearDen/assets/pixel/bear-" + bear.kind + ".png"
            width: bear.width
            height: bear.height
            smooth: false
            sourceClipRect: Qt.rect(Rig.poses.indexOf(bear.pose) * bear.frameW,
                                    (bear.blink && !bear.asleep ? 1 : 0) * bear.frameH,
                                    bear.frameW, bear.frameH)
        }
        // A hat, sitting on the head.
        Ornament {
            visible: bear.wornHat.toString().length > 0
            url: bear.wornHat
            width: bear.rig.head[0] * 0.66 * bear.unit
            height: width * 0.72
            x: bear.at.head[0] * bear.unit - width / 2
            y: bear.at.head[1] * bear.unit - height * 0.62
        }
        // What the bear holds in its right paw.
        Ornament {
            visible: bear.held !== "" && bear.held !== "stick" && bear.kind !== "mama"
            url: bear.held !== "stick" ? bear.held : ""
            width: 8 * bear.unit
            height: width
            x: bear.at.paw[0] * bear.unit - width / 2
            y: bear.at.paw[1] * bear.unit - height / 2
        }
        // A leaf umbrella in the rain: a stem of whole art pixels from the paw
        // up to a leaf canopy over the head.
        Item {
            objectName: "bearUmbrella"
            visible: bear.dress === "umbrella"
            readonly property int canopyTop: Math.round(bear.at.head[1]) - 14     // canopy top, art pixels
            readonly property int stemX: Math.round(bear.at.paw[0])
            PixelSprite {
                name: parent.visible ? "scene-wx-umbrella" : ""
                unit: bear.unit
                x: (parent.stemX - 13) * bear.unit
                y: parent.canopyTop * bear.unit
            }
            Rectangle {
                x: parent.stemX * bear.unit
                y: (parent.canopyTop + 11) * bear.unit
                width: bear.unit
                height: Math.max(0, Math.round(bear.at.paw[1]) - parent.canopyTop - 11) * bear.unit
                color: "#5E7A3A"
            }
        }
        // A toasting stick, one art pixel thick, with a marshmallow.
        Item {
            id: stick
            visible: bear.held === "stick" && bear.kind !== "mama"
            readonly property real angle: (-50 + 30 * bear.reach) * Math.PI / 180   // raised, or held out a little above level
            readonly property int length: Math.round(bear.stickLength * bear.size / 100 / bear.unit)
            x: bear.at.paw[0] * bear.unit
            y: bear.at.paw[1] * bear.unit
            Repeater {
                model: stick.visible ? stick.length : 0
                Rectangle {
                    required property int index
                    width: bear.unit; height: bear.unit
                    color: index % 5 === 4 ? "#6B4A30" : "#8A6443"
                    x: Math.round(Math.cos(stick.angle) * index) * bear.unit
                    y: Math.round(Math.sin(stick.angle) * index) * bear.unit
                }
            }
            Grid {
                columns: 3
                x: Math.round(Math.cos(stick.angle) * stick.length) * bear.unit - bear.unit
                y: Math.round(Math.sin(stick.angle) * stick.length) * bear.unit - bear.unit
                Repeater {
                    model: ["#F7F0E2", "#F7F0E2", "#EDE3D1", "#F7F0E2", "#FFFFFF", "#E3C08A", "#EDE3D1", "#E3C08A", "#D9A441"]
                    Rectangle { required property string modelData; width: bear.unit; height: bear.unit; color: modelData }
                }
            }
        }
    }
}
