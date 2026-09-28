---
name: "resume-bear-den-tv-progress"
description: "Orient on Bear Den TV work in progress: repo state, status docs, build/test health and the TV's condition, in one read-only pass."
whenToUse: "When asked where work left off, to review progress, or before resuming implementation or deploy work in a Bear Den TV checkout."
---

# Resume Bear Den TV progress

Goal: know exactly where work stopped (code state and live TV state) before
touching anything. This pass is read-only: no edits, no deploys, no restarts.

## 1. Repo and docs (run in parallel)

- `git log --oneline -15` and `git status --short`: the last commit and any
  uncommitted work.
- `docs/IMPLEMENTATION_STATUS.md`: read "Current checkpoint", "Known test
  gaps", "Blockers and permissions" and "Next steps". It is the project's
  memory; trust it, but check that each blocker is still true.
- `docs/VALIDATION_REPORT.md`: what was really seen on the TV, and what was
  blocked or not run.

## 2. Build and test health

```sh
. scripts/env.sh          # toolchain from scripts/bootstrap-toolchain.sh
make test                 # Go (race) + web + shell (offscreen)
make lint                 # gofmt/vet, eslint/tsc, qmllint (qmllint warns only)
go test ./tests/docs      # doc links and anchors
```

If the toolchain is missing, run `scripts/bootstrap-toolchain.sh` first (it
installs into `~/.bdtv-toolchain`, user-space only). `git diff --stat` shows
uncommitted deltas.

## 3. The TV (only if `target.env` exists)

The scripts read your TV's ssh destination from `target.env` (copy
`target.env.example`). Without it, skip this step and say so.

```sh
scripts/target.sh ssh 'build/bin/bear-den-tv doctor'   # what runs, what is in front, what is allowed
scripts/target.sh ssh 'who; pgrep -af bear-den-tv'     # an active graphical session? Bear Den running?
```

- `scripts/target.sh ssh` runs without syncing; `run` syncs first. Use `ssh`
  here: this pass changes nothing.
- A seat on the login greeter (no user on `:0` in `who`) means windows never
  map; live checks must wait.

## 4. Report

Summarize as:

1. Done and green (with the commands that proved it).
2. Gaps that block the next step.
3. Blockers whose status changed since the status doc was written.
4. A proposed order to resume.

Keep *implemented*, *automatically tested* and *seen on the TV* separate. A
skipped or not-run check is never a pass.
