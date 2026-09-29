#!/usr/bin/env bash
# `make package`: builds build/dist/bear-den-tv_<version>_amd64.deb
# (docs/operations.md → Packaging; package definition in packaging/nfpm.yaml).
#
#   1. the coordinator/CLI, static (CGO_ENABLED=0: every Go dependency is pure
#      Go, SQLite included), with the version stamped in;
#   2. the TV shell against the glibc 2.28 sysroot with no rpath at all (the
#      bundle's wrapper sets LD_LIBRARY_PATH; a DT_RPATH into the toolchain
#      would win over it and load this machine's Qt);
#   3. the shell + its Qt runtime bundled by packaging/bundle-qt.sh;
#   4. checks that fail the build: an rpath/runpath into the toolchain, or a
#      shipped ELF needing a glibc newer than 2.28;
#   5. nfpm writes the .deb.
# The package enables nothing at install time (autostart stays opt-in).
#   VERSION  override the version (default: packaging/version.sh)
set -euo pipefail
cd "$(dirname "$0")/.."
TC="${BDTV_TOOLCHAIN:-$HOME/.bdtv-toolchain}"
export PATH="$TC/env/bin:$PATH" GOTOOLCHAIN=local
SYSROOT="$TC/sysroot-2.28/x86_64-conda-linux-gnu/sysroot"
STAGE=build/package
DIST=build/dist
GLIBC_FLOOR=2.28

command -v nfpm >/dev/null || { echo "nfpm not found: run scripts/bootstrap-toolchain.sh" >&2; exit 1; }
[ -d "$SYSROOT" ] || { echo "missing $SYSROOT: run scripts/bootstrap-toolchain.sh" >&2; exit 1; }
VERSION="$(packaging/version.sh)"
export VERSION
echo "[package] version $VERSION"
rm -rf "$STAGE"
mkdir -p "$STAGE" "$DIST"

echo "[package] coordinator (static)"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$VERSION" -o "$STAGE/bear-den-tv" ./cmd/bear-den-tv

echo "[package] shell (glibc $GLIBC_FLOOR sysroot, no rpath)"
# conda's GCC specs add "-rpath <toolchain>/env/lib" to every link. A specs
# file that redefines only *link_command without it, passed with -specs=,
# drops that (the later definition wins; a full dump would also override the
# sysroot handling and break linking against the 2.28 sysroot).
mkdir -p build/tv-shell-package
SPECS="$PWD/build/tv-shell-package/norpath.specs"
x86_64-conda-linux-gnu-c++ -dumpspecs | awk '/^\*link_command:/{p=1} p{print} p&&/^$/{exit}' | sed 's|-rpath [^ ]* ||' >"$SPECS"
grep -q '^\*link_command:' "$SPECS" || { echo "no *link_command in the GCC specs" >&2; exit 1; }
if grep -q -- "-rpath $TC" "$SPECS"; then echo "could not drop the toolchain rpath from the GCC specs" >&2; exit 1; fi
CC=x86_64-conda-linux-gnu-cc CXX=x86_64-conda-linux-gnu-c++ cmake -S apps/tv-shell -B build/tv-shell-package -G Ninja \
  -DCMAKE_BUILD_TYPE=Release -DBDTV_BUILD_TESTS=OFF \
  -DCMAKE_C_FLAGS="--sysroot=$SYSROOT" -DCMAKE_CXX_FLAGS="--sysroot=$SYSROOT" \
  -DCMAKE_SKIP_RPATH=ON \
  -DCMAKE_EXE_LINKER_FLAGS="--sysroot=$SYSROOT -specs=$SPECS" >/dev/null
cmake --build build/tv-shell-package
x86_64-conda-linux-gnu-strip -o "$STAGE/bear-den-tv-shell" build/tv-shell-package/bear-den-tv-shell

echo "[package] bundle Qt"
packaging/bundle-qt.sh "$STAGE/bear-den-tv-shell" "$STAGE/shell"
rm -f "$STAGE/bear-den-tv-shell"

echo "[package] checks"
fail=0
while IFS= read -r f; do
  file -b "$f" | grep -q '^ELF' || continue
  if readelf -d "$f" 2>/dev/null | grep -E 'R(UN)?PATH' | grep -q "$TC"; then
    echo "  rpath into the toolchain: $f" >&2; fail=1
  fi
  need=$({ objdump -T "$f" 2>/dev/null || true; } | { grep -o 'GLIBC_[0-9.]*' || true; } | sed 's/GLIBC_//' | sort -Vu | tail -1)
  if [ -n "$need" ] && [ "$(printf '%s\n%s\n' "$need" "$GLIBC_FLOOR" | sort -V | tail -1)" != "$GLIBC_FLOOR" ]; then
    echo "  needs glibc $need > $GLIBC_FLOOR: $f" >&2; fail=1
  fi
done < <(find "$STAGE" -type f)
[ "$fail" = 0 ] || { echo "[package] checks failed" >&2; exit 1; }
# Reported, not fatal: conda patches its install prefix into some libraries
# (Qt's compiled-in prefix, fontconfig/glib/dbus data paths). The bundle's
# qt.conf and the wrapper's environment override the ones that matter.
leaks=$(grep -rlaF "$TC" "$STAGE" || true)
if [ -n "$leaks" ]; then
  echo "[package] note: $(echo "$leaks" | wc -l) shipped files mention the toolchain path $TC:"
  echo "$leaks" | sed 's/^/  /'
fi

echo "[package] nfpm"
# Package modes do not depend on this machine's umask: 0755 dirs and
# executables, 0644 everything else.
chmod -R u=rwX,go=rX "$STAGE"
nfpm package -f packaging/nfpm.yaml -p deb -t "$DIST/"
ls -l "$DIST"/bear-den-tv_"$VERSION"_amd64.deb
