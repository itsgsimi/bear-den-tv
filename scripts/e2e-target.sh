#!/usr/bin/env bash
# Runs the end-to-end suite (tests/e2e, build tag e2e) against the live Bear Den
# session on the TV machine and copies the screenshots back.
#   scripts/e2e-target.sh [go test flags, e.g. -run 'TestTargetEndToEnd/App/plex-htpc']
# Requires: the TV session unlocked, and the session started with the loopback
# test listener: scripts/target.sh ssh 'bash scripts/start-session.sh --watch --dev-listen 127.0.0.1:18791'
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$HERE/scripts/target-env.sh"
cd "$HERE"
. scripts/env.sh >/dev/null
mkdir -p build .tmp/e2e-shots
go test -c -tags e2e -o build/e2e.test ./tests/e2e
scp -q build/e2e.test "$TARGET:$TDIR/build/e2e.test"
status=0
ssh -o BatchMode=yes "$TARGET" "cd $TDIR && rm -rf .tmp/e2e-shots && mkdir -p .tmp/e2e-shots && \
  DISPLAY=:0 XAUTHORITY=\$HOME/.Xauthority XDG_RUNTIME_DIR=/run/user/\$(id -u) \
  BDTV_E2E_BIN=\$PWD/build/bin/bear-den-tv BDTV_E2E_SHOTS=\$PWD/.tmp/e2e-shots \
  ./build/e2e.test -test.v -test.count=1 -test.timeout=20m $*" || status=$?
rm -rf .tmp/e2e-shots && mkdir -p .tmp/e2e-shots
scp -q "$TARGET:$TDIR/.tmp/e2e-shots/*.png" .tmp/e2e-shots/ 2>/dev/null || true
echo "screenshots: $HERE/.tmp/e2e-shots"
exit $status
