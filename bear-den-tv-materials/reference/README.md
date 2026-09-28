# Bear Den TV — design and implementation bundle

**Prepared:** September 21, 2026

Bear Den TV is a proposed configurable TV interface for Linux, with Plex HTPC and VacuumTube integration and a built-in LAN browser remote. This is a research/design package, not an application installer.

## Start here

Read `Bear_Den_TV_Design_and_Implementation.md` for the complete self-contained specification, research references, architecture, security model, and implementation milestones. Give `IMPLEMENTATION_HANDOFF.md` and the main specification to a coding agent together.

`ACCEPTANCE_CHECKLIST.md` is an unexecuted release checklist. `RESEARCH_SOURCES.md` provides the source register separately. `examples/` contains a proposed JSON configuration subset and its schema; these are not yet accepted by a real Bear Den TV application.

The earlier user-approved generated reference image (`assets/original-visual-concept.png`) is not included in the public repository: it showed third-party film artwork and streaming-service logos. The implemented product is named Bear Den TV and must not imply integrations beyond those it ships.

The design distinguishes running on a Linux distribution from reliably controlling external clients in that desktop session. All device/player integration tests still need to be performed on the actual target hardware.
