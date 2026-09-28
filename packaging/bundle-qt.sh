#!/usr/bin/env bash
# Bundles the built TV shell with the pinned Qt runtime into a relocatable
# directory (default build/shell-bundle) so the .deb/.rpm does not depend on a
# distro Qt 6.8. Copies only libraries resolved from the toolchain prefix plus
# the Qt plugin/QML directories the shell imports. System glibc and Mesa are
# used at runtime; libstdc++/libgcc from the toolchain are bundled because the
# shell is built with the toolchain's GCC.
#   usage: packaging/bundle-qt.sh [shell-binary] [out-dir]
set -euo pipefail
ROOT="${BDTV_TOOLCHAIN:-$HOME/.bdtv-toolchain}/env"
BIN="${1:-build/bin/bear-den-tv-shell}"
OUT="${2:-build/shell-bundle}"
[ -x "$BIN" ] || { echo "shell binary not found: $BIN (run make shell)" >&2; exit 1; }
rm -rf "$OUT"; mkdir -p "$OUT/bin" "$OUT/lib" "$OUT/plugins" "$OUT/qml"
cp "$BIN" "$OUT/bin/bear-den-tv-shell.bin"

copy_deps() { # copy every dependency of $1 that lives under the toolchain prefix
  ldd "$1" | awk '/=> \//{print $3}' | grep "^$ROOT/" | while read -r lib; do
    base=$(basename "$lib")
    [ -e "$OUT/lib/$base" ] || { cp -L "$lib" "$OUT/lib/$base"; copy_deps "$lib"; }
  done
}
copy_deps "$BIN"
# Plugins the shell needs at runtime (platform, GL integration, image formats, icons).
for d in platforms xcbglintegrations egldeviceintegrations imageformats iconengines platformthemes platforminputcontexts; do
  [ -d "$ROOT/lib/qt6/plugins/$d" ] && cp -r "$ROOT/lib/qt6/plugins/$d" "$OUT/plugins/"
done
find "$OUT/plugins" -name '*.so' | while read -r p; do copy_deps "$p"; done
# QML modules imported by the shell (only what qmlimportscanner reports, plus QtQuick basics).
mods=$( "$ROOT/lib/qt6/qmlimportscanner" -rootPath apps/tv-shell/qml -importPath "$ROOT/lib/qt6/qml" 2>/dev/null | grep '"path"' | sed 's/.*: "\(.*\)".*/\1/' | sort -u || true)
for m in $mods QtQml QtQuick QtQuick/Controls QtQuick/Controls/Basic QtQuick/Layouts QtQuick/Templates QtQuick/Window QtQml/Models QtQml/WorkerScript; do
  src="${m#$ROOT/lib/qt6/qml/}"; src="$ROOT/lib/qt6/qml/$src"
  [ -d "$src" ] || continue
  rel="${src#$ROOT/lib/qt6/qml/}"
  mkdir -p "$OUT/qml/$(dirname "$rel")"; cp -rn "$src" "$OUT/qml/$(dirname "$rel")/" 2>/dev/null || true
done
find "$OUT/qml" -name '*.so' | while read -r p; do copy_deps "$p"; done
# Never bundle glibc pieces.
rm -f "$OUT"/lib/{libc.so*,libm.so*,libpthread.so*,libdl.so*,librt.so*,ld-linux*}
cp packaging/bear-den-tv-shell.sh "$OUT/bin/bear-den-tv-shell"; chmod +x "$OUT/bin/bear-den-tv-shell"
du -sh "$OUT"; ls "$OUT/lib" | wc -l | xargs echo "libraries:"
