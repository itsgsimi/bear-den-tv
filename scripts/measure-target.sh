#!/usr/bin/env bash
# Measures CPU (% of one core) and memory of the Bear Den processes on the TV
# over a window, from /proc tick counters (no extra tools needed).
#   scripts/measure-target.sh [seconds]      # default 20
set -euo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/target-env.sh"
SECS="${1:-20}"
ssh -o BatchMode=yes "$TARGET" "SECS=$SECS bash -s" <<'REMOTE'
hz=$(getconf CLK_TCK)
declare -A a
pids="$(pgrep -u "$(id -u)" -x bear-den-tv-she || true) $(pgrep -u "$(id -u)" -x bear-den-tv || true)"
for p in $pids; do a[$p]=$(awk '{print $14+$15}' /proc/$p/stat); done
sleep "$SECS"
for p in $pids; do
  [ -r /proc/$p/stat ] || continue
  b=$(awk '{print $14+$15}' /proc/$p/stat)
  cpu=$(awk -v d=$((b - a[$p])) -v hz=$hz -v s=$SECS 'BEGIN{printf "%.1f", d*100/(hz*s)}')
  rss=$(awk '/VmRSS/{printf "%d", $2/1024}' /proc/$p/status)
  printf '%-16s pid %-8s %6s%% of one core  RSS %4s MB\n' "$(cat /proc/$p/comm)" "$p" "$cpu" "$rss"
done
printf 'load average: %s\n' "$(cut -d' ' -f1-3 /proc/loadavg)"
REMOTE
