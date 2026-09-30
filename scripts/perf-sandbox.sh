#!/usr/bin/env bash
# Performance sandbox: runs the real TV shell here, offscreen, on the demo
# fixture, squeezed onto two CPU cores under a CPU quota (a stand-in for the
# reference Celeron), and measures each phase of Home's life:
#
#   awake       just started, nobody touching the remote
#   resting     after BDTV_REST_SECONDS without input (45 s on the TV)
#   screensaver after BDTV_SCREENSAVER_SECONDS (5 min on the TV)
#
# For each phase it prints frames per second (BDTV_FPS_LOG) and CPU use, and
# fails when a phase that should be quiet keeps drawing (budgets below).
#
# What it is and isn't: offscreen Qt draws with the CPU, not the TV's GPU, and
# the quota only approximates the Celeron. Use it to catch animations that
# never stop and to compare before/after a change. Absolute numbers come from
# the TV: scripts/measure-target.sh (docs/operations.md).
#
# Bear visits are switched off unless --bears: a visit legitimately draws at
# 20 fps for its few seconds and would make the quiet-phase budgets random.
#
# --tips turns bear tips on with a 3 s quiet spell: a tip bear walks in while
# awake and sits waiting by its sign through resting (it must draw nothing
# new), then leaves for the screensaver.
#
# Usage: scripts/perf-sandbox.sh [--quota 50%] [--fixture PATH] [--theme ID] [--bears] [--tips]
set -euo pipefail
cd "$(dirname "$0")/.."

QUOTA=${BDTV_SANDBOX_QUOTA:-50%}
FIXTURE=apps/tv-shell/tests/fixtures/state.demo.json
THEME=""
BEARS=100000
TIPS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --quota) QUOTA=$2; shift 2 ;;
    --fixture) FIXTURE=$2; shift 2 ;;
    --theme) THEME=$2; shift 2 ;;
    --bears) BEARS=""; shift ;;
    --tips) TIPS=3; shift ;;
    *) echo "usage: $0 [--quota 50%] [--fixture PATH] [--theme ID] [--bears] [--tips]" >&2; exit 2 ;;
  esac
done

# Budgets (frames per second, averaged over the phase). Everything moves on
# World's heartbeat (qml/World.qml): 20 beats/s awake, 4 resting. A beat costs
# one frame, plus one per Canvas it repaints (Qt paints a Canvas during a frame,
# then shows it in the next); the focused decoration has two Canvas corners, so
# resting is at most 4 × 3 = 12. The screensaver moves once every 15 s.
REST_FPS_MAX=12
SAVER_FPS_MAX=1

REST=20 SAVER=45 END=70
SHELL_BIN=build/bin/bear-den-tv-shell
[ -x "$SHELL_BIN" ] || make -s shell
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fixture=$FIXTURE
if [ -n "$THEME" ] || [ -n "$TIPS" ]; then
  fixture=$work/state.json
  python3 - "$FIXTURE" "$fixture" "$THEME" "$TIPS" <<'EOF'
import json, sys
d = json.load(open(sys.argv[1]))
if sys.argv[3]:
    d["layout"]["ui"]["background"] = sys.argv[3]
if sys.argv[4]:
    d["tips"] = {"enabled": True, "done": [], "stopped": False}
    d["onboarding"] = {"completed": True}
json.dump(d, open(sys.argv[2], "w"))
EOF
fi

run=(env ${BEARS:+BDTV_BEARS_SECONDS=$BEARS} ${TIPS:+BDTV_TIP_SECONDS=$TIPS} QT_QPA_PLATFORM=offscreen BDTV_FPS_LOG=1 BDTV_REST_SECONDS=$REST BDTV_SCREENSAVER_SECONDS=$SAVER
     taskset -c 0,1 "$SHELL_BIN" --dev --fixture "$fixture" --windowed --size 1920x1080 --exit-after $((END * 1000)))
if systemd-run --user --scope --quiet -p CPUQuota=100% true 2>/dev/null; then
  run=(systemd-run --user --scope --quiet -p "CPUQuota=$QUOTA" "${run[@]}")
else
  echo "note: no user systemd; running without a CPU quota (two cores only)" >&2
fi

start=$(date +%s.%N)
"${run[@]}" 2> >(while IFS= read -r line; do
  printf '%s %s\n' "$(awk -v s="$start" -v n="$(date +%s.%N)" 'BEGIN{printf "%.1f", n-s}')" "$line"
done > "$work/log") &
launcher=$!

# Sample the shell's CPU time (utime+stime ticks) at each phase boundary.
pid=""
for _ in $(seq 50); do
  pid=$(pgrep -n -f "$SHELL_BIN --dev --fixture" || true)
  [ -n "$pid" ] && break
  sleep 0.1
done
[ -n "$pid" ] || { echo "shell did not start" >&2; exit 1; }
ticks() { awk '{print $14 + $15}' "/proc/$pid/stat" 2>/dev/null || echo 0; }
hz=$(getconf CLK_TCK)
declare -A cpu
mark() { # mark NAME AT_SECONDS
  local now; now=$(awk -v s="$start" -v n="$(date +%s.%N)" 'BEGIN{print n-s}')
  sleep "$(awk -v a="$2" -v n="$now" 'BEGIN{d=a-n; print (d>0?d:0)}')"
  cpu[$1]=$(ticks)
}
mark awake0 3; mark awake1 $((REST - 2))
mark rest0 $((REST + 3)); mark rest1 $((SAVER - 2))
mark saver0 $((SAVER + 3)); mark saver1 $((END - 2))
wait "$launcher" || true

fps() { # average of fps lines logged between two times
  awk -v a="$1" -v b="$2" '$2=="bdtv" && $3=="fps:" && $1>=a && $1<=b {s+=$4; n++} END{printf "%.1f", n? s/n : 0}' "$work/log"
}
pct() { awk -v t0="${cpu[$1]}" -v t1="${cpu[$2]}" -v hz="$hz" -v d="$3" 'BEGIN{printf "%.0f", (t1-t0)/hz/d*100}'; }

# fps lines cover the 5 s before them, so each window starts 7 s into its phase
# to leave out the transition (grow, fade to rest, screensaver fade-in).
a_fps=$(fps 7 $((REST - 1))); r_fps=$(fps $((REST + 7)) $((SAVER - 1))); s_fps=$(fps $((SAVER + 7)) $((END - 1)))
printf '%-12s %8s %16s\n' phase fps "CPU (% of a core)"
printf '%-12s %8s %16s\n' awake "$a_fps" "$(pct awake0 awake1 $((REST - 5)))"
printf '%-12s %8s %16s\n' resting "$r_fps" "$(pct rest0 rest1 $((SAVER - REST - 5)))"
printf '%-12s %8s %16s\n' screensaver "$s_fps" "$(pct saver0 saver1 $((END - SAVER - 5)))"
echo "(offscreen software rendering on 2 cores at a $QUOTA quota; compare runs, not absolute TV numbers)"

fail=0
awk -v v="$r_fps" -v m=$REST_FPS_MAX 'BEGIN{exit !(v>m)}' && { echo "FAIL: resting draws $r_fps fps (budget $REST_FPS_MAX)"; fail=1; }
awk -v v="$s_fps" -v m=$SAVER_FPS_MAX 'BEGIN{exit !(v>m)}' && { echo "FAIL: screensaver draws $s_fps fps (budget $SAVER_FPS_MAX)"; fail=1; }
exit $fail
