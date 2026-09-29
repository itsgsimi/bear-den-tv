// Local weather over the wallpaper (World.weather*; spec: state.weather in
// contracts/state.schema.json, guide docs/THEMES.md → Weather in the scene):
// a static veil for cloud, fog and rain (drawn once, never animated, so it
// also stays with reduced motion), and for thunder a rare lightning flash
// stepped on World's heartbeat, published as World.flashing so corner scenes
// startle their bears (SceneWeather.qml). The particles themselves are Ambient's
// (`rain`/`snow`, chosen by World). Nothing moves unless `alive`.

import QtQuick
import BearDen

Item {
    id: root
    readonly property bool alive: visible && World.weatherThunder && !Theme.reducedMotion && !Theme.resting
                                  && !Theme.screensaver && (Session.target.kind === "shell" || !Session.loaded)
    Rectangle {
        anchors.fill: parent
        visible: World.weatherVeil > 0
        color: World.weatherVeilColor
        opacity: World.weatherVeil
    }
    // Lightning: every 14–30 s a double flicker lasting a few beats.
    Rectangle {
        id: flash
        anchors.fill: parent
        color: "#EEF3FF"
        // Shell.lightningSeconds (BDTV_LIGHTNING_SECONDS) fixes the gap, for checks.
        readonly property int fixedGap: Shell.lightningSeconds
        property real wait: fixedGap >= 0 ? fixedGap : 8
        property int step: -1   // -1 idle; 0.. the flicker frames
        readonly property var frames: [0.32, 0.08, 0.22, 0.1, 0.04]
        opacity: step >= 0 && step < frames.length ? frames[step] : 0
        visible: opacity > 0
        property int n: 0
        // Corner scenes startle their bears while the flash is lit.
        Binding { target: World; property: "flashing"; value: flash.step >= 0 }
        Connections {
            target: World
            enabled: root.alive
            function onBeat(dt) {
                if (flash.step >= 0) {
                    flash.step = flash.step + 1 < flash.frames.length ? flash.step + 1 : -1
                    return
                }
                flash.wait -= dt
                if (flash.wait <= 0) {
                    flash.n += 1
                    flash.wait = flash.fixedGap >= 0 ? flash.fixedGap : 14 + ((flash.n * 7919) % 17)
                    flash.step = 0
                }
            }
        }
        // Stopping mid-flicker must not leave the screen lit.
        Connections {
            target: root
            function onAliveChanged() { if (!root.alive) flash.step = -1 }
        }
    }
}
