// First-run setup: shown once on a fresh install (state.onboarding.completed
// is false when the shell first gets a snapshot; ShellRoot opens it), and
// again from Settings → About → Run setup again. Six steps with progress
// dots; every step has Skip, and Back returns to the step you came from (on
// the first step Back leaves setup, which then shows again next start):
//   0 Welcome      the bear mark, what Bear Den is, how to move
//   1 Your look    Pixel or Classic, then the theme cards (ThemePicker:
//                  ◀ ▶ tries a theme on the whole TV, OK uses it)
//   2 Your apps    what is installed, and one-press installs of the rest
//                  (the install card shows the honest size and installs only
//                  on its own Install press), incl. "Streaming sites"
//                  (turns Netflix, Disney+ and Hulu on and offers their
//                  browser; up to 720p)
//   3 Phone remote plain-language consent, the network to allow it on
//                  (Shell.configureRemote, as RemoteSetupScreen does), then
//                  pairing the first phone (PairingScreen)
//   4 Extras       Plex sign-in (PlexScreen), weather (WeatherScreen) and
//                  "Start Bear Den when this PC starts" (Shell.setAutostart
//                  → IPC autostart.configure, state.autostart)
//   5 Done         Go Home: Shell.completeOnboarding() → IPC
//                  onboarding.complete (config onboarding.completed: true)
// Nothing is installed, enabled or stored without an OK on that thing. No
// motion of its own (reduced motion changes nothing here). Reports focus as
// section `onboarding-<step>`; the contract's screen name is `setup`.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    objectName: "onboardingScreen"
    property bool active: false
    property int step: 0
    property var history: []
    // Per step: which line has focus and the column within it.
    property int line: 0
    property int column: 0
    property var interfaces: []
    signal openScreen(string name)
    // forApp: opened for that app itself (InstallCard.forApp).
    signal openInstall(string appId, bool forApp)
    signal finished()

    readonly property var stepNames: [qsTr("Welcome"), qsTr("Your look"), qsTr("Your apps"), qsTr("Phone remote"), qsTr("Extras"), qsTr("Done")]
    readonly property int lastStep: stepNames.length - 1

    // Step 1: Pixel or Classic.
    // (Session.layout is read so this follows every snapshot.)
    readonly property var persistedUi: Session.layout && (Session.layoutForEdit().ui || ({}))
    readonly property string artStyle: persistedUi.art_style === "classic" ? "classic" : "pixel"

    // Step 2: apps (from the snapshot, never display names).
    readonly property var installedApps: Session.applications.filter(a => a.installed === true && a.hidden !== true)
    readonly property var addable: {
        const out = []
        const seen = {}
        for (const a of Session.applications) {
            if (a.installed === true || a.install === undefined || Apps.browserOf(a.adapter) !== null) continue
            const key = Shell.flatpakIdFor(a.adapter) || a.id
            if (seen[key]) continue
            seen[key] = true
            out.push({ kind: "app", app: a })
        }
        // The streaming sites, as one card: turns them on and offers their browser.
        const sites = Session.applications.filter(a => a.enabled !== undefined && Apps.isStreamingSite(a.adapter))
        if (sites.length > 0) out.push({ kind: "streaming", sites: sites })
        return out
    }
    readonly property var installCap: Session.capabilities["app.install"] || ({})
    function streamingLine(sites) {
        const off = sites.filter(s => s.enabled !== true)
        const missing = sites.filter(s => s.installed === false)
        const browser = sites.length > 0 ? Apps.browserLabel(sites[0].adapter) : ""
        if (off.length === 0 && missing.length === 0) return qsTr("On · up to 720p")
        if (missing.length > 0) return qsTr("Installs %1 · up to 720p").arg(browser)
        return qsTr("Turns them on · up to 720p")
    }
    function appLine(app) {
        switch (app.install.state) {
        case "preparing": return qsTr("Getting ready…")
        case "downloading": return qsTr("Installing %1%").arg(app.install.progress)
        case "installing": return qsTr("Finishing…")
        case "done": return qsTr("Ready")
        case "failed": return qsTr("Try again")
        case "available": return app.install.size_bytes > 0 ? qsTr("Install · %1").arg(sizeText(app.install.size_bytes)) : qsTr("Install")
        }
        return qsTr("Can't install")
    }
    // A download size as people say it (the install card shows the details).
    function sizeText(bytes) {
        return bytes >= 1e9 ? qsTr("%1 GB").arg((bytes / 1e9).toFixed(1)) : qsTr("%1 MB").arg(Math.max(1, Math.round(bytes / 1e6)))
    }

    // Step 3: the phone remote.
    readonly property bool remoteOn: Session.remote.enabled === true
    readonly property var remoteButtons: remoteOn
        ? [{ id: "pair", text: qsTr("Pair a phone"), primary: true }, { id: "next", text: qsTr("Next") }]
        : interfaces.map(i => ({ id: "on:" + i.name, text: qsTr("Allow on %1").arg(i.label), detail: i.addresses + " · " + i.name, primary: true }))
                    .concat([{ id: "skip", text: qsTr("Skip") }])

    // Step 4: extras.
    readonly property var autostart: Session.autostart || ({})
    readonly property var extras: [
        { id: "plex", kind: "link", icon: "play", label: qsTr("Sign in to Plex"),
          description: qsTr("Continue Watching and Recently Added on Home"),
          value: Session.plex.status === "connected" ? (Session.plex.server || qsTr("On")) : Session.plex.status === undefined ? qsTr("Not available") : "" },
        { id: "weather", kind: "link", icon: "home", label: qsTr("Weather"),
          description: qsTr("Choose your town for the weather by the clock"),
          value: Session.weather.status === undefined || Session.weather.status === "disabled" ? qsTr("Off") : (Session.weather.place || qsTr("On")) },
        { id: "autostart", kind: "toggle", icon: "power", label: qsTr("Start Bear Den when this PC starts"),
          description: autostart.available === false ? (autostart.reason || qsTr("Not available here"))
                     : autostart.enabled === undefined ? qsTr("Not available in this version of Bear Den") : qsTr("Opens Bear Den when you log in"),
          value: autostart.enabled === true ? "on" : "off" }
    ]

    // The focusable lines of the current step; each is a row of `count` things.
    readonly property var lines: {
        switch (step) {
        case 0: return [{ id: "buttons", count: 2 }]
        case 1: return [{ id: "art", count: 2 }, { id: "themes", count: Themes.list.length }, { id: "buttons", count: 2 }]
        case 2: return (addable.length > 0 ? [{ id: "add", count: addable.length }] : []).concat([{ id: "buttons", count: 2 }])
        case 3: return remoteButtons.map((b, i) => ({ id: "remote-" + i, count: 1 }))
        case 4: return extras.map(e => ({ id: e.id, count: 1 })).concat([{ id: "buttons", count: 2 }])
        case 5: return [{ id: "buttons", count: 1 }]
        }
        return []
    }
    readonly property string lineId: lines[Math.max(0, Math.min(lines.length - 1, line))] ? lines[Math.max(0, Math.min(lines.length - 1, line))].id : ""
    // The buttons of the step (the "buttons" line).
    readonly property var buttons: {
        switch (step) {
        case 0: return [{ id: "start", text: qsTr("Let's set up"), primary: true }, { id: "skip-all", text: qsTr("Skip setup") }]
        case 5: return [{ id: "finish", text: qsTr("Go Home"), primary: true }]
        }
        return [{ id: "next", text: qsTr("Next"), primary: true }, { id: "skip", text: qsTr("Skip") }]
    }

    // Opened fresh (first start, or Run setup again): from the welcome.
    function opened() { step = 0; history = []; line = 0; column = 0 }
    function enter() {
        if (step === 3) interfaces = Shell.lanInterfaces()
        if (step === 1) picker.reset()
        report()
    }
    function leave() { picker.cancel() }
    function itemId() {
        switch (lineId) {
        case "buttons": return buttons[Math.min(column, buttons.length - 1)].id
        case "art": return column === 0 ? "pixel" : "classic"
        case "themes": return "theme:" + picker.focusedId
        case "add": {
            const e = addable[Math.min(column, addable.length - 1)]
            return e.kind === "streaming" ? "streaming" : e.app.id
        }
        }
        if (lineId.startsWith("remote-")) return remoteButtons[line] ? remoteButtons[line].id : "none"
        return lineId
    }
    function report() { Nav.reportFocus("onboarding-" + step, itemId(), 0) }

    function goTo(next) {
        if (step === 1) picker.cancel()
        history = history.concat([step])
        showStep(next)
    }
    function showStep(s) {
        step = s
        line = 0
        column = s === 1 && artStyle === "classic" ? 1 : 0   // Your look starts on the art style in use
        if (s === 1) picker.reset()
        if (s === 3) interfaces = Shell.lanInterfaces()
        report()
    }
    function back() {
        if (history.length === 0) return false   // leave setup (it shows again next start)
        if (step === 1) picker.cancel()
        const prev = history[history.length - 1]
        history = history.slice(0, history.length - 1)
        showStep(prev)
        return true
    }
    function editUi(mutate) {
        const layout = JSON.parse(JSON.stringify(Shell.layoutForEdit()))
        if (!layout.ui) return
        mutate(layout.ui)
        Shell.updateLayout(layout)
    }
    function activate() {
        switch (lineId) {
        case "buttons": {
            const b = buttons[Math.min(column, buttons.length - 1)]
            if (b.id === "start" || b.id === "next" || b.id === "skip") goTo(step + 1)
            else if (b.id === "skip-all") goTo(lastStep)
            else if (b.id === "finish") { Shell.completeOnboarding(); finished() }
            return
        }
        case "art": {
            const style = column === 0 ? "pixel" : "classic"
            if (style !== artStyle) editUi(u => u.art_style = style)
            return
        }
        case "themes": picker.apply(); return
        case "add": {
            const e = addable[Math.min(column, addable.length - 1)]
            if (e.kind === "app") { openInstall(e.app.id, true); return }
            // Streaming sites: the press turns them on; a missing browser
            // opens its install card (installed only on its Install press).
            for (const s of e.sites)
                if (s.enabled !== true) Shell.setAppEnabled(s.id, true)
            const missing = e.sites.filter(s => s.installed === false)
            if (missing.length > 0) openInstall(missing[0].id, true)
            return
        }
        case "plex": openScreen("plex"); return
        case "weather": openScreen("weather"); return
        case "autostart":
            if (autostart.available === true) Shell.setAutostart(autostart.enabled !== true)
            return
        }
        if (lineId.startsWith("remote-")) {
            const b = remoteButtons[line]
            if (!b) return
            if (b.id === "pair") openScreen("pairing")
            else if (b.id === "next" || b.id === "skip") goTo(step + 1)
            else if (b.id.startsWith("on:")) { Shell.configureRemote(true, b.id.slice(3)); line = 0 }
        }
    }
    function navigate(action) {
        const l = lines[line] || { count: 1 }
        switch (action) {
        case "nav.up":
        case "nav.down": {
            const next = Math.max(0, Math.min(lines.length - 1, line + (action === "nav.up" ? -1 : 1)))
            if (next !== line) {
                if (lineId === "themes") { picker.cancel(); picker.reset() }
                line = next
                column = lineId === "themes" ? picker.index : lineId === "art" ? (artStyle === "classic" ? 1 : 0) : 0
            }
            report()
            return true
        }
        case "nav.left":
        case "nav.right": {
            const d = action === "nav.left" ? -1 : 1
            if (lineId === "themes") { picker.move(d); column = picker.index }
            else column = Math.max(0, Math.min(l.count - 1, column + d))
            report()
            return true
        }
        case "select": activate(); report(); return true
        case "back": return back()
        }
        return false
    }
    onRemoteButtonsChanged: if (active && step === 3 && line >= remoteButtons.length) { line = 0; report() }
    onAddableChanged: if (active && step === 2 && lineId === "add" && column >= addable.length) { column = Math.max(0, addable.length - 1); report() }

    // --- Drawing ------------------------------------------------------------
    ScreenFrame {
        anchors.fill: parent
        title: root.stepNames[root.step]
        subtitle: root.step === 0 ? qsTr("Setting up Bear Den")
                : root.step === root.lastStep ? qsTr("Step %1 of %2").arg(root.step + 1).arg(root.stepNames.length)
                : qsTr("Step %1 of %2 · Skip anything you don't need").arg(root.step + 1).arg(root.stepNames.length)
        hints: [["◀ ▶ ▲ ▼", qsTr("Move")], ["OK", qsTr("Choose")], ["Back", root.step === 0 ? qsTr("Later") : qsTr("Back")]]

        // A calm card behind the step, so text reads over any world.
        PixelBox {
            anchors.fill: parent
            anchors.bottomMargin: 128 * Theme.scale
            radius: 24 * Theme.scale
            color: Theme.surface
            borderColor: Theme.surfaceBorder
            borderWidth: 1
            opacity: 0.94
        }
        Item {
            id: stepArea
            anchors { fill: parent; margins: 36 * Theme.scale; bottomMargin: 164 * Theme.scale }
        }
        // Progress dots, above the step's buttons.
        Row {
            objectName: "onboardingDots"
            anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: 98 * Theme.scale }
            spacing: 14 * Theme.scale
            Repeater {
                model: root.stepNames.length
                PixelBox {
                    required property int index
                    width: (index === root.step ? 44 : 18) * Theme.scale
                    height: 18 * Theme.scale
                    radius: height / 2
                    color: index === root.step ? Theme.accent : index < root.step ? Theme.alpha(Theme.accent, 0.55) : Theme.alpha("#ffffff", 0.22)
                }
            }
        }

        // 0 Welcome
        Column {
            visible: root.step === 0
            parent: stepArea
            anchors.centerIn: parent
            anchors.verticalCenterOffset: -40 * Theme.scale
            width: parent.width * 0.7
            spacing: 26 * Theme.scale
            BearMark { size: 190 * Theme.scale; anchors.horizontalCenter: parent.horizontalCenter }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WordWrap
                text: qsTr("Bear Den turns this PC into a cosy TV you drive with your phone.")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 44 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WordWrap
                text: qsTr("Move with the arrow keys, choose with OK and go back with Back, on a keyboard or a TV remote. Pairing a phone comes later.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
            }
            KeyHints {
                anchors.horizontalCenter: parent.horizontalCenter
                hints: [["◀ ▶ ▲ ▼", qsTr("Move")], ["OK", qsTr("Choose")], ["Back", qsTr("Go back")]]
            }
        }

        // 1 Your look
        Column {
            visible: root.step === 1
            parent: stepArea
            width: parent.width
            spacing: 18 * Theme.scale
            Text {
                text: qsTr("Art style")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.DemiBold
                x: 20 * Theme.scale
            }
            Row {
                x: 20 * Theme.scale
                spacing: 22 * Theme.scale
                Repeater {
                    model: [{ id: "pixel", label: qsTr("Pixel"), detail: qsTr("Everything drawn in pixel art") },
                            { id: "classic", label: qsTr("Classic"), detail: qsTr("Smooth drawings and pictures") }]
                    FocusButton {
                        required property var modelData
                        required property int index
                        objectName: "artChoice"
                        width: 380 * Theme.scale
                        text: modelData.label + (root.artStyle === modelData.id ? "  ✓" : "")
                        detail: modelData.detail
                        primary: root.artStyle === modelData.id
                        focused: root.lineId === "art" && root.column === index
                    }
                }
            }
            Text {
                text: qsTr("Theme: ◀ ▶ tries it on, OK uses it")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.DemiBold
                x: 20 * Theme.scale
            }
            ThemePicker {
                id: picker
                width: parent.width
                active: root.step === 1 && root.lineId === "themes"
            }
        }

        // 2 Your apps
        Column {
            visible: root.step === 2
            parent: stepArea
            width: parent.width
            spacing: 18 * Theme.scale
            Text {
                text: root.installedApps.length > 0 ? qsTr("On this TV") : qsTr("No apps are installed yet")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.DemiBold
                x: 20 * Theme.scale
            }
            Row {
                x: 20 * Theme.scale
                spacing: 18 * Theme.scale
                Repeater {
                    model: root.installedApps
                    Column {
                        required property var modelData
                        spacing: 6 * Theme.scale
                        AppIcon { size: 72 * Theme.scale; adapter: parent.modelData.adapter; label: parent.modelData.label; anchors.horizontalCenter: parent.horizontalCenter }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: parent.modelData.label
                            color: Theme.textSecondary
                            font.family: Theme.fontFamily
                            font.pixelSize: 20 * Theme.fontUnit
                        }
                    }
                }
            }
            Text {
                text: qsTr("Add with one press")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.DemiBold
                x: 20 * Theme.scale
                topPadding: 10 * Theme.scale
            }
            Text {
                visible: root.addable.length === 0
                x: 20 * Theme.scale
                width: parent.width * 0.7
                wrapMode: Text.WordWrap
                text: root.installCap.available === false && root.installCap.reason ? root.installCap.reason : qsTr("Every app Bear Den knows is installed.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
            }
            Row {
                x: 20 * Theme.scale
                spacing: 22 * Theme.scale
                Repeater {
                    model: root.addable
                    PixelBox {
                        id: addCard
                        required property var modelData
                        required property int index
                        readonly property bool focused: root.lineId === "add" && root.column === index
                        readonly property bool streaming: modelData.kind === "streaming"
                        objectName: "onboardingAddCard"
                        width: 340 * Theme.scale
                        height: 200 * Theme.scale
                        radius: 18 * Theme.scale
                        color: focused ? Theme.surfaceRaised : Theme.surface
                        borderColor: Theme.surfaceBorder
                        borderWidth: 1
                        scale: focused ? 1.04 : 1
                        Behavior on scale { NumberAnimation { duration: Theme.durationFast } }
                        AppIcon {
                            visible: !addCard.streaming
                            anchors { left: parent.left; top: parent.top; margins: 20 * Theme.scale }
                            size: 72 * Theme.scale
                            installed: false
                            adapter: addCard.streaming ? "" : addCard.modelData.app.adapter
                            label: addCard.streaming ? "" : addCard.modelData.app.label
                        }
                        UiIcon {
                            visible: addCard.streaming
                            anchors { left: parent.left; top: parent.top; margins: 20 * Theme.scale }
                            name: "globe"
                            size: 72 * Theme.scale
                        }
                        Column {
                            anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: 20 * Theme.scale }
                            spacing: 2 * Theme.scale
                            Text {
                                width: parent.width
                                elide: Text.ElideRight
                                text: addCard.streaming ? qsTr("Streaming sites") : addCard.modelData.app.label
                                color: Theme.textPrimary
                                font.family: Theme.fontFamily
                                font.pixelSize: 26 * Theme.fontUnit
                                font.weight: Font.DemiBold
                            }
                            Text {
                                visible: addCard.streaming
                                width: parent.width
                                elide: Text.ElideRight
                                text: qsTr("Netflix, Disney+, Hulu")
                                color: Theme.textSecondary
                                font.family: Theme.fontFamily
                                font.pixelSize: 20 * Theme.fontUnit
                            }
                            Text {
                                objectName: "onboardingAddLine"
                                width: parent.width
                                wrapMode: Text.WordWrap
                                maximumLineCount: 2
                                text: addCard.streaming ? root.streamingLine(addCard.modelData.sites) : root.appLine(addCard.modelData.app)
                                color: Theme.accent
                                font.family: Theme.fontFamily
                                font.pixelSize: 20 * Theme.fontUnit
                                font.weight: Font.DemiBold
                            }
                        }
                        FocusFrame { shown: addCard.focused; cornerRadius: addCard.radius }
                    }
                }
            }
            HelpPanel {
                x: 20 * Theme.scale
                width: parent.width * 0.62
                readonly property var focused: root.lineId === "add" ? root.addable[Math.min(root.column, root.addable.length - 1)] : null
                visible: focused !== null
                title: !focused ? "" : focused.kind === "streaming" ? qsTr("Streaming sites") : focused.app.label
                help: !focused ? "" : focused.kind === "streaming"
                      ? qsTr("OK turns Netflix, Disney+ and Hulu on and offers the browser they need. In a Linux browser they play at up to 720p; sign in with your own account.")
                      : qsTr("OK shows its size first; it installs only when you press Install there. From Flathub, for this user.")
                notes: !focused ? [] : focused.kind === "streaming" ? (focused.sites[0].notes || []) : (focused.app.notes || [])
            }
        }

        // 3 Phone remote
        Row {
            visible: root.step === 3
            parent: stepArea
            anchors.fill: parent
            spacing: 64 * Theme.scale
            Column {
                width: parent.width * 0.5
                spacing: 18 * Theme.scale
                Text {
                    width: parent.width
                    wrapMode: Text.WordWrap
                    text: root.remoteOn ? qsTr("The phone remote is on.") : qsTr("Use your phone as the remote?")
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 34 * Theme.fontUnit
                    font.weight: Font.DemiBold
                }
                Text {
                    objectName: "remoteConsent"
                    width: parent.width
                    wrapMode: Text.WordWrap
                    lineHeight: 1.2
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 25 * Theme.fontUnit
                    text: qsTr("• Your phone's browser becomes the remote: arrows, OK, Back, volume and your apps.\n• Only phones you pair here, with a code shown on this TV, can control it.\n• The phone talks to this PC directly on your home network; nothing goes through the internet. It is not encrypted, so use it on a network you trust.\n• You can turn it off any time: Settings → Phones & remote.")
                }
                Text {
                    visible: root.remoteOn
                    width: parent.width
                    wrapMode: Text.WordWrap
                    text: (Session.remote.addresses || []).length > 0 ? qsTr("Listening at %1").arg((Session.remote.addresses || []).join(", ")) : qsTr("Turned on; starting…")
                    color: Theme.accent
                    font.family: Theme.monoFamily
                    font.pixelSize: 26 * Theme.fontUnit
                }
                Text {
                    visible: !root.remoteOn && root.interfaces.length === 0
                    width: parent.width
                    wrapMode: Text.WordWrap
                    color: Theme.warning
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                    text: qsTr("No home network connection was found. Skip for now and turn it on later in Settings.")
                }
            }
            Column {
                width: parent.width * 0.4
                spacing: 18 * Theme.scale
                Text {
                    visible: !root.remoteOn && root.interfaces.length > 0
                    width: parent.width
                    wrapMode: Text.WordWrap
                    text: qsTr("Allow it on:")
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 26 * Theme.fontUnit
                }
                Repeater {
                    model: root.remoteButtons
                    FocusButton {
                        required property var modelData
                        required property int index
                        objectName: "onboardingRemoteButton"
                        width: parent.width
                        text: modelData.text
                        detail: modelData.detail || ""
                        primary: modelData.primary === true
                        focused: root.step === 3 && root.line === index
                    }
                }
                Text {
                    visible: root.remoteOn
                    width: parent.width
                    wrapMode: Text.WordWrap
                    readonly property int phones: Session.remote.paired_device_count || 0
                    text: phones === 0 ? qsTr("No phone paired yet.") : phones === 1 ? qsTr("1 phone paired.") : qsTr("%1 phones paired.").arg(phones)
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                }
            }
        }

        // 4 Extras
        Column {
            visible: root.step === 4
            parent: stepArea
            width: parent.width * 0.62
            x: 20 * Theme.scale
            spacing: 14 * Theme.scale
            Repeater {
                model: root.extras
                SettingsRow {
                    required property var modelData
                    required property int index
                    objectName: "onboardingExtra"
                    width: parent.width
                    kind: modelData.kind
                    icon: modelData.icon
                    label: modelData.label
                    description: modelData.description
                    value: modelData.value
                    focused: root.step === 4 && root.line === index
                }
            }
        }
        HelpPanel {
            visible: root.step === 4 && root.line < root.extras.length
            parent: stepArea
            anchors { right: parent.right; rightMargin: 12 * Theme.scale; top: parent.top }
            width: parent.width * 0.32
            title: root.step === 4 && root.extras[root.line] ? root.extras[root.line].label : ""
            help: root.step === 4 && root.extras[root.line] ? Help.text(root.extras[root.line].id) : ""
        }

        // 5 Done
        Column {
            visible: root.step === 5
            parent: stepArea
            anchors.centerIn: parent
            anchors.verticalCenterOffset: -40 * Theme.scale
            width: parent.width * 0.7
            spacing: 24 * Theme.scale
            BearMark { size: 150 * Theme.scale; anchors.horizontalCenter: parent.horizontalCenter }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: qsTr("You're all set")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 48 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                objectName: "onboardingSummary"
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WordWrap
                lineHeight: 1.2
                text: qsTr("Theme: %1 · %2 · Phone remote %3")
                        .arg(Themes.get(Themes.canonical(root.persistedUi.background || "")).name || "")
                        .arg(root.installedApps.length === 1 ? qsTr("1 app on this TV") : qsTr("%1 apps on this TV").arg(root.installedApps.length))
                        .arg(root.remoteOn ? qsTr("on") : qsTr("off"))
                      + "\n" + qsTr("Change any of it later in Apps, Themes and Settings. Settings → About → Run setup again brings this back.")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 28 * Theme.fontUnit
            }
        }

        // The step's buttons.
        Row {
            visible: root.lines.some(l => l.id === "buttons")
            anchors { bottom: parent.bottom; horizontalCenter: parent.horizontalCenter }
            spacing: 24 * Theme.scale
            Repeater {
                model: root.buttons
                FocusButton {
                    required property var modelData
                    required property int index
                    objectName: "onboardingButton"
                    text: modelData.text
                    primary: modelData.primary === true
                    focused: root.lineId === "buttons" && root.column === index
                }
            }
        }
    }
}
