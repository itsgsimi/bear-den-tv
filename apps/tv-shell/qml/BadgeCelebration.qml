// A new Den badge, celebrated on Home: the medal on a card, "New badge!" and
// its name, with a burst of confetti (the pairing screen's celebration, on
// the heartbeat). It plays for badges in Session.achievements.celebrate
// only while `allowed` (ShellRoot: Home in front, no dialog, no screensaver,
// nothing locked); a badge earned while an app is in front waits there until
// Home appears. Afterwards the ids go back as IPC achievements.celebrated,
// so the coordinator stops listing them.
//
// Motion: the confetti moves on World.beat and only while `alive` (never
// with reduced motion, resting, the screensaver or an app in front); with
// reduced motion (or resting) the card simply shows, without confetti. The card leaves after `seconds`
// (one single-shot timer, not an animation).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property bool allowed: false
    property real seconds: 5
    readonly property var queue: Session.achievements.celebrate || []
    // The badges on the card now, and every id already shown this run (the
    // ack's new state may arrive after `queue` changed again).
    property var showing: []
    property var shown: ({})
    property real t: 0
    readonly property bool active: showing.length > 0
    readonly property bool alive: active && visible && !Theme.reducedMotion && !Theme.resting
                                  && !Theme.screensaver && Session.target.kind === "shell"
    visible: active

    function maybeStart() {
        // Forget ids the coordinator no longer lists (acknowledged, or reset),
        // so a badge earned again after a reset is celebrated again.
        const kept = {}
        for (const id of queue) if (shown[id]) kept[id] = true
        shown = kept
        if (!allowed || active) return
        const fresh = queue.filter(id => !shown[id])
        if (fresh.length === 0) return
        const s = Object.assign({}, shown)
        for (const id of fresh) s[id] = true
        shown = s
        t = 0
        showing = fresh
        done.restart()
    }
    function finish() {
        if (!active) return
        Shell.achievementsCelebrated(showing)
        showing = []
        Qt.callLater(maybeStart)
    }
    // After the bindings settle: a snapshot changes the queue and `allowed`
    // (target, screen) together.
    onAllowedChanged: Qt.callLater(maybeStart)
    onQueueChanged: Qt.callLater(maybeStart)
    Timer { id: done; interval: root.seconds * 1000; onTriggered: root.finish() }
    Connections {
        target: World
        enabled: root.alive
        // Past 3.3 s nothing moves any more: stop, so no frame is drawn.
        function onBeat(dt) { if (root.t < 3.3) root.t = Math.min(3.3, root.t + dt) }
    }

    // Confetti: 28 pieces thrown up from behind the card, falling back.
    // Positions are a function of t alone, so a frame is only drawn per beat.
    Item {
        anchors.fill: parent
        visible: root.alive
        Repeater {
            model: root.active ? 28 : 0
            Rectangle {
                required property int index
                readonly property real angle: -Math.PI / 2 + ((index * 0.61803) % 1 - 0.5) * 2.4
                readonly property real speed: (520 + (index * 97) % 380) * Theme.scale
                readonly property real life: Math.min(root.t, 2.6)
                readonly property var palette: [Theme.accent, "#F2C14E", "#E86A8A", "#7AE0FF", "#8AD86A", "#FFFFFF"]
                width: (World.classic ? 12 : World.px * 3) * (index % 3 === 0 ? 1.4 : 1)
                height: World.classic ? width * 0.55 : width
                radius: World.classic ? height / 2 : 0
                color: palette[index % palette.length]
                readonly property real fx: parent.width / 2 + Math.cos(angle) * speed * life - width / 2
                readonly property real fy: parent.height * 0.46 + Math.sin(angle) * speed * life + 620 * Theme.scale * life * life - height / 2
                x: World.classic ? fx : World.snap(fx)
                y: World.classic ? fy : World.snap(fy)
                rotation: World.classic ? (index * 47 + root.t * 240 * (index % 2 ? 1 : -1)) % 360 : 0
                opacity: Math.max(0, 1 - root.t / 3.2)
            }
        }
    }

    PixelBox {
        id: card
        objectName: "badgeCelebration"
        anchors.centerIn: parent
        width: Math.max(560 * Theme.scale, words.implicitWidth + medal.width + 120 * Theme.scale)
        height: medal.height + 64 * Theme.scale
        radius: 32 * Theme.scale
        color: Theme.surfaceRaised
        borderColor: Theme.accent
        borderWidth: 3
        // A small pop on the heartbeat (none with reduced motion).
        scale: root.alive ? Math.min(1, 0.7 + root.t * 1.5) : 1
        BadgeMedal {
            id: medal
            x: 40 * Theme.scale
            anchors.verticalCenter: parent.verticalCenter
            zoom: 1
            badgeId: root.showing[0] || ""
            earned: true
        }
        Column {
            id: words
            anchors { left: medal.right; leftMargin: 36 * Theme.scale; verticalCenter: parent.verticalCenter }
            spacing: 6 * Theme.scale
            Text {
                text: root.showing.length > 1 ? qsTr("New badges!") : qsTr("New badge!")
                color: Theme.accent
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                objectName: "badgeCelebrationName"
                text: Badges.name(root.showing[0] || "")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 44 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                visible: root.showing.length > 1
                text: qsTr("and %n more on the shelf in Settings → Badges", "", root.showing.length - 1)
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
            }
        }
    }
}
