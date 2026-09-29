// One Den badge medal (Badges.art): 32 art pixels square, `zoom` world
// pixels per art pixel (whole numbers, so pixel art stays on the grid); in
// Classic the same box, drawn smooth. `earned` false shows the silhouette.

import QtQuick
import BearDen

Image {
    id: root
    property string badgeId
    property bool earned: false
    property int zoom: 1
    width: 32 * World.px * zoom
    height: width
    source: Badges.art(badgeId, earned)
    sourceSize: World.classic ? Qt.size(width, height) : Qt.size(32, 32)
    smooth: World.classic
    mipmap: false
    fillMode: Image.PreserveAspectFit
    asynchronous: false
}
