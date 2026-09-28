// An ornament. `name` resolves through the active theme — its own
// <name>.svg|png first, else the built-in assets/ornaments/<name>.svg|png — so
// a theme can restyle any ornament by shipping a file with the same name. Set
// `url` instead for a resolved URL. `mirror` flips horizontally, `flip`
// vertically.
// PNG ornaments are pixel art (tools/pixelart): drawn unsmoothed at a
// whole-number multiple of World.px and centred in the box the caller asked
// for, so their pixels match the rest of the world. SVG ornaments are drawn
// crisp at any size.

import QtQuick
import BearDen

Item {
    id: root
    property string name
    property url url: name.length > 0 ? World.ornament(name) : ""
    property bool mirror: false
    property bool flip: false
    readonly property bool pixel: url.toString().toLowerCase().endsWith(".png")
    readonly property alias status: image.status

    Image {
        id: image
        source: root.url
        // Pixel art: k art pixels per sprite pixel, the largest whole number
        // that fits (at least one), times the world's pixel size.
        readonly property int k: Math.max(1, Math.round(Math.min(root.width / Math.max(1, implicitWidth),
                                                                root.height / Math.max(1, implicitHeight)) / World.px))
        width: root.pixel ? implicitWidth * k * World.px : root.width
        height: root.pixel ? implicitHeight * k * World.px : root.height
        x: root.pixel ? Math.round((root.width - width) / 2) : 0
        y: root.pixel ? Math.round((root.height - height) / 2) : 0
        sourceSize: root.pixel ? undefined : Qt.size(root.width * 2, root.height * 2)
        fillMode: Image.PreserveAspectFit
        smooth: !root.pixel
        mipmap: false
        transform: Scale {
            origin.x: image.width / 2
            origin.y: image.height / 2
            xScale: root.mirror ? -1 : 1
            yScale: root.flip ? -1 : 1
        }
    }
}
