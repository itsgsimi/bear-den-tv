// Life in the backdrop: the particle kind the theme asks for (manifest
// ambient.kind, count, colors; docs/THEMES.md) —
//   fireflies  glowing motes and dust drifting and fading
//   leaves     leaves drifting down, turning as they fall
//   stars      stars twinkling in the sky, now and then a shooting star
//   embers     embers rising from below, flickering
//   snow       snowflakes falling softly
//   rain       pixel streaks falling fast (weather-only: World puts it here
//              for local rain, drizzle and thunder; not a theme.schema.json kind)
// Classic art style (World.classic): AmbientClassic draws the same kinds
// smooth — round particles with soft halos, turning leaves, a gradient comet,
// soft antialiased rain streaks — on this item's clock.
// Pixel art: every particle is a square of whole art pixels (World.px) at a
// position snapped to the grid, glows are a faint plus around the core, and
// nothing rotates. Cheap (rectangles, position/opacity only). Every particle is a function of
// one clock `t` advanced by World's heartbeat, so the screen redraws 20 times a
// second at most, never at the TV's 120 Hz (docs/THEMES.md, Performance rules).
// Paused unless Bear Den itself is in front (or still connecting) and motion
// is allowed.

import QtQuick
import BearDen

Item {
    id: root
    readonly property string kind: World.ambient.kind || ""
    readonly property int count: World.ambient.count || 0
    readonly property var colors: World.ambient.colors || []
    function color(i, fallback) { return colors.length > 0 ? colors[i % colors.length] : fallback[i % fallback.length] }
    property bool running: !Theme.reducedMotion && !Theme.resting && !Theme.screensaver && (Session.target.kind === "shell" || !Session.loaded)
    clip: true
    // Deterministic pseudo-random numbers per particle, so layouts are stable.
    function rnd(i, k) { return ((i * [7919, 104729, 1299709, 15485863][k]) % [97, 89, 83, 79][k]) / [97, 89, 83, 79][k] }
    // The shared clock, in seconds.
    property real t: 0
    Connections {
        target: World
        enabled: root.running && root.kind.length > 0
        function onBeat(dt) { root.t += dt }
    }
    function wave(period, phase) { return Math.sin(2 * Math.PI * (root.t / period + phase)) }
    function cycle(period, phase) { const u = root.t / period + phase; return u - Math.floor(u) }
    readonly property int px: World.px
    function snap(v) { return Math.round(v / px) * px }

    // A pixel glow: a faint plus, one art pixel beyond the core on each side.
    component Plus: Item {
        property color tint
        property real strength: 0.14
        anchors.centerIn: parent
        width: parent.width + 2 * root.px; height: parent.height + 2 * root.px
        Rectangle { x: root.px; width: parent.width - 2 * root.px; height: parent.height; color: Theme.alpha(parent.tint, parent.strength) }
        Rectangle { y: root.px; width: parent.width; height: parent.height - 2 * root.px; color: Theme.alpha(parent.tint, parent.strength) }
    }

    // Fireflies: every third mote glows (colour 0); the rest are dust (colour 1).
    Repeater {
        model: World.pixel && root.kind === "fireflies" ? root.count : 0
        Rectangle {
            id: mote
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            readonly property bool firefly: index % 3 === 0
            readonly property color tint: firefly ? root.color(0, ["#E3B35C"]) : root.color(1, ["#E3B35C", "#EDE3D1"])
            width: (firefly ? 2 : 1) * root.px
            height: width
            color: tint
            x: root.snap(r1 * root.width + 30 * Theme.scale * root.wave(14 + r2 * 16, r3))
            y: root.snap(root.height * (0.08 + 0.6 * r2) - 30 * Theme.scale * (1 + root.wave(18 + r1 * 18, r2)))
            // Glow for part of each cycle, dark for the rest.
            opacity: (firefly ? 0.85 : 0.35) * Math.pow(Math.max(0, root.wave(7 + r3 * 9, r1)), 1.5)
            Plus { visible: mote.firefly; tint: mote.tint; strength: 0.3 }
        }
    }

    // Leaves and snow: something falling, drifting side to side.
    Repeater {
        model: World.pixel && (root.kind === "leaves" || root.kind === "snow") ? root.count : 0
        Rectangle {
            id: flake
            required property int index
            readonly property bool snow: root.kind === "snow"
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            // A leaf tumbles by turning between flat (3×1) and on edge (2×2).
            readonly property bool flat: root.wave(5.4 + r1 * 3, r3) > 0
            width: (snow ? (r2 > 0.6 ? 2 : 1) : flat ? 3 : 2) * root.px
            height: (snow ? (r2 > 0.6 ? 2 : 1) : flat ? 1 : 2) * root.px
            color: snow ? root.color(index, ["#F4FAFF"]) : root.color(index, ["#C9A65A", "#7FAE72", "#5E8F5E"])
            opacity: snow ? 0.55 + r3 * 0.4 : 0.8
            x: root.snap(r1 * root.width + (snow ? 20 : 40) * Theme.scale * root.wave(6 + r3 * 4, r2))
            y: root.snap(-20 * Theme.scale + root.cycle((snow ? 20 : 16) + r2 * 12, r3) * (root.height + 40 * Theme.scale))
        }
    }

    // Rain: thin streaks of 1×4–6 art pixels falling fast with a little wind.
    // Straight columns, no rotation; each moves a few art pixels per beat.
    Repeater {
        model: World.pixel && root.kind === "rain" ? root.count : 0
        Rectangle {
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            readonly property real p: root.cycle(1.1 + r2 * 0.7, r3)
            width: root.px
            height: (r2 > 0.5 ? 6 : 4) * root.px
            color: root.color(index, ["#B8D0EE", "#E2ECF8"])
            opacity: 0.55 + r1 * 0.3
            x: root.snap(r1 * (root.width + 60 * Theme.scale) - 60 * Theme.scale * p)
            y: root.snap(-height + p * (root.height + height))
        }
    }

    // Stars twinkling in the upper sky, and now and then a shooting star.
    Repeater {
        model: World.pixel && root.kind === "stars" ? root.count : 0
        Rectangle {
            id: star
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            width: root.px
            height: width
            color: index % 4 === 0 ? root.color(1, ["#E8EEFF", "#F4E3A1"]) : root.color(0, ["#E8EEFF"])
            x: root.snap(r1 * root.width)
            y: root.snap(root.height * 0.45 * r2)
            // Twinkle in steps: dim, lit, and now and then a sparkle (a plus).
            readonly property real tw: root.wave(2 + r1 * 3.4, r3)
            opacity: tw > -0.3 ? 1 : 0.55
            Plus { visible: star.tw > 0.8 && star.index % 3 === 0; tint: star.color; strength: 0.6 }
        }
    }
    // A shooting star: a head and a fading trail of pixels, flying on `p`.
    Item {
        id: comet
        property real p: 0
        visible: World.pixel && root.kind === "stars" && p > 0 && p < 1
        x: root.snap(root.width * (0.85 - 0.5 * p * p))
        y: root.snap(root.height * (0.05 + 0.15 * p * p))
        Repeater {
            model: 8
            Rectangle {
                required property int index
                width: root.px; height: root.px
                x: index * 3 * root.px
                y: -index * root.px
                color: "#FFFFFF"
                opacity: (1 - index / 8) * Math.min(1, comet.p * 5, (1 - comet.p) * 3)
            }
        }
        NumberAnimation on p { id: fly; running: false; from: 0; to: 1; duration: 1300 }
        Timer {
            interval: 9000
            repeat: true
            running: World.pixel && root.kind === "stars" && root.running
            onTriggered: { fly.restart(); interval = 14000 + Math.random() * 14000 }
        }
    }

    // Embers rising from below, flickering as they go.
    Repeater {
        model: World.pixel && root.kind === "embers" ? root.count : 0
        Rectangle {
            id: ember
            required property int index
            readonly property real r1: root.rnd(index, 0)
            readonly property real r2: root.rnd(index, 1)
            readonly property real r3: root.rnd(index, 2)
            // 0 → 1 over one rise; eased out like a spark losing speed.
            readonly property real p: root.cycle(9 + r2 * 8, r3)
            width: (r2 > 0.55 ? 2 : 1) * root.px
            height: width
            color: index % 3 === 0 ? root.color(0, ["#FFD490"]) : root.color(1, ["#FFD490", "#FF8A3D"])
            x: root.snap(r1 * root.width + 25 * Theme.scale * root.wave(5.4 + r3 * 4, r1))
            y: root.snap(root.height + 10 * Theme.scale - (1 - (1 - p) * (1 - p)) * root.height * (0.75 - 0.3 * r3))
            opacity: Math.max(0, Math.min(1, p * 8) * (1 - p) * (0.8 + 0.2 * root.wave(1.3, r2)))
            Plus { tint: ember.color; strength: 0.22 }
        }
    }

    // Classic: the smooth particles, on this clock.
    AmbientClassic {
        anchors.fill: parent
        host: root
        visible: World.classic
    }
}
