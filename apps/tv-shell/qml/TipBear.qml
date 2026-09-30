// Bear tips (owner request 2026-09-29: "on first use … point out themes or
// talk about apps … in a fun manner utilizing the bears, but not annoying as
// clippy"). Once Home has been quiet for a while, a bear waddles in along
// just under the top bar, stops beside the thing the tip is about (a pill in
// the top bar), holds up its sign right under it with the sign's arrow
// pointing up at it, sits and waits. The sign has one short line and two keys: "OK · Show me" (the bear
// hops and Bear Den goes there) and "Back · Not now" (the bear shrugs and
// leaves; that tip never shows again). The arrows keep working as always:
// the bear then walks off, and the tip may come back another day.
//
// Anti-Clippy rules (ShellRoot sets `allowed`; the rest is here):
//  - never before first-run setup is done, never while an app is in front,
//    over a dialog, card or the launch overlay, on the screensaver or locked
//    (all in `allowed`), and only on Home;
//  - only after Home has been quiet for Shell.tipSeconds (8 s; any key
//    starts the wait again: poke());
//  - at most one tip per local calendar day (`today`, injected by tests;
//    state.tips.last_day and this run's shownDay);
//  - each tip once (answered tips are state.tips.done); after three "Not
//    now" in a row the bears stop (state.tips.stopped); Settings → Home
//    screen → Bear tips turns them off, Show tips again starts over;
//  - it never takes focus: while the bear waits, OK and Back answer it (the
//    sign says so) and nothing else changes; leaving Home is not an answer.
// Tips come in the order of `table`, skipping any whose feature is already
// in use (derived from the snapshot). Motion runs on World.beat only while
// the bear walks or hops (World.visiting); waiting, it sits still and costs
// nothing. With reduced motion the bear simply appears, sitting. Events go
// to the coordinator as IPC tips.event (seen, ok, not_now).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    objectName: "tipBear"
    property bool allowed: false
    // HomeScreen, for where the top bar's pills are.
    property var home: null
    // The local calendar day, "yyyy-MM-dd" (tests set it: the injected clock).
    property string today: Qt.formatDate(new Date(), "yyyy-MM-dd")
    property string shownDay: ""
    readonly property real s: Theme.scale

    signal showMe(string tipId)

    readonly property var tips: Session.tips || ({})
    // Absent (an older coordinator): no tips.
    readonly property bool tipsOn: tips.enabled === true && tips.stopped !== true
    readonly property bool dayFree: (tips.last_day || "") !== today && shownDay !== today
    property var answered: ({})   // this run's answers, before the next snapshot

    // Copy and targets in one table, in the order the bears offer them.
    // pill: the top bar pill the sign points at; hat/carry: the bear's props.
    readonly property var table: [
        { id: "themes", pill: "themes", hat: "hat-beret", carry: "",
          line: qsTr("Psst! Your den can look all sorts of ways. Want to try a new theme?") },
        { id: "add-apps", pill: "apps", hat: "", carry: "",
          line: qsTr("There are more apps I can fetch for you, one press each.") },
        { id: "phone-remote", pill: "pairing", hat: "", carry: "phone",
          line: qsTr("Your phone can be the remote too. Want to pair it?") },
        { id: "now-playing", pill: "pairing", hat: "", carry: "phone",
          line: qsTr("Your phone shows what's playing and can pause it. Neat, right?") },
        { id: "sleep-timer", pill: "settings", hat: "hat-nightcap", carry: "",
          line: qsTr("Dozing off on the couch? A sleep timer turns things off for you.") },
        { id: "badges", pill: "themes", hat: "", carry: "",
          line: qsTr("You earned a Den badge! There are more to find on the shelf.") },
        { id: "guest-pass", pill: "pairing", hat: "", carry: "phone",
          line: qsTr("Visitors can have a remote too, that ends by itself: a guest pass.") }
    ]
    readonly property int pairedPhones: Session.remote.paired_device_count || 0
    // Whether a tip still has something to say (its feature not used yet).
    function applies(id) {
        switch (id) {
        case "themes": return ((Session.layout.ui || {}).background || "den") === "den"
        case "add-apps": return Session.applications.some(a => a.installed !== true && a.install !== undefined && a.install.state === "available")
        case "phone-remote": return pairedPhones === 0
        case "now-playing": return pairedPhones > 0
        case "sleep-timer": return !(Session.power && Session.power.sleep_at_ms)
        case "badges": return ((Session.achievements || {}).earned || []).length > 0
        case "guest-pass": return pairedPhones > 0
        }
        return false
    }
    readonly property var next: {
        const done = tips.done || []
        for (const t of table)
            if (done.indexOf(t.id) < 0 && !answered[t.id] && applies(t.id)) return t
        return null
    }

    // --- The quiet spell ----------------------------------------------------
    property string phase: "idle"    // idle | arriving | waiting | hopping | leaving
    property var tip: null
    readonly property bool waiting: phase === "waiting"
    readonly property bool active: phase !== "idle"
    Timer {
        id: quiet
        objectName: "tipQuiet"
        interval: Math.max(1, Shell.tipSeconds) * 1000
        running: root.allowed && root.tipsOn && root.dayFree && root.next !== null && root.phase === "idle"
        onTriggered: root.begin()
    }
    // A key: the quiet spell starts again.
    function poke() { if (quiet.running) quiet.restart() }
    onAllowedChanged: if (!allowed && phase !== "idle") vanish()

    // --- Where things are ---------------------------------------------------
    // The pill's middle and bottom (or the top middle without one).
    property real pillMid: width / 2
    property real pillBottom: 110 * s
    function locate(name) {
        const item = findPill(name)
        if (item) {
            const p = item.mapToItem(root, item.width / 2, item.height)
            pillMid = p.x
            pillBottom = p.y
        } else {
            pillMid = width / 2
            pillBottom = 110 * s
        }
    }
    // The sign right under the pill; the bear beside it, feet at its foot.
    readonly property real signX: Math.max(8 * s, Math.min(width - sign.width - 8 * s, pillMid - sign.width / 2))
    readonly property real signY: pillBottom + 66 * s
    function bearSpot() {
        const right = signX + sign.width + 10 * s
        return right + bear.width < width - 8 * s ? right : signX - bear.width - 10 * s
    }
    function findPill(name) {
        const want = "navPill-" + name
        function walk(item) {
            if (!item) return null
            if (item.objectName === want && item.visible) return item
            for (let i = 0; i < item.children.length; ++i) {
                const f = walk(item.children[i])
                if (f) return f
            }
            return null
        }
        return walk(home)
    }

    // --- Moves on the heartbeat --------------------------------------------
    // Follows the sign's size (its Column lays out a frame later).
    readonly property real targetX: bearSpot()
    property real t: 0
    Binding { target: World; property: "visiting"; value: root.phase === "arriving" || root.phase === "hopping" || root.phase === "leaving"; when: root.active }
    Connections {
        target: World
        enabled: root.phase === "arriving" || root.phase === "hopping" || root.phase === "leaving"
        function onBeat(dt) { root.step(dt) }
    }
    function step(dt) {
        const speed = 170 * s
        if (phase === "arriving") {
            bear.walking = true
            bear.facing = -1
            bear.walk += dt * 8
            bear.x = Math.max(targetX, bear.x - speed * dt)
            if (bear.x <= targetX) arrive()
        } else if (phase === "leaving") {
            bear.sitting = false
            bear.walking = true
            bear.facing = 1
            bear.walk += dt * 8
            bear.x += speed * 1.3 * dt
            if (bear.x > width + 20 * s) vanish()
        } else if (phase === "hopping") {
            t += dt
            bear.sitting = false
            bear.squash = -0.6 * Math.sin(Math.PI * Math.min(1, t / 0.45))
            bear.y = standY() - Math.sin(Math.PI * Math.min(1, t / 0.45)) * 46 * s
            if (t >= 0.5) { const id = tip.id; vanish(); root.showMe(id) }
            return
        }
    }
    function standY() { return signY + sign.height - bear.height }

    function begin() {
        tip = next
        if (!tip) return
        locate(tip.pill)
        bear.sitting = false
        bear.squash = 0
        bear.wave = 0
        bear.y = Qt.binding(() => root.standY())
        bear.x = width + 20 * s
        bear.visible = true
        if (Theme.reducedMotion) { bear.x = targetX; arrive(); return }
        phase = "arriving"
    }
    function arrive() {
        bear.walking = false
        bear.sitting = true
        bear.facing = -1   // looking at its sign
        bear.x = Qt.binding(() => root.targetX)
        phase = "waiting"
        shownDay = today
        Shell.tipEvent(tip.id, "seen")
    }
    // OK (show me) or Back (not now) while the bear waits.
    function answer(ok) {
        if (!waiting) return
        const a = Object.assign({}, answered)
        a[tip.id] = true
        answered = a
        Shell.tipEvent(tip.id, ok ? "ok" : "not_now")
        if (ok) {
            if (Theme.reducedMotion) { const id = tip.id; vanish(); showMe(id); return }
            t = 0
            phase = "hopping"
        } else {
            // A little shrug (the arm up once), then off it goes.
            bear.wave = 1
            shrug.restart()
        }
    }
    Timer { id: shrug; interval: 450; onTriggered: { bear.wave = 0; if (Theme.reducedMotion) root.vanish(); else root.phase = "leaving" } }
    // An arrow while the bear waits: it walks off; no answer, it may come
    // back another day.
    function leaveQuietly() {
        if (!waiting) return
        if (Theme.reducedMotion) vanish()
        else phase = "leaving"
    }
    function vanish() {
        shrug.stop()
        bear.visible = false
        bear.walking = false
        bear.sitting = false
        bear.squash = 0
        bear.wave = 0
        phase = "idle"
    }

    // --- The bear and its sign ---------------------------------------------
    BearPuppet {
        id: bear
        objectName: "tipBearPuppet"
        kind: "cub"
        size: 120 * Theme.scale
        visible: false
        hat: root.tip && root.tip.hat ? World.ornament(root.tip.hat) : ""
        carry: root.tip && root.tip.carry ? World.ornament(root.tip.carry) : ""
    }
    PixelBox {
        id: sign
        objectName: "tipSign"
        visible: root.waiting
        width: Math.min(root.width * 0.42, signColumn.implicitWidth + 56 * Theme.scale)
        height: signColumn.implicitHeight + 40 * Theme.scale
        x: root.signX
        y: root.signY
        radius: 16 * Theme.scale
        color: Theme.surfaceRaised
        borderColor: Theme.accent
        borderWidth: 3
        // The wooden arrow on top, pointing up at what the tip is about.
        Ornament {
            objectName: "tipArrow"
            name: "pointer-up"
            width: 56 * Theme.scale
            height: 64 * Theme.scale
            x: Math.max(12 * root.s, Math.min(sign.width - width - 12 * root.s, root.pillMid - sign.x - width / 2))
            y: -height + 6 * root.s
        }
        Column {
            id: signColumn
            anchors.centerIn: parent
            width: Math.min(root.width * 0.42 - 56 * Theme.scale, Math.max(lineText.implicitWidth, keys.implicitWidth))
            spacing: 14 * Theme.scale
            Text {
                id: lineText
                objectName: "tipLine"
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.tip ? root.tip.line : ""
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Row {
                id: keys
                spacing: 22 * Theme.scale
                KeyCap { key: qsTr("OK"); label: root.tip && root.tip.id === "now-playing" ? qsTr("Thanks!") : qsTr("Show me") }
                KeyCap { key: qsTr("Back"); label: qsTr("Not now") }
            }
        }
    }
    component KeyCap: Row {
        property string key
        property string label
        spacing: 8 * Theme.scale
        PixelBox {
            implicitWidth: keyText.implicitWidth + 16 * Theme.scale
            implicitHeight: 32 * Theme.scale
            radius: 8 * Theme.scale
            color: Theme.surface
            borderColor: Theme.surfaceBorder
            anchors.verticalCenter: parent.verticalCenter
            Text { id: keyText; anchors.centerIn: parent; text: parent.parent.key; color: Theme.textPrimary; font.family: Theme.fontFamily; font.pixelSize: 18 * Theme.fontUnit; font.weight: Font.Bold }
        }
        Text { anchors.verticalCenter: parent.verticalCenter; text: parent.label; color: Theme.textSecondary; font.family: Theme.fontFamily; font.pixelSize: 22 * Theme.fontUnit }
    }
}
