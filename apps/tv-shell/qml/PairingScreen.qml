// Shows the live invitation: QR code (URL with a single-use fragment token)
// and the six-digit code, with expiry and remaining attempts. Leaving the
// screen withdraws the invitation.

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    signal openScreen(string name)
    readonly property var pairing: Session.pairing
    readonly property bool listening: Session.remote.listening === true
    readonly property bool active: pairing.active === true
    readonly property string code: pairing.code || ""

    property int _devicesSeen: Session.devices.length
    property bool celebrating: false
    Connections {
        target: Session
        function onSnapshotChanged() {
            const n = Session.devices.length
            if (root.visible && n > root._devicesSeen) { root.celebrating = true; celebrateTimer.restart() }
            root._devicesSeen = n
        }
    }
    Timer { id: celebrateTimer; interval: 4000; onTriggered: root.celebrating = false }

    property bool requested: false
    function enter() {
        focusIndex = 0
        requested = false
        if (listening) { Shell.issuePairing(); requestTimer.restart() }
        Nav.reportFocus("pairing", listening ? "new-code" : "setup", 0)
    }
    function leave() { if (active) Shell.cancelPairing() }
    // A code shown once and then gone was used or expired; never re-issue
    // silently: the owner presses OK for a new one.
    Timer { id: requestTimer; interval: 1500; onTriggered: root.requested = true }
    function navigate(action) {
        switch (action) {
        case "select":
            if (listening) { root.requested = false; Shell.issuePairing(); requestTimer.restart() }
            else openScreen("remote-setup")
            return true
        case "nav.up": case "nav.down": case "nav.left": case "nav.right":
            return true
        }
        return false
    }

    // Celebration: hearts and sparkles float up, "Paired!" pops.
    Item {
        anchors.fill: parent
        z: 10
        visible: root.celebrating
        Rectangle { anchors.fill: parent; anchors.margins: -200; color: Theme.alpha("#000000", 0.45) }
        Repeater {
            model: root.celebrating ? 14 : 0
            Ornament {
                required property int index
                name: index % 3 === 0 ? "sparkle" : "heart"
                width: (28 + (index * 13) % 30) * Theme.scale; height: width
                x: parent.width * (0.15 + ((index * 37) % 70) / 100)
                y: parent.height * 0.85
                NumberAnimation on y { running: root.celebrating; to: -60 * Theme.scale; duration: Theme.reducedMotion ? 0 : 2200 + (index * 173) % 1200; easing.type: Easing.OutQuad }
                NumberAnimation on opacity { running: root.celebrating; from: 1; to: 0; duration: Theme.reducedMotion ? 0 : 2600 }
            }
        }
        PixelBox {
            anchors.centerIn: parent
            width: pairedText.implicitWidth + 96 * Theme.scale
            height: 120 * Theme.scale
            radius: height / 2
            color: Theme.surfaceRaised
            borderColor: Theme.accent
            borderWidth: 3
            scale: root.celebrating ? 1 : 0.6
            Behavior on scale { NumberAnimation { duration: Theme.ms(420); easing.type: Easing.OutBack } }
            Text {
                id: pairedText
                anchors.centerIn: parent
                text: qsTr("Paired! Welcome to the den")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 44 * Theme.fontUnit
                font.weight: Font.Bold
            }
        }
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Pair a phone")
        subtitle: root.listening ? qsTr("Scan the code with your phone's camera") : qsTr("Turn on the phone remote first")
        hints: [["OK", root.listening ? qsTr("New code") : qsTr("Set up")], ["Back", qsTr("Done")]]

        Row {
            visible: root.listening
            anchors.centerIn: parent
            spacing: 96 * Theme.scale

            PixelBox {
                width: 440 * Theme.scale; height: width
                radius: 28 * Theme.scale
                color: "#ffffff"
                anchors.verticalCenter: parent.verticalCenter
                QrCode {
                    anchors.fill: parent
                    anchors.margins: 24 * Theme.scale
                    modules: root.pairing.qr_modules || []
                    dark: "#111416"
                    light: "#ffffff"
                    quietZone: 1
                    visible: root.active
                }
                Text {
                    visible: !root.active
                    anchors.centerIn: parent
                    text: root.requested ? qsTr("Code used or expired.\nPress OK for a new one.") : qsTr("Preparing…")
                    horizontalAlignment: Text.AlignHCenter
                    color: "#333"
                    font.family: Theme.fontFamily
                    font.pixelSize: 28 * Theme.fontUnit
                }
            }

            Column {
                anchors.verticalCenter: parent.verticalCenter
                spacing: 22 * Theme.scale
                Text {
                    text: qsTr("Or open this address and enter the code")
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 26 * Theme.fontUnit
                }
                Text {
                    text: (Session.remote.addresses || [])[0] || ""
                    color: Theme.accent
                    font.family: Theme.monoFamily
                    font.pixelSize: 32 * Theme.fontUnit
                }
                Row {
                    spacing: 14 * Theme.scale
                    Repeater {
                        model: root.code.length === 6 ? root.code.split("") : ["–", "–", "–", "–", "–", "–"]
                        PixelBox {
                            required property string modelData
                            width: 92 * Theme.scale; height: 120 * Theme.scale
                            radius: 18 * Theme.scale
                            color: Theme.surfaceRaised
                            borderColor: Theme.surfaceBorder
                            Text {
                                anchors.centerIn: parent
                                text: modelData
                                color: Theme.textPrimary
                                font.family: Theme.monoFamily
                                font.pixelSize: 72 * Theme.fontUnit
                                font.weight: Font.Bold
                            }
                        }
                    }
                }
                Text {
                    visible: root.active
                    text: qsTr("Expires in %1:%2 · %3 attempts left")
                          .arg(Math.floor((root.pairing.expires_in_s || 0) / 60))
                          .arg(String((root.pairing.expires_in_s || 0) % 60).padStart(2, "0"))
                          .arg(root.pairing.attempts_left || 0)
                    color: Theme.textMuted
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                }
                FocusButton {
                    text: qsTr("New code")
                    focused: true
                }
            }
        }

        Column {
            visible: !root.listening
            anchors.centerIn: parent
            spacing: 28 * Theme.scale
            width: parent.width * 0.6
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WordWrap
                text: qsTr("Phones can only pair while the phone remote is on. It stays off until you allow it on your home network.")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
            }
            FocusButton {
                anchors.horizontalCenter: parent.horizontalCenter
                text: qsTr("Set up phone remote")
                primary: true
                focused: true
            }
        }
    }
}
