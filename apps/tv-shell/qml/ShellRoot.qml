// The shell's focus graph. Every key — physical keyboard, or remote input the
// coordinator injects through Nav — arrives here as a named action and goes
// to exactly one owner: the topmost dialog, the launch overlay, or the
// current screen. Back pops one level; at Home it is a reported no-op.
// While Bear Den turned the display off, or the sleep timer is in its last
// minute (state.power), a key only wakes the TV or keeps it awake: it is
// swallowed and reported as IPC power.activity (SleepWarning shows the
// warning). Remote input never lands here in that state: the coordinator
// wakes or cancels first and its new state reaches the shell before the input.
// An app the owner chose to install (InstallCard) opens by itself when its
// install is done, if the owner is still on its card or its Home tile.
// First run: when the first snapshot says state.onboarding.completed is
// false, the setup (OnboardingScreen) opens over Home. The old screen names
// `add-apps` and `streaming` open the Apps page at that section, and
// `onboarding-<step>` opens the setup at a step (sandbox screenshots).

import QtQuick
import BearDen

FocusScope {
    id: root
    objectName: "shellRoot"
    focus: true
    property var stack: ["home"]
    readonly property string screen: stack[stack.length - 1]
    readonly property var screens: ({
        "home": home, "settings": settings, "remote-setup": remoteSetup,
        "pairing": pairing, "devices": devices, "diagnostics": diagnostics, "playback": playback,
        "advanced-playback": advancedPlayback, "weather": weather, "plex": plex, "badges": badges,
        "apps": apps, "themes": themes, "onboarding": onboarding
    })
    // Names that land on a section of another screen.
    readonly property var aliases: ({ "add-apps": ["apps", "add-apps"], "streaming": ["apps", "streaming"] })
    readonly property var topDialog: confirmDialog.visible ? confirmDialog
                                   : messageDialog.visible ? messageDialog
                                   : installCard.visible ? installCard
                                   : null
    readonly property bool blocked: !Session.loaded || Session.locked

    function navScreenName() {
        if (topDialog) return "dialog"
        if (screen === "remote-setup" || screen === "onboarding") return "setup"
        if (screen === "playback" || screen === "advanced-playback") return "diagnostics"   // the contract's screen names
        if (screen === "weather" || screen === "plex" || screen === "badges" || screen === "apps" || screen === "themes") return "settings"
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
        const alias = aliases[name]
        if (alias) {
            open(alias[0])
            if (screen === alias[0]) current().focusSection(alias[1])
            return
        }
        const step = /^onboarding-([0-9])$/.exec(name)
        if (step) {
            open("onboarding")
            if (screen === "onboarding") onboarding.showStep(Math.min(Number(step[1]), onboarding.lastStep))
            return
        }
        if (!screens[name] || name === screen) return
        if (current().leave) current().leave()
        stack = stack.concat([name])
        syncNav()
        // A fresh visit (not a return from a page it opened) starts at the top.
        if (current().opened) current().opened()
        current().enter()
    }
    // First run: open the setup once, when the first snapshot that carries
    // state.onboarding says it is not completed (an older coordinator sends
    // none: no setup). Later snapshots never reopen it.
    property bool onboardingChecked: false
    function checkOnboarding() {
        if (onboardingChecked || !Session.loaded || Session.onboarding === undefined || Session.onboarding.completed === undefined) return
        onboardingChecked = true
        if (Session.onboarding.completed === false && screen === "home" && !blocked) open("onboarding")
    }
    function pop() {
        if (stack.length <= 1) return false
        if (current().leave) current().leave()
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
        installCard.visible = false
        launchOverlay.dismissed = true
        if (current().leave) current().leave()
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
        if (Session.power.display === "off" || Session.power.warning === true) {
            // Like a TV: the press that wakes it does nothing else.
            Shell.powerActivity()
            screensaver.active = false
            wake()
            if (action === "back") Nav.noteAtRoot()
            return
        }
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
        Shell.achievementEvent("parade") // Parade Spotter (Den badges)
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

    // The app whose Install the owner pressed: it opens by itself once
    // installed, if the owner is still on its card or on its Home tile;
    // otherwise its tile is simply ready. A failure or a cancel forgets it.
    property string pendingOpen: ""
    property bool pendingStarted: false   // a snapshot showed the install running
    onPendingOpenChanged: pendingStarted = false
    function openWhenInstalled() {
        if (pendingOpen.length === 0) return
        const app = Session.application(pendingOpen)
        if (app.installed !== true) {
            const st = app.install ? app.install.state : ""
            if (st === "preparing" || st === "downloading" || st === "installing") pendingStarted = true
            else if (st === "failed" || (pendingStarted && st === "available")) pendingOpen = "" // failed or cancelled
            return
        }
        const id = pendingOpen
        pendingOpen = ""
        const onCard = installCard.visible && installCard.appId === id
        const onTile = !topDialog && screen === "home" && home.focusedAppId() === id && !screensaver.active
        if (onCard) installCard.close()
        if (onCard || onTile) Shell.launchApp(id)
    }
    Connections {
        target: Session
        function onSnapshotChanged() { root.openWhenInstalled() }
    }

    Component.onCompleted: {
        syncNav()
        if (Shell.startScreen !== "home") Qt.callLater(() => open(Shell.startScreen))
    }
    property bool _wasLoaded: false
    Connections {
        target: Session
        function onSnapshotChanged() {
            if (Session.loaded && !root._wasLoaded) { root._wasLoaded = true; if (root.screen === "home") home.restoreFocus() }
            root.checkOnboarding()
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
            onAppUnavailable: (app) => installCard.openFor(app.id)
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
        AppsScreen {
            id: apps
            width: parent.width; height: parent.height
            active: root.screen === "apps" && !root.topDialog
            opacity: root.screen === "apps" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "apps" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenInstall: (appId) => installCard.openFor(appId)
        }
        ThemesScreen {
            id: themes
            width: parent.width; height: parent.height
            active: root.screen === "themes" && !root.topDialog
            opacity: root.screen === "themes" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "themes" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
        }
        OnboardingScreen {
            id: onboarding
            width: parent.width; height: parent.height
            active: root.screen === "onboarding" && !root.topDialog
            opacity: root.screen === "onboarding" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "onboarding" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onOpenScreen: (name) => root.open(name)
            onOpenInstall: (appId) => installCard.openFor(appId)
            onFinished: root.goHome()
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
        PlexScreen {
            id: plex
            width: parent.width; height: parent.height
            active: root.screen === "plex" && !root.topDialog
            opacity: root.screen === "plex" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "plex" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onConfirm: (title, body, label, accept) => confirmDialog.open({ title: title, body: body, confirmLabel: label, onAccept: accept })
        }
        BadgesScreen {
            id: badges
            width: parent.width; height: parent.height
            active: root.screen === "badges" && !root.topDialog
            opacity: root.screen === "badges" ? 1 : 0
            visible: opacity > 0
            y: root.screen === "badges" ? 0 : 24 * Theme.scale
            Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
            Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }
            onConfirm: (title, body, label, accept) => confirmDialog.open({ title: title, body: body, confirmLabel: label, onAccept: accept })
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
    // A Den badge earned: celebrated on Home, or queued until Home appears.
    BadgeCelebration {
        id: badgeCelebration
        objectName: "badgeCelebrationLayer"
        anchors.fill: parent
        allowed: root.screen === "home" && Session.target.kind === "shell" && !root.topDialog
                 && !launchOverlay.visible && !root.blocked && !screensaver.active
    }
    LaunchOverlay { id: launchOverlay; anchors.fill: parent }
    InstallCard {
        id: installCard
        anchors.fill: parent
        onInstallRequested: (appId) => root.pendingOpen = appId
    }
    MessageDialog { id: messageDialog; anchors.fill: parent }
    ConfirmDialog { id: confirmDialog; anchors.fill: parent }
    SleepWarning {
        anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: Math.max(Theme.safeY, 24 * Theme.scale) + 72 * Theme.scale }
    }
    LockedScreen { anchors.fill: parent; visible: Session.loaded && Session.locked }
    Screensaver { id: screensaver; anchors.fill: parent; onActiveChanged: Theme.screensaver = active }
    // An app coming to the front, a lock or the sleep warning ends the screensaver.
    Connections {
        target: Session
        function onSnapshotChanged() {
            if (screensaver.active && (Session.locked || Session.target.kind !== "shell" || Session.power.warning === true)) screensaver.active = false
        }
    }
}
