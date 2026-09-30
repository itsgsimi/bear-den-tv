// Settings → Plex: sign the TV in to Plex for the Home rows (state.plex in
// contracts/state.schema.json; plex.* in contracts/ipc.md). One screen that
// follows state.plex.status:
//   signed_out / error  what signing in does, and Sign in
//   linking             the code, plex.tv/link and its QR code; Cancel
//   choose_server       the account's servers; OK picks one
//   choose_libraries    a toggle per library (movies and shows proposed); Done
//   connected           the server, the libraries, how the rows are; Sign out
// Leaving the screen mid-flow cancels it (nothing is stored). The token never
// reaches the shell; the coordinator keeps it in the desktop keyring, or when
// that is locked in a private file (state.plex.stored_in, storedLine).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    // Set by ShellRoot: this screen is the one in front (focus reports only then).
    property bool active: false
    signal confirm(string title, string body, string confirmLabel, var onAccept)

    readonly property var plex: Session.plex
    readonly property string status: plex.status || ""
    readonly property var servers: plex.servers || []
    readonly property var libraries: plex.libraries || []
    readonly property bool inFlow: status === "linking" || status === "choose_server" || status === "choose_libraries"
    property bool busy: false
    property string hint: ""
    // choose_libraries: the owner's changes to the proposal (state.plex
    // libraries[].selected), by library id.
    property var picks: ({})
    // Where the sign-in is kept, in plain words (state.plex.stored_in).
    readonly property string storedLine: plex.stored_in === "file"
        ? qsTr("Your Plex sign-in is kept in a private file on this TV (only your user can read it), like Plex HTPC keeps its own. Sign out deletes it.")
        : plex.stored_in === "keyring"
            ? qsTr("Your Plex sign-in is kept in the desktop keyring. Sign out deletes it.")
            : ""
    function picked(lib) { return picks[lib.id] !== undefined ? picks[lib.id] : lib.selected === true }

    onStatusChanged: {
        busy = false
        focusIndex = 0
        picks = ({})
        if (active) Qt.callLater(report) // after `items` follows the new status
    }
    onItemsChanged: if (active) report()

    // The focusable things, top to bottom, for the current status.
    readonly property var items: {
        switch (status) {
        case "linking": return [{ id: "cancel" }]
        case "choose_server": return servers.map(s => ({ id: "server-" + s.id, server: s })).concat([{ id: "cancel" }])
        case "choose_libraries": return libraries.map(l => ({ id: "library-" + l.id, library: l })).concat([{ id: "done" }, { id: "cancel" }])
        case "connected": return [{ id: "sign-out" }]
        case "": return []
        }
        return [{ id: "sign-in" }]
    }
    function itemAt(i) { return items[Math.max(0, Math.min(items.length - 1, i))] || { id: "" } }

    function enter() { hint = ""; busy = false; focusIndex = 0; picks = ({}); report() }
    function leave() { if (inFlow) Shell.plexCancel() }
    function report() { Nav.reportFocus("plex", itemAt(focusIndex).id, 0) }
    function move(to) { focusIndex = Math.max(0, Math.min(items.length - 1, to)); report() }

    function pickedIds() { return libraries.filter(l => picked(l)).map(l => l.id) }
    function activate() {
        const it = itemAt(focusIndex)
        if (it.id === "sign-in") { busy = true; hint = ""; Shell.plexSignIn() }
        else if (it.id === "cancel") { hint = ""; Shell.plexCancel() }
        else if (it.server) { busy = true; hint = ""; Shell.plexChooseServer(it.server.id) }
        else if (it.library) {
            const p = Object.assign({}, picks)
            p[it.library.id] = !picked(it.library)
            picks = p
        } else if (it.id === "done") {
            const ids = pickedIds()
            if (ids.length === 0) { hint = qsTr("Choose at least one library."); return }
            busy = true; hint = ""
            Shell.plexChooseLibraries(ids)
        } else if (it.id === "sign-out") {
            confirm(qsTr("Sign out of Plex?"),
                    qsTr("The Home rows and their pictures go, and this TV deletes its Plex sign-in. Plex HTPC keeps its own sign-in."),
                    qsTr("Sign out"), () => { root.busy = true; Shell.plexSignOut() })
        }
    }
    Connections {
        target: Shell
        function onPlexReplied(type, ok, error) {
            root.busy = false
            root.hint = ok ? "" : error
        }
    }

    function navigate(action) {
        switch (action) {
        case "nav.up": move(focusIndex - 1); return true
        case "nav.down": move(focusIndex + 1); return true
        case "nav.left": case "nav.right": return true
        case "select": activate(); return true
        }
        return false
    }

    function kindLabel(k) {
        switch (k) {
        case "movie": return qsTr("Movies")
        case "show": return qsTr("TV shows")
        case "artist": return qsTr("Music")
        case "photo": return qsTr("Photos")
        }
        return qsTr("Other")
    }
    readonly property var content: Session.content
    readonly property string rowsLine: {
        if (status !== "connected") return ""
        switch (content.status) {
        case "ready": return qsTr("Continue Watching and Recently Added are on Home.")
        case "connecting": case undefined: return qsTr("The rows load when you go Home.")
        case "stale": return qsTr("Showing the last rows: %1").arg(content.message || "")
        case "error": return content.message || qsTr("The rows could not be loaded.")
        }
        return ""
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Plex")
        subtitle: root.status === "connected" ? qsTr("Your Plex rows on Home")
                : root.status === "linking" ? qsTr("Link this TV to your Plex account")
                : qsTr("Continue Watching and Recently Added on Home")
        hints: [["▲ ▼", qsTr("Move")], ["OK", qsTr("Select")], ["Back", root.inFlow ? qsTr("Cancel") : qsTr("Back")]]

        // A surface behind whichever part is showing, so text stays readable
        // over the theme's wallpaper.
        PixelBox {
            readonly property Item shown: signedOut.visible ? signedOut : linking.visible ? linking
                                          : choose.visible ? choose : connected.visible ? connected : null
            visible: shown !== null
            x: shown ? shown.x - 36 * Theme.scale : 0
            y: shown ? shown.y - 32 * Theme.scale : 0
            width: shown ? shown.width + 72 * Theme.scale : 0
            height: shown ? shown.height + 64 * Theme.scale : 0
            radius: 28 * Theme.scale
            color: Theme.surface
            borderColor: Theme.surfaceBorder
            borderWidth: 1
        }

        // Signed out, or an error: what signing in does, and the button.
        Column {
            id: signedOut
            objectName: "plexSignedOut"
            visible: root.status === "signed_out" || root.status === "error"
            x: 56 * Theme.scale
            y: 32 * Theme.scale
            width: parent.width * 0.62
            spacing: 26 * Theme.scale
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: qsTr("Sign in once and Home shows what you are watching and what is new on your Plex server. Choosing an item opens Plex.")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
            }
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: qsTr("Nothing is sent to Plex until you sign in. Then this TV talks only to plex.tv and the server you choose. It keeps your sign-in in the desktop keyring, or, when that is locked, in a private file on this TV that only your user can read (as Plex HTPC keeps its own).")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Text {
                objectName: "plexMessage"
                visible: text.length > 0
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.hint.length > 0 ? root.hint : (root.plex.message || "")
                color: root.status === "error" ? Theme.danger : Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
            }
            FocusButton {
                text: root.busy ? qsTr("Asking Plex…") : root.status === "error" ? qsTr("Try again") : qsTr("Sign in")
                primary: true
                focused: root.itemAt(root.focusIndex).id === "sign-in"
            }
        }

        // Linking: the code, where to type it, and a QR code of that address.
        Row {
            id: linking
            objectName: "plexLinking"
            visible: root.status === "linking"
            anchors.centerIn: parent
            spacing: 96 * Theme.scale
            PixelBox {
                width: 400 * Theme.scale; height: width
                radius: 28 * Theme.scale
                color: "#ffffff"
                anchors.verticalCenter: parent.verticalCenter
                QrCode {
                    anchors.fill: parent
                    anchors.margins: 24 * Theme.scale
                    modules: root.plex.qr_modules || []
                    dark: "#111416"
                    light: "#ffffff"
                    quietZone: 1
                }
            }
            Column {
                anchors.verticalCenter: parent.verticalCenter
                spacing: 22 * Theme.scale
                Text {
                    text: qsTr("On your phone or computer, open")
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 28 * Theme.fontUnit
                }
                Text {
                    text: (root.plex.link_url || "").replace(/^https:\/\//, "")
                    color: Theme.accent
                    font.family: Theme.monoFamily
                    font.pixelSize: 44 * Theme.fontUnit
                    font.weight: Font.Bold
                }
                Text {
                    text: qsTr("sign in to Plex and enter this code")
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 28 * Theme.fontUnit
                }
                Row {
                    spacing: 14 * Theme.scale
                    Repeater {
                        model: (root.plex.code || "").split("")
                        PixelBox {
                            required property string modelData
                            width: 92 * Theme.scale; height: 120 * Theme.scale
                            radius: 18 * Theme.scale
                            color: Theme.surfaceRaised
                            borderColor: Theme.surfaceBorder
                            Text {
                                anchors.centerIn: parent
                                text: parent.modelData
                                color: Theme.textPrimary
                                font.family: Theme.monoFamily
                                font.pixelSize: 72 * Theme.fontUnit
                                font.weight: Font.Bold
                            }
                        }
                    }
                }
                Text {
                    text: qsTr("Waiting for Plex… this screen moves on by itself.")
                    color: Theme.textMuted
                    font.family: Theme.fontFamily
                    font.pixelSize: 24 * Theme.fontUnit
                }
                FocusButton {
                    text: qsTr("Cancel")
                    focused: true
                }
            }
        }

        // Choosing a server, or the libraries on it.
        Column {
            id: choose
            objectName: "plexChoose"
            visible: root.status === "choose_server" || root.status === "choose_libraries"
            x: 56 * Theme.scale
            y: 32 * Theme.scale
            width: parent.width * 0.62
            spacing: 14 * Theme.scale
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.status === "choose_server"
                      ? qsTr("Which Plex server should Home show?")
                      : qsTr("Which libraries on %1 should Home show?").arg(root.plex.server || qsTr("your server"))
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
            }
            Repeater {
                model: root.status === "choose_server" ? root.servers : []
                SettingsRow {
                    required property var modelData
                    required property int index
                    width: parent.width
                    kind: "link"
                    label: modelData.name
                    description: (modelData.local ? qsTr("On your home network") : qsTr("Away from home"))
                                 + (modelData.owned ? "" : qsTr(" · shared with you"))
                    value: ""
                    focused: root.focusIndex === index
                }
            }
            Repeater {
                model: root.status === "choose_libraries" ? root.libraries : []
                SettingsRow {
                    required property var modelData
                    required property int index
                    width: parent.width
                    kind: "toggle"
                    label: modelData.title
                    description: root.kindLabel(modelData.kind)
                    value: root.picked(modelData) ? "on" : "off"
                    focused: root.focusIndex === index
                }
            }
            Text {
                visible: text.length > 0
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.busy ? qsTr("Checking with Plex…") : root.hint.length > 0 ? root.hint : (root.plex.message || "")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Row {
                spacing: 20 * Theme.scale
                FocusButton {
                    visible: root.status === "choose_libraries"
                    text: qsTr("Done")
                    primary: true
                    focused: root.itemAt(root.focusIndex).id === "done"
                }
                FocusButton {
                    text: qsTr("Cancel")
                    focused: root.itemAt(root.focusIndex).id === "cancel"
                }
            }
        }

        // Connected: where the rows come from and how they are.
        Column {
            id: connected
            objectName: "plexConnected"
            visible: root.status === "connected"
            x: 56 * Theme.scale
            y: 32 * Theme.scale
            width: parent.width * 0.62
            spacing: 22 * Theme.scale
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.plex.server ? qsTr("Signed in · %1").arg(root.plex.server) : qsTr("Signed in to Plex")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 34 * Theme.fontUnit
                font.weight: Font.DemiBold
            }
            Text {
                visible: text.length > 0
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.libraries.filter(l => l.selected).map(l => l.title).join(" · ")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
            }
            Text {
                objectName: "plexRowsLine"
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.hint.length > 0 ? root.hint : root.rowsLine
                color: root.content.status === "error" ? Theme.danger : Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 26 * Theme.fontUnit
            }
            Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: qsTr("Choosing an item on Home opens Plex. Phones see the rows, never this sign-in.")
                color: Theme.textMuted
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
            }
            Text {
                objectName: "plexStoredLine"
                visible: text.length > 0
                width: parent.width
                wrapMode: Text.WordWrap
                text: root.storedLine
                color: Theme.textMuted
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
            }
            FocusButton {
                text: root.busy ? qsTr("Signing out…") : qsTr("Sign out")
                danger: true
                focused: root.itemAt(root.focusIndex).id === "sign-out"
            }
        }

        // No connector in this session (an older coordinator, or dev without
        // --dev-plex-fake).
        Text {
            visible: root.status === ""
            x: 56 * Theme.scale
            y: 32 * Theme.scale
            width: parent.width * 0.62
            wrapMode: Text.WordWrap
            text: qsTr("Plex is not available in this session.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 30 * Theme.fontUnit
        }
    }
}
