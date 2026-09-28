#!/usr/bin/env bash
# Syncs this checkout to the TV target and runs a command there inside the toolchain env.
#   scripts/target.sh sync            # rsync only
#   scripts/target.sh run <cmd...>    # sync, then run <cmd> in ~/bear-den-tv on the target
#   scripts/target.sh ssh <cmd...>    # run without syncing
# Target: BDTV_TARGET, dir BDTV_TARGET_DIR (default ~/bear-den-tv), from target.env (scripts/target-env.sh)
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$HERE/scripts/target-env.sh"
sync() {
  rsync -az --delete \
    --exclude '.git/' --exclude 'build/' --exclude 'node_modules/' --exclude '.toolchain/' \
    --exclude 'apps/remote-web/node_modules/' --exclude 'apps/tv-shell/build/' \
    "$HERE/" "$TARGET:$TDIR/"
}
case "${1:-}" in
  sync) sync ;;
  run) shift; sync; ssh -o BatchMode=yes "$TARGET" "cd $TDIR && . scripts/env.sh && $*" ;;
  ssh) shift; ssh -o BatchMode=yes "$TARGET" "cd $TDIR && . scripts/env.sh && $*" ;;
  *) echo "usage: $0 sync|run <cmd>|ssh <cmd>" >&2; exit 2 ;;
esac
