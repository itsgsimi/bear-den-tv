// The corner scene's weather: the building block every corner scene uses to
// react to the local weather (World.weatherLook; guide docs/THEMES.md →
// Weather in the corner scene). The scene only declares *what* happens per
// look and *where*, in its own units, as data:
//
//   SceneWeather {
//       side: "back"                        // behind the bears ("front": over them)
//       unit: root.p                        // screen pixels per scene unit
//       reactions: ({
//           wet:   { puddles: [{x, y, w}], shelter: {x, y, w}, drips: [{x, y}] },
//           storm: { startle: [{x, y}] },   // thunder: everything in wet, plus this
//           snow:  { caps: [{x, y, w}] },
//           fog:   { fade: 0.3, mist: [{y, h}] },
//           night: { stars: [{x, y}] }
//       })
//   }
//
// Pieces (pixel art in the Pixel style, on whole art pixels; smooth in Classic):
//   puddles  wet ground with a glint that runs along it     (back)
//   shelter  a tarp on two poles (scene-wx-tarp / classic/scene-tarp.svg; w is
//            its width in Classic, the pixel sprite is 50 art pixels) (back)
//   stars    a few stars twinkling over the scene            (back)
//   caps     snow lying on top of a prop; in Classic an optional `a` tilts
//            it by that many degrees along a slope (pixels never rotate) (front)
//   drips    drops falling a few pixels from an edge         (front)
//   fade     how much the scene fades its props in fog (it binds `fade`) (front)
//   mist     drifting wisps of mist at {y, h}                  (front)
//   startle  a "!" over a bear when the lightning flashes (front)
// `wet` and `startled` let the scene change its own art too (the campfire
// smoulders under its tarp; bears hop on the flash).
//
// Performance: a handful of Rectangles/Images, only for the current look.
// Motion (glints, drips, twinkles, mist) moves on World's heartbeat and only
// while `alive` (not resting, on the screensaver, behind apps or with reduced
// motion); reduced motion shows the still version. The startle starts on
// World.flashing (set by WeatherSky only while its flash is alive) and lasts
// 1.5 s of heartbeat; it is dropped when the scene stops being alive.

pragma ComponentBehavior: Bound

import QtQuick
import BearDen

Item {
    id: root
    objectName: "sceneWeather"
    property string side: "front"
    property real unit: World.px
    property var reactions: ({})

    readonly property string look: World.weatherLook
    // The pieces for the current look (thunder adds its own to the rain's).
    readonly property var now: {
        if (look === "") return ({})
        if (look === "storm") return Object.assign({}, reactions.wet || {}, reactions.storm || {})
        return reactions[look] || ({})
    }
    readonly property bool wet: look === "wet" || look === "storm"
    readonly property var backKinds: ["puddles", "shelter", "stars"]
    readonly property var frontKinds: ["caps", "drips", "fade", "mist", "startle"]
    // What this side draws now, by piece name (tests read it).
    readonly property var drawn: Object.keys(now).filter(k => (side === "back" ? backKinds : frontKinds).indexOf(k) >= 0).sort()
    function has(kind) { return drawn.indexOf(kind) >= 0 }
    function list(kind) { return has(kind) ? (now[kind] || []) : [] }

    readonly property bool alive: visible && drawn.length > 0 && !Theme.reducedMotion && !Theme.resting
                                  && !Theme.screensaver && (Session.target.kind === "shell" || !Session.loaded)
    // Fog: how much the scene fades its props (the scene applies it).
    readonly property real fade: has("fade") ? now.fade : 0
    // Thunder: the bears stay startled for a moment after each flash starts
    // (the flash itself lasts a quarter of a second, too short to read).
    property real startleLeft: 0
    readonly property bool startled: startleLeft > 0
    Connections {
        target: World
        function onFlashingChanged() {
            if (World.flashing && root.alive && root.has("startle")) root.startleLeft = 1.5
        }
    }
    onAliveChanged: if (!alive) startleLeft = 0
    readonly property bool pixel: World.pixel
    readonly property real u: unit

    property real t: 0
    Connections {
        target: World
        enabled: root.alive && (root.has("puddles") || root.has("drips") || root.has("stars") || root.has("mist")
                                || root.startleLeft > 0)
        function onBeat(dt) {
            root.t += dt
            if (root.startleLeft > 0) root.startleLeft = Math.max(0, root.startleLeft - dt)
        }
    }
    function wave(period, phase) { return Math.sin(2 * Math.PI * (t / period + phase)) }
    function cycle(period, phase) { const x = t / period + phase; return x - Math.floor(x) }

    // --- back ----------------------------------------------------------------
    Repeater {                                         // stars on a clear night
        model: root.list("stars")
        Item {
            id: star
            required property var modelData
            required property int index
            x: modelData.x * root.u; y: modelData.y * root.u
            readonly property real tw: root.wave(2.2 + (index % 3) * 0.7, index * 0.37)
            Rectangle {
                visible: root.pixel
                width: root.u; height: root.u
                color: star.index % 3 === 0 ? "#F4E3A1" : "#E8EEFF"
                opacity: parent.tw > -0.4 ? 1 : 0.4
            }
            Ornament {
                visible: !root.pixel
                name: root.pixel ? "" : "star"
                width: 11 * root.u; height: width
                x: -width / 2; y: -height / 2
                opacity: 0.6 + 0.35 * parent.tw
            }
        }
    }
    Item {                                             // the tarp
        visible: root.has("shelter")
        readonly property var at: root.has("shelter") ? root.now.shelter : ({ x: 0, y: 0, w: 0 })
        x: at.x * root.u; y: at.y * root.u
        PixelSprite { visible: root.pixel; name: root.pixel ? "scene-wx-tarp" : ""; unit: root.u }
        Image {
            visible: !root.pixel
            source: root.pixel ? "" : "qrc:/qt/qml/BearDen/assets/classic/scene-tarp.svg"
            width: parent.at.w * root.u; height: width * 216 / 200
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
        }
    }
    Repeater {                                         // puddles with a running glint
        model: root.list("puddles")
        Item {
            id: puddle
            required property var modelData
            required property int index
            x: modelData.x * root.u; y: modelData.y * root.u
            width: modelData.w * root.u
            // Pixel: the glint hops one art pixel along each few beats.
            readonly property int hop: Math.floor(root.t * 3 + index * 2) % Math.max(1, modelData.w - 2)
            Rectangle {
                visible: root.pixel
                width: parent.width; height: 2 * root.u
                color: "#26334A"; opacity: 0.9
                Rectangle { x: root.u; width: parent.width - 2 * root.u; height: root.u; color: "#4E6688" }
                Rectangle { x: (1 + puddle.hop) * root.u; width: root.u; height: root.u; color: "#DDE8F6" }
            }
            Rectangle {
                visible: !root.pixel
                width: parent.width; height: 4 * root.u; radius: height / 2
                color: "#2E3C54"; opacity: 0.8
                Rectangle { x: parent.width * 0.15; y: root.u * 0.6; width: parent.width * 0.7; height: 1.4 * root.u; radius: height / 2; color: "#7D95BA"; opacity: 0.7 }
                Rectangle {
                    x: parent.width * (0.2 + 0.6 * root.cycle(2.4 + puddle.index * 0.5, puddle.index * 0.3)) - width / 2
                    y: root.u * 0.5
                    width: 3 * root.u; height: 1.4 * root.u; radius: height / 2
                    color: "#EEF4FF"; opacity: 0.85
                }
            }
        }
    }

    // --- front ---------------------------------------------------------------
    Repeater {                                         // snow on the props
        model: root.list("caps")
        Item {
            required property var modelData
            x: modelData.x * root.u; y: modelData.y * root.u
            width: modelData.w * root.u
            Rectangle { visible: root.pixel; x: root.u; width: parent.width - 2 * root.u; height: root.u; color: "#F7FBFF" }
            Rectangle { visible: root.pixel; y: root.u; width: parent.width; height: root.u; color: "#D6E2F0" }
            Rectangle {
                visible: !root.pixel
                y: -root.u; width: parent.width; height: 4.5 * root.u; radius: height / 2
                rotation: parent.modelData.a || 0          // Classic caps may follow a slope
                gradient: Gradient {
                    GradientStop { position: 0; color: "#FFFFFF" }
                    GradientStop { position: 1; color: "#C9D8EA" }
                }
            }
        }
    }
    Repeater {                                         // drops falling from an edge
        model: root.list("drips")
        Rectangle {
            required property var modelData
            required property int index
            readonly property real q: root.alive ? root.cycle(1.3 + index * 0.35, index * 0.41) : 0.1
            width: (root.pixel ? 1 : 1.8) * root.u; height: 2 * root.u
            radius: root.pixel ? 0 : width / 2
            color: "#B8D0EE"
            x: modelData.x * root.u
            y: (modelData.y + (root.pixel ? Math.floor(q * 8) : q * 8)) * root.u
            opacity: q < 0.85 ? 0.9 : 0
        }
    }
    Repeater {                                         // fog: drifting mist bands
        model: root.list("mist")
        Item {
            id: band
            required property var modelData
            required property int index
            readonly property real w: root.width
            readonly property real drift: 3 * root.wave(9 + index * 3, index * 0.4)
            x: (root.pixel ? Math.round(drift) : drift) * root.u
            y: modelData.y * root.u
            // Three wisps with gaps, stair-stepped (Pixel) or rounded (Classic) ends.
            Repeater {
                model: [ { a: 0.02, b: 0.34 }, { a: 0.4, b: 0.62 }, { a: 0.68, b: 0.98 } ]
                Item {
                    required property var modelData
                    readonly property real inset: root.pixel ? 2 * root.u : 0
                    x: Math.round((modelData.a + (band.index % 2) * 0.05) * band.w / root.u) * root.u
                    width: Math.round((modelData.b - modelData.a) * band.w / root.u) * root.u
                    height: band.modelData.h * root.u
                    Rectangle { x: parent.inset; width: parent.width - 2 * parent.inset; height: root.pixel ? root.u : parent.height
                                radius: root.pixel ? 0 : height / 2; color: "#D8DEE6"; opacity: root.pixel ? 0.22 : 0.14 }
                    Rectangle { visible: root.pixel; y: root.u; width: parent.width; height: parent.height - root.u; color: "#D8DEE6"; opacity: 0.22 }
                }
            }
        }
    }
    Repeater {                                         // thunder: a "!" over each bear
        model: root.list("startle")
        Item {
            required property var modelData
            visible: root.startled
            x: modelData.x * root.u; y: modelData.y * root.u
            PixelSprite { visible: root.pixel; name: root.pixel ? "scene-wx-startle" : ""; unit: root.u }
            Image {
                visible: !root.pixel
                source: root.pixel ? "" : "qrc:/qt/qml/BearDen/assets/classic/startle.svg"
                width: 14 * root.u; height: width * 36 / 20
                sourceSize: Qt.size(width * 2, height * 2)
                smooth: true
            }
        }
    }
}
