# Materials-package validation

This report applies to the Codex handoff package, not a running Bear Den TV application.

The preparation checks validated the example configuration against the supplied Draft 2020-12 schema, rejected six negative schema cases, checked example app/section references and safe onboarding defaults, verified Markdown code-fence balance, parsed the JSON files, verified original reference-file preservation, and checked ZIP integrity. The package includes a SHA-256 manifest for detecting changed files.

The included Python checker re-runs package integrity, JSON parsing, basic example semantic checks, and Markdown fence checks. It also performs the schema checks when `jsonschema` is installed. `--require-schema` makes that dependency mandatory instead of skipping it. This utility does not download or install dependencies.

From the extracted package directory:

```bash
python3 tools/validate_materials.py --require-schema
```

The schema is a proposed subset. Passing it does not certify authorization, safe application launching, interface ownership, certificate trust, or a production implementation.

No Bear Den TV source was built, no clients were installed, no LAN service was exposed, and no phone/client/desktop/hardware acceptance test was performed while preparing this handoff. All application evidence templates are intentionally unexecuted.

The original source register and its dated observations were preserved, not re-audited in full. The original visual reference is included as a reference only; this is not a license grant for its media artwork, and it must not be used as proof of working integrations.

## Bundle revision 2 — single-download prompt

The previously separate `Bear_Den_TV_Codex_Prompt.md` is now included under that exact filename and matches the original Markdown file byte-for-byte. `START_PROMPT.md` contains the short copy/paste starter. Start, resume, and repository-template instructions point to that canonical prompt. `CODEX_BUILD_PROMPT.md` is retained as a compatibility pointer, not a second editable copy.

The revised archive passed ZIP integrity checks, the included validator with `--require-schema`, and explicit comparison of every original `reference/` file. Prompt-reference checks were added to the validator, and the SHA-256 manifest was regenerated for the revised package.

Editing the prompt or other materials intentionally changes their integrity hashes. The shipped manifest describes the downloaded package, not a prohibition on modifying your copy. No application or hardware tests were run for this packaging revision.
