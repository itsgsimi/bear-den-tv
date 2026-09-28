// Selecting an app that is not installed explains how to install it. Bear Den
// never installs clients on its own (AGENTS.md).

import QtQuick
import BearDen

MessageDialog {
    id: root
    function openFor(app) {
        const label = app.label || qsTr("This app")
        const fid = Shell.flatpakIdFor(app.adapter || "")
        open(qsTr("%1 isn't installed").arg(label),
             qsTr("Bear Den opens %1 but does not install it for you. Install it from Flathub on this computer, then come back — the tile will light up on its own.").arg(label),
             fid.length > 0 ? "flatpak install --user flathub " + fid : "")
    }
}
