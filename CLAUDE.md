# Claude Code: start here

Read [`AGENTS.md`](AGENTS.md) first, then the `AGENTS.md` of every directory you
touch. [`CONTRIBUTING.md`](CONTRIBUTING.md) has the workflow; words are in
[`docs/GLOSSARY.md`](docs/GLOSSARY.md).

The three rules broken most often:

1. **Contracts first, in one commit.** A shape change touches the schema, the
   fixtures, the Go, C++ and TypeScript validators and the spec together
   ([`contracts/AGENTS.md`](contracts/AGENTS.md)).
2. **Prove the test bites.** Run the checks (`. scripts/env.sh && make test`),
   break the code your test protects, watch it fail, restore it.
3. **No personal details, no secrets.** Never commit `target.env`, IPs, host
   names, user names or tokens. The TV is "your TV".
