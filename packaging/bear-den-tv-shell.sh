#!/usr/bin/env bash
# Launcher for the bundled TV shell (packaging/bundle-qt.sh; installed as
# /opt/bear-den-tv/shell/bin/bear-den-tv-shell with a /usr/bin symlink, which
# the coordinator finds next to itself). Resolves the bundle root relative to
# this script, points Qt at the bundled libraries, plugins and QML, keeps the
# system's fonts, glibc and Mesa, and applies the EGL workaround the bundled
# Qt needs (docs/decisions/0001-standalone-project-and-user-space-toolchain.md).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
BUNDLE="$(dirname "$HERE")"
libs="$BUNDLE/lib"
# The shell was built with a newer GCC than older distros ship. Use the
# bundled libstdc++/libgcc_s only when the system's lacks the symbol version
# the bundle needs; a newer system copy stays in charge, so the system's Mesa
# (which links libstdc++ through LLVM) never gets an older one than it needs.
needs="$(cat "$libs/compat/needs" 2>/dev/null || true)"
sys="$( { /sbin/ldconfig -p 2>/dev/null || ldconfig -p 2>/dev/null || true; } | awk '/libstdc\+\+\.so\.6 .*x86-64/{print $NF; exit}')"
if [ -z "$needs" ] || [ -z "$sys" ] || [ "$(tr '\0' '\n' <"$sys" | grep -cxF "$needs")" = 0 ]; then
  libs="$libs:$BUNDLE/lib/compat"
fi
export LD_LIBRARY_PATH="$libs${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export QT_PLUGIN_PATH="$BUNDLE/plugins"
export QML_IMPORT_PATH="$BUNDLE/qml"
export QML2_IMPORT_PATH="$BUNDLE/qml"
export QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}"
export QT_XCB_GL_INTEGRATION="${QT_XCB_GL_INTEGRATION:-xcb_egl}"
export FONTCONFIG_FILE="${FONTCONFIG_FILE:-/etc/fonts/fonts.conf}"
exec "$HERE/bear-den-tv-shell.bin" "$@"
