// Human descriptions and brand colours of the supported client adapters
// (closed set, mirrors internal/applications/adapters). Bear Den launches these
// apps; it does not reproduce their interfaces. Tiles and the hero are drawn in
// each app's own colours around Bear Den's own icon for it (never the official
// logo; docs/THEMES.md → App icons), so they read as that app by label and colour.

pragma Singleton
import QtQuick

QtObject {
    function tagline(adapter) {
        switch (adapter) {
        case "plex-htpc": return qsTr("Your movies, shows & music")
        case "vacuumtube": return qsTr("YouTube, made for the big screen")
        case "moonlight": return qsTr("Stream games from your PC")
        case "spotify": return qsTr("Music and podcasts")
        case "jellyfin": return qsTr("Your own media server")
        case "retroarch": return qsTr("Classic games, one place")
        case "netflix": return qsTr("Netflix in the browser")
        case "disney-plus": return qsTr("Disney+ in the browser")
        case "hulu": return qsTr("Hulu in the browser")
        case "browser": return qsTr("The web, with a keyboard or your phone")
        }
        return ""
    }
    function about(adapter) {
        switch (adapter) {
        case "plex-htpc": return qsTr("Your movies, shows and music, from your own Plex library.")
        case "vacuumtube": return qsTr("YouTube on the big screen. Sign in once inside YouTube.")
        case "moonlight": return qsTr("Play games streamed from your PC. A controller works best.")
        case "spotify": return qsTr("Music and podcasts on the TV's speakers.")
        case "jellyfin": return qsTr("Films and shows from your own Jellyfin server.")
        case "retroarch": return qsTr("Play your classic games. A controller works best.")
        case "netflix": return qsTr("Netflix's website, full screen. Sign in once with a keyboard or your phone's touchpad.")
        case "disney-plus": return qsTr("Disney+'s website, full screen. Sign in once with a keyboard or your phone's touchpad.")
        case "hulu": return qsTr("Hulu's website, full screen. Sign in once with a keyboard or your phone's touchpad.")
        case "browser": return qsTr("Chromium in its own profile. Use a keyboard and mouse, the remote, or your phone's touchpad.")
        }
        return ""
    }
    // A how-to line under the description on the featured panel ("" = none).
    function hint(adapter) {
        switch (adapter) {
        case "spotify": return qsTr("Play from your phone: open Spotify and pick this TV in the device list.")
        case "netflix":
        case "disney-plus":
        case "hulu": return qsTr("Plays at up to 720p in a Linux browser.")
        case "browser": return qsTr("Stuck? The Touchpad on your phone reaches anything on the page.")
        }
        return ""
    }
    // What an install of this app actually fetches, when it is not the app
    // itself ("" = the app): the web apps all run in Flathub Chromium
    // (internal/applications/adapters ChromiumFlatpakID), installed once.
    function installName(adapter) {
        switch (adapter) {
        case "netflix":
        case "disney-plus":
        case "hulu":
        case "browser": return qsTr("Chromium")
        }
        return ""
    }
    // Why that shared install is needed (Settings → Add apps, the install card).
    function installWhy(adapter) {
        switch (adapter) {
        case "netflix":
        case "disney-plus":
        case "hulu":
        case "browser": return qsTr("Browser for Netflix, Disney+, Hulu")
        }
        return ""
    }
    // A short note on the tile until the app is first opened in this session
    // ("" = none): the streaming sites' honest quality cap.
    function note(adapter) {
        switch (adapter) {
        case "netflix":
        case "disney-plus":
        case "hulu": return qsTr("Up to 720p")
        }
        return ""
    }
    // Brand palette: `top`/`bottom` for the background, `glow` behind the icon.
    function brand(adapter) {
        switch (adapter) {
        case "plex-htpc": return { top: "#2E3136", bottom: "#121315", glow: "#E5A00D" }   // Plex charcoal + amber
        case "vacuumtube": return { top: "#34110F", bottom: "#0F0F0F", glow: "#FF0033" }  // YouTube dark + red
        case "moonlight": return { top: "#3F454D", bottom: "#14171B", glow: "#A9C1DC" }   // Moonlight slate + moonlight
        case "spotify": return { top: "#17402A", bottom: "#0C1410", glow: "#1DB954" }     // Spotify green on black
        case "jellyfin": return { top: "#34245A", bottom: "#0E1226", glow: "#AA5CC3" }    // Jellyfin purple into blue
        case "retroarch": return { top: "#2E2654", bottom: "#0E0C1C", glow: "#E0564A" }   // indigo + arcade red
        // Web apps: Bear Den's own badge colours (tools/*/appicons.py), not the services'.
        case "netflix": return { top: "#3A1A28", bottom: "#120810", glow: "#D8404A" }      // plum + popcorn red
        case "disney-plus": return { top: "#242E6A", bottom: "#0A0E26", glow: "#FFD34F" }  // night blue + star gold
        case "hulu": return { top: "#163A26", bottom: "#08140E", glow: "#4ED88A" }         // pine + mint
        case "browser": return { top: "#1A4A58", bottom: "#081A20", glow: "#6AB4C8" }      // teal + sky
        }
        return null
    }
    // The app's stage in Bear Den's world (docs/THEMES.md → Pixel art):
    //   scene  the room behind its icon on the featured panel (HeroRig.js):
    //          cinema, cabin, arcade, nook (music), theatre (woods), retro
    //   prop   what the cub holds when it peeks over the app's tile
    //   react  the bear that walks onto the panel for it, how it stands, and
    //          what it holds (BearPuppet poses; an ornament name or "")
    function stage(adapter) {
        switch (adapter) {
        case "plex-htpc": return { scene: "cinema", prop: "popcorn", react: { kind: "cub", pose: "sit", carry: "popcorn" } }
        case "vacuumtube": return { scene: "cabin", prop: "remote", react: { kind: "dad", pose: "wave", carry: "remote" } }
        case "moonlight": return { scene: "arcade", prop: "controller", react: { kind: "cub", pose: "reach", carry: "controller" } }
        case "spotify": return { scene: "nook", prop: "heart", react: { kind: "mama", pose: "wave", carry: "heart" } }
        case "jellyfin": return { scene: "theatre", prop: "popcorn", react: { kind: "dad", pose: "sit", carry: "popcorn" } }
        case "retroarch": return { scene: "retro", prop: "controller", react: { kind: "cub", pose: "reach", carry: "controller" } }
        // Web apps share the existing rooms (no rooms of their own yet).
        case "netflix": return { scene: "cinema", prop: "popcorn", react: { kind: "mama", pose: "sit", carry: "popcorn" } }
        case "disney-plus": return { scene: "theatre", prop: "heart", react: { kind: "cub", pose: "sit", carry: "heart" } }
        case "hulu": return { scene: "cabin", prop: "remote", react: { kind: "dad", pose: "sit", carry: "remote" } }
        case "browser": return { scene: "nook", prop: "remote", react: { kind: "mama", pose: "wave", carry: "" } }
        }
        return { scene: "cabin", prop: "", react: { kind: "mama", pose: "wave", carry: "" } }
    }
}
