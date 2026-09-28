# Sourced by the *-target.sh scripts: loads the TV machine's address from the
# untracked target.env at the repo root (see target.env.example and
# docs/operations.md "Point the scripts at your TV"). Variables already set in
# the environment win over the file. Sets TARGET and TDIR; fails with a clear
# message when BDTV_TARGET is not configured.
_bdtv_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -f "$_bdtv_root/target.env" ]; then
  _bdtv_saved="$(declare -p BDTV_TARGET BDTV_TARGET_DIR BDTV_TARGET_HEALTH_CMD BDTV_TARGET_TOOLCHAIN 2>/dev/null || true)"
  # shellcheck disable=SC1091
  . "$_bdtv_root/target.env"
  eval "$_bdtv_saved"
  unset _bdtv_saved
fi
if [ -z "${BDTV_TARGET:-}" ]; then
  echo "BDTV_TARGET is not set: copy target.env.example to target.env and set your TV's ssh destination (e.g. you@tv.local)" >&2
  exit 2
fi
TARGET="$BDTV_TARGET"
TDIR="${BDTV_TARGET_DIR:-bear-den-tv}"
BDTV_TARGET_HEALTH_CMD="${BDTV_TARGET_HEALTH_CMD:-}"
unset _bdtv_root
