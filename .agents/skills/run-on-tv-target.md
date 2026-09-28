---
name: "run-on-tv-target"
description: "Deploy and run Bear Den TV binaries on your TV over ssh: target.env, the deploy script, X credentials for :0, lock-screen etiquette, recording evidence."
whenToUse: "When deploying to, running on, or probing the TV machine configured in target.env: live display checks, X11 activation tests, coordinator or shell runs on :0."
---

# Run Bear Den TV on your TV

Setup: copy `target.env.example` to `target.env` (untracked) and set
`BDTV_TARGET` (ssh destination, key-based login) and optionally
`BDTV_TARGET_DIR` (default `bear-den-tv` under the remote home) and
`BDTV_TARGET_HEALTH_CMD`. Details: `docs/operations.md` → "Point the scripts
at your TV". Never ssh anywhere else; never install system packages.

## Ship and run

| Command | What it does |
|---|---|
| `scripts/deploy-target.sh` | builds everything here, syncs, installs binaries by copy + rename, restarts only if no app is on screen, verifies hashes, the shell connection and `BDTV_TARGET_HEALTH_CMD` |
| `scripts/deploy-target.sh --dry-run` / `--no-restart` / `--now` | print the steps / install only / restart even if an app is on screen |
| `scripts/target.sh sync` | rsync the checkout only (skips `.git`, `build/`, `node_modules/`) |
| `scripts/target.sh run '<cmd>'` | sync, then run in the checkout on the TV inside `. scripts/env.sh` |
| `scripts/target.sh ssh '<cmd>'` | the same without syncing |
| `scripts/measure-target.sh 20` | CPU and RSS on the TV for 20 s |

Nothing is compiled on the TV: the deploy script builds the Go binaries and
the shell (`make shell-target`) here. Prefer it over building remotely.

## Display access

- Graphical commands need the TV's session environment:
  `DISPLAY=:0 XAUTHORITY=$HOME/.Xauthority DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$(id -u)/bus`.
  This works when the ssh user owns the `:0` session (`who` shows it).
- The bundled Qt needs `QT_XCB_GL_INTEGRATION=xcb_egl` (ADR 0001).
- A seat on the login greeter never maps windows. Check `who` first.
- Check the lock before visual work: `build/bin/bear-den-tv doctor --probe` (with the variables above) or the
  `lock` section of `build/bin/bdtv-probe`. **Never bypass the lock screen
  and never handle the session password.** If it is locked, ask the owner to
  unlock the TV in person.
- Don't restart Bear Den while something is playing (the deploy script checks).
  The TV may host other services: no reboots and no network restarts.

## Recording evidence

- Save raw probe output, for example
  `scripts/target.sh ssh 'DISPLAY=:0 build/bin/bdtv-probe' > /tmp/probe.json`,
  and file it as `tests/compatibility/YYYY-MM-DD-<box>-<what>.json` with when
  it was recorded, the commit, what was observed and the support level.
- Update `docs/IMPLEMENTATION_STATUS.md` and `docs/VALIDATION_REPORT.md`.
  *Implemented*, *automatically tested* and *seen on the TV* stay separate.

## Ask the owner first (never self-serve)

- Installing apps: `flatpak install --user flathub <app id>` (ids are in
  `internal/applications/adapters/adapters.go`).
- Exposing the phone remote on the LAN:
  `bear-den-tv remote enable --interface <IF> --accept-lan-exposure`, run by
  the owner, on a real LAN interface (never `docker0`, `br-*` or other
  virtual ones).
