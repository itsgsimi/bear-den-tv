# Bear Den TV — Codex implementation materials

This package contains the build prompt and the complete previous research/design bundle. It is **not application source, an installer, or a report of successful hardware testing**.

## Start an implementation

Extract the archive into the repository or working directory where Bear Den TV should be built. Keep the folder named `bear-den-tv-materials`. Existing project files stay outside this reference folder and must be preserved.

Open that workspace in Codex and paste the contents of `START_PROMPT.md` (also shown below). The full Markdown prompt is included as `Bear_Den_TV_Codex_Prompt.md`; no separate download is needed. All prompt and materials paths resolve from the workspace containing `bear-den-tv-materials/`.

```text
Build Bear Den TV in this repository.

Read and execute the complete implementation prompt at:
bear-den-tv-materials/Bear_Den_TV_Codex_Prompt.md

Use all supporting materials it references, including the design specification,
implementation notes, acceptance criteria, configuration examples, and visual
concept. Everything is bundled inside bear-den-tv-materials/.

Inspect the existing repository, preserve existing work, make a short plan,
and begin implementing, building, and testing. Do not stop at planning or a
static mockup. Follow the full prompt's scope, approval requirements, and
validation rules. Record blocked live-device checks honestly and continue
independent implementation work.
```

The original generated concept image (`reference/assets/original-visual-concept.png`) is not included in the public repository: it showed third-party film artwork and streaming-service logos.

For a fresh session after work has begun, use `RESUME_PROMPT.md` rather than asking for a new architecture from scratch.

## What is included

| Material | Purpose |
|---|---|
| `START_PROMPT.md` | Short copy/paste starter pointing to the bundled full prompt |
| `Bear_Den_TV_Codex_Prompt.md` | Full implementation assignment and boundaries; edit this file to customize the build instructions |
| `CODEX_BUILD_PROMPT.md` | Compatibility pointer for older starter messages; no duplicate prompt to maintain |
| `IMPLEMENTATION_NOTES.md` | Execution clarifications and difficult edge cases |
| `ACCEPTANCE_MATRIX.md` | Requirement IDs, verification methods, and evidence expectations |
| `reference/Bear_Den_TV_Design_and_Implementation.md` | Complete product/architecture specification and source register |
| `reference/IMPLEMENTATION_HANDOFF.md` | Original high-level handoff, preserved unchanged |
| `reference/ACCEPTANCE_CHECKLIST.md` | Original unexecuted release checklist |
| `reference/RESEARCH_SOURCES.md` | Original 35-source technical research register |
| `reference/examples/` | Proposed configuration, Draft 2020-12 schema, and caveats |
| `templates/` | Small repository-guidance, status, validation, and compatibility templates |
| `tools/validate_materials.py` | Local package-integrity and example-schema checks, not application tests |
| `VALIDATION.md` | Checks actually performed on this materials package |
| `SHA256SUMS` | Integrity hashes for package contents; not a signed authenticity claim |

## Document interpretation

Respect the actual user request, repository guidance, and tool approval policy. Within this package, the build prompt and implementation notes clarify execution; the main design specifies product behavior; the original handoff summarizes it. Examples and the screenshot do not override security, product scope, or observed platform behavior.

`reference/` is preserved byte-for-byte from the earlier bundle. Its original validation report applies to that original bundle only. The newly generated top-level `VALIDATION.md` applies to this package. Historical source/version observations are not a fresh client-compatibility certification; verify the relevant primary documentation and installed versions during implementation.

Do not copy the entire specification into `AGENTS.md`. Use concise repository guidance pointing to the authoritative files. The templates are suggestions, not existing production files or commands.

## External prerequisites

All materials created for this project handoff are included. Build toolchains, operating-system packages, Plex HTPC, VacuumTube, service accounts, certificates, and hardware access are **not bundled**. They must be discovered or configured with the user's authorization where needed. Do not expect secrets, a working Plex server, or a universal HTTPS certificate in this archive.
