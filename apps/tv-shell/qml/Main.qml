// The shell's top-level window: fullscreen by default, holds ShellRoot (guide
// apps/tv-shell/AGENTS.md).

import QtQuick
import QtQuick.Window
import BearDen

Window {
    id: window
    property int initialWidth: 1920
    property int initialHeight: 1080
    property bool fullscreen: true
    width: initialWidth
    height: initialHeight
    visible: true
    visibility: fullscreen ? Window.FullScreen : Window.Windowed
    color: Theme.bgBottom
    title: qsTr("Bear Den TV")

    onWidthChanged: Theme.windowWidth = width
    onHeightChanged: Theme.windowHeight = height

    ShellRoot {
        anchors.fill: parent
        focus: true
    }
}
