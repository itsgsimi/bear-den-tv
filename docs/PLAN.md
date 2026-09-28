# Implementation plan (historical)

This was the first plan, written on 2026-09-22 before most of the code existed. It is kept for history and is no longer updated. For what is built, tested and live today, read [`IMPLEMENTATION_STATUS.md`](IMPLEMENTATION_STATUS.md).

Environment at the time: development on a workstation (Ubuntu 24.04) with the pinned user-space toolchain; the TV is a small mini PC running Linux Mint 21.3 Xfce on X11 with Intel Haswell graphics, at 1920×1080.

## Milestone 0 — control-loop proof
- Target capability report (`tests/compatibility/`)
- Toolchain bootstrap on both machines (Go 1.24, Qt 6.8.4, CMake, Node 22)
- Shared contracts (`contracts/`)
- Coordinator skeleton with fake desktop, shell IPC and HTTP remote on loopback
- Minimal shell over IPC; D-pad/Select/Back from the phone observed
- X11 adapter probe on the TV (activation, foreground, XTEST)
- Client launch and Home experiments

## Milestone 1 — polished shell + LAN remote
Home screen, focus graph, settings, pairing UI, device list; phone remote with pairing, D-pad, Home, app shortcuts, capability-gated controls; onboarding consent + interface selection; config persistence with last-known-good.

## Milestone 2 — client adapters
Flatpak discovery/launch/instance tracking, window matching, XTEST key maps, MPRIS probe, pause-on-home gating, normal/forced close, crash/exit feedback.

## Milestone 3 — customization + optional Plex rows
Web layout editor (preview/apply/undo/reset/timed confirm), TV settings, theme tokens, export/import, Plex connector with Secret Service tokens.

## Milestone 4 — packaging/portability
nfpm `.deb`/`.rpm`, autostart opt-in, uninstall, doctor, Wayland reduced-capability profile, compatibility reports.

## Milestone 5 — security/release validation
Negative HTTP/WS suites, race tests, dependency/license manifest, acceptance matrix evidence.
