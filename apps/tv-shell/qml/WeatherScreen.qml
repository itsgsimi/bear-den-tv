// Settings → Weather: turn local weather on or off, °C/°F, whether it drives
// the Home scene, and find the town by name (spec: state.weather in
// contracts/state.schema.json; weather.search / weather.configure in
// contracts/ipc.md). Left column: the three toggles and the search field;
// right column: the places found. The search field takes a physical keyboard
// or the phone's keyboard (text.submit reaches a focused field); Return runs
// weather.search. Choosing a place sends weather.configure with it, enabled.
//
// state.weather carries only the place's name, so the toggles send
// `place: null`, which keeps the stored place; only a chosen search result
// sends a place object (it replaces the stored one).

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    // Set by ShellRoot: this screen is the one in front.
    property bool active: false
    property string hint: ""

    readonly property var w: Session.weather
    readonly property bool enabled_: w.status !== undefined && w.status !== "disabled"
    readonly property string units: w.units || "celsius"
    readonly property bool scene: w.scene !== false
    readonly property var places: Shell.weatherPlaces
    readonly property int searchIndex: 3
    readonly property int count: 4 + places.length
    readonly property var rowIds: ["enabled", "units", "scene", "search"]

    function itemId(i) { return i < 4 ? rowIds[i] : "place-" + (i - 4) }
    function enter() { hint = ""; syncField(); report() }
    function report() { Nav.reportFocus("weather", itemId(focusIndex), 0) }
    function syncField() {
        if (active && focusIndex === searchIndex) field.forceActiveFocus()
        else if (field.activeFocus) field.focus = false
    }
    onActiveChanged: syncField()
    function move(to) {
        focusIndex = Math.max(0, Math.min(count - 1, to))
        syncField()
        report()
    }

    // place null keeps the stored place; turning on needs one stored first.
    function configure(enabled, place, units, scene) {
        if (enabled && !place && !w.place) { hint = qsTr("Search for your town first."); move(searchIndex); return }
        hint = qsTr("Saving…")
        Shell.weatherConfigure(enabled, place, units, scene)
    }
    function activate(i) {
        if (i === 0) {
            configure(!enabled_, null, units, scene)
        } else if (i === 1 || i === 2) {
            configure(enabled_, null, i === 1 ? (units === "celsius" ? "fahrenheit" : "celsius") : units, i === 2 ? !scene : scene)
        } else if (i === searchIndex) {
            hint = ""
            Shell.weatherSearch(field.text)
        } else {
            const p = places[i - 4]
            configure(true, { name: p.name, region: p.region || "", country: p.country || "", latitude: p.latitude, longitude: p.longitude }, units, scene)
        }
    }
    Connections {
        target: Shell
        function onWeatherConfigured(ok, error) { root.hint = ok ? "" : error }
        function onWeatherPlacesChanged() {
            // New results: the first place is one press away.
            if (Shell.weatherPlaces.length > 0 && root.focusIndex >= 4) root.move(4)
            else if (root.focusIndex >= root.count) root.move(root.count - 1)
        }
    }

    function navigate(action) {
        switch (action) {
        case "nav.up": move(focusIndex - 1); return true
        case "nav.down": move(focusIndex + 1); return true
        case "nav.right":
        case "nav.left":
            if (focusIndex === 1) activate(1)                       // °C ◀ ▶ °F
            else if (action === "nav.right" && focusIndex < 4 && places.length > 0) move(4)
            else if (action === "nav.left" && focusIndex >= 4) move(searchIndex)
            return true
        case "select": activate(focusIndex); return true
        case "back": field.focus = false; return false
        }
        return false
    }

    function placeLine(p) { return [p.region, p.country].filter(s => s && s.length > 0).join(", ") }
    readonly property string statusLine: {
        switch (w.status) {
        case undefined: case "disabled": return w.place ? qsTr("Weather is off · %1").arg(w.place) : qsTr("Weather is off.")
        case "connecting": return qsTr("Getting the weather for %1…").arg(w.place)
        case "ready": return w.place
        case "stale": return qsTr("%1 · last reading, can't reach the weather service").arg(w.place)
        case "error": return w.message || qsTr("No weather right now.")
        }
        return ""
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Weather")
        subtitle: qsTr("By the clock on Home, and in the scene if you like")
        hints: [["▲ ▼", qsTr("Move")], ["◀ ▶", qsTr("Places")], ["OK", qsTr("Select")], ["Back", qsTr("Back")]]

        Column {
            id: left
            width: parent.width * 0.5
            x: 20 * Theme.scale
            spacing: 14 * Theme.scale

            Row {
                spacing: 16 * Theme.scale
                height: 64 * Theme.scale
                PixelSprite {
                    visible: World.weatherNow !== null && World.pixel
                    name: World.pixel ? World.weatherIcon(World.weatherNow) : ""
                    anchors.verticalCenter: parent.verticalCenter
                }
                // Classic: the smooth icon (assets/classic/weather-<name>.svg), same size.
                Image {
                    objectName: "weatherScreenIconClassic"
                    visible: World.weatherNow !== null && World.classic
                    readonly property string icon: World.classic ? World.weatherIcon(World.weatherNow) : ""
                    source: icon.length > 0 ? "qrc:/qt/qml/BearDen/assets/classic/" + icon + ".svg" : ""
                    width: 16 * World.px
                    height: width
                    sourceSize: Qt.size(width, height)
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    visible: World.weatherNow !== null
                    text: World.weatherNow ? World.weatherNow.temperature + "°" : ""
                    color: Theme.textPrimary
                    font.family: Theme.fontFamily
                    font.pixelSize: 44 * Theme.fontUnit
                    font.weight: Font.Bold
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    text: root.statusLine
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 26 * Theme.fontUnit
                    anchors.verticalCenter: parent.verticalCenter
                }
            }
            SettingsRow {
                width: parent.width
                kind: "toggle"; label: qsTr("Weather"); value: root.enabled_ ? "on" : "off"
                description: qsTr("Checks Open-Meteo every half hour for your town")
                focused: root.focusIndex === 0
            }
            SettingsRow {
                width: parent.width
                kind: "choice"; label: qsTr("Units"); value: root.units === "fahrenheit" ? "°F" : "°C"
                focused: root.focusIndex === 1
            }
            SettingsRow {
                width: parent.width
                kind: "toggle"; label: qsTr("Weather in the scene"); value: root.scene ? "on" : "off"
                description: qsTr("Rain, snow and cloud over the Home backdrop")
                focused: root.focusIndex === 2
            }
            PixelBox {
                id: searchBox
                width: parent.width
                height: 104 * Theme.scale
                radius: 18 * Theme.scale
                color: root.focusIndex === root.searchIndex ? Theme.surfaceRaised : Theme.surface
                borderColor: root.focusIndex === root.searchIndex ? Theme.accent : Theme.surfaceBorder
                borderWidth: 1
                Column {
                    anchors { left: parent.left; right: parent.right; leftMargin: 32 * Theme.scale; rightMargin: 32 * Theme.scale; verticalCenter: parent.verticalCenter }
                    spacing: 6 * Theme.scale
                    Item {
                        width: parent.width
                        height: field.height
                        TextInput {
                            id: field
                            objectName: "weatherSearchField"
                            width: parent.width
                            color: Theme.textPrimary
                            font.family: Theme.fontFamily
                            font.pixelSize: 28 * Theme.fontUnit
                            maximumLength: 80
                            clip: true
                            selectByMouse: false
                            cursorVisible: activeFocus
                        }
                        Text {
                            visible: field.text.length === 0
                            text: qsTr("Search for your town")
                            color: Theme.textMuted
                            font: field.font
                        }
                    }
                    Text {
                        text: root.focusIndex === root.searchIndex
                              ? qsTr("Type here, or on your phone's keyboard, then press OK")
                              : qsTr("Type on a keyboard or your phone's keyboard")
                        color: Theme.textMuted
                        font.family: Theme.fontFamily
                        font.pixelSize: 20 * Theme.fontUnit
                    }
                }
                FocusFrame { shown: root.focusIndex === root.searchIndex; cornerRadius: searchBox.radius }
            }
            Text {
                visible: text.length > 0
                width: parent.width
                wrapMode: Text.Wrap
                text: root.hint.length > 0 ? root.hint
                      : Shell.weatherSearching ? qsTr("Searching…")
                      : !Shell.weatherSearchOk ? Shell.weatherSearchError : ""
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 22 * Theme.fontUnit
            }
        }

        Column {
            anchors { left: left.right; leftMargin: 48 * Theme.scale; right: parent.right; rightMargin: 20 * Theme.scale; top: parent.top; topMargin: 78 * Theme.scale }
            spacing: 12 * Theme.scale
            Text {
                text: root.places.length > 0 ? qsTr("Places") : qsTr("Places you find show up here")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 24 * Theme.fontUnit
            }
            Repeater {
                model: root.places
                SettingsRow {
                    required property var modelData
                    required property int index
                    width: parent.width
                    implicitHeight: 84 * Theme.scale
                    kind: "link"
                    label: modelData.name
                    description: root.placeLine(modelData)
                    value: ""
                    focused: root.focusIndex === 4 + index
                }
            }
        }
    }
}
