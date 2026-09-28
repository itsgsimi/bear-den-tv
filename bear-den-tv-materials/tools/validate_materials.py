#!/usr/bin/env python3
"""Validate this handoff package, not the Bear Den TV application.

Uses the standard library for integrity and basic semantic checks. If jsonschema
is installed, also validates the example and a set of negative schema cases.
No network access, package installation, or host configuration changes occur.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
from pathlib import Path
import sys


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--require-schema', action='store_true',
                        help='Fail rather than skip when jsonschema is unavailable.')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    manifest = root / 'SHA256SUMS'
    require(manifest.is_file(), 'SHA256SUMS is missing.')
    entries = manifest.read_text(encoding='utf-8').splitlines()
    for line in entries:
        expected, rel = line.split('  ', 1)
        file = (root / rel).resolve()
        require(file.is_relative_to(root), f'Unsafe manifest path: {rel}')
        require(file.is_file(), f'Missing file: {rel}')
        digest = hashlib.sha256(file.read_bytes()).hexdigest()
        require(digest == expected, f'Hash mismatch: {rel}')
    print(f'PASS: {len(entries)} file integrity checks (not signed authenticity).')

    prompt = root / 'Bear_Den_TV_Codex_Prompt.md'
    starter = root / 'START_PROMPT.md'
    require(prompt.is_file() and prompt.stat().st_size > 1000, 'Full bundled prompt is missing.')
    prompt_path = 'bear-den-tv-materials/Bear_Den_TV_Codex_Prompt.md'
    for relative in ['START_PROMPT.md', 'START_HERE.md', 'RESUME_PROMPT.md',
                     'templates/AGENTS.template.md']:
        require(prompt_path in (root / relative).read_text(encoding='utf-8'),
                f'Bundled prompt reference is missing from {relative}.')
    require('Bear_Den_TV_Codex_Prompt.md' in
            (root / 'CODEX_BUILD_PROMPT.md').read_text(encoding='utf-8'),
            'Legacy build prompt does not point to the bundled prompt.')
    print('PASS: Full Markdown prompt is bundled and start/resume references resolve.')

    for file in root.rglob('*.json'):
        json.loads(file.read_text(encoding='utf-8'))
    print('PASS: JSON files parse.')

    example_dir = root / 'reference' / 'examples'
    config = json.loads((example_dir / 'config.example.json').read_text())
    schema = json.loads((example_dir / 'config.schema.json').read_text())
    apps = [item['id'] for item in config['applications']]
    sections = [item['id'] for item in config['sections']]
    require(len(set(apps)) == len(apps), 'Duplicate application IDs in example.')
    require(len(set(sections)) == len(sections), 'Duplicate section IDs in example.')
    for section in config['sections']:
        require(set(section.get('application_ids', [])) <= set(apps),
                'Unresolved application reference in example.')
    require(config['remote']['enabled'] is False, 'Example exposes LAN by default.')
    require(config['startup']['autostart_enabled'] is False, 'Example enables autostart.')
    print('PASS: Example IDs/references and onboarding defaults.')

    try:
        from jsonschema import Draft202012Validator
    except ImportError:
        require(not args.require_schema, 'jsonschema is not installed; schema check required.')
        print('SKIP: JSON Schema validation (jsonschema is not installed).')
    else:
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema)
        validator.validate(config)
        cases = []
        item = copy.deepcopy(config)
        item['remote']['enabled'] = True
        cases.append(('LAN without interface', item))
        item = copy.deepcopy(config)
        item['remote'].update(enabled=True, transport='https', interfaces=['test-interface'])
        cases.append(('HTTPS without certificate/key', item))
        item = copy.deepcopy(config)
        item['remote']['pairing_expiry_seconds'] = 999
        cases.append(('Unbounded pairing expiry', item))
        item = copy.deepcopy(config)
        item['applications'][0]['launch']['command'] = 'unexpected field'
        cases.append(('Unexpected launch command', item))
        item = copy.deepcopy(config)
        item['plex_content']['enabled'] = True
        cases.append(('Connector enabled without reference', item))
        item = copy.deepcopy(config)
        item['schema_version'] = 99
        cases.append(('Unknown schema version', item))
        for label, item in cases:
            require(not validator.is_valid(item), f'Negative case wrongly accepted: {label}')
        print(f'PASS: Draft 2020-12 example and {len(cases)} negative schema cases.')

    for file in root.rglob('*.md'):
        fences = [line for line in file.read_text(encoding='utf-8').splitlines()
                  if line.lstrip().startswith('```')]
        require(len(fences) % 2 == 0, f'Unbalanced fenced blocks in {file.name}.')
    print('PASS: Markdown fenced-code balance.')
    print('No application, client, network, or hardware acceptance tests were run.')
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f'FAIL: {error}', file=sys.stderr)
        sys.exit(1)
