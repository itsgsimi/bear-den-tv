#!/usr/bin/env bash
# Release guard (docs/operations.md → Cutting a release): fails if any file in
# the .deb mentions a /home/<name> path other than the allowed ones.
#
# Why: conda patches the toolchain's install prefix into a few bundled
# libraries (`make package` lists them: fontconfig, xcb-cursor, glib, libuuid,
# libcrypto). Built on a workstation, that prefix is ~/.bdtv-toolchain and
# carries the builder's user name. Built on the GitHub runner it is
# /home/runner/..., which names nobody. /home/conda/ is conda-forge's own build
# path, compiled into upstream libraries.
#
#   packaging/check-home-paths.sh [deb]   default: newest build/dist/bear-den-tv_*_amd64.deb
#   ALLOW_HOMES="runner conda"            the /home/<name> entries that pass
set -euo pipefail
cd "$(dirname "$0")/.."
deb="${1:-$(ls -t build/dist/bear-den-tv_*_amd64.deb 2>/dev/null | head -1)}"
[ -n "$deb" ] && [ -f "$deb" ] || { echo "no .deb given and none in build/dist: run make package" >&2; exit 1; }
allow="${ALLOW_HOMES:-runner conda}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
dpkg-deb -x "$deb" "$tmp"

# One line per (file, /home/<name>) pair; the same grep -a as build-deb.sh's
# toolchain-path note, widened from that one prefix to any home directory.
hits=$(cd "$tmp" && grep -raoE '/home/[A-Za-z0-9._-]+' . 2>/dev/null | sed 's|^\./|/|' | sort -u || true)
bad=""
while IFS= read -r line; do
  [ -n "$line" ] || continue
  name="${line##*/home/}"
  ok=0
  for a in $allow; do [ "$name" = "$a" ] && ok=1; done
  [ "$ok" = 1 ] || bad+="$line"$'\n'
done <<<"$hits"

total=$(printf '%s' "$hits" | grep -c . || true)
echo "[home-paths] $deb: $total file/path pairs mention /home/<name>; allowed: $allow"
if [ -n "$bad" ]; then
  echo "[home-paths] FAIL: shipped files mention other home directories (file:path):" >&2
  printf '%s' "$bad" | sed 's/^/  /' >&2
  exit 1
fi
[ -z "$hits" ] || printf '%s\n' "$hits" | sed 's/^/  allowed  /'
echo "[home-paths] PASS"
