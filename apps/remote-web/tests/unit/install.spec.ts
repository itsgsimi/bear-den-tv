// Unit tests for Add apps (src/views/install.tsx): drawn only for the
// owner's phone (never a family phone or a guest pass) while app.install is
// listed; one row per Flatpak (the web apps share Chromium's); progress and
// Cancel while an install runs; Install and Cancel send app.install and
// app.install_cancel to the shell target. The "+ Add apps" tile: same
// audience, only with something left to add, a subline built from the
// missing apps, and a tap that scrolls to the section and focuses its heading.
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import { createApp, type PageEnvironment, type WindowEnvironment } from '../../src/app.ts';
import type { ApiEnvironment, SocketLike } from '../../src/api.ts';
import type { ActionResult, Application, Install, Permission, StateSnapshot } from '../../src/contract.ts';
import { AddAppsPanel, AddAppsSection, AddAppsTile, addAppsSubline, addAppsTileShown, installEntries, mayInstall, type Revealable } from '../../src/views/install.tsx';
import { notInstalledReason, tileStatusText } from '../../src/views/remote.tsx';
import { INSTALL_READY_MS, readyAfter, tileStatus, type AppState } from '../../src/state.ts';
import type { App } from '../../src/app.ts';

type Props = Record<string, unknown> & { children?: unknown };

function walk(node: unknown, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) {
    for (const c of node) walk(c, out);
  } else if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    out.push(v);
    if (typeof v.type === 'function') walk((v.type as (p: Props) => unknown)(v.props), out);
    else walk(v.props.children, out);
  }
  return out;
}

function text(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(text).join(' ');
  if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    if (typeof v.type === 'function') return text((v.type as (p: Props) => unknown)(v.props));
    return text(v.props.children);
  }
  return '';
}

const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);

function app(id: string, adapter: string, installed: boolean, install?: Install): Application {
  return {
    id, label: id === 'youtube' ? 'YouTube' : id.charAt(0).toUpperCase() + id.slice(1), adapter, installed, version: null,
    installation: installed ? 'user' : 'none', running: false, foreground: false, launch_state: 'idle', last_error: null,
    ...(install ? { install } : {}),
  };
}

const idle = (state: Install['state'] = 'available'): Install => ({ state, progress: 0, phase: '' });

function snapshot(permissions: Permission[], apps: Application[]): StateSnapshot {
  return {
    applications: apps,
    capabilities: { 'app.install': { available: true, backend: 'flathub' } },
    me: { device_id: 'dev', device_name: 'Phone', permissions, transport_secure: false, ...(permissions.includes('guest') ? { expires_at_ms: 1 } : {}) },
  } as unknown as StateSnapshot;
}

const BROWSERS = [
  { id: 'sb', label: 'Streamer X', flatpak_id: 'org.example.StreamerX', streaming_unverified: false },
  { id: 'bt', label: 'Surfer Y', flatpak_id: 'org.example.SurferY', streaming_unverified: true },
];

const APPS = [
  app('plex-htpc', 'plex-htpc', true, idle('none')),
  app('moonlight', 'moonlight', false, idle()),
  app('netflix', 'netflix', false, { state: 'downloading', progress: 42, phase: 'runtime', size_bytes: 433_000_000 }),
  app('hulu', 'hulu', false, { state: 'downloading', progress: 42, phase: 'runtime' }),
  app('browser', 'browser', false, { state: 'downloading', progress: 42, phase: 'runtime' }),
];

function stateFor(permissions: Permission[], apps = APPS): AppState {
  return { snapshot: snapshot(permissions, apps), session: null } as unknown as AppState;
}

function panel(state: AppState) {
  const taps: [string, unknown][] = [];
  const fake = { tap: async (action: string, args: unknown) => void taps.push([action, args]) } as unknown as App;
  const vnode = AddAppsPanel({ app: fake, state });
  return { vnode, tree: walk(vnode), taps, text: text(vnode) };
}

describe('Add apps', () => {
  it('is the owner\'s only: never a family phone, a layout editor or a guest pass', () => {
    expect(mayInstall(stateFor(['owner']))).toBe(true);
    expect(panel(stateFor(['owner'])).vnode).not.toBeNull();
    for (const perms of [['controller'], ['controller', 'layout_editor'], ['guest']] as Permission[][]) {
      expect(mayInstall(stateFor(perms))).toBe(false);
      expect(panel(stateFor(perms)).vnode).toBeNull();
    }
  });

  it('lists each missing Flatpak once, the web apps as their browser from state', () => {
    // An older coordinator names no browser: one row, a plain name.
    const entries = installEntries(snapshot(['owner'], APPS));
    expect(entries.map((e) => [e.id, e.label])).toEqual([['moonlight', 'Moonlight'], ['netflix', 'Web browser']]);
    expect(entries[1]?.why).toBe('Needed for Netflix, Hulu and Browser');
    // The streaming sites and the Browser tile in different browsers: two rows, named by the TV.
    const two = { ...snapshot(['owner'], APPS), apps: { auto_update: true, streaming_browser: 'sb', browser: 'bt', browsers: BROWSERS } } as StateSnapshot;
    expect(installEntries(two).map((e) => [e.id, e.label, e.why])).toEqual([
      ['moonlight', 'Moonlight', ''],
      ['netflix', 'Streamer X', 'Needed for Netflix and Hulu'],
      ['browser', 'Surfer Y', 'Needed for Browser'],
    ]);
    // The same browser for both: one row.
    const one = { ...two, apps: { ...two.apps!, browser: 'sb' } } as StateSnapshot;
    expect(installEntries(one).map((e) => e.label)).toEqual(['Moonlight', 'Streamer X']);
  });

  it('shows progress and Cancel while installing, Install otherwise', () => {
    const { tree, taps, text: shown } = panel(stateFor(['owner']));
    expect(shown).toContain('Installing… 42%');
    expect(byTestId(tree, 'install-progress-netflix')?.props.value).toBe(42);
    expect(byTestId(tree, 'install-button-netflix')).toBeUndefined();
    (byTestId(tree, 'install-cancel-netflix')?.props.onClick as () => void)();
    (byTestId(tree, 'install-button-moonlight')?.props.onClick as () => void)();
    expect(taps).toEqual([['app.install_cancel', { app_id: 'netflix' }], ['app.install', { app_id: 'moonlight' }]]);
  });

  it('is not drawn when every app is installed, and says why installs are off', () => {
    expect(byTestId(panel(stateFor(['owner'], APPS.slice(0, 1))).tree, 'add-apps')).toBeUndefined();
    expect(byTestId(panel(stateFor(['owner'])).tree, 'add-apps')).toBeDefined();
    const off = AddAppsSection({ entries: installEntries(snapshot(['owner'], APPS)), available: false, reason: "Flatpak isn't installed on this box", art: 'pixel', onInstall: () => undefined, onCancel: () => undefined });
    const tree = walk(off);
    expect(text(off)).toContain("Flatpak isn't installed on this box");
    expect(byTestId(tree, 'install-button-moonlight')?.props.disabled).toBe(true);
  });
});

describe('Add apps tile', () => {
  function tile(state: AppState) {
    const calls: string[] = [];
    const heading: Revealable = {
      scrollIntoView: (o) => void calls.push(`scroll:${JSON.stringify(o)}`),
      focus: (o) => void calls.push(`focus:${JSON.stringify(o)}`),
    };
    const vnode = AddAppsTile({ state, heading: { current: heading } });
    return { vnode, tree: walk(vnode), calls };
  }
  const listing = (perms: Permission[], apps: Application[], capabilities: Record<string, unknown> = { 'app.install': { available: true } }): AppState =>
    ({ snapshot: { ...snapshot(perms, apps), capabilities }, session: null }) as unknown as AppState;

  it('is shown to the owner with something to add, and to no one else', () => {
    expect(addAppsTileShown(stateFor(['owner']))).toBe(true);
    expect(byTestId(tile(stateFor(['owner'])).tree, 'add-apps-tile')).toBeDefined();
    for (const perms of [['guest'], ['controller'], ['controller', 'layout_editor']] as Permission[][]) {
      expect(addAppsTileShown(stateFor(perms))).toBe(false);
      expect(tile(stateFor(perms)).vnode).toBeNull();
    }
  });

  it('is not shown when nothing is left to add, or app.install is not listed', () => {
    expect(tile(stateFor(['owner'], APPS.slice(0, 1))).vnode).toBeNull();
    expect(tile(listing(['owner'], APPS, {})).vnode).toBeNull();
    // Listed but unavailable still shows it: the section says why.
    expect(tile(listing(['owner'], APPS, { 'app.install': { available: false, reason: 'no flatpak' } })).vnode).not.toBeNull();
  });

  it('names what is missing, from the data', () => {
    const three = [app('spotify', 'spotify', false, idle()), app('netflix', 'netflix', false, idle()), app('moonlight', 'moonlight', false, idle())];
    expect(addAppsSubline(installEntries(snapshot(['owner'], three)))).toBe('Spotify, Netflix and more');
    expect(addAppsSubline(installEntries(snapshot(['owner'], three.slice(0, 2))))).toBe('Spotify and Netflix');
    expect(addAppsSubline(installEntries(snapshot(['owner'], [app('jellyfin', 'jellyfin', false, idle())])))).toBe('Jellyfin');
    expect(addAppsSubline([])).toBe('');
    // The web apps share one Chromium row: counted once, named by the app.
    expect(addAppsSubline(installEntries(snapshot(['owner'], APPS)))).toBe('Moonlight and Netflix');
    expect(text(byTestId(tile(listing(['owner'], three)).tree, 'add-apps-tile-sub'))).toBe('Spotify, Netflix and more');
  });

  it('scrolls to the section and focuses its heading when tapped', () => {
    const { tree, calls } = tile(stateFor(['owner']));
    (byTestId(tree, 'add-apps-tile')?.props.onClick as () => void)();
    expect(calls).toEqual(['scroll:{"block":"start"}', 'focus:{"preventScroll":true}']);
  });

  it('gives the section heading a focus target', () => {
    const ref = { current: null };
    const section = AddAppsSection({ entries: installEntries(snapshot(['owner'], APPS)), available: true, reason: null, art: 'pixel', onInstall: () => undefined, onCancel: () => undefined, headingRef: ref });
    const heading = walk(section).find((v) => v.props.id === 'add-apps-heading');
    expect(heading?.props.tabIndex).toBe(-1);
    expect(heading?.ref).toBe(ref);
  });
});

describe('notes in Add apps', () => {
  const noted = (a: Application, notes: string[]): Application => ({ ...a, notes });
  const rows = (apps: Application[]) => walk(AddAppsSection({ entries: installEntries(snapshot(['owner'], apps)), available: true, reason: null, art: 'pixel', onInstall: () => undefined, onCancel: () => undefined }));
  const notesIn = (tree: VNode<Props>[], id: string) => {
    const row = tree.find((v) => v.props['data-testid'] === `install-${id}`);
    return row ? walk(row).filter((v) => v.props['data-testid'] === 'app-notes').map((v) => text(v)) : [];
  };

  it("shows an app's notes in its row before Install is pressed", () => {
    const tree = rows([noted(app('moonlight', 'moonlight', false, idle()), ['Needs a gaming PC running a host.', 'Pair it once.'])]);
    expect(notesIn(tree, 'moonlight')).toEqual(['Needs a gaming PC running a host. Pair it once.']);
    expect(byTestId(tree, 'install-button-moonlight')).toBeDefined();
  });

  it("gives a browser row its first streaming site's notes", () => {
    const tree = rows([
      noted(app('browser', 'browser', false, idle()), ['Tile note.']),
      noted(app('hulu', 'hulu', false, idle()), ['Site note.']),
    ]);
    expect(installEntries(snapshot(['owner'], [noted(app('browser', 'browser', false, idle()), ['Tile note.'])]))[0]?.notes).toEqual(['Tile note.']);
    expect(notesIn(tree, 'browser')).toEqual(['Site note.']);
  });

  it('draws nothing for an app without notes', () => {
    expect(notesIn(rows([app('moonlight', 'moonlight', false, idle())]), 'moonlight')).toEqual([]);
    expect(notesIn(rows([noted(app('moonlight', 'moonlight', false, idle()), [])]), 'moonlight')).toEqual([]);
  });
});

describe('app.install request', () => {
  it('targets the shell', async () => {
    const sent: Record<string, unknown>[] = [];
    const fetch = async (path: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
      if (path === '/api/v1/actions') sent.push(body);
      const result: ActionResult = {
        protocol: 1, request_id: String(body.request_id), outcome: 'delivered', code: 'ok', message: '',
        context_epoch: 1, target: { kind: 'shell', app_id: null, label: 'Bear Den TV' }, detail: {},
      };
      return new Response(JSON.stringify(result), { status: 200 });
    };
    const socket: SocketLike = { readyState: 0, send: () => undefined, close: () => undefined, onopen: null, onclose: null, onerror: null, onmessage: null };
    const env: ApiEnvironment = {
      fetch, createSocket: () => socket, setTimeout: () => 0, clearTimeout: () => undefined, setInterval: () => 0,
      clearInterval: () => undefined, random: () => 0.5, origin: 'http://192.0.2.10:8090',
    };
    const page: PageEnvironment = { hidden: false, addEventListener: () => undefined, removeEventListener: () => undefined };
    const win: WindowEnvironment = {
      addEventListener: () => undefined, removeEventListener: () => undefined, location: { hash: '', pathname: '/', search: '' },
      history: { replaceState: () => undefined }, navigator: { userAgent: 'test' }, localStorage: null,
    };
    const client = createApp(env, page, win, { now: () => 1000 });
    await client.tap('app.install', { app_id: 'moonlight' });
    await client.tap('app.install_cancel', { app_id: 'moonlight' });
    expect(sent.map((r) => [r.action, r.target, r.args])).toEqual([
      ['app.install', 'shell', { app_id: 'moonlight' }],
      ['app.install_cancel', 'shell', { app_id: 'moonlight' }],
    ]);
  });
});

// The app tile while an install runs (state.ts tileStatus, views/remote.tsx):
// "Installing 42%" from the snapshot, "Ready" for a few seconds after it
// finished while this phone watched, never "Not installed" meanwhile.
describe('app tile install status', () => {
  const running = (progress: number): Install => ({ state: 'downloading', progress, phase: 'app' });
  it('says Installing with the percent, then Ready, then nothing', () => {
    const moon = (installed: boolean, install: Install) => app('moonlight', 'moonlight', installed, install);
    expect(tileStatusText(tileStatus(moon(false, idle()), {}))).toBe('Not installed');
    expect(tileStatusText(tileStatus(moon(false, running(42)), {}))).toBe('Installing 42%');
    expect(tileStatusText(tileStatus(moon(false, { state: 'preparing', progress: 0, phase: 'checking' }), {}))).toBe('Installing 0%');
    expect(tileStatusText(tileStatus(moon(true, idle('done')), { moonlight: 5000 }))).toBe('Ready');
    expect(tileStatusText(tileStatus(moon(true, idle('done')), {}))).toBeNull();
  });

  it('marks Ready only for an install this phone saw finish', () => {
    const before = snapshot(['owner'], [app('moonlight', 'moonlight', false, running(97)), app('plex-htpc', 'plex-htpc', true, idle('done'))]);
    const after = snapshot(['owner'], [app('moonlight', 'moonlight', true, idle('done')), app('plex-htpc', 'plex-htpc', true, idle('done'))]);
    expect(readyAfter(before, after, {}, 1000)).toEqual({ moonlight: 1000 + INSTALL_READY_MS });
    expect(readyAfter(null, after, {}, 1000)).toEqual({}); // first snapshot: nothing seen finishing
    expect(readyAfter(after, after, {}, 1000)).toEqual({});
    // Installing again drops it.
    expect(readyAfter(after, before, { moonlight: 9000 }, 1000)).toEqual({});
  });

  it('the controller ends Ready after INSTALL_READY_MS', async () => {
    const snaps = [
      snapshot(['owner'], [app('moonlight', 'moonlight', false, running(97))]),
      snapshot(['owner'], [app('moonlight', 'moonlight', true, idle('done'))]),
    ];
    let clock = 1000;
    const timers: [() => void, number][] = [];
    const fetch = async () => new Response(JSON.stringify(snaps.shift()), { status: 200 });
    const socket: SocketLike = { readyState: 0, send: () => undefined, close: () => undefined, onopen: null, onclose: null, onerror: null, onmessage: null };
    const env = {
      fetch, createSocket: () => socket, setTimeout: (fn: () => void, ms: number) => timers.push([fn, ms]), clearTimeout: () => undefined,
      setInterval: () => 0, clearInterval: () => undefined, random: () => 0.5, origin: 'http://192.0.2.10:8090',
    } as unknown as ApiEnvironment;
    const page: PageEnvironment = { hidden: false, addEventListener: () => undefined, removeEventListener: () => undefined };
    const win: WindowEnvironment = {
      addEventListener: () => undefined, removeEventListener: () => undefined, location: { hash: '', pathname: '/', search: '' },
      history: { replaceState: () => undefined }, navigator: { userAgent: 'test' }, localStorage: null,
    };
    const client = createApp(env, page, win, { now: () => clock });
    await client.refreshState();
    expect(timers).toHaveLength(0);
    await client.refreshState();
    expect(client.store.getState().installReady).toEqual({ moonlight: 1000 + INSTALL_READY_MS });
    expect(timers.map(([, ms]) => ms)).toEqual([INSTALL_READY_MS]);
    clock += INSTALL_READY_MS;
    timers[0]![0]();
    expect(client.store.getState().installReady).toEqual({});
  });
});

// Wording for a tile that is not installed: the owner's phone points at Add
// apps; other phones learn only the owner installs (on the TV or their phone).
describe('not installed wording', () => {
  it('differs for the owner and for other phones, and while installing', () => {
    const moon = app('moonlight', 'moonlight', false, idle());
    expect(notInstalledReason(moon, stateFor(['owner']))).toBe('Moonlight is not installed on this TV. Install it under Add apps below.');
    expect(notInstalledReason(moon, stateFor(['controller']))).toBe("Moonlight is not installed on this TV. Only the TV's owner can install apps.");
    expect(notInstalledReason(moon, stateFor(['guest']))).toBe("Moonlight is not installed on this TV. Only the TV's owner can install apps.");
    const busy = app('moonlight', 'moonlight', false, { state: 'downloading', progress: 10, phase: 'app' });
    expect(notInstalledReason(busy, stateFor(['controller']))).toBe('Moonlight is installing on the TV.');
  });
});
