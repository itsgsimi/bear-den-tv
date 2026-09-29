// Reducer and selector tests for src/state.ts: pure transitions only, fixed clock values.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { ActionRequest, ActionResult, Layout, Session, StateSnapshot } from '../../src/contract.ts';
import {
  type AppState,
  canWriteLayout,
  capabilityFor,
  closableApp,
  cloneLayout,
  createStore,
  initialState,
  latestResultFor,
  layoutsEqual,
  reduce,
  visibleApps,
  visibleTabs,
} from '../../src/state.ts';

const fixture = <T>(name: string): T => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as T;

const snapshot = fixture<StateSnapshot>('state.phone-controller.valid.json');
const layout = fixture<Layout>('layout.default.valid.json');

const session: Session = {
  device_id: 'dev-1',
  device_name: 'Pixel',
  permissions: ['controller'],
  csrf_token: 'csrf',
  transport_secure: false,
};

function online(): AppState {
  let s = reduce(initialState('Pixel'), { type: 'session_started', session });
  s = reduce(s, { type: 'connection_changed', connection: 'online' });
  return reduce(s, { type: 'state_received', snapshot, at: 1000 });
}

function request(id: string): ActionRequest {
  return { protocol: 1, request_id: id, context_epoch: snapshot.context_epoch, target: 'active', action: 'nav.left', args: {} };
}

function result(id: string, outcome: ActionResult['outcome'], code: ActionResult['code'] = 'ok'): ActionResult {
  return {
    protocol: 1,
    request_id: id,
    outcome,
    code,
    message: code === 'ok' ? '' : 'nope',
    context_epoch: snapshot.context_epoch,
    target: { kind: 'app', app_id: 'youtube', label: 'YouTube' },
    detail: {},
  };
}

describe('reduce', () => {
  it('starts on the loading screen and moves to the app on session start', () => {
    const s0 = initialState('Pixel');
    expect(s0.screen).toBe('loading');
    const s1 = reduce(s0, { type: 'session_started', session });
    expect(s1.screen).toBe('app');
    expect(s1.tab).toBe('remote');
    expect(s1.session).toBe(session);
  });

  it('keeps delivered, observed and failed outcomes distinct per request', () => {
    let s = online();
    s = reduce(s, { type: 'action_sent', request: request('a'), at: 10 });
    expect(s.pending.a?.outcome).toBeNull();
    s = reduce(s, { type: 'action_result', result: result('a', 'delivered'), at: 11 });
    expect(s.pending.a?.outcome).toBe('delivered');
    s = reduce(s, { type: 'action_result', result: result('a', 'observed'), at: 12 });
    expect(s.pending.a?.outcome).toBe('observed');
    expect(s.lastError).toBeNull();
    s = reduce(s, { type: 'action_sent', request: request('b'), at: 13 });
    s = reduce(s, { type: 'action_result', result: result('b', 'failed', 'stale_epoch'), at: 14 });
    expect(s.pending.b?.outcome).toBe('failed');
    expect(s.lastError).toEqual({ code: 'stale_epoch', message: 'nope' });
    expect(latestResultFor(s, 'nav.left')?.request_id).toBe('b');
  });

  it('ignores results for requests it never sent', () => {
    const s = online();
    expect(reduce(s, { type: 'action_result', result: result('ghost', 'observed'), at: 1 })).toBe(s);
  });

  it('marks a transport failure as failed/transport', () => {
    let s = reduce(online(), { type: 'action_sent', request: request('a'), at: 1 });
    s = reduce(s, { type: 'action_send_failed', request_id: 'a', message: 'offline', at: 2 });
    expect(s.pending.a).toMatchObject({ outcome: 'failed', code: 'transport', message: 'offline' });
  });

  it('prunes old pending entries and returns the same object when nothing changes', () => {
    let s = reduce(online(), { type: 'action_sent', request: request('a'), at: 1 });
    s = reduce(s, { type: 'action_sent', request: request('b'), at: 100 });
    const pruned = reduce(s, { type: 'pending_pruned', before: 50 });
    expect(Object.keys(pruned.pending)).toEqual(['b']);
    expect(reduce(pruned, { type: 'pending_pruned', before: 50 })).toBe(pruned);
  });

  it('cancels an active hold when the connection drops', () => {
    let s = reduce(online(), { type: 'hold_started', hold_id: 'h1', action: 'nav.right' });
    expect(s.hold.phase).toBe('active');
    s = reduce(s, { type: 'connection_changed', connection: 'reconnecting' });
    expect(s.hold).toMatchObject({ phase: 'cancelled', reason: 'disconnected' });
  });

  it('applies server hold events only for the current hold id', () => {
    const s = reduce(online(), { type: 'hold_started', hold_id: 'h1', action: 'nav.right' });
    expect(reduce(s, { type: 'hold_event', event: { type: 'hold', hold_id: 'other', state: 'busy' } })).toBe(s);
    const busy = reduce(s, { type: 'hold_event', event: { type: 'hold', hold_id: 'h1', state: 'busy', reason: 'Kitchen phone' } });
    expect(busy.hold).toMatchObject({ phase: 'busy', reason: 'Kitchen phone' });
    expect(reduce(busy, { type: 'hold_stopped' }).hold.phase).toBe('idle');
  });

  it('drops back to the remote tab when a permission disappears', () => {
    let s = reduce(online(), { type: 'tab_selected', tab: 'editor' });
    s = reduce(s, { type: 'state_received', snapshot: { ...snapshot, me: { ...snapshot.me!, permissions: ['controller'] } }, at: 2 });
    expect(s.tab).toBe('remote');
  });

  it('returns to pairing on session end while keeping the device name and info', () => {
    const s = reduce(online(), { type: 'session_ended', reason: 'revoked' });
    expect(s.screen).toBe('pair');
    expect(s.session).toBeNull();
    expect(s.snapshot).toBeNull();
    expect(s.pair.notice).toBe('revoked');
    expect(s.pair.device_name).toBe('Pixel');
  });

  it('loads the editor with an independent draft and records pendingAt', () => {
    const pending = { revision: 4, previous_revision: 3, expires_in_s: 15, source: 'web' as const };
    let s = reduce(online(), { type: 'editor_loaded', doc: { revision: 3, layout, defaults: layout, pending }, at: 5000 });
    expect(s.editor).toMatchObject({ status: 'ready', revision: 3, pending, pendingAt: 5000 });
    expect(s.editor.draft).not.toBe(layout);
    expect(layoutsEqual(s.editor.draft, layout)).toBe(true);
    s = reduce(s, { type: 'editor_pending', pending: null, at: 6000 });
    expect(s.editor).toMatchObject({ pending: null, pendingAt: 6000 });
  });

  it('refreshes the editor pending state from snapshots only once the editor is ready', () => {
    const pending = { revision: 9, previous_revision: 8, expires_in_s: 20, source: 'tv' as const };
    const idle = reduce(online(), { type: 'state_received', snapshot: { ...snapshot, layout_pending: pending }, at: 7 });
    expect(idle.editor.pending).toBeNull();
    let s = reduce(online(), { type: 'editor_loaded', doc: { revision: 8, layout, defaults: layout, pending: null }, at: 1 });
    s = reduce(s, { type: 'state_received', snapshot: { ...snapshot, layout_pending: pending }, at: 7 });
    expect(s.editor).toMatchObject({ pending, pendingAt: 7 });
  });
});

describe('selectors', () => {
  it('treats unlisted capabilities as unavailable', () => {
    const s = online();
    expect(capabilityFor(s.snapshot, 'nav.left')).toMatchObject({ available: true, holdable: true });
    expect(capabilityFor(s.snapshot, 'media.pause')).toMatchObject({ available: false });
    expect(capabilityFor(s.snapshot, 'shell.restart')).toEqual({ available: false });
    expect(capabilityFor(null, 'select')).toEqual({ available: false });
  });

  it('shows tabs by permission', () => {
    const s = online();
    const withPerms = (permissions: NonNullable<StateSnapshot['me']>['permissions']) =>
      reduce(s, { type: 'state_received', snapshot: { ...snapshot, me: { ...snapshot.me!, permissions } }, at: 3 });
    expect(visibleTabs(withPerms(['controller']))).toEqual(['remote', 'about']);
    expect(visibleTabs(withPerms(['controller', 'layout_editor', 'owner']))).toEqual(['remote', 'editor', 'devices', 'about']);
  });

  it('refuses layout writes over trusted-LAN HTTP unless the TV allows it', () => {
    const base = online();
    const editorSnap = (https: boolean, http_layout_editing: boolean): AppState =>
      reduce(base, {
        type: 'state_received',
        snapshot: {
          ...snapshot,
          me: { ...snapshot.me!, permissions: ['controller', 'layout_editor'] },
          remote: { ...snapshot.remote, https, transport: https ? 'https' : 'trusted-lan-http', http_layout_editing },
        },
        at: 4,
      });
    expect(canWriteLayout(editorSnap(false, false))).toBe(false);
    expect(canWriteLayout(editorSnap(false, true))).toBe(true);
    expect(canWriteLayout(editorSnap(true, false))).toBe(true);
  });

  it('cloneLayout produces an independent copy', () => {
    const copy = cloneLayout(layout);
    expect(copy).toEqual(layout);
    copy.ui.accent = '#000000';
    copy.sections.forEach((s) => {
      s.title = 'changed';
      s.application_ids?.push('extra');
    });
    expect(layout.ui.accent).not.toBe('#000000');
    expect(layout.sections.every((s) => s.title !== 'changed' && !s.application_ids?.includes('extra'))).toBe(true);
  });
});

describe('createStore', () => {
  it('notifies subscribers only on change and supports unsubscribe', () => {
    const store = createStore(initialState());
    const seen: string[] = [];
    const off = store.subscribe((s) => seen.push(s.tab));
    store.dispatch({ type: 'tab_selected', tab: 'remote' });
    expect(seen).toEqual([]);
    store.dispatch({ type: 'tab_selected', tab: 'about' });
    expect(seen).toEqual(['about']);
    off();
    store.dispatch({ type: 'tab_selected', tab: 'remote' });
    expect(seen).toEqual(['about']);
  });
});

describe('closableApp', () => {
  const withApps = (flags: Array<[string, boolean, boolean]>): StateSnapshot => ({
    ...snapshot,
    applications: flags.map(([id, running, foreground]) => ({
      id,
      label: id,
      adapter: id,
      installed: true,
      version: null,
      installation: 'user',
      running,
      foreground,
      launch_state: running ? 'running' : 'idle',
      last_error: null,
    })),
  });

  it('prefers the app in front, then one running behind Home, else none', () => {
    expect(closableApp(withApps([['plex-htpc', true, false], ['youtube', true, true]]))?.id).toBe('youtube');
    expect(closableApp(withApps([['plex-htpc', false, false], ['moonlight', true, false]]))?.id).toBe('moonlight');
    expect(closableApp(withApps([['plex-htpc', false, false]]))).toBeNull();
    expect(closableApp(null)).toBeNull();
  });
});

describe('visibleApps', () => {
  it('drops optional apps the coordinator marks hidden and keeps the rest in order', () => {
    const optional = fixture<StateSnapshot>('state.phone-optional-apps.valid.json');
    expect(optional.applications.map((a) => a.id)).toEqual(['plex-htpc', 'youtube', 'spotify', 'retroarch']);
    expect(visibleApps(optional).map((a) => a.id)).toEqual(['plex-htpc', 'youtube', 'spotify']);
    expect(visibleApps(snapshot)).toHaveLength(snapshot.applications.length);
    expect(visibleApps(null)).toEqual([]);
  });
});
