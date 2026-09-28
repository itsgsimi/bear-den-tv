// Bears on the featured panel (HeroPanel), in pixel art or, in Classic, smooth
// (the puppets size themselves; positions are not snapped to art pixels):
//  - A visitor: a second or two after focus settles on an app, a bear walks in
//    along the panel's bottom edge, stops in front of the app's room and
//    reacts to it for a few seconds (Apps.stage(adapter).react: the cub with
//    popcorn at the cinema, dad waving the remote at the cabin TV, the cub on
//    a controller at the arcade), then walks off. At most one visit every 12 s.
//  - A sleeper: while Bear Den rests, dad dozes on the panel's edge with "z"s
//    over him (drawn once, nothing moves); the first press wakes him and he
//    stretches before he goes.
// Moves on World's heartbeat only while a bear is walking; none in Plain or
// Performance, with reduced motion, behind apps or on the screensaver.

import QtQuick
import BearDen

Item {
    id: root
    property var item: ({})
    property var stage: null            // Apps.stage(adapter) for an app, else null
    property real standX: width * 0.5   // where the visitor stops (its centre)
    readonly property int p: World.px
    // A position on screen: whole art pixels in Pixel, continuous in Classic.
    function place(v) { return World.classic ? v : Math.round(v / p) * p }
    readonly property bool allowed: World.decorated && !Theme.reducedMotion && !Theme.screensaver
                                    && (Session.target.kind === "shell" || !Session.loaded)
    clip: true

    // --- The visitor ------------------------------------------------------------
    property string phase: ""          // "" | in | react | out
    property real t: 0
    property real lastVisit: -1e9
    property string itemId: item && item.itemId ? item.itemId : ""
    onItemIdChanged: { settle.restart(); if (phase !== "") leave() }
    onAllowedChanged: if (!allowed) { phase = ""; visitor.visible = false }
    Timer {
        id: settle
        interval: 1600
        onTriggered: {
            const now = Date.now() / 1000
            if (root.allowed && root.visible && root.stage && !Theme.resting && now - root.lastVisit > 12) {
                root.lastVisit = now
                root.begin()
            }
        }
    }
    function begin() {
        const r = stage.react
        visitor.kind = r.kind
        visitor.carry = r.carry ? World.ornament(r.carry) : ""
        visitor.sitting = false; visitor.reach = 0; visitor.wave = 0
        visitor.facing = -1
        visitor.x = width
        visitor.visible = true
        phase = "in"; t = 0
    }
    function leave() { visitor.sitting = false; visitor.reach = 0; visitor.wave = 0; visitor.facing = 1; phase = "out"; t = 0 }
    Connections {
        target: World
        enabled: root.phase !== ""
        function onBeat(dt) {
            root.t += dt
            const stopX = root.standX - visitor.width / 2
            if (root.phase === "in") {
                const q = Math.min(1, root.t / 1.4)
                visitor.walking = true
                visitor.walk += dt * 9
                visitor.x = root.place(root.width + (stopX - root.width) * q)
                if (q >= 1) { visitor.walking = false; root.phase = "react"; root.t = 0; root.pose(root.stage.react.pose) }
            } else if (root.phase === "react") {
                if (root.stage && root.stage.react.pose === "wave")
                    visitor.wavePhase += dt * 7
                visitor.blink = Math.sin(root.t * 2.1) > 0.97
                if (root.t > 4.2) root.leave()
            } else if (root.phase === "out") {
                const q = Math.min(1, root.t / 1.4)
                visitor.walking = true
                visitor.walk += dt * 9
                visitor.x = root.place(stopX + (root.width - stopX) * q)
                if (q >= 1) { visitor.walking = false; visitor.visible = false; root.phase = "" }
            }
        }
    }
    function pose(name) {
        visitor.sitting = name === "sit" || name === "reach"
        visitor.reach = name === "reach" ? 1 : 0
        visitor.wave = name === "wave" ? 1 : 0
    }
    BearPuppet {
        id: visitor
        visible: false
        size: (kind === "dad" ? 46 : kind === "mama" ? 43 : 35) * root.p
        hat: ""
        y: root.height - height - root.p
    }

    // --- The sleeper ----------------------------------------------------------------
    property bool dozing: World.decorated && !World.performance && Theme.resting && !Theme.screensaver
    property bool stretching: false
    // Stretch only when someone wakes the TV, not when the screensaver takes over.
    onDozingChanged: if (!dozing && !Theme.resting && sleeper.visible) { stretching = true; stretch.restart() }
    Timer { id: stretch; interval: 900; onTriggered: root.stretching = false }
    BearPuppet {
        id: sleeper
        kind: "dad"
        size: 46 * root.p
        hat: World.bears.hat || ""
        carry: ""
        visible: root.dozing || root.stretching
        sitting: root.dozing
        asleep: root.dozing
        wave: root.stretching ? 1 : 0
        wavePhase: 1.5
        facing: 1
        x: root.place(root.standX - width / 2)
        y: root.height - height - root.p
        Repeater {
            model: root.dozing ? 3 : 0
            Item {
                id: zz
                required property int index
                readonly property int unit: root.p * (index === 2 ? 2 : 1)
                x: sleeper.width * 0.7 + index * 5 * root.p
                y: (8 - index * 7) * root.p
                width: 5 * unit
                height: 5 * unit
                opacity: 0.85 - index * 0.2
                PixelSprite { visible: World.pixel; name: World.pixel ? "scene-moon-z" : ""; unit: zz.unit }
                // Classic: the smooth "z" (tools/classicart/extras.py), same size.
                Image {
                    visible: World.classic
                    anchors.fill: parent
                    source: World.classic ? "qrc:/qt/qml/BearDen/assets/classic/sleep-z.svg" : ""
                    sourceSize: Qt.size(width, height)
                }
            }
        }
    }
}
