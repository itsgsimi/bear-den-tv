// Shows the live invitation: QR code (URL with a single-use fragment token)
// and the six-digit code, with expiry and remaining attempts. Leaving the
// screen withdraws the invitation. "Who is it for?" (◀ ▶) chooses a family
// phone or a guest pass (tonight, 24 hours, 7 days: IPC pair.issue "pass",
// contracts/http.md#guest-passes); changing it issues a new code. The
// guest note and the address sit on a surface panel in the theme's text
// colours, so they read over any scene and in both art styles. Once a
// phone pairs, a panel names it ("“Kitchen tablet” is paired") with Pair
// another and Done instead of the spent code (UX-06: the TV used to sit on
// "Code used or expired").

import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    signal openScreen(string name)
    signal done()
    readonly property var pairing: Session.pairing
    readonly property bool listening: Session.remote.listening === true
    readonly property bool active: pairing.active === true
    readonly property string code: pairing.code || ""

    property int _devicesSeen: Session.devices.length
    property var _idsSeen: Session.devices.map(d => d.id)
    property bool celebrating: false
    // The phone that just paired, named on the success panel ("" = none).
    property string pairedName: ""
    property int doneIndex: 0   // 0 = Pair another, 1 = Done
    Connections {
        target: Session
        function onSnapshotChanged() {
            const n = Session.devices.length
            if (root.visible && n > root._devicesSeen) {
                root.celebrating = true
                celebrateTimer.restart()
                const fresh = Session.devices.filter(d => root._idsSeen.indexOf(d.id) < 0)
                root.pairedName = fresh.length > 0 ? fresh[fresh.length - 1].name : qsTr("Your phone")
                root.doneIndex = 0
                root.report()
            }
            root._devicesSeen = n
            root._idsSeen = Session.devices.map(d => d.id)
        }
    }
    Timer { id: celebrateTimer; interval: 4000; onTriggered: root.celebrating = false }

    // Who the code is for; pass is the IPC pair.issue value ("" = family).
    readonly property var kinds: [
        { pass: "", label: qsTr("Family phone") },
        { pass: "tonight", label: qsTr("Guest pass · Tonight") },
        { pass: "24h", label: qsTr("Guest pass · 24 hours") },
        { pass: "7d", label: qsTr("Guest pass · 7 days") }
    ]
    property int kindIndex: 0
    readonly property string pass: kinds[kindIndex].pass
    readonly property bool guestLive: active && pairing.guest === true

    // When a guest pass ends, as the TV's clock shows it: a time today or
    // tonight, a weekday and time further out.
    // In the header clock's format (UX-28).
    function clockTime(d) { return Qt.formatTime(d, Qt.locale().timeFormat(Locale.ShortFormat)) }
    function endsText(ms) {
        const d = new Date(ms)
        return ms - Date.now() < 20 * 3600 * 1000 ? clockTime(d) : Qt.formatDate(d, "ddd") + " " + clockTime(d)
    }
    function kindDescription() {
        if (root.pass === "")
            return qsTr("Stays paired until you remove it in Paired phones")
        const ends = root.guestLive && root.pairing.pass_expires_at_ms ? qsTr("ends %1").arg(endsText(root.pairing.pass_expires_at_ms))
                                                                       : (root.pass === "tonight" ? qsTr("ends at %1").arg(clockTime(new Date(2000, 0, 1, 4, 0))) : "")
        return qsTr("A remote for a visitor · %1").arg(ends || qsTr("ends by itself"))
    }

    property bool requested: false
    function issue() { root.requested = false; Shell.issuePairing(root.pass); requestTimer.restart() }
    function report() {
        if (pairedName.length > 0) { Nav.reportFocus("pairing", doneIndex === 0 ? "pair-another" : "done", 0); return }
        Nav.reportFocus("pairing", !listening ? "setup" : (focusIndex === 0 ? "pair-kind" : "new-code"), 0)
    }
    function enter() {
        focusIndex = 0
        kindIndex = 0 // a new visit starts as a family phone; a guest pass is always chosen
        requested = false
        pairedName = ""
        if (listening) issue()
        report()
    }
    function leave() { if (active) Shell.cancelPairing() }
    // A code shown once and then gone was used or expired; never re-issue
    // silently: the owner presses OK for a new one.
    Timer { id: requestTimer; interval: 1500; onTriggered: root.requested = true }
    function navigate(action) {
        if (pairedName.length > 0) {
            switch (action) {
            case "nav.left": case "nav.right":
                doneIndex = action === "nav.left" ? 0 : 1; report(); return true
            case "select":
                if (doneIndex === 0) { pairedName = ""; issue(); report() }
                else done()
                return true
            case "nav.up": case "nav.down": return true
            }
            return false
        }
        switch (action) {
        case "select":
            if (listening) issue()
            else openScreen("remote-setup")
            return true
        case "nav.up": case "nav.down":
            if (listening) { focusIndex = action === "nav.up" ? 0 : 1; report() }
            return true
        case "nav.left": case "nav.right":
            if (listening && focusIndex === 0) {
                const next = Math.max(0, Math.min(kinds.length - 1, kindIndex + (action === "nav.left" ? -1 : 1)))
                if (next !== kindIndex) { kindIndex = next; issue() }
            }
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
        subtitle: !root.listening ? qsTr("Turn on the phone remote first")
                  : (root.pass !== "" ? qsTr("A guest pass: a remote for a visitor that ends by itself") : qsTr("Scan the code with your phone's camera"))
        hints: root.listening ? [["◀ ▶", qsTr("Family or guest")], ["OK", qsTr("New code")], ["Back", qsTr("Done")]]
                              : [["OK", qsTr("Set up")], ["Back", qsTr("Done")]]

        Row {
            visible: root.listening && root.pairedName.length === 0
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
                    width: parent.width - 48 * Theme.scale
                    wrapMode: Text.WordWrap
                    horizontalAlignment: Text.AlignHCenter
                    color: "#333"
                    font.family: Theme.fontFamily
                    font.pixelSize: 28 * Theme.fontUnit
                }
            }

            Column {
                anchors.verticalCenter: parent.verticalCenter
                spacing: 22 * Theme.scale
                SettingsRow {
                    objectName: "pairKindRow"
                    width: 740 * Theme.scale
                    kind: "choice"
                    label: qsTr("Who is it for?")
                    value: root.kinds[root.kindIndex].label
                    description: root.kindDescription()
                    focused: root.focusIndex === 0
                }
                PixelBox {
                    objectName: "pairAddressPanel"
                    width: 740 * Theme.scale
                    height: addressColumn.implicitHeight + 2 * addressColumn.pad
                    radius: 18 * Theme.scale
                    color: Theme.surface
                    borderColor: Theme.surfaceBorder
                    borderWidth: 1
                    Column {
                        id: addressColumn
                        readonly property real pad: 22 * Theme.scale
                        x: pad; y: pad
                        width: parent.width - 2 * pad
                        spacing: 10 * Theme.scale
                        Text {
                            visible: root.pass !== ""
                            width: parent.width
                            wrapMode: Text.WordWrap
                            text: qsTr("Guests can move around, open apps, play, pause and change the volume. They can't close apps, change settings or turn anything off.")
                            color: Theme.textSecondary
                            font.family: Theme.fontFamily
                            font.pixelSize: 22 * Theme.fontUnit
                        }
                        Text {
                            width: parent.width
                            wrapMode: Text.WordWrap
                            text: qsTr("Or open this address and enter the code")
                            color: Theme.textSecondary
                            font.family: Theme.fontFamily
                            font.pixelSize: 26 * Theme.fontUnit
                        }
                        Text {
                            objectName: "pairAddress"
                            width: parent.width
                            wrapMode: Text.WrapAnywhere
                            text: (Session.remote.addresses || [])[0] || ""
                            color: Theme.textPrimary
                            font.family: Theme.monoFamily
                            font.pixelSize: 32 * Theme.fontUnit
                            font.weight: Font.Bold
                        }
                    }
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
                    focused: root.focusIndex === 1
                }
            }
        }

        // Paired: who, and what next (UX-06).
        PixelBox {
            objectName: "pairedPanel"
            visible: root.pairedName.length > 0 && !root.celebrating
            anchors.centerIn: parent
            width: Math.max(pairedColumn.implicitWidth, 700 * Theme.scale) + 112 * Theme.scale
            height: pairedColumn.implicitHeight + 96 * Theme.scale
            radius: Theme.radius * 1.4
            color: Theme.surfaceRaised
            borderColor: Theme.surfaceBorder
            Column {
                id: pairedColumn
                anchors.centerIn: parent
                spacing: 24 * Theme.scale
                Text {
                    objectName: "pairedTitle"
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: qsTr("“%1” is paired").arg(root.pairedName)
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 44 * Theme.fontUnit
                    font.weight: Font.Bold
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: qsTr("It can control this TV now. You can rename or remove it in Paired phones.")
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 26 * Theme.fontUnit
                }
                Row {
                    anchors.horizontalCenter: parent.horizontalCenter
                    spacing: 20 * Theme.scale
                    FocusButton { text: qsTr("Pair another"); focused: root.doneIndex === 0 }
                    FocusButton { text: qsTr("Done"); primary: true; focused: root.doneIndex === 1 }
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
