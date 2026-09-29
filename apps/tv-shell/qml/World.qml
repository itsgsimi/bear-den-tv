// The active theme ("world"), as the rest of the shell sees it: a thin view of
// the theme package chosen in Settings → Theme (layout.ui.background), resolved
// by the Themes registry (src/ThemeRegistry.h; manifest spec
// contracts/theme.schema.json; guide docs/THEMES.md).
//
// Components never switch on a theme's name. They read what the theme asks
// for — a focus style, an ambient kind, a scene, ornament URLs — so a new
// theme needs only a theme.json and its art.
// The Plain style (layout.ui.theme "plain-dark") keeps the theme's wallpaper
// and colours but drops every decoration.
// The art style (layout.ui.art_style) is independent of the style: Pixel draws
// everything as pixel art on one grid (px, below); Classic draws smooth art,
// the theme's classic wallpaper and the SVG ornaments.

pragma Singleton
import QtQuick
import BearDen

QtObject {
    readonly property var theme: Themes.get(Theme.background, Theme.artStyle)
    // Art style: pixel art (default), or smooth Classic art.
    readonly property bool classic: Theme.artStyle === "classic"
    readonly property bool pixel: !classic
    readonly property string id: theme.id || "den"
    readonly property string title: theme.name || qsTr("Den")
    // Plain and Performance both drop decorations and bears; Performance also
    // stops every animation (Theme.reducedMotion is forced on).
    readonly property bool plain: Theme.style !== "den-dark"
    readonly property bool decorated: !plain
    // Performance style: no bear artwork beyond the logo, and nothing animates.
    readonly property bool performance: Theme.style === "performance"

    readonly property var palette: theme.palette || ({ stem: "#5E8C6E", light: "#A3D1AE", dark: "#6A9C7A", bloom: "#F7F0E2", glow: "#E3B35C" })
    readonly property var wallpaper: theme.wallpaper || ({ image: "", top: "#1F2A26", bottom: "#0C0E10" })
    // Decorations, already empty in Plain: {style, tip, tipUpright, extras, extrasBottom, extrasUpright}.
    readonly property var focus: decorated && theme.focus ? theme.focus : ({ style: "" })
    readonly property var panel: decorated && theme.panel ? theme.panel : ({ style: "" })
    // {kind, count, colors}; kind "" = none. Local weather in the scene
    // replaces the theme's particles (below).
    readonly property var themeAmbient: decorated && theme.ambient ? theme.ambient : ({ kind: "", count: 0, colors: [] })
    readonly property var ambient: weatherKind.length > 0 ? ({ kind: weatherKind, count: weatherCount, colors: [] }) : themeAmbient

    // Local weather (state.weather; spec contracts/state.schema.json, guide
    // docs/THEMES.md → Weather in the scene). `weatherNow` is the reading the
    // header shows: present only while status is ready or stale.
    readonly property var weather: Session.weather
    readonly property var weatherNow: (weather.status === "ready" || weather.status === "stale") && weather.current ? weather.current : null
    // The owner's "Weather in the scene" with a reading to show.
    readonly property bool weatherScene: weather.scene === true && weatherNow !== null
    readonly property string weatherCondition: weatherScene ? weatherNow.condition : ""
    // Particles: rain for drizzle/rain/thunder, snow for snow; none with
    // reduced motion (only the static veil stays).
    readonly property string weatherKind: Theme.reducedMotion ? ""
        : weatherCondition === "drizzle" || weatherCondition === "rain" || weatherCondition === "thunder" ? "rain"
        : weatherCondition === "snow" ? "snow" : ""
    // Count by intensity, capped low for the 2-core floor (each is one Rectangle).
    readonly property int weatherCount: {
        if (!weatherScene) return 0
        const n = ({ light: 24, moderate: 40, heavy: 56 })[weatherNow.intensity] || 40
        return weatherCondition === "drizzle" ? Math.round(n * 0.6) : n
    }
    // A static veil over the wallpaper (0 = none): grey sky for cloud, fog and rain.
    readonly property real weatherVeil: ({ cloudy: 0.16, fog: 0.3, drizzle: 0.14, rain: 0.22, thunder: 0.3, snow: 0.08 })[weatherCondition] || 0
    readonly property color weatherVeilColor: weatherCondition === "fog" ? "#B9C0CA" : weatherCondition === "snow" ? "#DDE6F0" : "#2E3642"
    readonly property bool weatherThunder: weatherCondition === "thunder"
    // What the corner scene and the visiting bears react to (SceneWeather.qml,
    // BearPuppet's weatherDress; docs/THEMES.md → Weather in the corner scene):
    //   "wet"   drizzle, rain     "storm" thunder (wet, plus the startle)
    //   "snow"  snow              "fog"   fog
    //   "night" clear or partly cloudy at night
    //   ""      no weather in the scene, or cloudy/clear by day: unchanged
    // Kept with reduced motion (the scene then shows its still version).
    readonly property string weatherLook: {
        switch (weatherCondition) {
        case "drizzle": case "rain": return "wet"
        case "thunder": return "storm"
        case "snow": return "snow"
        case "fog": return "fog"
        case "clear": case "partly-cloudy": return weatherNow.is_day ? "" : "night"
        }
        return ""
    }
    // Set by WeatherSky while its lightning flash is lit: bears startle.
    property bool flashing: false
    // The pixel icon (assets/pixel/weather-<name>.png) for a reading.
    function weatherIcon(current) {
        if (!current) return ""
        switch (current.condition) {
        case "clear": return current.is_day ? "weather-sun" : "weather-moon"
        case "partly-cloudy": return current.is_day ? "weather-sun-cloud" : "weather-moon-cloud"
        case "cloudy": return "weather-cloud"
        default: return "weather-" + current.condition
        }
    }
    readonly property string scene: decorated ? (theme.scene || "") : ""
    // {hat, carry, chase}: ornament URLs, "stick", or a built-in chase kind.
    readonly property var bears: theme.bears || ({ hat: "", carry: "", chase: "" })
    readonly property url heading: decorated ? (theme.heading || "") : ""

    // The heartbeat: one clock every decoration moves on (particles, the focus
    // decoration, panel corners, the wallpaper drift), so all their changes
    // land in the same frame. 20 beats a second awake, 4 while resting, none
    // behind apps, on the screensaver or with reduced motion. Independent
    // timers would each trigger their own frames and their rates would add up.
    signal beat(real dt)
    // Set while a visiting bear is on screen: the heartbeat stays at full speed.
    property bool visiting: false
    readonly property bool beating: !Theme.reducedMotion && !Theme.screensaver
                                    && (Session.target.kind === "shell" || !Session.loaded)
    property Timer heartbeat: Timer {
        interval: Theme.resting && !visiting ? 250 : 50
        repeat: true
        running: beating
        onTriggered: beat(interval / 1000)
    }

    // Pixel art: the size of one art pixel on screen. The worlds are drawn on a
    // 480×270 grid, so a 1080p TV shows each art pixel as a 4×4 block; every
    // pixel-art item (PixelImage, the corner engine, particles, bears) snaps
    // to this grid so nothing is drawn at an in-between size.
    readonly property int px: Math.max(1, Math.round(4 * Theme.scale))
    function snap(v) { return Math.round(v / px) * px }

    // The default accent of a theme (Settings applies it when switching).
    function accentFor(background) { return Themes.get(background).accent || "#79A889" }
    // An ornament by name: the theme's own first, else the built-in; the PNG
    // in Pixel, the SVG in Classic.
    function ornament(name) { return Themes.ornament(id, name, Theme.artStyle) }
}
