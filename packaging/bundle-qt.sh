#!/usr/bin/env bash
# Bundles a built TV shell with the pinned Qt runtime into a relocatable
# directory, so the .deb does not depend on a distro Qt 6.8
# (docs/operations.md → Packaging; called by packaging/build-deb.sh).
#
#   <out>/bin/bear-den-tv-shell       wrapper (packaging/bear-den-tv-shell.sh)
#   <out>/bin/bear-den-tv-shell.bin   the shell itself
#   <out>/bin/qt.conf                 Qt paths relative to the bundle
#   <out>/lib/                        toolchain libraries the shell needs
#   <out>/lib/compat/                 libstdc++/libgcc_s, used only when the
#                                     system's are older (see the wrapper)
#   <out>/plugins/, <out>/qml/        the Qt plugins and QML modules it loads
#
# What is NOT bundled comes from the system and is listed in `depends` in
# packaging/nfpm.yaml: glibc; the GL/EGL/DRM stack (libglvnd must find the
# system's Mesa vendor files); the core X client libraries shared with Mesa
# (libX11, libxcb and its extensions); xkbcommon (it reads the system's XKB
# and compose data); D-Bus.
#   usage: packaging/bundle-qt.sh [shell-binary] [out-dir]
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="${BDTV_TOOLCHAIN:-$HOME/.bdtv-toolchain}/env"
BIN="${1:-build/bin/bear-den-tv-shell}"
OUT="${2:-build/shell-bundle}"
QT="$ROOT/lib/qt6"
[ -x "$BIN" ] || { echo "shell binary not found: $BIN (run make shell)" >&2; exit 1; }
rm -rf "$OUT"; mkdir -p "$OUT/bin" "$OUT/lib/compat" "$OUT/plugins" "$OUT/qml"
cp "$BIN" "$OUT/bin/bear-den-tv-shell.bin"

# Sonames taken from the system instead (see the header).
system_lib() {
  case "$1" in
    libc.so*|libm.so*|libpthread.so*|libdl.so*|librt.so*|libresolv.so*|libutil.so*|ld-linux*) return 0 ;;
    libGL.so*|libEGL.so*|libGLX.so*|libGLdispatch.so*|libOpenGL.so*|libGLES*|libdrm*|libgbm.so*) return 0 ;;
    libX11.so*|libX11-xcb.so*|libXau.so*|libXdmcp.so*|libxcb.so*|libxcb-glx.so*|libxcb-randr.so*|libxcb-render.so*) return 0 ;;
    libxcb-shape.so*|libxcb-shm.so*|libxcb-sync.so*|libxcb-xfixes.so*|libxcb-xkb.so*|libxcb-xinput.so*|libxcb-present.so*|libxcb-dri*) return 0 ;;
    libxkbcommon.so*|libxkbcommon-x11.so*|libdbus-1.so*) return 0 ;;
  esac
  return 1
}

copy_deps() { # copy every dependency of $1 that resolves into the toolchain
  LD_LIBRARY_PATH="$ROOT/lib" ldd "$1" | awk '/=> \//{print $3}' | { grep "^$ROOT/" || true; } | while read -r lib; do
    base=$(basename "$lib")
    system_lib "$base" && continue
    dest="$OUT/lib/$base"
    case "$base" in libstdc++.so*|libgcc_s.so*) dest="$OUT/lib/compat/$base" ;; esac
    [ -e "$dest" ] || { cp -L "$lib" "$dest"; copy_deps "$lib"; }
  done
}
copy_deps "$BIN"

# Plugins: X11 (xcb with EGL, GLX as a fallback), offscreen (checks and
# screenshots), the image formats artwork and themes use, SVG icons, compose
# and IBus keyboard input.
plugin() { # plugin <dir> <file>...
  local d=$1; shift
  mkdir -p "$OUT/plugins/$d"
  for f in "$@"; do cp "$QT/plugins/$d/$f" "$OUT/plugins/$d/"; done
}
plugin platforms libqxcb.so libqoffscreen.so libqminimal.so
plugin xcbglintegrations libqxcb-egl-integration.so libqxcb-glx-integration.so
plugin imageformats libqjpeg.so libqsvg.so libqgif.so libqwebp.so
plugin iconengines libqsvgicon.so
plugin platforminputcontexts libcomposeplatforminputcontextplugin.so libibusplatforminputcontextplugin.so
find "$OUT/plugins" -name '*.so' | while read -r p; do copy_deps "$p"; done

# QML modules: exactly the ones qmlimportscanner reports for the shell's QML
# (today QML, QtQml, QtQml.Models, QtQml.WorkerScript, QtQuick,
# QtQuick.Window). Only each module's own files, not nested modules.
mods=$("$QT/qmlimportscanner" -rootPath apps/tv-shell/qml -importPath "$QT/qml" 2>/dev/null \
  | sed -n 's/.*"path": "\(.*\)".*/\1/p' | { grep "^$QT/qml/" || true; } | sort -u)
[ -n "$mods" ] || { echo "qmlimportscanner found no QML modules" >&2; exit 1; }
for src in $mods; do
  rel="${src#"$QT/qml/"}"
  mkdir -p "$OUT/qml/$rel"
  find "$src" -maxdepth 1 -type f -exec cp {} "$OUT/qml/$rel/" \;
done
find "$OUT/qml" -name '*.so' | while read -r p; do copy_deps "$p"; done

# The highest libstdc++ symbol version anything bundled needs: the wrapper
# uses the bundled copy only when the system's libstdc++ lacks it.
find "$OUT" -type f \( -name '*.so*' -o -name '*.bin' \) -exec objdump -T {} + 2>/dev/null \
  | grep -o 'GLIBCXX_[0-9.]*' | sort -Vu | tail -1 >"$OUT/lib/compat/needs"

cat >"$OUT/bin/qt.conf" <<'EOF'
[Paths]
Prefix = ..
Libraries = lib
Plugins = plugins
QmlImports = qml
EOF
cp packaging/bear-den-tv-shell.sh "$OUT/bin/bear-den-tv-shell"
chmod 755 "$OUT/bin/bear-den-tv-shell" "$OUT/bin/bear-den-tv-shell.bin"
du -sh "$OUT"
echo "libraries: $(find "$OUT/lib" -name '*.so*' | wc -l), needs $(cat "$OUT/lib/compat/needs")"
