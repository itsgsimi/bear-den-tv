// Contract tests: every fixture in contracts/fixtures validates (or is rejected)
// against the JSON Schemas with Ajv (draft 2020-12), and messages built by the
// client code (action requests, hold messages, edited layouts) conform too.
// The two `config.*.invalid.json` fixtures are semantic rejections (dangling
// reference, secret key) enforced by the Go config loader, not by JSON Schema,
// so they are listed as out of scope here rather than asserted.
import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { Ajv2020 } from 'ajv/dist/2020.js';
import type { ActionRequest, HoldMessage, Layout, StateSnapshot } from '../src/contract.ts';
import { PROTOCOL } from '../src/contract.ts';
import { HoldController } from '../src/hold.ts';
import { cloneLayout } from '../src/state.ts';
import { uuidV4 } from '../src/uuid.ts';
import { moveSection } from '../src/views/editor.tsx';

const contracts = new URL('../../../contracts/', import.meta.url);
const fixtures = new URL('fixtures/', contracts);
const BASE = 'https://bear-den-tv.local/contracts/';

const readJson = (url: URL): unknown => JSON.parse(readFileSync(url, 'utf8'));

const ajv = new Ajv2020({ allErrors: true, strict: false });
for (const name of ['layout.schema.json', 'action.schema.json', 'state.schema.json', 'config.schema.json']) {
  ajv.addSchema(readJson(new URL(name, contracts)) as object);
}

function validator(ref: string) {
  const validate = ajv.getSchema(BASE + ref);
  if (!validate) throw new Error(`schema ${ref} not found`);
  return validate;
}

const SCHEMA_FOR: Record<string, string> = {
  'action.request.nav-left.valid.json': 'action.schema.json#/$defs/request',
  'action.request.app-launch.valid.json': 'action.schema.json#/$defs/request',
  'action.request.app-install.valid.json': 'action.schema.json#/$defs/request',
  'action.request.app-install-cancel.valid.json': 'action.schema.json#/$defs/request',
  'action.request.app-install-ref.invalid.json': 'action.schema.json#/$defs/request',
  'action.request.app-uninstall.valid.json': 'action.schema.json#/$defs/request',
  'action.request.app-uninstall-system.invalid.json': 'action.schema.json#/$defs/request',
  'state.install-installed-bytes.invalid.json': 'state.schema.json',
  'state.phone-audio.valid.json': 'state.schema.json',
  'state.shell-audio.invalid.json': 'state.schema.json',
  'state.locked-audio.invalid.json': 'state.schema.json',
  'state.phone-owner-installs.valid.json': 'state.schema.json',
  'state.family-install.invalid.json': 'state.schema.json',
  'state.guest-install.invalid.json': 'state.schema.json',
  'state.install-bad-state.invalid.json': 'state.schema.json',
  'state.install-progress-range.invalid.json': 'state.schema.json',
  'state.install-ref.invalid.json': 'state.schema.json',
  'config.apps-auto-update-not-bool.invalid.json': 'config.schema.json',
  'action.request.shell-string.invalid.json': 'action.schema.json#/$defs/request',
  'action.request.extra-args.invalid.json': 'action.schema.json#/$defs/request',
  'action.result.observed.valid.json': 'action.schema.json#/$defs/result',
  'action.result.failed-stale.valid.json': 'action.schema.json#/$defs/result',
  'action.request.power-sleep-timer.valid.json': 'action.schema.json#/$defs/request',
  'action.request.sleep-timer-odd-minutes.invalid.json': 'action.schema.json#/$defs/request',
  'action.request.display-off.valid.json': 'action.schema.json#/$defs/request',
  'action.result.display-off-woke.valid.json': 'action.schema.json#/$defs/result',
  'state.phone-sleep-warning.valid.json': 'state.schema.json',
  'state.power-bad-display.invalid.json': 'state.schema.json',
  'action.request.tv-power.valid.json': 'action.schema.json#/$defs/request',
  'action.request.tv-power-off.invalid.json': 'action.schema.json#/$defs/request',
  'state.phone-cec.valid.json': 'state.schema.json',
  'state.phone-cec-unavailable.valid.json': 'state.schema.json',
  'state.cec-bad-tv-power.invalid.json': 'state.schema.json',
  'state.shell-setup.valid.json': 'state.schema.json',
  'state.onboarding-not-bool.invalid.json': 'state.schema.json',
  'state.autostart-no-available.invalid.json': 'state.schema.json',
  'state.phone-autostart.invalid.json': 'state.schema.json',
  'state.shell-tips.valid.json': 'state.schema.json',
  'state.tips-unknown-id.invalid.json': 'state.schema.json',
  'state.phone-tips.invalid.json': 'state.schema.json',
  'config.tips.valid.json': 'config.schema.json',
  'config.tips-streak-too-high.invalid.json': 'config.schema.json',
  'state.phone-app-notes.valid.json': 'state.schema.json',
  'state.app-notes-too-many.invalid.json': 'state.schema.json',
  'state.app-note-url.invalid.json': 'state.schema.json',
  'state.app-note-too-long.invalid.json': 'state.schema.json',
  'config.cec-bad-volume-target.invalid.json': 'config.schema.json',
  'state.shell-home.valid.json': 'state.schema.json',
  'state.phone-controller.valid.json': 'state.schema.json',
  'state.phone-now-playing.valid.json': 'state.schema.json',
  'state.phone-optional-apps.valid.json': 'state.schema.json',
  'state.hidden-not-bool.invalid.json': 'state.schema.json',
  'state.now-playing-bad-status.invalid.json': 'state.schema.json',
  'state.now-playing-no-title.invalid.json': 'state.schema.json',
  'state.phone-now-playing-behind-home.valid.json': 'state.schema.json',
  'state.now-playing-behind-home-stopped.invalid.json': 'state.schema.json',
  'state.now-playing-foreground-not-bool.invalid.json': 'state.schema.json',
  'state.phone-now-playing-plex-server.valid.json': 'state.schema.json',
  'state.now-playing-bad-source.invalid.json': 'state.schema.json',
  'state.shell-plex-linking.valid.json': 'state.schema.json',
  'state.shell-plex-libraries.valid.json': 'state.schema.json',
  'state.plex-bad-status.invalid.json': 'state.schema.json',
  'state.plex-token-field.invalid.json': 'state.schema.json',
  'state.plex-stored-in-bad.invalid.json': 'state.schema.json',
  'state.phone-guest.valid.json': 'state.schema.json',
  'state.shell-guest-pass.valid.json': 'state.schema.json',
  'state.guest-with-controller.invalid.json': 'state.schema.json',
  'state.guest-no-expiry.invalid.json': 'state.schema.json',
  'state.achievements-time.invalid.json': 'state.schema.json',
  'state.achievements-title.invalid.json': 'state.schema.json',
  'state.guest-achievements.invalid.json': 'state.schema.json',
  'state.locked-achievements.invalid.json': 'state.schema.json',
  'state.phone-celebrate.invalid.json': 'state.schema.json',
  'config.achievements-not-bool.invalid.json': 'config.schema.json',
  'config.default.valid.json': 'config.schema.json',
  'config.now-playing-not-bool.invalid.json': 'config.schema.json',
  'layout.default.valid.json': 'layout.schema.json',
  'layout.art-style-bad.invalid.json': 'layout.schema.json',
  'layout.app-icons-bear-den.valid.json': 'layout.schema.json',
  'layout.app-icons-bad.invalid.json': 'layout.schema.json',
  
  'config.weather-no-place.invalid.json': 'config.schema.json',
  'action.request.pointer-move.valid.json': 'action.schema.json#/$defs/request',
  'action.request.pointer-click.valid.json': 'action.schema.json#/$defs/request',
  'action.request.pointer-scroll.valid.json': 'action.schema.json#/$defs/request',
  'action.request.pointer-move-too-far.invalid.json': 'action.schema.json#/$defs/request',
  'action.request.pointer-click-coordinates.invalid.json': 'action.schema.json#/$defs/request',
  'state.phone-web-app.valid.json': 'state.schema.json',
  'state.enabled-not-bool.invalid.json': 'state.schema.json',
  'config.web-apps.valid.json': 'config.schema.json',
  'config.browsers-swapped.valid.json': 'config.schema.json',
  'config.browser-unknown.invalid.json': 'config.schema.json',
  'state.phone-owner-browsers.valid.json': 'state.schema.json',
  'state.browser-no-note.invalid.json': 'state.schema.json',
  'config.web-http.invalid.json': 'config.schema.json',
  'config.web-no-url.invalid.json': 'config.schema.json',
};
const SEMANTIC_ONLY = new Set(['config.dangling-ref.invalid.json', 'config.token-leak.invalid.json', 'config.weather-precise.invalid.json',
  // Rule 11 (config.md): user info, another host and launch args are Go-only semantic checks.
  'config.web-credentials.invalid.json', 'config.web-wrong-host.invalid.json', 'config.web-launch-args.invalid.json',
  // Rule 3 (config.md): a web row's launch.app_id must be the browser config apps names for it.
  'config.browser-mismatch.invalid.json']);

describe('contracts/fixtures', () => {
  const files = readdirSync(fixtures).filter((f) => f.endsWith('.json'));

  it('has a schema mapping for every fixture', () => {
    expect(files.length).toBeGreaterThan(0);
    for (const file of files) expect(SCHEMA_FOR[file] !== undefined || SEMANTIC_ONLY.has(file), file).toBe(true);
  });

  for (const file of files.filter((f) => !SEMANTIC_ONLY.has(f))) {
    const valid = file.endsWith('.valid.json');
    it(`${file} is ${valid ? 'accepted' : 'rejected'} by ${SCHEMA_FOR[file]}`, () => {
      const validate = validator(SCHEMA_FOR[file] ?? '');
      const ok = validate(readJson(new URL(file, fixtures)));
      expect(ok, JSON.stringify(validate.errors)).toBe(valid);
    });
  }
});

describe('client-built messages', () => {
  it('target accepts "active", "shell", and application ids', () => {
    const validate = validator('action.schema.json#/$defs/target');
    for (const t of ['active', 'shell', 'plex-htpc']) expect(validate(t), t).toBe(true);
    expect(validate('Not An Id')).toBe(false);
  });

  it('action requests as built by the controller validate', () => {
    const validate = validator('action.schema.json#/$defs/request');
    const requests: ActionRequest[] = [
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'active', action: 'nav.up', args: {} },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'home', args: {} },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'app.launch', args: { app_id: 'plex-htpc' } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'active', action: 'media.seek_relative', args: { seconds: -30 } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'active', action: 'audio.volume_delta', args: { delta: 5 } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'active', action: 'audio.mute', args: { muted: true } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'active', action: 'text.submit', args: { text: 'hello' } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'power.sleep_timer', args: { minutes: 90 } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'power.sleep_timer', args: { minutes: 0 } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'display.off', args: {} },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'tv.power', args: { power: 'on' } },
      { protocol: PROTOCOL, request_id: uuidV4(), context_epoch: 3, target: 'shell', action: 'tv.power', args: { power: 'standby' } },
    ];
    for (const request of requests) expect(validate(request), `${request.action}: ${JSON.stringify(validate.errors)}`).toBe(true);
  });

  it('hold messages sent by HoldController validate', () => {
    const validate = validator('action.schema.json#/$defs/holdMessage');
    const sent: HoldMessage[] = [];
    let tick: () => void = () => undefined;
    const controller = new HoldController(
      { send: (m) => (sent.push(m), true) },
      { renewMs: 200, newId: () => uuidV4(), timers: { setInterval: (h) => ((tick = h), 1), clearInterval: () => undefined } },
      { onStart: () => undefined, onEnd: () => undefined, onUnavailable: () => undefined },
    );
    controller.begin('nav.left', 12);
    tick();
    controller.release();
    expect(sent.map((m) => m.type)).toEqual(['hold.start', 'hold.renew', 'hold.stop']);
    for (const message of sent) expect(validate(message), JSON.stringify(validate.errors)).toBe(true);
  });

  it('playback settings in the shell fixture are typed and offer their own values', () => {
    const state = readJson(new URL('state.shell-home.valid.json', fixtures)) as StateSnapshot;
    const settings = (state.playback?.apps ?? []).flatMap((a) => a.settings ?? []);
    expect(settings.length).toBeGreaterThan(0);
    for (const s of settings) {
      const offered = s.options.map((o) => o.value);
      expect(offered, s.id).toContain(s.auto);
      expect(offered, s.id).toContain(s.value);
    }
    expect(settings.some((s) => s.overridden && s.value !== s.auto)).toBe(true);
  });

  it('config playback overrides follow the schema', () => {
    const validate = validator('config.schema.json');
    const base = readJson(new URL('config.default.valid.json', fixtures)) as Record<string, unknown>;
    expect(validate({ ...base, playback: { overrides: { moonlight: { fps: '60' } } } }), JSON.stringify(validate.errors)).toBe(true);
    expect(validate({ ...base, playback: { overrides: { moonlight: { fps: 'sixty fps!' } } } })).toBe(false);
    expect(validate({ ...base, playback: { overrides: { 'Moon Light': { fps: '60' } } } })).toBe(false);
  });

  it('layouts edited like the editor does still validate', () => {
    const validate = validator('layout.schema.json');
    const base = readJson(new URL('layout.default.valid.json', fixtures)) as Layout;
    const edited = moveSection(cloneLayout(base), 0, 1);
    edited.ui = { ...edited.ui, accent: '#AABBCC', text_scale: 1.3, safe_margin_percent: 5, tile_density: 'large', background: 'forest', art_style: 'classic' };
    const first = edited.sections[0];
    if (first) first.enabled = !first.enabled;
    expect(validate(edited), JSON.stringify(validate.errors)).toBe(true);
    expect(validate({ ...edited, ui: { ...edited.ui, accent: 'red' } })).toBe(false);
  });
});
