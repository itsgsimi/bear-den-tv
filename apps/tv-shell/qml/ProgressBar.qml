// Thin watch-progress bar; value in 0..1.

import QtQuick
import BearDen

PixelBox {
    property real value: 0
    implicitHeight: 6 * Theme.scale
    radius: height / 2
    color: Theme.alpha("#ffffff", 0.18)
    PixelBox {
        width: parent.width * Math.max(0, Math.min(1, parent.value))
        height: parent.height
        radius: parent.radius
        color: Theme.accent
    }
}
