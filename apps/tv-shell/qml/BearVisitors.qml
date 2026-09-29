// Bears that drop by now and then on Home, Settings and Pair phone: one walks
// across and stops to wave, pops up from the bottom edge, hops onto an app tile
// for a little dance, chases something across the screen (what the theme's
// bears.chase names), or the whole family parades past.
//
// How it runs (and why it is cheap on a small TV box):
//  - Between visits nothing moves: one single-shot Timer waits 20–50 s (45–105 s
//    while Bear Den rests). For checks, BDTV_BEARS_SECONDS sets a fixed gap and
//    BDTV_BEARS_ACT (walk|peek|hop|parade|chase) picks the act.
//  - A visit is a short plan of steps (walk, wave, jump, dance, rise, sink, …)
//    played by one 25 fps Timer that sets a few numbers on BearPuppets, so the
//    screen redraws at 25 fps only while a bear is on it, never 60–120.
//  - They dress for the local weather (BearPuppet.weatherDress: a leaf
//    umbrella in the rain, a beanie in the snow) and hurry in the rain.
//  - It stops at once (bears vanish) when the screen changes, a dialog or the
//    launch overlay opens, an app comes to the front, the screensaver starts,
//    with reduced motion, and in the Plain style.
// ShellRoot sets `allowed` (right screen, nothing on top) and `home` (for the
// app tiles' positions, so bears can land on them).

import QtQuick
import BearDen

Item {
    id: root
    property bool allowed: false
    property string screen: "home"
    property var home: null
    readonly property real s: Theme.scale
    readonly property bool canVisit: allowed && World.decorated && !Theme.reducedMotion && !Theme.screensaver
                                     && Session.target.kind === "shell" && width > 0
    readonly property real ground: height - 2 * s            // feet on the bottom edge
    readonly property var actors: [cub, mama, dad]
    // In the rain (World.weatherLook) the bears hurry: every walk is faster.
    readonly property real pace: World.weatherLook === "wet" || World.weatherLook === "storm" ? 1.4 : 1

    // --- Scheduling --------------------------------------------------------
    property bool busy: false
    function scheduleNext() {
        nextVisit.interval = Shell.bearsSeconds > 0 ? Shell.bearsSeconds * 1000
                           : Theme.resting ? 45000 + Math.random() * 60000
                           : 20000 + Math.random() * 30000
    }
    Timer {
        id: nextVisit
        interval: 20000
        running: root.canVisit && !root.busy
        onTriggered: root.begin()
    }
    Component.onCompleted: scheduleNext()
    onCanVisitChanged: if (!canVisit) stop()
    onScreenChanged: stop()

    // --- Plan playback ------------------------------------------------------
    property var plan: []
    property int stepIndex: 0
    property real elapsed: 0
    // Moves on World's heartbeat (which runs at full speed while a bear is
    // out, even when resting), so a visit shares frames with everything else.
    Binding { target: World; property: "visiting"; value: root.busy }
    Connections {
        target: World
        enabled: root.busy
        function onBeat(dt) { root.advance(dt * 1000) }
    }
    function advance(dt) {
        if (stepIndex >= plan.length) { finish(); return }
        const st = plan[stepIndex]
        if (elapsed === 0 && st.start) st.start()
        elapsed += dt
        const p = Math.min(1, elapsed / st.dur)
        st.run(p, dt)
        if (p >= 1) { stepIndex++; elapsed = 0 }
    }
    function reset(a) {
        a.visible = false; a.walking = false; a.wave = 0; a.squash = 0; a.blink = false
    }
    function finish() {
        for (const a of actors) reset(a)
        chased.visible = false
        plan = []
        scheduleNext()
        busy = false
    }
    function stop() {
        if (busy) finish()
    }
    // Start the family parade now (the remote's secret code, ShellRoot).
    function celebrate() {
        if (!canVisit)
            return false
        stop()
        plan = parade()
        stepIndex = 0
        elapsed = 0
        if (plan.length > 0) busy = true
        return busy
    }
    function begin() {
        const acts = []
        const add = (w, f) => { for (let i = 0; i < w; ++i) acts.push(f) }
        add(3, walkBy)
        add(2, peek)
        if (screen === "home") {
            if (tiles().length > 1) add(3, tileHop)
            add(1, parade)
        }
        if (screen !== "pairing") add(2, chase)
        const forced = ({ walk: walkBy, peek: peek, hop: tileHop, parade: parade, chase: chase })[Shell.bearsAct]
        const act = forced ? forced : acts[Math.floor(Math.random() * acts.length)]
        plan = act()
        stepIndex = 0
        elapsed = 0
        if (plan.length > 0) busy = true
        else scheduleNext()
    }

    // --- Helpers ------------------------------------------------------------
    function pick(list) { return list[Math.floor(Math.random() * list.length)] }
    function standY(a) { return ground - a.height }
    function tiles() { return home && home.tileRects ? home.tileRects(root).filter(r => r.x > 0 && r.x + r.w < width) : [] }
    function enter(a, kind, fromLeft) {
        a.kind = kind
        a.facing = fromLeft ? 1 : -1
        a.x = fromLeft ? -a.width - 10 * s : width + 10 * s
        a.y = standY(a)
        a.visible = true
    }
    // Walk from the current x to `to`, feet on the ground (or at `feetY`).
    function walkTo(a, to, speed, feetY) {
        let from = 0
        return {
            dur: 1,
            start() {
                from = a.x
                this.dur = Math.max(200, Math.abs(to - from) / (speed * root.pace * s) * 1000)
                a.facing = to >= from ? 1 : -1
                a.walking = true
                if (feetY !== undefined) a.y = feetY - a.height
            },
            run(p, dt) {
                a.x = from + (to - from) * p
                a.walk += dt * 0.0105 * (speed / 120)
                if (p >= 1) a.walking = false
            }
        }
    }
    function waveFor(a, ms) {
        return {
            dur: ms,
            run(p, dt) {
                a.walking = false
                a.wave = p < 0.15 ? p / 0.15 : p > 0.85 ? (1 - p) / 0.15 : 1
                a.wavePhase += dt * 0.012
                a.blink = p > 0.45 && p < 0.5
            }
        }
    }
    function pause(ms) { return { dur: ms, run() {} } }
    function squashFor(a, ms, amount) {
        return { dur: ms, run(p) { a.squash = amount * Math.sin(Math.PI * p) } }
    }
    // Jump to (tx, feet at ty) with an arc `lift` pixels above the higher end.
    function jumpTo(a, txFn, tyFn, lift, ms) {
        let x0 = 0, y0 = 0
        return {
            dur: ms,
            start() { x0 = a.x; y0 = a.y + a.height },
            run(p) {
                const tx = txFn(), ty = tyFn()
                a.facing = tx >= x0 ? 1 : -1
                const top = Math.min(y0, ty) - lift
                // Quadratic arc through the start, the apex and the landing.
                const feet = (1 - p) * (1 - p) * y0 + 2 * (1 - p) * p * (2 * top - (y0 + ty) / 2) + p * p * ty
                a.x = x0 + (tx - x0) * p
                a.y = feet - a.height
                a.squash = -0.35 * Math.sin(Math.PI * p)
            }
        }
    }
    function dance(a, ms, feetYFn) {
        return {
            dur: ms,
            run(p, dt) {
                a.walking = false
                a.y = feetYFn() - a.height - Math.abs(Math.sin(p * Math.PI * 5)) * 16 * s
                a.wave = 1
                a.wavePhase += dt * 0.014
                a.facing = Math.sin(p * Math.PI * 2.5) >= 0 ? 1 : -1
            }
        }
    }

    // --- Acts -----------------------------------------------------------------
    // A bear walks across, stops somewhere along the way to wave, and walks on.
    function walkBy() {
        const a = pick([cub, cub, mama, dad])
        const kind = a === cub ? "cub" : a === mama ? "mama" : "dad"
        const fromLeft = Math.random() < 0.5
        const mid = width * (0.3 + Math.random() * 0.4)
        const exit = fromLeft ? width + 20 * s : -a.width - 20 * s
        const speed = a === cub ? 170 : 130
        return [
            { dur: 1, run() { enter(a, kind, fromLeft) } },
            walkTo(a, mid, speed),
            waveFor(a, 1900),
            walkTo(a, exit, speed)
        ]
    }
    // A bear pops up from the bottom edge near a corner, waves, and ducks away.
    function peek() {
        const a = pick([cub, mama, dad])
        const kind = a === cub ? "cub" : a === mama ? "mama" : "dad"
        const x = Math.random() < 0.5 ? width * 0.06 : width * 0.86
        const hidden = height + 4 * s, shown = ground - a.height * 0.55
        return [
            { dur: 1, run() { a.kind = kind; a.facing = x < width / 2 ? 1 : -1; a.x = x; a.y = hidden; a.visible = true } },
            { dur: 520, run(p) { const e = 1 - Math.pow(1 - p, 3); a.y = hidden + (shown - hidden) * e } },
            waveFor(a, 2000),
            { dur: 380, run(p) { a.y = shown + (hidden - shown) * p * p } }
        ]
    }
    // The cub walks in, hops onto the top of an app tile (not the focused one),
    // dances there for a moment, hops down and walks off.
    function tileHop() {
        const list = tiles()
        const choices = list.map((r, i) => i).filter(i => !list[i].focused)
        if (choices.length === 0) return []
        const index = pick(choices)
        const a = cub
        const rect = () => { const r = tiles()[index]; return r ? r : list[index] }
        const landX = () => { const r = rect(); return r.x + r.w * 0.5 - a.width / 2 }
        const landY = () => rect().y + 2 * s
        const fromLeft = landX() > width / 2 ? false : true
        const start = landX() + (fromLeft ? -1 : 1) * 150 * s
        const exit = fromLeft ? width + 20 * s : -a.width - 20 * s
        return [
            { dur: 1, run() { enter(a, "cub", fromLeft) } },
            walkTo(a, start, 170),
            squashFor(a, 220, 0.8),
            jumpTo(a, landX, landY, 70 * s, 620),
            squashFor(a, 200, 0.9),
            dance(a, 2600, landY),
            squashFor(a, 200, 0.7),
            jumpTo(a, () => landX() + (fromLeft ? 1 : -1) * 170 * s, () => ground, 50 * s, 600),
            squashFor(a, 200, 0.9),
            walkTo(a, exit, 170)
        ]
    }
    // The whole family parades across, the cub bouncing along behind.
    function parade() {
        const fromLeft = Math.random() < 0.5
        const order = [dad, mama, cub]
        const gap = 110 * s
        const dir = fromLeft ? 1 : -1
        const span = width + 3 * gap + 200 * s
        const x0 = fromLeft ? -dad.width - 10 * s : width + 10 * s
        return [
            { dur: 1, run() { enter(dad, "dad", fromLeft); enter(mama, "mama", fromLeft); enter(cub, "cub", fromLeft) } },
            {
                dur: span / (120 * root.pace * s) * 1000,
                run(p, dt) {
                    for (let k = 0; k < order.length; ++k) {
                        const a = order[k]
                        a.walking = true
                        a.facing = dir
                        a.x = x0 + dir * (p * span - k * gap)
                        a.walk += dt * (a === cub ? 0.016 : 0.0105)
                        a.y = standY(a) - (a === cub ? Math.abs(Math.sin(a.walk)) * 10 * s : 0)
                        // Everyone waves as they pass the middle.
                        const c = a.x + a.width / 2
                        const near = Math.max(0, 1 - Math.abs(c - width / 2) / (width * 0.12))
                        a.wave = near
                        a.wavePhase += dt * 0.012
                    }
                }
            }
        ]
    }
    // The cub runs after something fluttering just out of reach.
    function chase() {
        const a = cub
        const fromLeft = Math.random() < 0.5
        const exit = fromLeft ? width + 140 * s : -a.width - 140 * s
        return [
            { dur: 1, run() { enter(a, "cub", fromLeft); chased.visible = true; chased.t = 0 } },
            {
                dur: (width + 300 * s) / (230 * s) * 1000,
                start() { this.from = a.x },
                run(p, dt) {
                    a.walking = true
                    a.x = this.from + (exit - this.from) * p
                    a.walk += dt * 0.017
                    chased.t += dt / 1000
                    chased.x = a.x + a.width / 2 + a.facing * 90 * s - chased.width / 2
                    chased.y = ground - a.height * 1.05 + Math.sin(chased.t * 7) * 22 * s
                }
            }
        ]
    }

    // --- Cast ---------------------------------------------------------------
    BearPuppet { id: dad; kind: "dad"; size: 168 * Theme.scale; visible: false; weatherDress: true }
    BearPuppet { id: mama; kind: "mama"; size: 150 * Theme.scale; visible: false; weatherDress: true }
    BearPuppet { id: cub; kind: "cub"; size: 112 * Theme.scale; visible: false; weatherDress: true }
    // What the cub chases (manifest bears.chase): "firefly", "ember", "leaf",
    // "star", or any ornament.
    Item {
        id: chased
        readonly property string kind: World.bears.chase || "firefly"
        readonly property bool glow: kind === "firefly" || kind === "ember"
        property real t: 0
        visible: false
        width: 22 * Theme.scale
        height: width
        PixelBox {
            visible: chased.glow
            anchors.centerIn: parent
            width: parent.width * 0.4; height: width; radius: width / 2
            color: chased.kind === "ember" ? "#FFB45E" : "#F3D58F"
            PixelBox { anchors.centerIn: parent; width: parent.width * 3.2; height: width; radius: width / 2; color: chased.kind === "ember" ? "#40FF7A2F" : "#33F3D58F" }
        }
        PixelBox {
            visible: chased.kind === "leaf"
            anchors.centerIn: parent
            width: parent.width * 0.8; height: parent.width * 0.38; radius: height / 2
            color: "#C9A65A"
            rotation: Math.sin(chased.t * 5) * 50
        }
        Ornament {
            visible: !chased.glow && chased.kind !== "leaf"
            url: chased.glow || chased.kind === "leaf" ? "" : chased.kind === "star" ? World.ornament("star") : chased.kind
            anchors.fill: parent
            rotation: chased.t * 90
        }
    }
}
