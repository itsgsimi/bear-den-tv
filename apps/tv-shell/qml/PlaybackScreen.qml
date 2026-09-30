// Settings → Playback: the playback detection test the coordinator runs after
// startup (state.playback; internal/applications/tuning): this box, its
// display, and for each app what it decodes on the GPU, what to expect, the
// caveats, and whether the best settings are applied. Read-only; the owner
// re-runs it with `bear-den-tv apps detect` (docs/APP_PERFORMANCE.md).
// Each app leads with a plain outcome (who does the video work, how it plays)
// and its caveats; OK shows the details (this box, the codecs, what to
// expect, the settings changed), which are written for the curious (UX-22).

import QtQuick
import BearDen

Item {
    id: root
    property bool details: false
    function enter() { details = false; view.contentY = 0; Nav.reportFocus("diagnostics", "playback", 0) }
    function navigate(action) {
        switch (action) {
        case "nav.down": view.contentY = Math.min(Math.max(0, view.contentHeight - view.height), view.contentY + 240 * Theme.scale); return true
        case "nav.up": view.contentY = Math.max(0, view.contentY - 240 * Theme.scale); return true
        case "select": details = !details; return true
        case "nav.left": case "nav.right": return true
        }
        return false
    }

    readonly property var p: Session.playback
    readonly property bool ready: p && p.apps !== undefined
    function statusText(app) {
        switch (app.status) {
        case "tuned": return qsTr("Tuned for this TV")
        case "applied": return qsTr("Best settings applied")
        case "pending": return qsTr("Will be tuned when %1 closes").arg(app.label)
        case "off": return qsTr("Automatic tuning is off")
        case "error": return qsTr("Could not apply the settings")
        }
        return qsTr("Suggested settings")
    }
    // The plain outcome, from what the detection found.
    function outcome(app) {
        return app.hardware.length > 0 ? qsTr("Plays smoothly: the graphics chip does the video work.")
                                       : qsTr("Plays, but the processor does the video work: keep the quality modest.")
    }
    function statusColor(app) {
        return app.status === "tuned" || app.status === "applied" ? Theme.success
             : app.status === "error" ? Theme.danger : Theme.warning
    }

    component NoteLine: Row {
        property var note: ({})
        width: parent ? parent.width : 0
        spacing: 12 * Theme.scale
        Text {
            text: note.level === "warn" ? "⚠" : "•"
            color: note.level === "warn" ? Theme.warning : Theme.textMuted
            font.family: Theme.fontFamily
            font.pixelSize: 22 * Theme.fontUnit
        }
        Text {
            width: parent.width - 40 * Theme.scale
            text: note.text || ""
            wrapMode: Text.WordWrap
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 21 * Theme.fontUnit
        }
    }

    ScreenFrame {
        anchors.fill: parent
        title: qsTr("Playback")
        subtitle: root.ready ? qsTr("How each app plays on this TV") : qsTr("Checking what this TV can play…")
        hints: [["▲ ▼", qsTr("Scroll")], ["OK", root.details ? qsTr("Hide details") : qsTr("Show details")], ["Back", qsTr("Back")]]

        Flickable {
            id: view
            anchors.fill: parent
            contentHeight: col.height
            interactive: false
            clip: true
            Behavior on contentY { enabled: !Theme.reducedMotion; NumberAnimation { duration: Theme.duration } }
            Column {
                id: col
                width: view.width
                spacing: 22 * Theme.scale
                Text {
                    objectName: "playbackBox"
                    visible: root.details && root.ready
                    width: col.width
                    wrapMode: Text.WordWrap
                    text: root.ready ? qsTr("This TV: %1").arg(root.p.summary) + (root.p.display ? " " + qsTr("Display: %1.").arg(root.p.display) : "") : ""
                    color: Theme.textSecondary
                    font.family: Theme.fontFamily
                    font.pixelSize: 22 * Theme.fontUnit
                }
                Column {
                    width: col.width
                    spacing: 8 * Theme.scale
                    Repeater {
                        model: root.ready ? root.p.notes : []
                        NoteLine { required property var modelData; note: modelData }
                    }
                }
                Repeater {
                    model: root.ready ? root.p.apps : []
                    PixelBox {
                        required property var modelData
                        width: col.width
                        height: card.height + 44 * Theme.scale
                        radius: 18 * Theme.scale
                        color: Theme.surface
                        borderColor: Theme.surfaceBorder
                        Column {
                            id: card
                            x: 30 * Theme.scale; y: 22 * Theme.scale
                            width: parent.width - 60 * Theme.scale
                            spacing: 8 * Theme.scale
                            Row {
                                spacing: 18 * Theme.scale
                                Text {
                                    text: modelData.label
                                    color: Theme.textPrimary
                                    font.family: Theme.fontFamily
                                    font.pixelSize: 30 * Theme.fontUnit
                                    font.weight: Font.DemiBold
                                }
                                PixelBox {
                                    anchors.verticalCenter: parent.verticalCenter
                                    implicitWidth: statusRow.implicitWidth + 28 * Theme.scale
                                    implicitHeight: 34 * Theme.scale
                                    radius: height / 2
                                    color: Theme.alpha("#000000", 0.35)
                                    Row {
                                        id: statusRow
                                        anchors.centerIn: parent
                                        spacing: 8 * Theme.scale
                                        PixelBox {
                                            width: 10 * Theme.scale; height: width; radius: width / 2
                                            color: root.statusColor(modelData)
                                            anchors.verticalCenter: parent.verticalCenter
                                        }
                                        Text {
                                            text: root.statusText(modelData)
                                            color: Theme.textPrimary
                                            font.family: Theme.fontFamily
                                            font.pixelSize: 18 * Theme.fontUnit
                                            anchors.verticalCenter: parent.verticalCenter
                                        }
                                    }
                                }
                            }
                            Text {
                                objectName: "playbackOutcome"
                                width: card.width
                                text: root.outcome(modelData)
                                color: Theme.textPrimary
                                wrapMode: Text.WordWrap
                                font.family: Theme.fontFamily
                                font.pixelSize: 24 * Theme.fontUnit
                            }
                            Text {
                                objectName: "playbackHardware"
                                visible: root.details
                                width: card.width
                                text: qsTr("Video formats the graphics chip plays: %1").arg(modelData.hardware.length > 0 ? modelData.hardware.join(", ") : qsTr("none"))
                                color: Theme.accent
                                wrapMode: Text.WordWrap
                                font.family: Theme.fontFamily
                                font.pixelSize: 21 * Theme.fontUnit
                            }
                            Repeater {
                                model: root.details ? modelData.expect : []
                                Text {
                                    required property string modelData
                                    width: card.width
                                    text: modelData
                                    color: Theme.textPrimary
                                    wrapMode: Text.WordWrap
                                    font.family: Theme.fontFamily
                                    font.pixelSize: 22 * Theme.fontUnit
                                }
                            }
                            Repeater {
                                // Caveats always; the rest with the details.
                                model: modelData.notes.filter(n => root.details || n.level === "warn")
                                NoteLine { required property var modelData; width: card.width; note: modelData }
                            }
                            Repeater {
                                model: modelData.status === "tuned" || !root.details ? [] : modelData.changes
                                Text {
                                    required property string modelData
                                    width: card.width
                                    text: "• " + modelData
                                    color: Theme.textMuted
                                    wrapMode: Text.WordWrap
                                    font.family: Theme.fontFamily
                                    font.pixelSize: 18 * Theme.fontUnit
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
