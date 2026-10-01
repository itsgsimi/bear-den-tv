#!/usr/bin/env bash
# Install smoke test of the built .deb in a clean distro container
# (docs/operations.md → Packaging). Needs Docker and a `make package` build.
#
#   packaging/smoke-deb.sh [image]      default ubuntu:22.04; try ubuntu:24.04
#
# In the container, as root, it:
#   1. apt-get installs build/dist/bear-den-tv_*.deb with
#      --no-install-recommends, so `depends` alone must be enough (dependencies
#      resolved from the distro archive), and checks no autostart was enabled;
#   2. runs `bear-den-tv version` and `--help`, and `autostart enable` from
#      /usr/bin must write Exec=/usr/lib/bear-den-tv/start-session.sh --watch;
#   3. `ldd -r` every bundled ELF with the wrapper's library path: any "not
#      found" or "undefined symbol" fails;
#   4. runs the shell offscreen for 5 s on the shell tests' demo snapshot and
#      fails on missing library, plugin or QML module messages;
#   5. runs `bear-den-tv dev --dev-fixtures` with the packaged shell offscreen
#      and saves one screenshot;
#   6. `apt-get remove --purge`, then fails if anything is left in /usr or /opt.
# Logs and screenshots go to build/smoke/<image>/.
set -euo pipefail
cd "$(dirname "$0")/.."
IMAGE="${1:-ubuntu:22.04}"

if [ "${1:-}" != "--inside" ]; then
  deb=$(ls -t build/dist/bear-den-tv_*_amd64.deb 2>/dev/null | head -1)
  [ -n "$deb" ] || { echo "no build/dist/bear-den-tv_*_amd64.deb: run make package" >&2; exit 1; }
  out="build/smoke/${IMAGE//[:\/]/-}"
  rm -rf "$out"; mkdir -p "$out"
  echo "[smoke] $deb in $IMAGE → $out"
  docker run --rm -e DEBIAN_FRONTEND=noninteractive -e LANG=C.UTF-8 \
    -v "$PWD/$deb:/pkg/$(basename "$deb"):ro" \
    -v "$PWD/packaging/smoke-deb.sh:/smoke.sh:ro" \
    -v "$PWD/apps/tv-shell/tests/fixtures/state.demo.json:/fixture.json:ro" \
    -v "$PWD/$out:/out" \
    "$IMAGE" bash /smoke.sh --inside 2>&1 | tee "$out/smoke.log"
  exit "${PIPESTATUS[0]}"
fi

# ---- inside the container ----
fail() { echo "[smoke] FAIL: $*"; exit 1; }
. /etc/os-release; echo "[smoke] $PRETTY_NAME, glibc $(ldd --version | head -1 | awk '{print $NF}')"
before=$(mktemp); { find /usr /opt 2>/dev/null || true; } | sort >"$before"

echo "[smoke] 1. install"
apt-get update -qq >/dev/null
apt-get install -y -qq --no-install-recommends /pkg/bear-den-tv_*.deb >/out/apt-install.log 2>&1 || { tail -20 /out/apt-install.log; fail "apt-get install"; }
grep -E '^(Setting up|Unpacking) ' /out/apt-install.log | wc -l | xargs echo "  packages set up/unpacked:"
[ ! -e /etc/xdg/autostart/bear-den-tv.desktop ] || fail "the package enabled autostart"
grep -qx 'Exec=/usr/bin/bear-den-tv open-url %u' /usr/share/applications/bear-den-tv-links.desktop \
  || fail "the web-link handler entry (Sign-ins on the TV)"

echo "[smoke] 2. CLI"
bear-den-tv version
bear-den-tv --help >/out/help.txt 2>&1 || true
grep -q autostart /out/help.txt || { cat /out/help.txt; fail "bear-den-tv --help"; }
echo "  --help lists $(grep -c '^  ' /out/help.txt) commands"
/usr/lib/bear-den-tv/start-session.sh which
XDG_CONFIG_HOME=/tmp/cfg bear-den-tv autostart enable
grep -qx 'Exec=/usr/lib/bear-den-tv/start-session.sh --watch' /tmp/cfg/autostart/bear-den-tv.desktop \
  || { cat /tmp/cfg/autostart/bear-den-tv.desktop; fail "autostart entry from /usr/bin"; }
XDG_CONFIG_HOME=/tmp/cfg bear-den-tv autostart disable

echo "[smoke] 3. ldd -r of the bundle"
B=/opt/bear-den-tv/shell
: >/out/ldd.txt
find "$B" -type f \( -name '*.so*' -o -name '*.bin' \) | sort | while read -r f; do
  out=$(LD_LIBRARY_PATH="$B/lib:$B/lib/compat" ldd -r "$f" 2>&1 || true)
  if echo "$out" | grep -qE 'not found|undefined symbol'; then
    echo "== $f"; echo "$out" | grep -E 'not found|undefined symbol'
  fi
done | tee /out/ldd.txt
[ -s /out/ldd.txt ] && fail "unresolved libraries or symbols (above)"
echo "  every bundled ELF resolves"

echo "[smoke] 4. shell offscreen, 5 s"
export QT_QPA_PLATFORM=offscreen XDG_RUNTIME_DIR=/tmp/xdg; mkdir -p -m 700 "$XDG_RUNTIME_DIR"
status=0
QT_DEBUG_PLUGINS=0 bear-den-tv-shell --fixture /fixture.json --screenshot /out/shell.png --screenshot-after 3000 --exit-after 5000 \
  >/out/shell.log 2>&1 || status=$?
echo "  exit $status; $(wc -l </out/shell.log) log lines"
if grep -iE 'cannot load library|could not load|not installed|module ".*" is not|library .* not found|no such file|failed to load|undefined symbol|could not find the Qt platform plugin' /out/shell.log; then
  fail "shell reported missing pieces (above)"
fi
[ "$status" = 0 ] || { tail -20 /out/shell.log; fail "shell exit $status"; }
[ -s /out/shell.png ] || fail "no shell screenshot"

echo "[smoke] 5. coordinator (dev, DEMO fixtures) + packaged shell"
printf '#!/bin/sh\nexec /usr/bin/bear-den-tv-shell "$@" --windowed --size 1920x1080 --screenshot /out/dev.png --screenshot-after 6000 --exit-after 8000\n' >/tmp/shot-shell
chmod +x /tmp/shot-shell
timeout 15 bear-den-tv dev --dev-fixtures --data-dir /tmp/bdtv-dev --shell-binary /tmp/shot-shell >/out/dev.log 2>&1 || true
echo "  $(wc -l </out/dev.log) log lines"
grep -iE 'cannot load library|module ".*" is not|could not find the Qt platform plugin|undefined symbol' /out/dev.log && fail "dev run reported missing pieces"
[ -s /out/dev.png ] || { tail -20 /out/dev.log; fail "no dev screenshot"; }

echo "[smoke] 6. remove --purge"
apt-get remove --purge -y -qq bear-den-tv >/out/apt-remove.log 2>&1 || fail "apt-get remove"
after=$(mktemp); { find /usr /opt 2>/dev/null || true; } | sort >"$after"
left=$(comm -13 "$before" "$after" | grep -i 'bear-den' || true)
[ -z "$left" ] || { echo "$left"; fail "files left behind"; }
echo "  nothing named bear-den left in /usr or /opt"
chmod -R a+rwX /out
echo "[smoke] PASS"
