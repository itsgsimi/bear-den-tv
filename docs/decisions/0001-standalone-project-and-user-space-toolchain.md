# ADR 0001 — Standalone project, user-space pinned toolchain

Date: 2026-09-22. Status: accepted.

## Context

The original design brief recommends a standalone Qt 6 / Go / TypeScript project rather than a Flex Launcher fork (`flex-launcher_2.2_amd64.deb` sits in the target's Downloads but is not installed). The TV target (Linux Mint 21.3 Xfce, X11, 2-core Haswell, 7.6 GiB) has no Go, no Qt 6, no C++ compiler, and the agent has no sudo. The development workstation (Ubuntu 24.04) has no Go or Qt 6 either.

## Decision

- Standalone repository at `~/bear-den-tv` (workstation is the authoritative git checkout; `scripts/target.sh` rsyncs to the target for builds and live checks).
- One pinned, user-space toolchain for both hosts installed by [`scripts/bootstrap-toolchain.sh`](../../scripts/bootstrap-toolchain.sh) from [`toolchain/environment.yml`](../../toolchain/environment.yml) (conda-forge via micromamba): Go 1.24.13, Qt 6.8.4 (`qt6-main`), CMake 4.4, Ninja, GCC 15, Node 22 (the versions resolved on that date; the file pins `go =1.24.*`, `qt6-main =6.8.4` (exact since CI was added, so CI and workstations match), `cmake >=3.28`, `nodejs =22.*`; micromamba itself is pinned in the script). Nothing is installed system-wide.
- Pure-Go dependencies only (modernc SQLite, jezek/xgb, godbus) so coordinator binaries built on the workstation run unchanged on the target (static, glibc-independent). The Qt shell is built on the target itself (same pinned Qt). *Superseded by [ADR 0003](0003-build-tv-shell-off-target.md): the shell is now built on the workstation against a glibc 2.28 sysroot.*

## Consequences

- On the target, the conda `libglvnd` GLX path deadlocks in `glXQueryServerString` under Qt's xcb plugin; `QT_XCB_GL_INTEGRATION=xcb_egl` initialises correctly. The launcher ([`scripts/start-session.sh`](../../scripts/start-session.sh)) sets it. A distro-built Qt would not need this; packaging (`packaging/`) must either bundle the conda Qt runtime with this environment variable or build against a distro Qt ≥ 6.5 and drop it. *Update 2026-09-29: `make package` took the first path: the .deb bundles the toolchain's Qt runtime and its shell wrapper ([`packaging/bear-den-tv-shell.sh`](../../packaging/bear-den-tv-shell.sh)) sets `QT_XCB_GL_INTEGRATION=xcb_egl` ([`operations.md` → Packaging](../operations.md#packaging)).*
- `modernc.org/sqlite` and `golang.org/x/*` are pinned to Go-1.24-compatible versions.
