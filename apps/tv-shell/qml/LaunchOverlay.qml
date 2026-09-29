// Shown while the coordinator is starting or activating an app. It never
// blocks navigation for long: Back hides it and the result still arrives.

import QtQuick
import BearDen

Rectangle {
    id: root
    property string appId: Shell.launchingAppId
    property bool dismissed: false
    readonly property var app: appId.length > 0 ? Session.application(appId) : ({})
    visible: appId.length > 0 && !dismissed
    onAppIdChanged: dismissed = false
    // Darker than the dialog scrim: Home stays faintly visible behind, but the
    // app card and the hopping cub stand out clearly.
    color: Theme.alpha("#06080A", 0.93)

    function navigate(action) {
        if (action === "back") { dismissed = true; return true }
        return true
    }

    Column {
        anchors.centerIn: parent
        spacing: 28 * Theme.scale
        // The app's card in its own colours, with its icon (AppIcon).
        Item {
            id: card
            readonly property var brand: Apps.brand(root.app.adapter || "")
            readonly property color tint: Theme.tintFor(root.appId)
            anchors.horizontalCenter: parent.horizontalCenter
            width: 368 * Theme.scale; height: 207 * Theme.scale
            BrandBackdrop {
                anchors.fill: parent
                topColor: card.brand ? card.brand.top : Qt.darker(card.tint, 1.7)
                bottomColor: card.brand ? card.brand.bottom : Qt.darker(card.tint, 3.4)
                glow: card.brand ? card.brand.glow : card.tint
                glowX: 0.5
                glowY: 0.5
                glowRadius: 0.45
            }
            AppIcon {
                anchors.centerIn: parent
                size: 112 * Theme.scale
                adapter: root.app.adapter || ""
                label: root.app.label || ""
                tint: card.tint
            }
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: qsTr("Opening %1…").arg(root.app.label || "")
            color: Theme.textPrimary
            font.family: Theme.fontFamily
            font.pixelSize: 40 * Theme.fontUnit
            font.weight: Font.DemiBold
        }
        // A cub hops along while the app opens, leaving paw prints behind it.
        Item {
            id: hop
            visible: !World.performance
            anchors.horizontalCenter: parent.horizontalCenter
            width: 380 * Theme.scale
            height: 104 * Theme.scale
            property real t: Theme.reducedMotion ? 0.5 : 0
            NumberAnimation on t {
                running: root.visible && !Theme.reducedMotion
                from: 0; to: 1; duration: 2600; loops: Animation.Infinite
            }
            Repeater {
                model: 6
                Ornament {
                    required property int index
                    readonly property real at: (index + 0.5) / 6
                    name: "paw"
                    width: 22 * Theme.scale
                    height: width
                    x: at * (hop.width - 58 * Theme.scale) + 18 * Theme.scale
                    y: hop.height - height - (index % 2 ? 0 : 8 * Theme.scale)
                    rotation: 90
                    opacity: hop.t > at ? 0.6 : 0.12
                }
            }
            BearHead {
                kind: "cub"
                // Pixel: on the grid. Classic: smooth, rocking as it hops.
                readonly property real hx: hop.t * (hop.width - width)
                readonly property real hy: hop.height - height - 26 * Theme.scale - Math.abs(Math.sin(hop.t * Math.PI * 5)) * 34 * Theme.scale
                width: (World.classic ? 60 : 76) * Theme.scale
                x: World.classic ? hx : World.snap(hx)
                y: World.classic ? hy : World.snap(hy)
                rotation: World.classic ? Math.sin(hop.t * Math.PI * 10) * 7 : 0
            }
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: qsTr("Press Home on the remote to come back to Bear Den at any time.")
            color: Theme.textSecondary
            font.family: Theme.fontFamily
            font.pixelSize: 24 * Theme.fontUnit
        }
    }
}
