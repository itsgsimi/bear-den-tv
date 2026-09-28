// Top bar: brand, navigation pills, status, local weather, clock. `focusIndex` >= 0 when the
// D-pad is on the pills.

import QtQuick
import BearDen

Item {
    id: root
    property string current: "home"
    property int focusIndex: -1
    readonly property var pills: [["home", qsTr("Home")], ["settings", qsTr("Settings")], ["pairing", qsTr("Pair phone")]]
    implicitHeight: Math.max(88 * Theme.scale, brand.implicitHeight)

    Row {
        id: brand
        anchors.verticalCenter: parent.verticalCenter
        spacing: 16 * Theme.scale
        BearMark { size: 64 * Theme.scale; anchors.verticalCenter: parent.verticalCenter }
        Column {
            anchors.verticalCenter: parent.verticalCenter
            Text {
                text: qsTr("Bear Den")
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 34 * Theme.fontUnit
                font.weight: Font.Bold
            }
            Text {
                id: greeting
                property int hour: new Date().getHours()
                Timer { interval: 60000; running: true; repeat: true; onTriggered: greeting.hour = new Date().getHours() }
                text: (hour < 5 ? qsTr("Late night in the den") : hour < 12 ? qsTr("Good morning") : hour < 18 ? qsTr("Good afternoon") : qsTr("Good evening"))
                      + (Session.deviceName.length > 0 && Session.deviceName !== "Bear Den" ? " · " + Session.deviceName : "")
                color: Theme.textSecondary
                font.family: Theme.fontFamily
                font.pixelSize: 19 * Theme.fontUnit
            }
        }
    }

    Row {
        anchors { left: brand.right; leftMargin: 72 * Theme.scale; verticalCenter: parent.verticalCenter }
        spacing: 12 * Theme.scale
        Repeater {
            model: root.pills
            NavPill {
                required property var modelData
                required property int index
                text: modelData[1]
                current: root.current === modelData[0]
                focused: root.focusIndex === index
            }
        }
    }

    Row {
        anchors { right: parent.right; verticalCenter: parent.verticalCenter }
        spacing: 18 * Theme.scale
        DemoBadge { visible: Session.devMode; anchors.verticalCenter: parent.verticalCenter }
        StatusChip {
            anchors.verticalCenter: parent.verticalCenter
            readonly property bool listening: Session.remote.listening === true
            readonly property int phones: Session.remote.paired_device_count || 0
            dot: listening ? Theme.success : Theme.textMuted
            text: listening ? (phones === 1 ? qsTr("1 phone paired") : qsTr("%1 phones paired").arg(phones))
                            : qsTr("Phone remote off")
        }
        // Local weather (state.weather): icon + temperature while a reading is
        // ready, dimmed while stale, hidden otherwise.
        Row {
            id: weatherChip
            objectName: "weatherChip"
            readonly property var now: World.weatherNow
            visible: now !== null
            opacity: Session.weather.status === "stale" ? 0.5 : 1
            anchors.verticalCenter: parent.verticalCenter
            spacing: 8 * Theme.scale
            PixelSprite {
                visible: World.pixel
                name: World.pixel ? World.weatherIcon(weatherChip.now) : ""
                unit: Math.max(1, Math.round(World.px * 0.75))
                anchors.verticalCenter: parent.verticalCenter
            }
            // Classic: the smooth icon (assets/classic/weather-<name>.svg), same size.
            Image {
                objectName: "weatherIconClassic"
                visible: World.classic && source != ""
                readonly property string icon: World.classic ? World.weatherIcon(weatherChip.now) : ""
                source: icon.length > 0 ? "qrc:/qt/qml/BearDen/assets/classic/" + icon + ".svg" : ""
                width: 16 * Math.max(1, Math.round(World.px * 0.75))
                height: width
                sourceSize: Qt.size(width, height)
                anchors.verticalCenter: parent.verticalCenter
            }
            Text {
                objectName: "weatherTemperature"
                anchors.verticalCenter: parent.verticalCenter
                text: weatherChip.now ? weatherChip.now.temperature + "°" : ""
                color: Theme.textPrimary
                font.family: Theme.fontFamily
                font.pixelSize: 30 * Theme.fontUnit
                font.weight: Font.Medium
            }
        }
        Text {
            id: clock
            visible: Theme.clockEnabled
            anchors.verticalCenter: parent.verticalCenter
            color: Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 32 * Theme.fontUnit
            font.weight: Font.Medium
            text: Qt.formatTime(new Date(), Qt.locale().timeFormat(Locale.ShortFormat))
            Timer {
                interval: 10000; running: Theme.clockEnabled; repeat: true
                onTriggered: clock.text = Qt.formatTime(new Date(), Qt.locale().timeFormat(Locale.ShortFormat))
            }
        }
    }
}
