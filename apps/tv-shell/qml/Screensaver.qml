// OLED-safe screensaver: after a long idle spell on Bear Den the screen goes
// almost black and a small clock with the bear drifts to a new place every
// 15 s, so no pixel stays lit in one spot. It only renders during the short
// moves (cheap on small boxes). Any input wakes it; that input is consumed.

import QtQuick
import BearDen

Rectangle {
    id: root
    objectName: "screensaver"
    property bool active: false
    color: "#000000"
    visible: opacity > 0
    opacity: active ? 1 : 0
    Behavior on opacity { NumberAnimation { duration: Theme.ms(900); easing.type: Easing.InOutQuad } }

    function moveSomewhere() {
        const maxX = Math.max(0, root.width - card.width)
        const maxY = Math.max(0, root.height - card.height)
        card.x = Math.random() * maxX
        card.y = Math.random() * maxY
        // Alternate the bear's side so it is not always at the same offset.
        row.layoutDirection = row.layoutDirection === Qt.LeftToRight ? Qt.RightToLeft : Qt.LeftToRight
    }
    onActiveChanged: if (active) moveSomewhere()

    Timer {
        interval: 15000
        repeat: true
        running: root.active
        onTriggered: root.moveSomewhere()
    }
    // After 30 minutes the clock dims further.
    property bool deep: false
    Timer {
        interval: 30 * 60 * 1000
        running: root.active
        onTriggered: root.deep = true
    }
    onVisibleChanged: if (!visible) deep = false

    Item {
        id: card
        width: row.implicitWidth
        height: row.implicitHeight
        opacity: root.deep ? 0.35 : 0.8
        // Jumps to its next spot (one frame every 15 s) instead of gliding:
        // OLED safety only needs the move, and a glide redraws for 2 s.
        Behavior on opacity { NumberAnimation { duration: Theme.ms(3000) } }

        Row {
            id: row
            spacing: 36 * Theme.scale
            // The sleeping bear, with its "z"s (static: nothing renders between moves).
            Item {
                anchors.verticalCenter: parent.verticalCenter
                width: 150 * Theme.scale
                height: width
                BearHead {
                    anchors.fill: parent
                    visible: !World.performance
                    kind: "dad"
                    night: true
                }
                Text {
                    text: "z"
                    x: parent.width * 0.66; y: -parent.height * 0.16
                    color: "#EDE3D1"; opacity: 0.6
                    font.family: Theme.fontFamily; font.pixelSize: 28 * Theme.scale; font.weight: Font.Bold
                }
                Text {
                    text: "Z"
                    x: parent.width * 0.82; y: -parent.height * 0.4
                    color: "#EDE3D1"; opacity: 0.4
                    font.family: Theme.fontFamily; font.pixelSize: 38 * Theme.scale; font.weight: Font.Bold
                }
            }
            Column {
                anchors.verticalCenter: parent.verticalCenter
                spacing: 6 * Theme.scale
                Text {
                    id: clock
                    color: "#EDE3D1"
                    font.family: Theme.fontFamily
                    font.pixelSize: 120 * Theme.scale
                    font.weight: Font.Light
                    text: Qt.formatTime(new Date(), Qt.locale().timeFormat(Locale.ShortFormat))
                }
                Text {
                    id: date
                    color: Theme.textMuted
                    font.family: Theme.fontFamily
                    font.pixelSize: 30 * Theme.scale
                    text: Qt.formatDate(new Date(), "dddd, MMMM d")
                }
                Text {
                    color: Theme.textMuted
                    font.family: Theme.fontFamily
                    font.pixelSize: 22 * Theme.scale
                    text: qsTr("Press any button to wake Bear Den")
                }
            }
        }
    }
    Timer {
        interval: 10000
        repeat: true
        running: root.active
        triggeredOnStart: true
        onTriggered: {
            clock.text = Qt.formatTime(new Date(), Qt.locale().timeFormat(Locale.ShortFormat))
            date.text = Qt.formatDate(new Date(), "dddd, MMMM d")
        }
    }
}
