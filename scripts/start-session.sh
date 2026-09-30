#!/usr/bin/env bash
# Starts (or restarts) the Bear Den coordinator, which supervises the TV shell,
# in this user's graphical session, detached from the terminal.
#   start-session.sh          # (re)start once
#   start-session.sh --watch  # (re)start with the watchdog (below)
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
# The watchdog (docs/operations.md → The watchdog) restarts the coordinator
# when it exits with an error, when it is stopped (state T in /proc), and when
# it stops answering `bear-den-tv doctor --ping` on the shell socket
# BDTV_WATCH_FAILURES times in a row: SIGCONT, SIGTERM, SIGKILL after
# BDTV_WATCH_GRACE seconds, then a fresh start. Every step is logged.
# stop finds processes by their exact executable (/proc/<pid>/exe) and
# argument list, never by a pattern in a command line, and continues a
# stopped process before asking it to end.
# Log: ${XDG_STATE_HOME:-~/.local/state}/bear-den-tv/session.log.
set -euo pipefail
here="$(readlink -f "$0")"
dir="$(dirname "$here")"
if [ -x "$dir/../build/bin/bear-den-tv" ]; then
  # Relative on purpose: the process line stays "build/bin/bear-den-tv
  # session", which deploy-target.sh matches.
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
bin_abs="$(realpath -ms "$bin")"
log="${XDG_STATE_HOME:-$HOME/.local/state}/bear-den-tv/session.log"
# Watchdog and stop timings, in whole seconds (tests shorten them).
watch_interval="${BDTV_WATCH_INTERVAL:-30}"  # between liveness checks
watch_failures="${BDTV_WATCH_FAILURES:-3}"   # failed pings in a row before a restart
watch_grace="${BDTV_WATCH_GRACE:-10}"        # SIGTERM → SIGKILL
ping_timeout="${BDTV_WATCH_PING_TIMEOUT:-5}" # one ping's answer
stop_timeout="${BDTV_STOP_TIMEOUT:-10}"      # stop: SIGTERM → SIGKILL

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

# proc_state PID: the one-letter state from /proc/PID/stat (R, S, D, T, Z...),
# empty when the process is gone.
proc_state() {
  local stat
  stat=$(cat "/proc/$1/stat" 2>/dev/null) || return 0
  stat=${stat##*) }   # the command name may contain spaces and parentheses
  echo "${stat%% *}"
}

# argv PID: the process's arguments, one per line.
argv() { tr '\0' '\n' <"/proc/$1/cmdline" 2>/dev/null; }

# coordinator_pids: this user's coordinators of this layout: executable
# $bin_abs (also after it was replaced on disk, "(deleted)") and first
# argument "session". A command line that only mentions the name (an ssh
# command, an editor) never matches.
coordinator_pids() {
  local d exe
  for d in /proc/[0-9]*; do
    [ -O "$d" ] || continue
    exe=$(readlink "$d/exe" 2>/dev/null) || continue
    [ "${exe% (deleted)}" = "$bin_abs" ] || continue
    [ "$(argv "${d#/proc/}" | sed -n 2p)" = "session" ] && echo "${d#/proc/}"
  done
  return 0
}

# watcher_pids: this user's watchdogs of this script: argument list exactly
# "bash <this script> __watch ...".
watcher_pids() {
  local d p
  for d in /proc/[0-9]*; do
    [ -O "$d" ] || continue
    p=${d#/proc/}
    [ "$p" = "$$" ] && continue
    [ "$(argv "$p" | sed -n 1,3p | paste -sd ' ')" = "bash $here __watch" ] && echo "$p"
  done
  return 0
}

# alive PIDS...: prints those still running (not gone, not a zombie).
alive() {
  local p s
  for p in "$@"; do
    s=$(proc_state "$p")
    [ -n "$s" ] && [ "$s" != Z ] && echo "$p"
  done
  return 0
}

# stop_all: watchdogs first (so none restarts the coordinator meanwhile), then
# the coordinators: SIGCONT (a stopped process holds SIGTERM until it runs
# again), SIGTERM, up to $stop_timeout s, then SIGKILL. Fails with a reason
# when something is still running after that.
stop_all() {
  local pids left i
  pids="$(watcher_pids) $(coordinator_pids)"
  pids=$(echo $pids)
  [ -z "$pids" ] && return 0
  kill -CONT $pids 2>/dev/null || true
  kill -TERM $pids 2>/dev/null || true
  for ((i = 0; i < stop_timeout * 5; i++)); do
    left=$(alive $pids)
    [ -z "$left" ] && return 0
    sleep 0.2
  done
  left=$(echo $left)
  echo "still running after ${stop_timeout}s: $left; sending SIGKILL" >&2
  kill -KILL $left 2>/dev/null || true
  for ((i = 0; i < 10; i++)); do
    left=$(alive $left)
    [ -z "$left" ] && return 0
    sleep 0.2
  done
  echo "could not stop pid $(echo $left) (state $(proc_state "${left%% *}"))" >&2
  return 1
}

say() { echo "$(date -Is) watchdog: $*" >&2; }

# Internal: keep the coordinator running and answering; a clean exit (stop,
# SIGTERM at logout) ends it.
if [ "${1:-}" = "__watch" ]; then
  shift
  session_env
  if [ "${BASH_VERSINFO[0]}" -lt 5 ] || { [ "${BASH_VERSINFO[0]}" -eq 5 ] && [ "${BASH_VERSINFO[1]}" -lt 1 ]; }; then
    say "needs bash 5.1 or newer (wait -n -p), have $BASH_VERSION; not starting"
    exit 1
  fi
  delay=1
  while true; do
    started=$(date +%s)
    "$bin" session --shell-binary "$shell" "$@" &
    coord=$!
    say "started coordinator pid $coord"
    fails=0
    restart=""  # why the watchdog ended it, or empty
    status=0
    while true; do
      sleep "$watch_interval" &
      nap=$!
      ended=""
      wait -n -p ended "$coord" "$nap" || status=$?
      if [ "$ended" = "$coord" ]; then
        kill "$nap" 2>/dev/null || true
        wait "$nap" 2>/dev/null || true
        break
      fi
      status=0
      state=$(proc_state "$coord")
      if [ "$state" = T ]; then
        restart="is stopped (state T in /proc/$coord/stat)"
      elif ping_out=$("$bin" doctor --ping --timeout "${ping_timeout}s" 2>&1); then
        [ "$fails" -gt 0 ] && say "coordinator pid $coord answers again after $fails failed ping(s)"
        fails=0
        continue
      else
        fails=$((fails + 1))
        say "coordinator pid $coord did not answer a ping ($fails of $watch_failures): ${ping_out//$'\n'/ }"
        [ "$fails" -ge "$watch_failures" ] && restart="did not answer $fails pings in a row"
      fi
      [ -z "$restart" ] && continue
      # Recover: a frozen coordinator serves nothing (not even playback
      # control), so it is restarted whatever is on screen.
      say "coordinator pid $coord $restart; sending SIGCONT and SIGTERM"
      kill -CONT "$coord" 2>/dev/null || true
      kill -TERM "$coord" 2>/dev/null || true
      sleep "$watch_grace" &
      nap=$!
      ended=""
      wait -n -p ended "$coord" "$nap" || true
      if [ "$ended" = "$coord" ]; then
        kill "$nap" 2>/dev/null || true
        wait "$nap" 2>/dev/null || true
        say "coordinator pid $coord ended after SIGTERM"
      else
        say "coordinator pid $coord still running ${watch_grace}s after SIGTERM; sending SIGKILL"
        kill -KILL "$coord" 2>/dev/null || true
        wait "$coord" 2>/dev/null || true
        say "coordinator pid $coord killed"
      fi
      break
    done
    if [ -z "$restart" ]; then
      [ "$status" -eq 0 ] && { say "coordinator pid $coord exited cleanly; watch ends"; exit 0; }
      # Signals used by stop/logout end the watch too.
      case "$status" in 130|143) say "coordinator pid $coord ended by signal ($status); watch ends"; exit 0 ;; esac
      say "coordinator pid $coord exited with $status"
    fi
    [ $(( $(date +%s) - started )) -gt 60 ] && delay=1
    say "restarting in ${delay}s"
    sleep "$delay"
    delay=$(( delay < 30 ? delay * 2 : 30 ))
  done
fi

if ! stop_all; then
  echo "not starting: an earlier Bear Den is still running (see above)" >&2
  exit 1
fi
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
