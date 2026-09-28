# Bear Den TV — repository guidance template

Adapt/merge into the repository's existing `AGENTS.md`; this template is not automatically active guidance. Keep it concise and update command references only after those commands exist.

## Product

Build a configurable Linux TV home screen and LAN browser remote. Use Plex HTPC and VacuumTube rather than replacement clients. The product is Bear Den TV. Authoritative design: `bear-den-tv-materials/reference/Bear_Den_TV_Design_and_Implementation.md`. Execution: `bear-den-tv-materials/Bear_Den_TV_Codex_Prompt.md`.

## Boundaries

QML/C++ owns TV presentation; Go owns coordination, authorization, app adapters, and configuration; static TypeScript owns the phone UI. Keep action/state contracts shared. No arbitrary network-to-shell/keycode bridge. Unknown foreground, lock, stale epochs, and revoked devices fail closed. Keep secrets out of layout files and logs.

## Working agreements

Preserve existing edits. Build runnable increments and keep tests with changed behavior. Implemented, mock-tested, and live-validated are separate statuses. Do not fabricate data, benchmarks, or compatibility. Do not silently drop required work when a live target is unavailable; record a blocked gate and continue independent tasks.

Follow the active approval policy before modifying the host, exposing a LAN service, installing clients, connecting accounts, or publishing code. Never bypass the sandbox.

## Navigation

Read `docs/IMPLEMENTATION_STATUS.md` for the next task and `docs/VALIDATION_REPORT.md` for evidence. Update both before handoff. Use the requirements in `bear-den-tv-materials/ACCEPTANCE_MATRIX.md` for traceability. Add real build/run/test commands here once implemented; do not list hypothetical commands as working.
