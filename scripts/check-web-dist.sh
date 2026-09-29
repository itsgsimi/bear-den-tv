#!/usr/bin/env bash
# Fails when the committed phone remote build (apps/remote-web/dist, embedded by
# Go via embed.go) differs from a fresh `make web`. The build is deterministic
# (apps/remote-web/scripts/build.mjs: no hashes or timestamps), so any difference
# means someone changed apps/remote-web without rebuilding and committing dist.
# Run by CI (.github/workflows/ci.yml) and by `make check-web-dist`.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
DIST=apps/remote-web/dist
if ! out="$(make --no-print-directory web 2>&1)"; then
  echo "$out" >&2
  echo "error: make web failed" >&2
  exit 1
fi
changes="$(git status --porcelain --untracked-files=all -- "$DIST")"
if [ -n "$changes" ]; then
  echo "error: $DIST does not match a fresh 'make web':" >&2
  echo "$changes" >&2
  echo "Run 'make web' and commit $DIST together with your apps/remote-web change." >&2
  exit 1
fi
echo "$DIST matches a fresh make web"
