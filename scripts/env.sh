# Source this to use the user-space toolchain: `. scripts/env.sh`
BDTV_TOOLCHAIN="${BDTV_TOOLCHAIN:-$HOME/.bdtv-toolchain}"
export BDTV_TOOLCHAIN
export PATH="$BDTV_TOOLCHAIN/env/bin:$PATH"
export CMAKE_PREFIX_PATH="$BDTV_TOOLCHAIN/env${CMAKE_PREFIX_PATH:+:$CMAKE_PREFIX_PATH}"
export GOFLAGS="${GOFLAGS:--mod=mod}"
export GOTOOLCHAIN=local
