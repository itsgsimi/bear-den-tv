#!/usr/bin/env bash
# Starts (or restarts) the Bear Den coordinator, which supervises the TV shell,
# in this user's graphical session, detached from the terminal.
#   start-session.sh          # (re)start once
#   start-session.sh --watch  # (re)start and restart the coordinator if it crashes
#   start-session.sh stop     # stop (and stop watching)
#   start-session.sh which    # print the coordinator and shell it would run
# Works from both layouts (docs/operations.md → Packaging):
#   checkout:  <repo>/scripts/start-session.sh → <repo>/build/bin/bear-den-tv
#   installed: <prefix>/lib/bear-den-tv/start-session.sh → <prefix>/bin/bear-den-tv
#              (the .deb installs it as /usr/lib/bear-den-tv/start-session.sh)
# In a checkout on the TV machine, e.g. `scripts/target.sh ssh scripts/start-session.sh`.
# `bear-den-tv autostart enable` runs this with --watch at every desktop login.
#
# Detached: what it starts runs in a new session (setsid) with no controlling
# terminal, stdin from /dev/null and stdout/stderr to the log, so closing the
# terminal or ssh session, or job-control signals, never reach it.
# Log: ${XDG_STATE_HOME:-~/.local/state}/bear-den-tv/session.log.
set -euo pipefail
here="$(readlink -f "$0")"
dir="$(dirname "$here")"
if [ -x "$dir/../build/bin/bear-den-tv" ]; then
  # Relative on purpose: the process line stays "build/bin/bear-den-tv
  # session", which stop_all here and deploy-target.sh match.
  cd "$dir/.."
  bin="build/bin/bear-den-tv"
  shell="build/bin/bear-den-tv-shell"
elif [ -x "$dir/../../bin/bear-den-tv" ]; then
  bin="$(cd "$dir/../../bin" && pwd)/bear-den-tv"
  shell="$(dirname "$bin")/bear-den-tv-shell"
  cd "$HOME"
else
  echo "bear-den-tv not found: looked for $dir/../build/bin/bear-den-tv (checkout) and $dir/../../bin/bear-den-tv (installed)" >&2
  exit 1
fi
if [ "${1:-}" = "which" ]; then realpath -ms "$bin" "$shell"; exit 0; fi
uid=$(id -u)
coord="$bin session"
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
    "$bin" session --shell-binary "$shell" "$@" || status=$?
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
# detached PID: wait until PID has left this session (setsid(2) done). Until
# then it is in this script's process group, and when this script is the
# session leader of a terminal (ssh -t, a terminal window) and exits, the
# kernel hangs that group up (SIGHUP) and the start is lost.
detached() {
  local i s
  for ((i = 0; i < 100; i++)); do
    s=$(cat "/proc/$1/stat" 2>/dev/null) || break
    s=${s##*) }
    set -- "$1" $s
    [ "$5" = "$1" ] && return 0   # state ppid pgrp session: its own session
    sleep 0.02
  done
  echo "not started: pid $1 did not become a session of its own (see $log)" >&2
  return 1
}

# setsid: a new session with no controlling terminal (see the header).
if [ "$watch" = 1 ]; then
  setsid bash "$here" __watch "$@" >>"$log" 2>&1 </dev/null &
  pid=$!
  detached "$pid" || exit 1
  echo "started with watchdog, pid $pid (log: $log)"
else
  session_env
  setsid "$bin" session --shell-binary "$shell" "$@" >>"$log" 2>&1 </dev/null &
  pid=$!
  detached "$pid" || exit 1
  echo "started pid $pid (log: $log)"
fi
