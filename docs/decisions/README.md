# Architecture decision records

Short records of decisions that shape the architecture: the context, what was
decided and what follows. A new one takes the next number; an old one is
amended with a dated update or superseded by a later ADR, never rewritten.
Current behaviour is described in the other docs; where an ADR and the code
disagree, the code and [`IMPLEMENTATION_STATUS.md`](../IMPLEMENTATION_STATUS.md)
win.

| ADR | Decision | Date |
|---|---|---|
| [0001](0001-standalone-project-and-user-space-toolchain.md) | A standalone project with one pinned toolchain in the user's home folder (Go, Qt 6.8, CMake, Node), nothing system-wide | 2026-09-22 |
| [0002](0002-one-cli-binary-embedded-remote.md) | One `bear-den-tv` binary for the coordinator and the CLI, with the phone remote embedded | 2026-09-22 |
| [0003](0003-build-tv-shell-off-target.md) | Build the TV shell on the workstation against a glibc 2.28 sysroot; nothing compiles on the TV | 2026-09-22 |
| [0004](0004-contract-versioning.md) | Additive contract changes keep protocol 1 | 2026-09-22 |
| [0005](0005-pixel-art.md) | Pixel art everywhere, on one grid, generated from code | 2026-09-23 |
| [0006](0006-classic-art-style.md) | A Classic (smooth) art style beside pixel art, `ui.art_style` | 2026-09-23 |
| [0007](0007-wayland-profile.md) | The Wayland profile: wlroots window control, honest reasons elsewhere | 2026-09-28 |
| [0008](0008-hdmi-cec.md) | TV control over HDMI-CEC through the kernel CEC API, off by default | 2026-09-28 |
| [0009](0009-den-badges-local-counters.md) | Den badges from local counters: ids, counts and days only | 2026-09-28 |
| [0010](0010-web-apps-over-cdp-pipe.md) | Web apps (Netflix, Disney+, Hulu, Browser): Flathub Chromium driven over the DevTools pipe | 2026-09-28 |
| [0011](0011-per-user-flathub-installs.md) | One-press app installs: per user from Flathub, with the owner's consent | 2026-09-28 |
| [0012](0012-app-icons-apps-own-by-default.md) | App icons: the app's own by default, Bear Den's as a choice, served safely to phones | 2026-09-29 |
