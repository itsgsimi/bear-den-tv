// A secondary screen, built the first time it is opened (ensure()), not at
// start: Home is ready sooner and a screen nobody opens costs nothing. Each
// builds in a few milliseconds (up to ~0.1 s on the reference Celeron), inside
// its fade-in. Once built it stays, so its state and focus survive leaving
// it, as before. It fades and slides in while `name` is the current screen.

import QtQuick
import BearDen

Loader {
    id: slot
    required property string name
    property string current: ""
    active: false
    opacity: current === name ? 1 : 0
    visible: opacity > 0
    y: current === name ? 0 : 24 * Theme.scale
    Behavior on opacity { NumberAnimation { duration: Theme.ms(220); easing.type: Easing.OutCubic } }
    Behavior on y { NumberAnimation { duration: Theme.ms(260); easing.type: Easing.OutCubic } }

    // The screen, built now if it is not yet.
    function ensure() {
        active = true
        return item
    }
}
