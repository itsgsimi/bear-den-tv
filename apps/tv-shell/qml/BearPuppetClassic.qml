// The whole bear in the Classic art style (World.classic; ADR 0006), as
// BearPuppet shows it: one of the family's SVG heads (dad wears the mark's
// face, assets/bear-mark.svg / bear-sleep.svg) on a smooth SVG body with arms
// and legs (assets/bear-body, bear-arm, bear-leg.svg), posed by the numbers
// on `puppet` (the BearPuppet: walk, walking, wave, wavePhase, squash, facing,
// sitting, reach, asleep, blink, stickLength, and the dressed wornHat, held
// and dress: assets/classic/umbrella-leaf.svg in the rain). Rotations and
// scales are continuous; nothing animates on its own.

import QtQuick
import BearDen

Item {
    id: root
    required property Item puppet
    readonly property real u: puppet.size / 100
    readonly property string head: puppet.kind === "dad" ? "bear-mark" : puppet.kind === "mama" ? "bear-mama" : "bear-cub"
    readonly property string sleepyHead: puppet.kind === "dad" ? "bear-sleep" : puppet.kind === "mama" ? "bear-mama-sleep" : "bear-cub-sleep"
    readonly property real step: puppet.walking ? Math.sin(puppet.walk) : 0
    readonly property real bob: puppet.walking ? Math.abs(Math.sin(puppet.walk)) : 0
    // Seated, the hips sit 16 units lower: the bear's bottom is at `height`.
    readonly property real seat: puppet.sitting ? 16 * u : 0
    width: puppet.width
    height: puppet.height

    Item {
        id: rig
        width: puppet.width
        height: puppet.height
        transform: [
            Scale { origin.x: rig.width / 2; xScale: puppet.facing },
            Scale { origin.x: rig.width / 2; origin.y: rig.height; xScale: 1 + 0.14 * puppet.squash; yScale: 1 - 0.18 * puppet.squash },
            Rotation { origin.x: rig.width / 2; origin.y: rig.height; angle: root.step * 3 },
            Translate { y: -root.bob * 3 * root.u + root.seat }
        ]

        Image {                                     // far leg
            source: "qrc:/qt/qml/BearDen/assets/bear-leg.svg"
            width: 24 * root.u; height: width * 64 / 50
            x: 25 * root.u - width / 2
            y: 69 * root.u - Math.max(0, root.step) * 7 * root.u
            transformOrigin: Item.Top
            rotation: puppet.sitting ? -72 : -root.step * 10
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
        }
        Image {                                     // near leg
            source: "qrc:/qt/qml/BearDen/assets/bear-leg.svg"
            width: 24 * root.u; height: width * 64 / 50
            x: 45 * root.u - width / 2
            y: 69 * root.u - Math.max(0, -root.step) * 7 * root.u
            transformOrigin: Item.Top
            rotation: puppet.sitting ? -82 : root.step * 10
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
        }
        Image {
            source: "qrc:/qt/qml/BearDen/assets/bear-body.svg"
            width: 58 * root.u; height: width * 112 / 120
            x: 35 * root.u - width / 2
            y: 42 * root.u
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
        }
        Image {                                     // left arm (viewer's left)
            source: "qrc:/qt/qml/BearDen/assets/bear-arm.svg"
            width: 15 * root.u; height: width * 84 / 40
            x: 10 * root.u - width / 2
            y: 49 * root.u
            transformOrigin: Item.Top
            rotation: 18 + root.step * 16
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
        }
        Image {                                     // right arm: swings, or waves
            id: rightArm
            source: "qrc:/qt/qml/BearDen/assets/bear-arm.svg"
            width: 15 * root.u; height: width * 84 / 40
            x: 60 * root.u - width / 2
            y: 49 * root.u
            transformOrigin: Item.Top
            readonly property real rest: -18 - root.step * 16
            readonly property real raised: -158 + Math.sin(puppet.wavePhase) * 22
            readonly property real forward: -80            // straight out towards what the bear faces
            rotation: rest + (forward - rest) * puppet.reach + (raised - rest) * puppet.wave
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
            // What the bear carries in this paw.
            Ornament {
                visible: puppet.held !== "" && puppet.held !== "stick" && puppet.kind !== "mama"
                url: puppet.held !== "stick" ? puppet.held : ""
                width: 18 * root.u; height: width
                x: rightArm.width / 2 - width / 2
                y: rightArm.height * 0.78 - height / 2
            }
            Item {
                visible: puppet.held === "stick" && puppet.kind !== "mama"
                x: rightArm.width / 2
                y: rightArm.height * 0.78
                Rectangle {
                    width: puppet.stickLength * root.u; height: 2.4 * root.u; radius: height / 2
                    color: "#8A6443"
                    transformOrigin: Item.Left
                    rotation: -40 + 100 * puppet.reach   // up and forward, hanging or reaching out
                    Rectangle {
                        x: parent.width - width / 2; y: -height / 2 + parent.height / 2
                        width: 9 * root.u; height: 8 * root.u; radius: 2.5 * root.u
                        color: "#F7F0E2"; border.color: "#D9A441"; border.width: 1.2 * root.u
                    }
                }
            }
        }
        // A leaf umbrella in the rain: the stem rises from the resting right
        // paw to a leaf canopy over the head.
        Item {
            visible: puppet.dress === "umbrella"
            Rectangle { x: 66 * root.u; y: -8 * root.u; width: 2.6 * root.u; height: 80 * root.u; radius: width / 2; color: "#5E7A3A" }
            Image {
                source: parent.visible ? "qrc:/qt/qml/BearDen/assets/classic/umbrella-leaf.svg" : ""
                width: 84 * root.u; height: width * 44 / 104
                x: 67 * root.u - width / 2; y: -40 * root.u
                sourceSize: Qt.size(width * 2, height * 2)
                smooth: true
            }
        }
        Image {
            id: headImage
            source: "qrc:/qt/qml/BearDen/assets/" + (puppet.blink || puppet.asleep ? root.sleepyHead : root.head) + ".svg"
            width: 64 * root.u; height: width
            x: 35 * root.u - width / 2
            y: 0
            transformOrigin: Item.Bottom
            rotation: root.step * 3 + puppet.wave * 6
            sourceSize: Qt.size(width * 2, height * 2)
            smooth: true
            Ornament {
                visible: puppet.wornHat.toString().length > 0
                url: puppet.wornHat
                width: headImage.width * 0.66; height: width * 0.72
                x: headImage.width / 2 - width / 2
                y: headImage.height * 0.08
            }
        }
    }
}
