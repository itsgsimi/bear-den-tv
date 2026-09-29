#!/usr/bin/env bash
# Fails when a committed web build embedded by Go (embed.go) differs from a
# fresh build: the phone remote (apps/remote-web/dist, `make web`) and the
# web apps navigation script (apps/web-nav/dist, `make webnav`). Both builds
# are deterministic (no hashes or timestamps), so any difference means
# someone changed the sources without rebuilding and committing dist.
# Run by CI (.github/workflows/ci.yml) and by `make check-web-dist`.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
status=0
for pair in "web:apps/remote-web/dist" "webnav:apps/web-nav/dist"; do
  target=${pair%%:*}
  dist=${pair#*:}
  if ! out="$(make --no-print-directory "$target" 2>&1)"; then
    echo "$out" >&2
    echo "error: make $target failed" >&2
    exit 1
  fi
  changes="$(git status --porcelain --untracked-files=all -- "$dist")"
  if [ -n "$changes" ]; then
    echo "error: $dist does not match a fresh 'make $target':" >&2
    echo "$changes" >&2
    echo "Run 'make $target' and commit $dist together with its source change." >&2
    status=1
  else
    echo "$dist matches a fresh make $target"
  fi
done
exit $status
