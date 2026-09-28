#!/usr/bin/env bash
# Deploys this checkout to the TV machine and restarts Bear Den there.
#   scripts/deploy-target.sh               # build, install, restart unless an app is on screen
#   scripts/deploy-target.sh --now         # restart even if an app is on screen
#   scripts/deploy-target.sh --no-restart  # install only; takes effect at the next start
#   scripts/deploy-target.sh --dry-run     # print the steps without changing anything
# Steps: build everything here in seconds: the Go binaries (with the embedded
# phone remote) and the TV shell (`make shell-target`: conda's compiler against
# a glibc 2.28 sysroot, so it runs on the TV's older glibc). Sync the checkout;
# install changed binaries on the target by copy + rename (never in place: a
# running executable cannot be overwritten); restart through
# start-session.sh --watch; verify the running binaries' hashes, the shell
# connection, and BDTV_TARGET_HEALTH_CMD from target.env (other services the
# box hosts; skipped when empty). The restart
# is skipped while Plex, YouTube or Moonlight is on screen, unless --now. Nothing
# is compiled on the TV.
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/target-env.sh

now=0 restart=1 dry=0
for arg in "$@"; do
  case "$arg" in
    --now) now=1 ;;
    --no-restart) restart=0 ;;
    --dry-run) dry=1 ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

say() { printf '\033[1m==> %s\033[0m\n' "$*"; }
fail() { printf '\033[31mFAILED: %s\033[0m\n' "$*" >&2; exit 1; }
run() { if [ "$dry" = 1 ]; then echo "  [dry-run] $*"; else "$@"; fi; }
remote() { ssh -o BatchMode=yes -o ConnectTimeout=8 "$TARGET" "cd $TDIR && $*"; }
rrun() { if [ "$dry" = 1 ]; then echo "  [dry-run] (target) $*"; else remote "$@"; fi; }

# --- Build here -------------------------------------------------------------
. scripts/env.sh
say "Building the phone remote and Go binaries"
if [ -d apps/remote-web/node_modules ]; then
  run bash -c 'cd apps/remote-web && npm run --silent build >/dev/null'
else
  run make web
fi
run go build -trimpath -o build/bin/ ./cmd/...
[ "$dry" = 1 ] || [ -x build/bin/bear-den-tv ] || fail "build/bin/bear-den-tv was not built"

# --- What is on the TV? -------------------------------------------------------
say "Checking the TV"
doctor=$(remote 'build/bin/bear-den-tv doctor 2>/dev/null' || true)
field() { printf '%s' "$doctor" | python3 -c "import json,sys
try: d=json.load(sys.stdin)['coordinator']
except Exception: print(''); sys.exit()
v=d
for k in sys.argv[1].split('.'): v=(v or {}).get(k) if isinstance(v, dict) else None
print('' if v is None else v)" "$1"; }
target_kind=$(field target.kind)
target_label=$(field target.label)
busy=0
if [ "$target_kind" = "app" ]; then busy=1; fi
echo "  on screen: ${target_label:-nothing (Bear Den not running)}"
if [ "$busy" = 1 ] && [ "$now" = 0 ]; then
  echo "  $target_label is on screen: installing only, no restart (use --now to override)"
  restart=0
fi

# --- Sync and install ---------------------------------------------------------
say "Syncing the checkout"
run scripts/target.sh sync
say "Installing the coordinator binaries (copy + rename)"
for bin in bear-den-tv bdtv-probe; do
  [ -f "build/bin/$bin" ] || continue
  run scp -q "build/bin/$bin" "$TARGET:$TDIR/build/bin/.$bin.new"
  rrun "chmod +x build/bin/.$bin.new && mv -f build/bin/.$bin.new build/bin/$bin"
done
if [ "$dry" = 0 ]; then
  want=$(sha256sum build/bin/bear-den-tv | cut -d' ' -f1)
  got=$(remote 'sha256sum build/bin/bear-den-tv' | cut -d' ' -f1)
  [ "$want" = "$got" ] || fail "installed coordinator hash $got != built $want"
  echo "  coordinator ${want:0:16} installed"
fi

# --- Shell: built here for the TV machine, shipped as a binary ---------------
say "Building the TV shell for the TV machine"
# The shell finds Qt through its rpath: the toolchain's path on the TV.
target_toolchain="${BDTV_TARGET_TOOLCHAIN:-}"
if [ -z "$target_toolchain" ]; then
  target_home=$(ssh -o BatchMode=yes "$TARGET" 'printf %s "$HOME"') || fail "could not read the TV's home directory"
  target_toolchain="$target_home/.bdtv-toolchain"
fi
echo "  Qt on the TV: $target_toolchain/env/lib"
if [ "$dry" = 1 ]; then echo "  [dry-run] make -s shell-target TARGET_TOOLCHAIN=$target_toolchain"
else make -s shell-target TARGET_TOOLCHAIN="$target_toolchain" >/tmp/bdtv-shell-target.log 2>&1 || { tail -30 /tmp/bdtv-shell-target.log >&2; fail "shell build failed (log: /tmp/bdtv-shell-target.log)"; }
fi
shell_bin=build/tv-shell-target/bear-den-tv-shell
if [ "$dry" = 0 ]; then
  want_shell=$(sha256sum "$shell_bin" | cut -d' ' -f1)
  have_shell=$(remote 'sha256sum build/bin/bear-den-tv-shell 2>/dev/null' | cut -d' ' -f1 || true)
  if [ "$have_shell" = "$want_shell" ]; then
    echo "  shell unchanged"
  else
    scp -q "$shell_bin" "$TARGET:$TDIR/build/bin/.bear-den-tv-shell.new"
    remote 'chmod +x build/bin/.bear-den-tv-shell.new && mv -f build/bin/.bear-den-tv-shell.new build/bin/bear-den-tv-shell'
    got=$(remote 'sha256sum build/bin/bear-den-tv-shell' | cut -d' ' -f1)
    [ "$got" = "$want_shell" ] || fail "installed shell hash $got != built $want_shell"
    echo "  shell ${want_shell:0:16} installed"
  fi
fi

# --- Restart ------------------------------------------------------------------
if [ "$restart" = 0 ]; then
  say "Installed; not restarted"
  echo "  takes effect at the next start (desktop icon, login, or: scripts/deploy-target.sh --now)"
  exit 0
fi
say "Restarting Bear Den"
rrun 'bash scripts/start-session.sh --watch'
[ "$dry" = 1 ] && exit 0

# --- Verify -------------------------------------------------------------------
say "Verifying"
ok=0
for _ in $(seq 1 30); do
  sleep 1
  doctor=$(remote 'build/bin/bear-den-tv doctor 2>/dev/null' || true)
  if [ "$(field session.shell_connected)" = "True" ] && [ "$(field session.shell_state)" = "running" ]; then ok=1; break; fi
done
[ "$ok" = 1 ] || { remote 'tail -20 ~/.local/state/bear-den-tv/session.log' >&2 || true; fail "the shell did not connect within 30 s"; }
running=$(remote 'pid=$(pgrep -u "$(id -u)" -f "^build/bin/bear-den-tv session" | head -1); [ -n "$pid" ] && sha256sum /proc/$pid/exe | cut -d" " -f1')
[ "$running" = "$want" ] || fail "running coordinator is ${running:0:16}, expected ${want:0:16}"
running_shell=$(remote 'pid=$(pgrep -u "$(id -u)" -f "^build/bin/bear-den-tv-shell" | head -1); [ -n "$pid" ] && sha256sum /proc/$pid/exe | cut -d" " -f1')
[ "$running_shell" = "$want_shell" ] || fail "running shell is ${running_shell:0:16}, expected ${want_shell:0:16}"
echo "  coordinator ${running:0:16} and shell ${running_shell:0:16} running; shell connected (screen: $(field shell_screen))"
if [ -n "$BDTV_TARGET_HEALTH_CMD" ]; then
  health=$(ssh -o BatchMode=yes -o ConnectTimeout=8 "$TARGET" "$BDTV_TARGET_HEALTH_CMD" 2>&1) || fail "target health check: $health"
  echo "  target health check passed"
fi
say "Deployed"
