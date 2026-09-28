// A pixel-art sprite from the shell's own art (assets/pixel/<name>.png, made by
// tools/pixelart): drawn unsmoothed at `unit` screen pixels per art pixel
// (World.px by default). A sheet of `frames` equal frames side by side shows
// frame `frame`; the owner advances it (on World's heartbeat), so a sprite
// never animates on its own.

import QtQuick
import BearDen

Image {
    id: root
    property string name
    property int frames: 1
    property int frame: 0
    property real unit: World.px
    // The sheet's own size, taken once it loads (sourceSize would be re-read
    // whenever the clip changes).
    property size sheet: Qt.size(0, 0)
    readonly property int frameWidth: Math.floor(sheet.width / Math.max(1, frames))
    source: name.length > 0 ? "qrc:/qt/qml/BearDen/assets/pixel/" + name + ".png" : ""
    smooth: false
    width: frameWidth * unit
    height: sheet.height * unit
    sourceClipRect: Qt.rect((frame % Math.max(1, frames)) * frameWidth, 0, frameWidth, sheet.height)
    onStatusChanged: if (status === Image.Ready && sheet.width === 0) sheet = sourceSize
}
