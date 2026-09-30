// Themes → Den badges: the shelf of Den badges (state.achievements,
// contracts/http.md#den-badges-stateachievements). Earned badges show their
// medal and the day they were earned; the rest a silhouette, a hint and how
// far along they are. Below the shelf: turning counting on or off
// (achievements.configure) and resetting every badge after a confirmation
// (achievements.reset). Nothing here moves: the shelf repaints only when
// the snapshot or the focus changes.

pragma ComponentBehavior: Bound
import QtQuick
import BearDen

Item {
    id: root
    property int focusIndex: 0
    // Set by ShellRoot: this screen is the one in front.
    property bool active: false
    signal confirm(string title, string body, string confirmLabel, var onAccept, bool danger)

    readonly property var ach: Session.achievements
    readonly property bool counting: ach.enabled !== false
    readonly property var progress: ach.progress || []
    readonly property var earnedDays: {
        const out = {}
        for (const e of (ach.earned || [])) out[e.id] = e.day
        return out
    }
    readonly property int earnedCount: (ach.earned || []).length
    readonly property int columns: 8
    readonly property int count: progress.length
    // Focus: the badges row by row, then the two buttons.
    readonly property int toggleIndex: count
    readonly property int resetIndex: count + 1

    function itemId(i) {
        if (i === toggleIndex) return "counting"
        if (i === resetIndex) return "reset"
        return progress[i] ? progress[i].id : ""
    }
    function report() { Nav.reportFocus("badges", itemId(focusIndex), 0) }
    function enter() { focusIndex = 0; report() }
    function move(to) {
        focusIndex = Math.max(0, Math.min(resetIndex, to))
        report()
    }
    function activate(i) {
        if (i === toggleIndex) {
            Shell.setAchievements(!counting)
        } else if (i === resetIndex) {
            root.confirm(qsTr("Reset badges?"), qsTr("Every badge and everything counted toward them is deleted. This cannot be undone."),
                         qsTr("Reset"), () => Shell.resetAchievements(), true)
        }
    }
    function navigate(action) {
        const onShelf = focusIndex < count
        switch (action) {
        case "nav.left":
            if (onShelf) { if (focusIndex % columns > 0) move(focusIndex - 1) }
            else if (focusIndex === resetIndex) move(toggleIndex)
            return true
        case "nav.right":
            if (onShelf) { if (focusIndex % columns < columns - 1 && focusIndex + 1 < count) move(focusIndex + 1) }
            else if (focusIndex === toggleIndex) move(resetIndex)
            return true
        case "nav.down":
            if (onShelf) move(focusIndex + columns < count ? focusIndex + columns : toggleIndex)
            return true
        case "nav.up":
            if (!onShelf) move(Math.max(0, count - columns))
            else if (focusIndex >= columns) move(focusIndex - columns)
            return true
        case "select":
            activate(focusIndex)
            return true
        }
        return false
    }

    readonly property var focused: focusIndex < count ? progress[focusIndex] : null
    readonly property bool focusedEarned: focused !== null && earnedDays[focused.id] !== undefined

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Den badges")
        subtitle: !root.counting ? qsTr("Badges are off: nothing is being counted")
                                 : qsTr("%1 of %2 earned · counted on this TV only").arg(root.earnedCount).arg(root.count)
        hints: [["◀ ▶ ▲ ▼", qsTr("Move")], ["OK", qsTr("Select")], ["Back", qsTr("Back")]]

        Column {
            anchors.horizontalCenter: parent.horizontalCenter
            y: 8 * Theme.scale
            spacing: 28 * Theme.scale

            // The shelf.
            PixelBox {
                anchors.horizontalCenter: parent.horizontalCenter
                width: shelf.width + 48 * Theme.scale
                height: shelf.height + 40 * Theme.scale
                radius: 28 * Theme.scale
                color: Theme.surface
                borderColor: Theme.surfaceBorder
                borderWidth: 1
                Grid {
                    id: shelf
                    objectName: "badgeShelf"
                    anchors.centerIn: parent
                    columns: root.columns
                    columnSpacing: 20 * Theme.scale
                    rowSpacing: 14 * Theme.scale
                    Repeater {
                        model: root.progress
                        Item {
                            id: cell
                            required property var modelData
                            required property int index
                            readonly property bool earned: root.earnedDays[modelData.id] !== undefined
                            readonly property bool isFocused: root.focusIndex === index
                            width: medal.width + 36 * Theme.scale
                            height: medal.height + caption.height + 16 * Theme.scale
                            PixelBox {
                                anchors.fill: parent
                                anchors.margins: -4 * Theme.scale
                                radius: 18 * Theme.scale
                                visible: cell.isFocused
                                color: Theme.pillActiveBg
                                borderColor: Theme.accent
                                borderWidth: 3
                            }
                            BadgeMedal {
                                id: medal
                                anchors.horizontalCenter: parent.horizontalCenter
                                y: 6 * Theme.scale
                                badgeId: cell.modelData.id
                                earned: cell.earned
                                opacity: cell.earned ? 1 : 0.85
                            }
                            Text {
                                id: caption
                                anchors { horizontalCenter: parent.horizontalCenter; top: medal.bottom; topMargin: 4 * Theme.scale }
                                text: cell.earned ? "★" : cell.modelData.count + "/" + cell.modelData.goal
                                color: cell.earned ? Theme.accent : (cell.isFocused ? Theme.pillActiveText : Theme.textSecondary)
                                font.family: Theme.fontFamily
                                font.pixelSize: 20 * Theme.fontUnit
                                font.weight: Font.DemiBold
                            }
                        }
                    }
                }
            }

            // The focused badge, up close.
            PixelBox {
                objectName: "badgeDetail"
                anchors.horizontalCenter: parent.horizontalCenter
                width: shelf.width + 48 * Theme.scale
                height: 200 * Theme.scale
                radius: 28 * Theme.scale
                color: Theme.surface
                borderColor: Theme.surfaceBorder
                borderWidth: 1
                visible: root.focused !== null
                BadgeMedal {
                    id: big
                    x: 36 * Theme.scale
                    anchors.verticalCenter: parent.verticalCenter
                    zoom: 1
                    badgeId: root.focused ? root.focused.id : ""
                    earned: root.focusedEarned
                }
                Column {
                    anchors { left: big.right; leftMargin: 36 * Theme.scale; right: parent.right; rightMargin: 36 * Theme.scale; verticalCenter: parent.verticalCenter }
                    spacing: 10 * Theme.scale
                    Text {
                        objectName: "badgeName"
                        text: root.focused ? Badges.name(root.focused.id) : ""
                        color: Theme.textPrimary
                        font.family: Theme.fontFamily
                        font.pixelSize: 38 * Theme.fontUnit
                        font.weight: Font.Bold
                    }
                    Text {
                        width: parent.width
                        wrapMode: Text.WordWrap
                        text: !root.focused ? ""
                              : root.focusedEarned ? qsTr("Earned %1 · %2").arg(Badges.dayText(root.earnedDays[root.focused.id])).arg(Badges.hint(root.focused.id))
                              : Badges.hint(root.focused.id)
                        color: root.focusedEarned ? Theme.accent : Theme.textSecondary
                        font.family: Theme.fontFamily
                        font.pixelSize: 26 * Theme.fontUnit
                    }
                    Row {
                        visible: root.focused !== null && !root.focusedEarned
                        spacing: 16 * Theme.scale
                        ProgressBar {
                            width: 360 * Theme.scale
                            height: 10 * Theme.scale
                            anchors.verticalCenter: parent.verticalCenter
                            value: root.focused ? root.focused.count / Math.max(1, root.focused.goal) : 0
                        }
                        Text {
                            text: root.focused ? qsTr("%1 of %2").arg(root.focused.count).arg(root.focused.goal) : ""
                            color: Theme.textSecondary
                            font.family: Theme.fontFamily
                            font.pixelSize: 22 * Theme.fontUnit
                        }
                    }
                }
            }

            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: 32 * Theme.scale
                FocusButton {
                    objectName: "badgesCounting"
                    text: root.counting ? qsTr("Counting: On") : qsTr("Counting: Off")
                    detail: qsTr("Only counts and days, on this TV")
                    focused: root.focusIndex === root.toggleIndex
                }
                FocusButton {
                    objectName: "badgesReset"
                    text: qsTr("Reset badges")
                    danger: true
                    focused: root.focusIndex === root.resetIndex
                }
            }
        }
    }
}
