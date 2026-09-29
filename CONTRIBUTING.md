# Contributing to Bear Den TV

Thanks for helping. This page is the workflow for people and for AI coding
agents. The rules of the codebase itself are in [`AGENTS.md`](AGENTS.md); read
that first. Project words are explained in [`docs/GLOSSARY.md`](docs/GLOSSARY.md).

## The workflow

1. **Start at [`AGENTS.md`](AGENTS.md).** Then read the `AGENTS.md` of every
   directory you will touch (for example [`internal/AGENTS.md`](internal/AGENTS.md)).
2. **Set up the toolchain** (once per machine, no root):

   ```sh
   scripts/bootstrap-toolchain.sh
   . scripts/env.sh        # in every new shell
   ```

3. **Make one coherent change.** One change per commit, with its tests and its
   docs in the same commit.
4. **Run the checks** (below). All must pass.
5. **Prove your new test bites** (below).
6. **Look at visual changes** in sandbox screenshots (below).
7. **Update the status honestly** in
   [`docs/IMPLEMENTATION_STATUS.md`](docs/IMPLEMENTATION_STATUS.md).

Done when: every box in [AGENTS.md → Definition of done](AGENTS.md#definition-of-done) is ticked.

## Run the checks

```sh
. scripts/env.sh
make test          # everything: Go (race), phone remote, shell (offscreen)
make lint          # gofmt/vet, eslint/tsc, qmllint
```

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs `make test`,
`make lint` and `make check-web-dist` (committed `apps/remote-web/dist` matches a
fresh `make web`) on every push to `main` and every pull request.

Faster, one part at a time:

| You changed | Run |
|---|---|
| Go (`cmd/`, `internal/`) | `go test -race ./internal/<pkg>` or `make test-go` |
| Phone remote (`apps/remote-web/`) | `npm --prefix apps/remote-web run test:unit` |
| TV shell (`apps/tv-shell/`) | `make test-shell` |
| Contracts (`contracts/`) | `make test-go` and `npm --prefix apps/remote-web run test:unit` |
| Any Markdown | `go test ./tests/docs` (every link and #anchor must resolve) |

## Prove a test bites

A test that cannot fail documents nothing.

1. Write the test. Run it. It passes.
2. Break the code it protects on purpose (flip a condition, delete a line).
3. Run the test again. It must fail, with a message that points at the break.
4. Restore the code. The test passes again.

Say in your commit or pull request which break you tried.

## Visual changes

Render the TV offscreen and open the picture:

```sh
make shell
scripts/sandbox.sh shot --screen home --theme den     # prints the PNG path
make shots                                            # every theme × main screens in build/shots/gallery
make perf                                             # fails if an idle phase keeps drawing
```

Look at every screenshot you make. Attach the ones that show your change to
the pull request.

## Honest statuses

- *Implemented*, *automatically tested* and *seen on the TV* are three
  different claims. Write down only the ones that are true.
- A skipped, blocked or not-run check is never a pass.
- A speed claim needs numbers from a real TV box
  (`scripts/measure-target.sh 20`). Otherwise write "not measured".

## Never commit

- `target.env` (your TV's address; it is in `.gitignore`).
- Secrets, tokens, pairing codes.
- Personal details: names, IP addresses, host names, user names, emails. The
  TV is "your TV"; scripts read `target.env`.

## For AI coding agents

### Reading order

1. [`AGENTS.md`](AGENTS.md): rules, the map, the definition of done.
2. The `AGENTS.md` of the part you will change (table below).
3. [`docs/GLOSSARY.md`](docs/GLOSSARY.md) for any word you do not know.
4. [`docs/HOW_IT_WORKS.md`](docs/HOW_IT_WORKS.md) if the change crosses parts.

### Find the right guide

| Change | Read |
|---|---|
| Go coordinator, CLI, an app adapter | [`internal/AGENTS.md`](internal/AGENTS.md) |
| Anything on the TV screen | [`apps/tv-shell/AGENTS.md`](apps/tv-shell/AGENTS.md) |
| The phone remote | [`apps/remote-web/AGENTS.md`](apps/remote-web/AGENTS.md) |
| A shape (action, state, config, layout, theme) | [`contracts/AGENTS.md`](contracts/AGENTS.md) |
| A theme | [`themes/AGENTS.md`](themes/AGENTS.md), [`docs/THEMES.md`](docs/THEMES.md) |
| Playback settings for an app | [`docs/APP_PERFORMANCE.md`](docs/APP_PERFORMANCE.md) |

### Rules small models most often break here

1. **Contracts first, in one commit.** A shape change updates the schema, the
   fixtures, the Go, C++ and TypeScript validators and the spec together. See
   [`contracts/AGENTS.md`](contracts/AGENTS.md).
2. **CMake lists files explicitly.** A new QML or C++ file in `apps/tv-shell/`
   must be added to [`apps/tv-shell/CMakeLists.txt`](apps/tv-shell/CMakeLists.txt),
   or it is not built.
3. **Header comments.** Every source file opens with a comment saying what it
   owns and where its spec or doc lives. Keep it true when you change the file.
4. **Never branch on a theme id or an app's display name.** Themes and apps are
   data: a theme is a package, an app is a row in the adapter and tuning tables.
5. **Heartbeat, not timers.** Anything that loops on the TV moves on
   `World.beat` ([`World.qml`](apps/tv-shell/qml/World.qml)). No new
   free-running animations or `Timer`s for motion.
6. **No personal details.** No IPs, host names, user names or real titles in
   code, tests, fixtures, docs or screenshots. Demo content is labelled DEMO.
7. **Do not commit, push or deploy** unless your user asks.
