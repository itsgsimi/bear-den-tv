# Bear Den TV — guide for contributors and coding agents

**Start here** (five steps, in order):

1. Read this file to the end. Words you don't know: [`docs/GLOSSARY.md`](docs/GLOSSARY.md).
2. Find the part you will change in the table below and read its `AGENTS.md`.
3. Set up once: `scripts/bootstrap-toolchain.sh`, then `. scripts/env.sh` in every shell.
4. Make one coherent change with its tests and docs ([`CONTRIBUTING.md`](CONTRIBUTING.md)).
5. Finish with the [Definition of done](#definition-of-done).

Bear Den TV turns a small Linux box into a TV that you drive with your phone.
It has three processes that talk through shared contracts:

| Part | Where | Owns | Guide |
|---|---|---|---|
| **Coordinator** (Go) | `cmd/`, `internal/` | the session: which app is in front, launching/closing apps, routing input, pairing and permissions, configuration, the LAN service, playback tuning | [`internal/AGENTS.md`](internal/AGENTS.md) |
| **TV shell** (Qt 6.8 QML/C++) | `apps/tv-shell/` | everything on the TV between apps: focus graph, screens, themes, bears | [`apps/tv-shell/AGENTS.md`](apps/tv-shell/AGENTS.md) |
| **Phone remote** (TypeScript, Preact) | `apps/remote-web/` | the phone UI, served by (and embedded in) the coordinator | [`apps/remote-web/AGENTS.md`](apps/remote-web/AGENTS.md) |
| **Contracts** | `contracts/` | JSON Schemas + specs + fixtures, validated in all three languages | [`contracts/AGENTS.md`](contracts/AGENTS.md) |
| **Themes** | `themes/` | theme packages (manifest + art, no code) | [`themes/AGENTS.md`](themes/AGENTS.md) |

Plex HTPC, VacuumTube (YouTube) and Moonlight are independent Flatpak apps. Bear
Den launches them, brings them to the front, tunes their settings and returns
Home. It never wraps, embeds or patches them.

## Read first

| Question | Read |
|---|---|
| How does it all work? | [`docs/HOW_IT_WORKS.md`](docs/HOW_IT_WORKS.md) |
| What does a word mean? | [`docs/GLOSSARY.md`](docs/GLOSSARY.md) |
| How do I contribute? | [`CONTRIBUTING.md`](CONTRIBUTING.md) |
| What is built, tested, live-validated? | [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md), [`docs/VALIDATION_REPORT.md`](docs/VALIDATION_REPORT.md) |
| Why is it like this? | [`docs/decisions/`](docs/decisions/) (ADRs) |
| Wire formats | [`contracts/README.md`](contracts/README.md), [`actions.md`](contracts/actions.md), [`ipc.md`](contracts/ipc.md), [`http.md`](contracts/http.md), [`config.md`](contracts/config.md) |
| Themes | [`docs/THEMES.md`](docs/THEMES.md) |
| App playback settings, box tiers | [`docs/APP_PERFORMANCE.md`](docs/APP_PERFORMANCE.md) |
| Security model | [`docs/security.md`](docs/security.md) |
| Everyday commands on the TV | [`docs/operations.md`](docs/operations.md) |

## Boundaries (non-negotiable)

- **Who owns what.**
  - QML/C++ owns presentation and the focus graph.
  - Go owns coordination, authorization, app adapters, configuration, tuning
    and the LAN service.
  - TypeScript owns the phone experience.
  - Shared shapes live in `contracts/` and are validated in all three
    languages.
- **Phones send named actions only** ([`contracts/actions.md`](contracts/actions.md)).
  No shell strings, keycodes, executable paths, fetch URLs or scripts cross the
  network.
- **Fail closed, with a reason.** An unknown foreground, a locked session, a
  stale epoch, a revoked device or an unverified capability is refused and
  says why. `delivered` is never reported as `observed`.
- **Nothing listens on the LAN** until onboarding consent and interface
  selection. Secrets never enter `config.json`, exports, logs or phone
  payloads.
- **Demo content** exists only behind `--dev-fixtures` and is labelled DEMO.

## Development philosophy

These are the habits this codebase was built with. Follow them.

1. **Build for the floor, scale up automatically.**
   - The reference box is the floor: a Celeron 2955U with 2 cores at 1.4 GHz,
     Haswell GT1 graphics and 7.6 GiB, driving a 4K TV at 1080p 120 Hz. Every
     feature must be smooth there.
   - Faster hardware gets more, from **measured** capability: hardware
     decoders, cores × clock tiers, display modes. See
     [`docs/APP_PERFORMANCE.md`](docs/APP_PERFORMANCE.md#box-tiers).
   - Never hard-code the floor's limits for everyone, and never assume desktop
     power.
   - Put work on the right hardware: video decoding on the GPU, transcoding
     on the Plex server, game encoding on the gaming PC.
2. **Idle is free; motion is budgeted.**
   - The shell draws nothing while an app is in front.
   - Everything that loops moves on one heartbeat, `World.beat` (20 per
     second awake, 4 resting), never free-running per-frame animations.
     They are gated on `Theme.resting`, `Theme.screensaver`,
     `Theme.reducedMotion` and the shell being in front.
   - Canvas repaints only when its inputs change. No custom shaders.
   - Rules: [`docs/THEMES.md` → Performance rules](docs/THEMES.md).
3. **Measure on the box.**
   - First `make perf`: the sandbox runs the shell here on two cores and
     fails when an idle phase keeps drawing.
   - A claim about speed needs numbers from the TV:
     `scripts/measure-target.sh 20` for CPU and RSS, `BDTV_FPS_LOG=1` for
     frames per second.
   - The workstation's 16 fast cores prove nothing about the TV's two slow
     ones.
4. **Honest statuses.**
   - *Implemented*, *automatically tested* and *live-validated* are separate
     claims, recorded separately in `docs/IMPLEMENTATION_STATUS.md` and traced
     to requirement ids.
   - A skipped, blocked or not-run check is never a pass. Say "not yet seen on
     the TV" when that's the truth.
5. **Contracts first.** A shape change is one commit that touches:
   - the schema;
   - the fixtures;
   - the Go, C++ and TypeScript validators;
   - the spec.

   Additive changes keep protocol 1 ([ADR 0004](docs/decisions/0004-contract-versioning.md)).
   Today Go validates fixtures and incoming messages, TypeScript validates
   fixtures in its tests, and the shell checks snapshots at runtime in
   `SessionModel`; the shell does not yet run `contracts/fixtures`.
6. **Tests beside the code, deterministic, and proven to bite.**
   - Logic uses fake clocks and fake backends; live checks use the real
     adapters.
   - Go runs with `-race`.
   - Visual work is checked with sandbox screenshots (`scripts/sandbox.sh`)
     you actually look at.
   - After writing a test, break the code it protects and watch it fail.
     A test that can't fail documents nothing.
   - Known gaps to close, not copy: `internal/remote`, `internal/doctor`
     and `providers/plex` have no tests yet; `make lint` reports qmllint
     warnings without failing.
7. **Plugins over patches.** Extend through data:
   - a theme is a package;
   - an app is a row in the adapter and tuning tables;
   - an action is a name in the contract.

   Engine code never branches on a theme id or an app's display name. If a
   theme needs something new, build a reusable building block.
8. **Self-documenting.**
   - Open every source file with a comment saying what it owns and where its
     spec or doc lives, and keep that comment true.
   - Docs change in the same commit as the behaviour, and a doc's examples
     point at real code and tests.
   - A decision that changes the architecture gets a short ADR in
     `docs/decisions/`.
9. **The owner's box, the owner's rules.**
   - Everything is user-space: `scripts/bootstrap-toolchain.sh`, then
     `. scripts/env.sh`.
   - Never install system packages, clients, autostart entries or firewall
     rules without the owner saying so.
   - The TV machine may host other home services: no reboots, and no
     Docker or network restarts without the owner. Set
     `BDTV_TARGET_HEALTH_CMD` in `target.env` to have deploys verify them.
   - Don't restart Bear Den while something is playing; the deploy script
     checks for you.
   - Never bypass the lock screen or handle the session password.
10. **Ship small, fast, safely.**
    - One coherent change per commit, with tests and docs.
    - `scripts/deploy-target.sh` builds here, installs on the TV by copy and
      rename, restarts only if nothing is on screen, and verifies the running
      hashes, the shell connection and `BDTV_TARGET_HEALTH_CMD`.

## Everyday commands

```sh
scripts/bootstrap-toolchain.sh && . scripts/env.sh   # pinned user-space toolchain (Go, Qt 6.8, Node, CMake)
make help                                            # every target that exists
make test                                            # Go (race) + web (unit; no browser tests yet) + shell (offscreen)
make lint                                            # gofmt/vet, eslint/tsc, qmllint
make dev DEV_ARGS=--dev-fixtures                     # coordinator + shell locally, fake desktop, DEMO rows and weather
scripts/sandbox.sh shot --screen settings --theme forest   # prototype without the TV: one screenshot
make shots                                           # every theme × main screens in build/shots/gallery
make perf                                            # frames and CPU per phase of Home (a guide, not a gate on taste)
make package                                         # installable .deb in build/dist (docs/operations.md#packaging)
scripts/deploy-target.sh [--now|--no-restart|--dry-run]   # ship to the TV
scripts/target.sh run '<cmd>'                        # sync, then run on the TV inside the toolchain env
build/bin/bear-den-tv doctor                         # what's running, what's in front, what's allowed
```

Live-display checks need an active graphical session on the TV
([`docs/operations.md`](docs/operations.md#working-against-the-tv-machine)).

## Where to start for common changes

| Change | Start here |
|---|---|
| A new theme | [`docs/THEMES.md` → Make a theme in five minutes](docs/THEMES.md), [`themes/AGENTS.md`](themes/AGENTS.md) |
| A new decoration style, particle kind or corner scene | [`docs/THEMES.md` → Extending the engine](docs/THEMES.md), [`apps/tv-shell/AGENTS.md`](apps/tv-shell/AGENTS.md) |
| Support another app | [`internal/AGENTS.md` → Add an app](internal/AGENTS.md#add-an-app), then [`apps/tv-shell/AGENTS.md`](apps/tv-shell/AGENTS.md) and [`docs/APP_PERFORMANCE.md` → Adding an app](docs/APP_PERFORMANCE.md#adding-an-app) |
| A new phone action | [`contracts/AGENTS.md`](contracts/AGENTS.md#add-an-action), [`internal/AGENTS.md`](internal/AGENTS.md#add-an-action), [`apps/remote-web/AGENTS.md`](apps/remote-web/AGENTS.md) |
| A new setting on the TV | [`apps/tv-shell/AGENTS.md` → Settings rows](apps/tv-shell/AGENTS.md) |
| A config or layout field | [`contracts/AGENTS.md`](contracts/AGENTS.md#change-a-contract), [`internal/AGENTS.md`](internal/AGENTS.md#add-a-config-field) |
| A field phones or the shell see | [`contracts/AGENTS.md`](contracts/AGENTS.md#change-a-contract) (state snapshot) |
| A CLI subcommand | [`internal/AGENTS.md` → CLI](internal/AGENTS.md#cli-subcommands) |
| Playback settings for an app | [`docs/APP_PERFORMANCE.md` → Adding an app](docs/APP_PERFORMANCE.md#adding-an-app) |

## Definition of done

- [ ] `make test` and `make lint` pass. New tests were seen failing against
      deliberately broken code.
- [ ] Contracts, fixtures and all three validators agree (if shapes changed).
- [ ] Visual changes were rendered offscreen and looked at.
- [ ] Performance-sensitive changes were measured on the TV, or marked "not
      measured".
- [ ] Docs, file header comments and `docs/IMPLEMENTATION_STATUS.md` are
      updated, with honest live status.
- [ ] Deployed with `scripts/deploy-target.sh` when the owner wants it live,
      and verified there.
