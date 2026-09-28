# ADR 0004 — Additive contract changes keep protocol 1

Date: 2026-09-22. Status: accepted.

[`contracts/README.md`](../../contracts/README.md) said any contract change bumps `protocol`. In practice
every consumer ships from one build: the coordinator embeds the phone remote
and supervises the shell it was deployed with, and [`scripts/deploy-target.sh`](../../scripts/deploy-target.sh)
installs both together. Several fields were added under protocol 1 without a
bump: `state.appearance`, `state.playback`, `config.startup.tune_apps`, the
`plain-dark` style and theme ids as patterns.

**Decision.** Protocol 1 covers additive changes:
- new optional fields or objects;
- new enum values, when every validator of that enum ships in the same build.

Bump `protocol`, and update the `hello`/`reject` handling in
[`contracts/ipc.md`](../../contracts/ipc.md), when a change can break a consumer built from an older
commit:
- removing or renaming a field;
- making a field required;
- changing a field's type or meaning;
- narrowing an enum or pattern;
- changing framing or the handshake.

Either way, one commit carries the schema, the fixtures, and all three
validators (Go, C++, TypeScript). [`contracts/AGENTS.md`](../../contracts/AGENTS.md) has the checklist.

**Consequence.** A phone holding a page from an older build can still see an
enum value it doesn't know. It rejects that state and recovers when the page
reloads; the coordinator serves the new page.
