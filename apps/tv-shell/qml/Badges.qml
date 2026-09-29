// Den badges as the TV shows them: each badge's name and hint (the copy lives
// here, keyed by the ids of the catalogue in internal/achievements) and its
// art in the active art style: tools/pixelart/badges.py (PNG) and
// tools/classicart/badges.py (SVG), a silhouette while not earned. State comes
// from Session.achievements (contracts/http.md#den-badges-stateachievements).
// An id this build does not know shows its id and the first silhouette.

pragma Singleton
import QtQuick
import BearDen

QtObject {
    readonly property var copy: ({
        "first-night-in": [qsTr("First Night In"), qsTr("Open any app from Home.")],
        "movie-night": [qsTr("Movie Night"), qsTr("Open Plex ten times.")],
        "couch-explorer": [qsTr("Couch Explorer"), qsTr("Open every installed app at least once.")],
        "night-owl": [qsTr("Night Owl"), qsTr("Visit Home after 11 pm on five nights.")],
        "early-cub": [qsTr("Early Cub"), qsTr("Visit Home before 7 am on five mornings.")],
        "rainy-day": [qsTr("Rainy Day Den"), qsTr("Visit Home while it rains, on three days. Needs Weather on.")],
        "snow-day": [qsTr("Snow Day"), qsTr("Visit Home while it snows. Needs Weather on.")],
        "thunder-buddy": [qsTr("Thunder Buddy"), qsTr("Keep the bears company in a thunderstorm. Needs Weather on.")],
        "all-seasons": [qsTr("All Seasons"), qsTr("Visit Home in spring, summer, autumn and winter.")],
        "style-switcher": [qsTr("Style Switcher"), qsTr("Try both art styles: Pixel and Classic.")],
        "theme-tourist": [qsTr("Theme Tourist"), qsTr("Visit every built-in theme.")],
        "family-den": [qsTr("Family Den"), qsTr("Pair two family phones.")],
        "good-host": [qsTr("Good Host"), qsTr("Give a visitor a guest pass.")],
        "sleepy-bear": [qsTr("Sleepy Bear"), qsTr("Set the sleep timer five times.")],
        "parade-spotter": [qsTr("Parade Spotter"), qsTr("The bears march for an old, old code…")],
        "loyal-den": [qsTr("Loyal Den"), qsTr("Spend time in the den on thirty different days.")]
    })
    function name(id) { return (copy[id] || [id])[0] }
    function hint(id) { return (copy[id] || ["", ""])[1] }
    function known(id) { return copy[id] !== undefined }
    // The medal in the active art style; `earned` false gives its silhouette.
    function art(id, earned) {
        const bid = known(id) ? id : "first-night-in"
        const suffix = earned && known(id) ? "" : "-locked"
        return World.classic ? "qrc:/qt/qml/BearDen/assets/classic/badge-" + bid + suffix + ".svg"
                             : "qrc:/qt/qml/BearDen/assets/pixel/badge-" + bid + suffix + ".png"
    }
    // "28 Sep 2026" from the day a badge was earned (YYYY-MM-DD, local).
    function dayText(day) {
        const p = String(day || "").split("-")
        if (p.length !== 3) return ""
        return Qt.formatDate(new Date(Number(p[0]), Number(p[1]) - 1, Number(p[2])), "d MMM yyyy")
    }
}
