// One plain sentence per setting, keyed by the row's stable id: what the
// setting does and when to use it. Shown beside the focused row in Settings,
// on the Themes and Apps pages and in the first-run setup (HelpPanel.qml).
// One table so every place says the same thing; the phone remote keeps the
// same texts for the settings it shows (apps/remote-web/src/i18n.ts).
// Engine code only looks a row's id up here; it never branches on it.
// Tests: tst_shell.cpp everySettingHasHelp.

pragma Singleton
import QtQuick

QtObject {
    readonly property var table: ({
        // Settings → Phones & remote
        "remote": qsTr("Lets phones on your home network control this TV once you pair them on the TV. Off: nothing listens on the network."),
        "pairing": qsTr("Shows a QR code and a six-digit code to pair a phone, or a guest pass that ends by itself."),
        "devices": qsTr("The phones that can control this TV. Remove one to take its access away."),
        "layout-editing": qsTr("Lets owner phones change the Home layout from their Layout tab. The phone remote is not encrypted: anyone else on your home network could read or change what a paired phone sends, so leave this off on a network you don't trust. Off by default."),
        "now-playing": qsTr("Paired phones see the title of what is playing. Never while the TV is locked."),
        // Settings → Display & accessibility
        "text": qsTr("Makes all text on the TV bigger or smaller."),
        "density": qsTr("Bigger tiles are easier to see from the couch; smaller ones fit more on a row."),
        "margin": qsTr("For TVs that cut off the picture's edges: raise it until nothing is cut off."),
        "motion": qsTr("Stops the moving decorations and slides; everything still works."),
        "contrast": qsTr("A thicker, brighter outline around whatever is selected."),
        // Settings → Home screen
        "hero": qsTr("The big panel above your apps that describes what is selected."),
        "clock": qsTr("Shows the time in the top bar."),
        "tips": qsTr("On a quiet Home a bear may walk in with a sign about something to try, at most once a day. OK shows it, Back says not now; three Not nows in a row and the bears stop."),
        "tips-again": qsTr("Forgets which tips you have seen, so the bears can offer them again."),
        "weather": qsTr("Your town's weather by the clock, and in the scene if you like. Only the place you choose is looked up."),
        // Settings → Playback
        "playback": qsTr("What this PC can play smoothly, and the settings Bear Den chose for each app."),
        "advanced-playback": qsTr("Change an app's playback settings by hand when the automatic choice doesn't suit you."),
        "plex": qsTr("Sign in to your Plex account to see Continue Watching and Recently Added on Home."),
        // Settings → Power & TV
        "sleep": qsTr("Pauses what is playing where it can, goes Home and turns the screen off after the time you pick."),
        "screen-off": qsTr("Turns the picture off now. Any button wakes it; that first press only wakes it."),
        "cec": qsTr("Lets Bear Den turn the TV on and off and switch its input, over HDMI. Needs a CEC adapter; most PCs don't have one."),
        "cec-volume": qsTr("Which volume your phone's volume buttons change: this PC's, or the TV's over HDMI."),
        "autostart": qsTr("Starts Bear Den whenever you log in to this PC. Off: you start it yourself."),
        // Settings → About
        "version": qsTr("The version of Bear Den running on this TV."),
        "diagnostics": qsTr("What is running and connected, and why something is unavailable: for when something goes wrong."),
        "setup-again": qsTr("Walks through the first-time setup again: your look, apps, the phone remote and extras."),
        "exit": qsTr("Leaves Bear Den and returns to the desktop until the next start. For maintenance."),
        // Themes page
        "background": qsTr("The world Bear Den lives in: colours, wallpaper and the corner scene."),
        "style": qsTr("How much decoration: Bear Den adds bears and ornaments, Plain keeps it simple, Performance turns animation off."),
        "art": qsTr("Pixel draws everything in pixel art; Classic uses smooth drawings."),
        "app-icons": qsTr("App's own uses each installed app's icon; Bear Den style draws every app in Bear Den's look."),
        "badges": qsTr("Den badges: playful milestones, counted on this TV only. Turn counting off or reset them."),
        // Apps page
        "installed": qsTr("The apps on this TV. Open one from Home."),
        "add-apps": qsTr("Installs apps Bear Den knows from Flathub, for this user only. Nothing installs until you press Install."),
        "remove-apps": qsTr("Removes an app from this TV, for this user only. OK shows what it frees and what else it turns off; nothing is removed until you press Remove."),
        "streaming": qsTr("Netflix, Disney+ and Hulu in a browser. Off: no tile on Home."),
        "browser-browser": qsTr("Which browser the Browser tile opens in."),
        "browser-streaming": qsTr("Which browser the streaming sites open in. They need its Widevine module to play."),
        "auto-update": qsTr("Updates the apps installed for the TV's user (not system-wide ones) while the TV is idle, about once a day.")
    })
    // The help for a row id ("" = none).
    function text(id) { return table[id] || "" }
}
