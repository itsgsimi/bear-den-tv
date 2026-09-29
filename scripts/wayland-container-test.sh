#!/bin/bash
# Wayland integration test in Docker: headless sway (wlroots) in an
# ubuntu:24.04 container, the coordinator's Wayland adapter against it, and
# `bear-den-tv doctor` inside it. Nothing touches this machine's display or
# packages: the compositor lives only in the container.
#
#   scripts/wayland-container-test.sh            # adapter test + doctor
#   scripts/wayland-container-test.sh --shell    # also run the TV shell (needs `make shell`)
#
# Needs Docker and the toolchain (. scripts/env.sh). Not part of `make test`.
# Artifacts: build/wayland-test/out/. Plan: docs/decisions/0007-wayland-profile.md.
set -euo pipefail
cd "$(dirname "$0")/.."

with_shell=0
[ "${1:-}" = "--shell" ] && with_shell=1

command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }
command -v go >/dev/null || { echo "go not found: run '. scripts/env.sh' first" >&2; exit 2; }

image=bdtv-wayland-test
work=build/wayland-test
rm -rf "$work" && mkdir -p "$work/bin" "$work/out"

echo "building the test image ($image)"
docker build -q -t "$image" tests/wayland >/dev/null

echo "building the coordinator and the live test"
CGO_ENABLED=0 go build -o "$work/bin/bear-den-tv" ./cmd/bear-den-tv
CGO_ENABLED=0 go test -c -tags wayland_live -o "$work/bin/wayland.test" ./tests/wayland

mounts=(-v "$PWD/$work/bin:/bdtv:ro" -v "$PWD/$work/out:/out" -v "$PWD/tests/wayland/in-container.sh:/in-container.sh:ro")
if [ "$with_shell" = 1 ]; then
  [ -x build/bin/bear-den-tv-shell ] || { echo "build/bin/bear-den-tv-shell missing: run 'make shell'" >&2; exit 2; }
  cp build/bin/bear-den-tv-shell "$work/bin/"
  # The shell links the toolchain's Qt by absolute rpath: mount it read-only
  # at the same path.
  mounts+=(-v "$BDTV_TOOLCHAIN:$BDTV_TOOLCHAIN:ro")
fi

docker run --rm --user "$(id -u):$(id -g)" "${mounts[@]}" "$image" bash /in-container.sh 2>&1 | tee "$work/out/run.log"
status=${PIPESTATUS[0]}

# The capability report must match the ADR's sway row.
fail=0
check() { # file, pattern, what
  grep -q -- "$2" "$work/out/$1" 2>/dev/null || { echo "$1: expected $3" >&2; fail=1; }
}
check live-test.txt '^--- PASS: TestLiveSway' "the live adapter test to pass"
check doctor-probe.json '"name": "wayland-wlr"' "adapter wayland-wlr"
check doctor-probe.json '"family": "wlroots"' "family wlroots"
check doctor-probe.json '"Backend": "wlr-foreign-toplevel"' "observe/activate on wlr-foreign-toplevel"
check doctor-probe.json 'input on Wayland is not implemented' "input unavailable with its reason"
check doctor-probe.json 'GlobalShortcuts portal' "physical_home unavailable with its reason"
if [ "$with_shell" = 1 ]; then
  check live-shell-test.txt '^--- PASS: TestLiveShellWindow' "the shell to be a fullscreen XWayland window in front"
  check doctor-live.json '"desktop_adapter": "wayland-wlr"' "the running coordinator on wayland-wlr"
  check doctor-live.json '"kind": "shell"' "the shell as the observed target"
  check doctor-live.json '"home": "available' "home available"
  check doctor-front.json '"kind": "unknown"' "another window in front as an unknown target"
  check doctor-front.json '"home": "available' "home available while another window is in front"
  check doctor-front.json '"nav.up": "unavailable' "nav refused for an unknown window"
fi
if [ "$status" != 0 ] || [ "$fail" != 0 ]; then
  echo "WAYLAND CONTAINER TEST FAILED (artifacts in $work/out)" >&2
  exit 1
fi
echo "wayland container test passed (artifacts in $work/out)"
