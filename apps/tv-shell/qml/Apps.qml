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
        }
        return ""
    }
    function about(adapter) {
        switch (adapter) {
        case "plex-htpc": return qsTr("Your movies, shows and music, from your own Plex library.")
        case "vacuumtube": return qsTr("YouTube on the big screen. Sign in once inside YouTube.")
        case "moonlight": return qsTr("Play games streamed from your PC. A controller works best.")
        }
        return ""
    }
    // Brand palette: `top`/`bottom` for the background, `glow` behind the icon.
    function brand(adapter) {
        switch (adapter) {
        case "plex-htpc": return { top: "#2E3136", bottom: "#121315", glow: "#E5A00D" }   // Plex charcoal + amber
        case "vacuumtube": return { top: "#34110F", bottom: "#0F0F0F", glow: "#FF0033" }  // YouTube dark + red
        case "moonlight": return { top: "#3F454D", bottom: "#14171B", glow: "#A9C1DC" }   // Moonlight slate + moonlight
        }
        return null
    }
    // The app's stage in Bear Den's world (docs/THEMES.md → Pixel art):
    //   scene  the pixel room behind its icon on the featured panel (HeroRig.js)
    //   prop   what the cub holds when it peeks over the app's tile
    //   react  the bear that walks onto the panel for it, how it stands, and
    //          what it holds (BearPuppet poses; an ornament name or "")
    function stage(adapter) {
        switch (adapter) {
        case "plex-htpc": return { scene: "cinema", prop: "popcorn", react: { kind: "cub", pose: "sit", carry: "popcorn" } }
        case "vacuumtube": return { scene: "cabin", prop: "remote", react: { kind: "dad", pose: "wave", carry: "remote" } }
        case "moonlight": return { scene: "arcade", prop: "controller", react: { kind: "cub", pose: "reach", carry: "controller" } }
        }
        return { scene: "cabin", prop: "", react: { kind: "mama", pose: "wave", carry: "" } }
    }
}
