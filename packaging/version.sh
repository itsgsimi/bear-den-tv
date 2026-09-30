#!/usr/bin/env bash
# Prints the package version as a valid Debian version, from
# `git describe --tags --always --dirty` (docs/operations.md → Packaging):
#   v1.2.0                   → 1.2.0
#   v1.2.0-rc1               → 1.2.0~rc1
#   v1.2.0-3-gabc1234-dirty  → 1.2.0+git3.gabc1234.dirty
#   abc1234 (no tags yet)    → 0.1.0~git<N>.abc1234 (N = commits on HEAD, so a
#                              later build always sorts higher; all before 0.1.0)
#   no git at all            → 0.1.0~unknown
# VERSION in the environment wins, unchanged.
#   usage: packaging/version.sh [describe-string [commit-count]]   (arguments: for tests)
set -euo pipefail
if [ -n "${VERSION:-}" ]; then echo "$VERSION"; exit 0; fi
BASE=0.1.0
d="${1:-$(git -C "$(dirname "$0")/.." describe --tags --always --dirty 2>/dev/null || true)}"
if [ -z "$d" ]; then echo "$BASE~unknown"; exit 0; fi
d="${d#v}"
dirty=""
if [[ "$d" == *-dirty ]]; then d="${d%-dirty}"; dirty=1; fi
if [[ "$d" =~ ^([0-9]+(\.[0-9A-Za-z]+)+)(-([A-Za-z][0-9A-Za-z.]*))?(-([0-9]+)-(g[0-9a-f]+))?$ ]]; then
  v="${BASH_REMATCH[1]}"
  if [ -n "${BASH_REMATCH[4]}" ]; then v="$v~${BASH_REMATCH[4]}"; fi  # 1.2.0-rc1 → 1.2.0~rc1
  sep=+
  if [ -n "${BASH_REMATCH[6]}" ]; then v="$v+git${BASH_REMATCH[6]}.${BASH_REMATCH[7]}"; sep=.; fi
else
  # An untagged commit: "<sha>". The commit count comes first so versions
  # rise with every commit (a bare hash would sort by its letters).
  n="${2:-$(git -C "$(dirname "$0")/.." rev-list --count HEAD 2>/dev/null || echo 0)}"
  v="$BASE~git$n.$d"
  sep=.
fi
if [ -n "$dirty" ]; then v="$v${sep}dirty"; fi
echo "$v"
