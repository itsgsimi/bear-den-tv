// The shell's focus graph. Every key — physical keyboard, or remote input the
// coordinator injects through Nav — arrives here as a named action and goes
// to exactly one owner: the topmost dialog, the launch overlay, or the
// current screen. Back pops one level; at Home it is a reported no-op.

import QtQuick
import BearDen

FocusScope {
    id: root
    focus: true
    property var stack: ["home"]
    readonly property string screen: stack[stack.length - 1]
    readonly property var screens: ({
        "home": home, "settings": settings, "remote-setup": remoteSetup,
        "pairing": pairing, "devices": devices, "diagnostics": diagnostics, "playback": playback,
        "advanced-playback": advancedPlayback, "weather": weather
    })
    readonly property var topDialog: confirmDialog.visible ? confirmDialog
                                   : messageDialog.visible ? messageDialog
                                   : appDialog.visible ? appDialog
                                   : null
    readonly property bool blocked: !Session.loaded || Session.locked

    function navScreenName() {
        if (topDialog) return "dialog"
        if (screen === "remote-setup") return "setup"
        if (screen === "playback" || screen === "advanced-playback") return "diagnostics"   // the contract's screen names
        if (screen === "weather") return "settings"
        return screen
    }
    function syncNav() { Nav.screen = navScreenName() }
    onTopDialogChanged: {
        syncNav()
        // Modal focus returns to the control that opened it.
        if (!topDialog) {
            if (screen === "home") home.restoreFocus()
            else if (current().report) current().report()
        }
    }

    function current() { return screens[screen] || home }
    function open(name) {
        if (!screens[name] || name === screen) return
        if (screen === "pairing") pairing.leave()
        stack = stack.concat([name])
        syncNav()
        current().enter()
    }
    function pop() {
        if (stack.length <= 1) return false
        if (screen === "pairing") pairing.leave()
        stack = stack.slice(0, stack.length - 1)
        syncNav()
        if (screen === "home") home.restoreFocus()
        else current().enter()
        return true
    }
    function goHome() {
        screensaver.active = false
        wake()
        confirmDialog.finish(false)
        messageDialog.visible = false
        appDialog.visible = false
        launchOverlay.dismissed = true
        if (screen === "pairing") pairing.leave()
        stack = ["home"]
        syncNav()
        home.restoreFocus()
    }

    // Idle rest: ambient motion stops after a quiet spell; input wakes it.
    Timer {
        id: restTimer
        interval: Shell.restSeconds * 1000
        running: true
        onTriggered: Theme.resting = true
    }
    function wake() {
        Theme.resting = false
        restTimer.restart()
        screensaverTimer.restart()
    }
    // OLED-safe screensaver after a long quiet spell while Bear Den is in front.
    Timer {
        id: screensaverTimer
        interval: Math.max(1, Shell.screensaverSeconds) * 1000
        running: Shell.screensaverSeconds > 0
        onTriggered: {
            if (Session.loaded && !Session.locked && Session.target.kind === "shell" && !root.topDialog)
                screensaver.active = true
            else
                restart()
        }
    }

    function handle(action) {
        if (screensaver.active) {
            // Waking consumes the press so it does not also move or select.
            screensaver.active = false
            wake()
            if (action === "back") Nav.noteAtRoot()
            return
        }
        wake()
        if (secretCode(action)) return
        if (action === "home") { goHome(); return }
        if (blocked) { if (action === "back") Nav.noteAtRoot(); return }
        if (topDialog) { topDialog.navigate(action); return }
        if (launchOverlay.visible) { launchOverlay.navigate(action); return }
        if (current().navigate(action)) return
        if (action === "back" && !pop()) Nav.noteAtRoot()
    }

    // The remote's secret code (up up down down left right left right OK, from
    // the TV remote or a phone): the whole bear family parades by. The presses
    // still do their normal job, except the final OK, which the parade uses up
    // (it would otherwise open the focused app and send the bears away).
    // Nothing else is unlocked.
    readonly property var code: ["nav.up", "nav.up", "nav.down", "nav.down", "nav.left", "nav.right", "nav.left", "nav.right", "select"]
    property int codeAt: 0
    function secretCode(action) {
        codeAt = action === code[codeAt] ? codeAt + 1 : (action === code[0] ? 1 : 0)
        if (codeAt < code.length)
            return false
        codeAt = 0
        if (!visitors.celebrate())
            return false
        toast.show("info", qsTr("The bears heard you!"))
        return true
    }

    Keys.onPressed: (event) => {
        const action = Nav.actionForKey(event.key)
        if (action.length === 0) return
        event.accepted = true
        handle(action)
    }

    Connections {
        target: Nav
        function onHomeRequested() { root.goHome() }
    }
    Connections {
        target: Shell
        function onConfirmRequested(confirmId, kind, summary, expiresInS) {
            confirmDialog.open({
                title: summary, body: qsTr("Your display settings changed. Keep them?"),
                confirmLabel: qsTr("Keep"), cancelLabel: qsTr("Revert"), countdown: expiresInS, focusConfirm: true,
                onAccept: () => Shell.answerConfirm(confirmId, true),
                onReject: () => Shell.answerConfirm(confirmId, false)
            })
        }
        function onNotify(kind, text) { toast.show(kind, text) }
        function onRequestFailed(what, message) { messageDialog.open(what, message) }
    }

    Component.onCompleted: {
        syncNav()
        if (Shell.startScreen !== "home" && screens[Shell.startScreen]) Qt.callLater(() => open(Shell.startScreen))
    }
    property bool _wasLoaded: false
    Connections {
        target: Session
        function onSnapshotChanged() {
            if (Session.loaded && !root._wasLoaded) { root._wasLoaded = true; if (root.screen === "home") home.restoreFocus() }
        }
    }

    Wallpaper { anchors.fill: parent }

    Item {
        id: safe
        anchors.fill: parent
        anchors.leftMargin: Math.max(Theme.safeX, 48 * Theme.scale)
        anchors.rightMargin: Math.max(Theme.safeX, 48 * Theme.scale)
        anchors.topMargin: Math.max(Theme.safeY, 32 * Theme.scale)
        anchors.bottomMargin: Math.max(Theme.safeY, 24 * Theme.scale)
        // Nothing private is drawn behind the lock screen.
        visible: Session.loaded && !Session.locked

        HomeScreen {
            id: home
            width: parent.width; height: parent.height
            opacity: root.screen === "home" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "home" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
            onAppUnavailable: (app) => appDialog.openFor(app)
            onMessage: (title, body) => messageDialog.open(title, body)
        }
        SettingsScreen {
            id: settings
            width: parent.width; height: parent.height
            opacity: root.screen === "settings" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "settings" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
            onConfirm: (title, body, label, accept) => confirmDialog.open({ title: title, body: body, confirmLabel: label, onAccept: accept })
        }
        RemoteSetupScreen {
            id: remoteSetup
            width: parent.width; height: parent.height
            opacity: root.screen === "remote-setup" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "remote-setup" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
        }
        PairingScreen {
            id: pairing
            width: parent.width; height: parent.height
            opacity: root.screen === "pairing" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "pairing" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
        }
        DevicesScreen {
            id: devices
            width: parent.width; height: parent.height
            opacity: root.screen === "devices" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "devices" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onConfirm: (title, body, label, accept) => confirmDialog.open({ title: title, body: body, confirmLabel: label, onAccept: accept })
        }
        DiagnosticsScreen {
            id: diagnostics
            width: parent.width; height: parent.height
            opacity: root.screen === "diagnostics" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "diagnostics" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
        }
        PlaybackScreen {
            id: playback
            width: parent.width; height: parent.height
            opacity: root.screen === "playback" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "playback" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
        }
        AdvancedPlaybackScreen {
            id: advancedPlayback
            width: parent.width; height: parent.height
            opacity: root.screen === "advanced-playback" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "advanced-playback" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
        }
        WeatherScreen {
            id: weather
            width: parent.width; height: parent.height
            active: root.screen === "weather" && !root.topDialog
            opacity: root.screen === "weather" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "weather" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
        }
        ErrorBanner {
            anchors { bottom: parent.bottom; horizontalCenter: parent.horizontalCenter }
        }
        Toast {
            id: toast
            anchors { top: parent.top; right: parent.right; topMargin: 110 * Theme.scale }
        }
    }

    ConnectingScreen { anchors.fill: parent; visible: !Session.loaded }
    // Bears dropping by on Home, Settings and Pair phone (never over dialogs,
    // the launch overlay, the lock screen or other screens).
    BearVisitors {
        id: visitors
        objectName: "bearVisitors"
        anchors.fill: parent
        screen: root.screen
        home: home
        allowed: (root.screen === "home" || root.screen === "settings" || root.screen === "pairing")
                 && !root.topDialog && !launchOverlay.visible && !root.blocked
    }
    LaunchOverlay { id: launchOverlay; anchors.fill: parent }
    AppUnavailableDialog { id: appDialog; anchors.fill: parent }
    MessageDialog { id: messageDialog; anchors.fill: parent }
    ConfirmDialog { id: confirmDialog; anchors.fill: parent }
    LockedScreen { anchors.fill: parent; visible: Session.loaded && Session.locked }
    Screensaver { id: screensaver; anchors.fill: parent; onActiveChanged: Theme.screensaver = active }
    // An app coming to the front (or a lock) ends the screensaver.
    Connections {
        target: Session
        function onSnapshotChanged() {
            if (screensaver.active && (Session.locked || Session.target.kind !== "shell")) screensaver.active = false
        }
    }
}
