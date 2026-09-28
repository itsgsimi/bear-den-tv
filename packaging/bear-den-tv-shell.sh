#!/usr/bin/env bash
# Launcher for the bundled TV shell. Resolves the bundle root relative to this
# script, points Qt at the bundled plugins/QML, keeps system fonts and Mesa, and
# applies the EGL integration workaround required by the bundled Qt build
# (see docs/decisions/0001-standalone-project-and-user-space-toolchain.md).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
BUNDLE="$(dirname "$HERE")"
export LD_LIBRARY_PATH="$BUNDLE/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export QT_PLUGIN_PATH="$BUNDLE/plugins"
export QML_IMPORT_PATH="$BUNDLE/qml"
export QML2_IMPORT_PATH="$BUNDLE/qml"
export QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}"
export QT_XCB_GL_INTEGRATION="${QT_XCB_GL_INTEGRATION:-xcb_egl}"
export FONTCONFIG_FILE="${FONTCONFIG_FILE:-/etc/fonts/fonts.conf}"
exec "$HERE/bear-den-tv-shell.bin" "$@"
