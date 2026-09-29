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
        }
        return ""
    }
    // A how-to line under the description on the featured panel ("" = none).
    function hint(adapter) {
        switch (adapter) {
        case "spotify": return qsTr("Play from your phone: open Spotify and pick this TV in the device list.")
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
        }
        return { scene: "cabin", prop: "", react: { kind: "mama", pose: "wave", carry: "" } }
    }
}
