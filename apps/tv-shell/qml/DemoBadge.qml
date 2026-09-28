// Every fixture item and the dev session carry this label (AGENTS.md: demo
// content exists only behind --dev-fixtures and is labeled DEMO).

import QtQuick
import BearDen

PixelBox {
    implicitWidth: label.implicitWidth + 20 * Theme.scale
    implicitHeight: label.implicitHeight + 8 * Theme.scale
    radius: height / 2
    color: Theme.alpha(Theme.demoBadge, 0.18)
    borderColor: Theme.demoBadge
    borderWidth: 1
    Text {
        id: label
        anchors.centerIn: parent
        text: "DEMO"
        color: Theme.demoBadge
        font.family: Theme.fontFamily
        font.pixelSize: 16 * Theme.fontUnit
        font.weight: Font.Bold
        font.letterSpacing: 2 * Theme.scale
    }
}
