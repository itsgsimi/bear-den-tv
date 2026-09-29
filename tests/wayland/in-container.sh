#!/bin/bash
# Runs inside the bdtv-wayland-test container (tests/wayland/Dockerfile),
# started by scripts/wayland-container-test.sh. Starts headless sway with
# XWayland, opens two foot terminals with distinct app_ids, then:
#   1. the live adapter test (tests/wayland/live_test.go): list, observe,
#      activate, fullscreen, close, cross-checked against swaymsg;
#   2. `bear-den-tv doctor --probe`: the capability report on sway;
#   3. when the TV shell is mounted: `bear-den-tv session` supervising the
#      real shell (xcb on XWayland), then `doctor` against that coordinator.
# Artifacts land in /out. Plan: docs/decisions/0007-wayland-profile.md.
set -euo pipefail

BIN=/bdtv
OUT=/out
export HOME=/tmp/home XDG_RUNTIME_DIR=/tmp/xdg
mkdir -p "$HOME" && mkdir -p -m 700 "$XDG_RUNTIME_DIR"
export WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman
export XDG_SESSION_TYPE=wayland XDG_CURRENT_DESKTOP=sway

printf 'xwayland enable\noutput HEADLESS-1 resolution 1920x1080\n' >"$HOME/sway.conf"
sway -c "$HOME/sway.conf" >"$OUT/sway.log" 2>&1 &
for _ in $(seq 100); do
  ls "$XDG_RUNTIME_DIR"/sway-ipc.* >/dev/null 2>&1 && break
  sleep 0.1
done
WAYLAND_DISPLAY=$(cd "$XDG_RUNTIME_DIR" && ls wayland-* | grep -v '\.lock$' | head -1)
SWAYSOCK=$(ls "$XDG_RUNTIME_DIR"/sway-ipc.*)
export WAYLAND_DISPLAY SWAYSOCK
echo "sway up on $WAYLAND_DISPLAY"

wait_app() { # app_id: wait until sway maps a view with it
  for _ in $(seq 100); do
    swaymsg -r -t get_tree | grep -q "\"$1\"" && return 0
    sleep 0.1
  done
  echo "window $1 never appeared" >&2
  return 1
}

foot --app-id=bdtv.test.one sh -c 'sleep 600' >/dev/null 2>&1 &
wait_app bdtv.test.one
foot --app-id=bdtv.test.two sh -c 'sleep 600' >/dev/null 2>&1 &
wait_app bdtv.test.two

echo "== 1. live adapter test"
BDTV_WL_APPS=bdtv.test.one,bdtv.test.two "$BIN/wayland.test" -test.v -test.count=1 2>&1 | tee "$OUT/live-test.txt"

echo "== 2. bear-den-tv doctor --probe"
dbus-run-session -- "$BIN/bear-den-tv" doctor --probe >"$OUT/doctor-probe.json" 2>"$OUT/doctor-probe.err"
cat "$OUT/doctor-probe.json"

if [ -x "$BIN/bear-den-tv-shell" ]; then
  echo "== 3. coordinator + TV shell on sway (shell via XWayland)"
  DISPLAY=:$(ls /tmp/.X11-unix/ | sed -n 's/^X//p' | head -1)
  export DISPLAY
  dbus-run-session -- "$BIN/bear-den-tv" session --shell-binary "$BIN/bear-den-tv-shell" >"$OUT/session.log" 2>&1 &
  wait_app bear-den-tv-shell
  sleep 2
  BDTV_WL_SHELL=1 "$BIN/wayland.test" -test.v -test.count=1 -test.run TestLiveShellWindow 2>&1 | tee "$OUT/live-shell-test.txt"
  "$BIN/bear-den-tv" doctor >"$OUT/doctor-live.json" 2>&1 || true
  cat "$OUT/doctor-live.json"
  swaymsg -r -t get_tree >"$OUT/sway-tree.json"
  # Another window in front: the coordinator sees an unknown target, and
  # Home stays available because it can bring the shell back (activate).
  foot --app-id=bdtv.test.front sh -c 'sleep 600' >/dev/null 2>&1 &
  wait_app bdtv.test.front
  swaymsg '[app_id="bdtv.test.front"] focus' >/dev/null # the shell is fullscreen, so sway keeps it in front otherwise
  sleep 1
  "$BIN/bear-den-tv" doctor >"$OUT/doctor-front.json" 2>&1 || true
  grep -E '"kind"|"home"|"nav.up"' "$OUT/doctor-front.json"
else
  echo "== 3. skipped: no TV shell mounted (build it with make shell)"
fi
echo "== done"
