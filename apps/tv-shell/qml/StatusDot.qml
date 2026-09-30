// The coloured mark before a status word (Running, Not installed, a phone
// count). A plain dot in Classic and a plain square in Pixel: never the
// stepped circle PixelBox draws at this size, which read as a "+" (UX-26).

import QtQuick
import BearDen

Rectangle {
    objectName: "statusDot"
    width: 10 * Theme.scale
    height: width
    radius: World.classic ? width / 2 : 0
    antialiasing: World.classic
}
