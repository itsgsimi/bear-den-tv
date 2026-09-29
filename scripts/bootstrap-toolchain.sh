#!/usr/bin/env bash
# Installs the pinned Bear Den TV build toolchain into a user-owned prefix.
# No root, no system package changes. Re-runnable; skips completed steps.
# Also run by CI (.github/workflows/ci.yml) on a toolchain cache miss.
#   BDTV_TOOLCHAIN           prefix (default: ~/.bdtv-toolchain)
#   BDTV_MICROMAMBA_VERSION  micromamba release to download (default: pinned below)
#   BDTV_SKIP_SYSROOT=1      skip the glibc 2.28 sysroot (only `make shell-target` needs it; CI sets this)
set -euo pipefail
ROOT="${BDTV_TOOLCHAIN:-$HOME/.bdtv-toolchain}"
MICROMAMBA_VERSION="${BDTV_MICROMAMBA_VERSION:-2.9.0}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "$ROOT/bin"
if [ ! -x "$ROOT/bin/micromamba" ]; then
  echo "[bootstrap] downloading micromamba $MICROMAMBA_VERSION"
  curl -fsSL "https://micro.mamba.pm/api/micromamba/linux-64/$MICROMAMBA_VERSION" | tar -xj -C "$ROOT" bin/micromamba
fi
export MAMBA_ROOT_PREFIX="$ROOT/mamba"
if [ ! -x "$ROOT/env/bin/go" ] || [ ! -x "$ROOT/env/bin/cmake" ] || [ ! -f "$ROOT/env/lib/libQt6Quick.so" ]; then
  echo "[bootstrap] creating env from toolchain/environment.yml"
  "$ROOT/bin/micromamba" create -y -p "$ROOT/env" -f "$HERE/../toolchain/environment.yml"
fi
# glibc 2.28 sysroot for building the TV shell here so it runs on older TV
# machines (Mint 21 ships glibc 2.35; conda's compiler defaults to 2.39).
if [ "${BDTV_SKIP_SYSROOT:-0}" = "1" ]; then
  echo "[bootstrap] skipping the glibc 2.28 target sysroot (BDTV_SKIP_SYSROOT=1; make shell-target will not work)"
elif [ ! -d "$ROOT/sysroot-2.28/x86_64-conda-linux-gnu/sysroot" ]; then
  echo "[bootstrap] installing the glibc 2.28 target sysroot"
  "$ROOT/bin/micromamba" create -y -p "$ROOT/sysroot-2.28" -c conda-forge "sysroot_linux-64=2.28"
fi
echo "[bootstrap] done: source scripts/env.sh"
"$ROOT/env/bin/go" version
"$ROOT/env/bin/cmake" --version | head -1
"$ROOT/env/bin/qmake6" -query QT_VERSION 2>/dev/null || true
