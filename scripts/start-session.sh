#!/usr/bin/env bash
# Starts (or restarts) the Bear Den coordinator, which supervises the TV shell,
# in this user's graphical session, detached from the terminal.
#   scripts/start-session.sh          # (re)start once
#   scripts/start-session.sh --watch  # (re)start and restart the coordinator if it crashes
#   scripts/start-session.sh stop     # stop (and stop watching)
# Run on the TV machine, e.g. `scripts/target.sh ssh scripts/start-session.sh`.
# `bear-den-tv autostart enable` runs this with --watch at every desktop login.
# Log: ${XDG_STATE_HOME:-~/.local/state}/bear-den-tv/session.log.
set -euo pipefail
cd "$(dirname "$0")/.."
here="$PWD/scripts/start-session.sh"
uid=$(id -u)
coord="build/bin/bear-den-tv session"
watcher="start-session.sh __watch"
log="${XDG_STATE_HOME:-$HOME/.local/state}/bear-den-tv/session.log"

session_env() {
  export DISPLAY="${DISPLAY:-:0}"
  export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"
  export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$uid}"
  export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"
  export XDG_SESSION_TYPE=x11
  if [ -z "${XDG_SESSION_ID:-}" ]; then
    XDG_SESSION_ID=$(loginctl list-sessions --no-legend | awk -v u="$(id -un)" '$3 == u && $4 == "seat0" { print $1; exit }')
    export XDG_SESSION_ID
  fi
  # Mint 21.3 / Haswell: GLX can deadlock under Qt 6; EGL works (ADR 0001).
  export QT_XCB_GL_INTEGRATION="${QT_XCB_GL_INTEGRATION:-xcb_egl}"
}

stop_all() {
  pkill -u "$uid" -f "$watcher" 2>/dev/null || true
  pkill -u "$uid" -f "$coord" 2>/dev/null || true
  for _ in $(seq 1 20); do pgrep -u "$uid" -f "$coord" >/dev/null || return 0; sleep 0.5; done
}

# Internal: keep the coordinator running; a clean exit (stop, SIGTERM) ends it.
if [ "${1:-}" = "__watch" ]; then
  shift
  session_env
  delay=1
  while true; do
    started=$(date +%s)
    status=0
    build/bin/bear-den-tv session --shell-binary build/bin/bear-den-tv-shell "$@" || status=$?
    [ "$status" -eq 0 ] && exit 0
    # Signals used by stop/logout end the watch too.
    case "$status" in 130|143) exit 0 ;; esac
    [ $(( $(date +%s) - started )) -gt 60 ] && delay=1
    echo "$(date -Is) bear-den-tv exited with $status; restarting in ${delay}s" >&2
    sleep "$delay"
    delay=$(( delay < 30 ? delay * 2 : 30 ))
  done
fi

stop_all
[ "${1:-}" = "stop" ] && { echo "stopped"; exit 0; }

watch=0
if [ "${1:-}" = "--watch" ]; then watch=1; shift; fi
mkdir -p "$(dirname "$log")"
# Keep the log bounded on small TV boxes: rotate at 5 MB, keep one old copy.
if [ -f "$log" ] && [ "$(stat -c %s "$log")" -gt 5242880 ]; then mv -f "$log" "$log.1"; fi
if [ "$watch" = 1 ]; then
  setsid bash "$here" __watch "$@" >>"$log" 2>&1 </dev/null &
  echo "started with watchdog, pid $! (log: $log)"
else
  session_env
  setsid build/bin/bear-den-tv session --shell-binary build/bin/bear-den-tv-shell "$@" >>"$log" 2>&1 </dev/null &
  echo "started pid $! (log: $log)"
fi
