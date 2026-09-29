// Application controller: the only place that performs side effects.
// Contract: `createApp` wires the API client, the events socket, the hold
// controller, and the store. Views call its methods; every outcome is reported
// back through store events, never through return values views must interpret.
// Boot order: `/api/v1/info`, then a `#pair=` fragment (the TV's QR link;
// `#invite=` is accepted too) stripped from the
// address before the claim request, else `/api/v1/session`; a session starts the
// socket, whose every open fetches `/api/v1/state`. Actions are one HTTP POST each
// with the epoch of the last rendered snapshot; nothing is queued or replayed.
// `revoked` (message or close 4001) and any 401 end the session and return to Pair.

import { ApiClient, ApiError, EventsSocket, type ApiEnvironment, type SocketStatus } from './api.ts';
import type { ActionArgs, ActionName, ActionRequest, ActionResult, ActionTarget, HoldEvent, Layout, LayoutPending, NavAction, Session } from './contract.ts';
import { PROTOCOL } from './contract.ts';
import { HoldController, bindHoldLifecycle, type HoldEndReason } from './hold.ts';
import { t } from './i18n.ts';
import {
  canWriteLayout,
  capabilityFor,
  createStore,
  currentEpoch,
  initialState,
  layoutsEqual,
  type NoticeKind,
  type SessionEndReason,
  type Store,
  type Tab,
} from './state.ts';
import { uuidV4 } from './uuid.ts';
import { defaultDeviceName } from './device-name.ts';

export interface PageEnvironment {
  readonly hidden: boolean;
  addEventListener(type: 'visibilitychange', listener: () => void): void;
  removeEventListener(type: 'visibilitychange', listener: () => void): void;
}

export interface WindowEnvironment {
  addEventListener(type: 'blur', listener: () => void): void;
  removeEventListener(type: 'blur', listener: () => void): void;
  readonly location: { hash: string; pathname: string; search: string };
  readonly history: { replaceState(data: unknown, unused: string, url?: string): void };
  readonly navigator: { userAgent: string };
  readonly localStorage?: { getItem(key: string): string | null; setItem(key: string, value: string): void } | null;
}

export interface AppOptions {
  now?: () => number;
  holdRenewMs?: number;
  toastMs?: number;
}

const DEVICE_NAME_KEY = 'bdtv.device_name';
const PRUNE_AFTER_MS = 60_000;
const DEFAULT_HOLD_RENEW_MS = 200;

/** Actions whose target is the shell rather than whatever is in the foreground. */
const SHELL_TARGETED: ReadonlySet<ActionName> = new Set<ActionName>(['home', 'app.launch', 'app.close', 'shell.restart', 'power.sleep_timer', 'display.off', 'tv.power']);

export interface App {
  readonly store: Store;
  readonly api: ApiClient;
  start(): Promise<void>;
  selectTab(tab: Tab): void;
  showToast(kind: NoticeKind, text: string): void;
  dismissToast(): void;
  setDeviceName(name: string): void;
  pairWithCode(code: string): Promise<void>;
  retryInfo(): Promise<void>;
  tap<A extends ActionName>(action: A, args: ActionArgs[A]): Promise<void>;
  pressStart(action: NavAction): void;
  pressEnd(): void;
  refreshState(): Promise<void>;
  loadLayout(): Promise<void>;
  updateDraft(layout: Layout): void;
  previewLayout(): Promise<void>;
  endPreview(): Promise<void>;
  applyLayout(): Promise<void>;
  confirmPending(): Promise<void>;
  cancelPending(): Promise<void>;
  undoLayout(): Promise<void>;
  resetLayout(sectionId?: string): Promise<void>;
  loadDevices(): Promise<void>;
  revokeDevice(id: string): Promise<void>;
  revokeAllDevices(): Promise<void>;
  logout(): Promise<void>;
}

/**
 * @param env Fetch, socket, and timer environment.
 * @param page Document-like visibility source.
 * @param win Window-like location, history, storage, and blur source.
 * @param options Clock and cadence overrides for tests.
 * @returns The controller; call `start()` once the UI is mounted.
 */
export function createApp(env: ApiEnvironment, page: PageEnvironment, win: WindowEnvironment, options: AppOptions = {}): App {
  const now = options.now ?? (() => Date.now());
  const api = new ApiClient(env);
  const store = createStore(initialState(readStoredName(win)));
  let toastSeq = 0;
  let toastTimer: unknown = null;
  let pruneTimer: unknown = null;

  const socket = new EventsSocket(env, {
    onStatus: (status: SocketStatus) => {
      const connection = status === 'open' ? 'online' : status === 'connecting' ? 'connecting' : status === 'reconnecting' ? 'reconnecting' : 'offline';
      if (connection !== 'online') hold.cancel('disconnected');
      store.dispatch({ type: 'connection_changed', connection });
    },
    onOpen: () => {
      void refreshState();
    },
    onMessage: (message) => {
      switch (message.type) {
        case 'state':
          store.dispatch({ type: 'state_received', snapshot: message.state, at: now() });
          break;
        case 'action_result':
          handleResult(message.result);
          break;
        case 'hold':
          hold.serverEvent(message);
          break;
        case 'pong':
          break;
        default:
          break;
      }
    },
    onRevoked: () => endSession('revoked'),
  });

  const hold = new HoldController(
    { send: (message) => socket.send(message) },
    {
      // Read at each `begin`: the server's advertised cadence wins over the default.
      get renewMs() {
        return options.holdRenewMs ?? store.getState().snapshot?.remote.limits.hold_renew_ms ?? DEFAULT_HOLD_RENEW_MS;
      },
      newId: () => uuidV4(),
    },
    {
      onStart: (holdId, action) => store.dispatch({ type: 'hold_started', hold_id: holdId, action }),
      onEnd: (holdId, reason: HoldEndReason, event?: HoldEvent) => {
        if (event) {
          store.dispatch({ type: 'hold_event', event });
          if (event.state === 'busy') showToast('warning', event.reason || t.remote.holdBusy(null));
          return;
        }
        if (reason === 'released' || reason === 'replaced') store.dispatch({ type: 'hold_stopped' });
        else store.dispatch({ type: 'hold_cancelled', reason });
      },
      onUnavailable: () => {
        if (store.getState().connection !== 'online') showToast('info', t.remote.holdOffline);
      },
    },
  );

  bindHoldLifecycle(hold, page, win);
  store.dispatch({ type: 'visibility_changed', hidden: page.hidden });
  page.addEventListener('visibilitychange', () => {
    socket.setHidden(page.hidden);
    store.dispatch({ type: 'visibility_changed', hidden: page.hidden });
  });

  function showToast(kind: NoticeKind, text: string): void {
    toastSeq += 1;
    const id = toastSeq;
    store.dispatch({ type: 'toast_shown', toast: { id, kind, text } });
    if (toastTimer !== null) env.clearTimeout(toastTimer);
    toastTimer = env.setTimeout(() => {
      toastTimer = null;
      store.dispatch({ type: 'toast_dismissed', id });
    }, options.toastMs ?? (kind === 'error' ? 6000 : 4000));
  }

  function endSession(reason: SessionEndReason): void {
    socket.stop();
    hold.cancel('disconnected');
    api.setCsrf(null);
    if (pruneTimer !== null) {
      env.clearInterval(pruneTimer);
      pruneTimer = null;
    }
    store.dispatch({ type: 'session_ended', reason });
  }

  function beginSession(session: Session): void {
    store.dispatch({ type: 'session_started', session });
    storeName(win, session.device_name);
    if (pruneTimer === null) {
      pruneTimer = env.setInterval(() => store.dispatch({ type: 'pending_pruned', before: now() - PRUNE_AFTER_MS }), 10_000);
    }
    socket.start();
  }

  async function refreshState(): Promise<void> {
    try {
      const snapshot = await api.state();
      store.dispatch({ type: 'state_received', snapshot, at: now() });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) endSession('unauthenticated');
    }
  }

  function handleResult(result: ActionResult): void {
    store.dispatch({ type: 'action_result', result, at: now() });
    if (result.outcome !== 'failed') return;
    switch (result.code) {
      case 'stale_epoch':
        void refreshState();
        showToast('warning', result.message || t.remote.staleRefreshed);
        break;
      case 'unauthorized':
        endSession('unauthenticated');
        break;
      case 'display_off':
        // The press only woke the screen, like a TV remote's first press.
        showToast('info', result.message || t.errors.generic(result.code));
        break;
      default:
        showToast('error', result.message || t.errors.generic(result.code));
        break;
    }
  }

  async function tap<A extends ActionName>(action: A, args: ActionArgs[A]): Promise<void> {
    const state = store.getState();
    const target: ActionTarget = SHELL_TARGETED.has(action) ? 'shell' : 'active';
    const request: ActionRequest<A> = {
      protocol: PROTOCOL,
      request_id: uuidV4(),
      context_epoch: currentEpoch(state),
      target,
      action,
      args,
    };
    store.dispatch({ type: 'action_sent', request, at: now() });
    try {
      const result = await api.action(request);
      handleResult(result);
    } catch (err) {
      const message = describeError(err);
      store.dispatch({ type: 'action_send_failed', request_id: request.request_id, message, at: now() });
      if (err instanceof ApiError && err.status === 401) {
        endSession('unauthenticated');
        return;
      }
      if (err instanceof ApiError && err.status === 403 && err.error === 'csrf_rejected') {
        try {
          await api.session();
        } catch {
          // The session refresh failing means the cookie is gone; the next request ends the session.
        }
        showToast('error', t.errors.csrf);
        return;
      }
      showToast('error', err instanceof ApiError && err.status === 0 ? t.remote.sendFailed : message);
    }
  }

  function describeError(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.status === 0) return t.errors.network;
      if (err.status === 403) return err.message || t.errors.forbidden;
      return t.errors.generic(err.message);
    }
    return t.errors.generic(err instanceof Error ? err.message : '');
  }

  async function claim(invitation: string | null, code: string | null): Promise<void> {
    const state = store.getState();
    const device_name = state.pair.device_name.trim() || defaultDeviceName(win.navigator.userAgent);
    store.dispatch({ type: 'pair_started', redeeming: invitation !== null });
    try {
      const session = await api.claim({ invitation, code, device_name });
      beginSession(session);
    } catch (err) {
      const error = err instanceof ApiError ? { code: err.error, message: pairMessage(err) } : { code: 'internal', message: t.pair.errors.generic('') };
      store.dispatch({ type: 'pair_failed', error });
    }
  }

  function pairMessage(err: ApiError): string {
    if (err.status === 0) return t.pair.errors.network;
    switch (err.error) {
      case 'invalid_invitation':
        return t.pair.errors.invalid_invitation;
      case 'invitation_expired':
        return t.pair.errors.invitation_expired;
      case 'too_many_attempts':
        return t.pair.errors.too_many_attempts;
      default:
        if (err.status === 401) return t.pair.errors.invalid_invitation;
        if (err.status === 410) return t.pair.errors.invitation_expired;
        if (err.status === 429) return t.pair.errors.too_many_attempts;
        return t.pair.errors.generic(err.message);
    }
  }

  async function loadInfo(): Promise<boolean> {
    try {
      const info = await api.info();
      store.dispatch({ type: 'info_loaded', info });
      return true;
    } catch {
      store.dispatch({ type: 'info_failed', message: t.pair.infoFailed });
      return false;
    }
  }

  async function withEditor(work: () => Promise<void>, reloadAfter: boolean, success?: string): Promise<void> {
    store.dispatch({ type: 'editor_busy', busy: true });
    store.dispatch({ type: 'editor_notice', notice: null });
    try {
      await work();
      if (success) store.dispatch({ type: 'editor_notice', notice: { kind: 'success', text: success } });
      if (reloadAfter) await loadLayout();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        endSession('unauthenticated');
        return;
      }
      if (err instanceof ApiError && err.status === 409) {
        await loadLayout();
        store.dispatch({ type: 'editor_notice', notice: { kind: 'warning', text: t.editor.conflict } });
      } else if (err instanceof ApiError && err.status === 422) {
        const errors = Array.isArray(err.body?.errors) ? (err.body?.errors as unknown[]).map((e) => String(e)) : [err.message];
        store.dispatch({ type: 'editor_notice', notice: { kind: 'error', text: t.editor.invalid(errors) } });
      } else if (err instanceof ApiError && err.status === 403) {
        store.dispatch({ type: 'editor_notice', notice: { kind: 'error', text: err.message || t.editor.forbidden } });
      } else {
        store.dispatch({ type: 'editor_notice', notice: { kind: 'error', text: describeError(err) } });
      }
    } finally {
      store.dispatch({ type: 'editor_busy', busy: false });
    }
  }

  async function loadLayout(): Promise<void> {
    store.dispatch({ type: 'editor_loading' });
    try {
      const doc = await api.layout();
      store.dispatch({ type: 'editor_loaded', doc: { ...doc, pending: normalisePending(doc.pending) }, at: now() });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        endSession('unauthenticated');
        return;
      }
      store.dispatch({ type: 'editor_load_failed', message: describeError(err) });
    }
  }

  async function loadDevices(): Promise<void> {
    store.dispatch({ type: 'devices_loading' });
    try {
      const devices = await api.devices();
      store.dispatch({ type: 'devices_loaded', devices });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        endSession('unauthenticated');
        return;
      }
      store.dispatch({ type: 'devices_load_failed', message: describeError(err) });
    }
  }

  const app: App = {
    store,
    api,
    async start() {
      const ok = await loadInfo();
      if (!ok) return;
      const invitation = readInvitation(win);
      if (invitation !== null) {
        await claim(invitation, null);
        return;
      }
      try {
        const session = await api.session();
        beginSession(session);
      } catch (err) {
        if (err instanceof ApiError && err.status !== 401 && err.status !== 0) {
          store.dispatch({ type: 'pair_failed', error: { code: err.error, message: t.pair.errors.generic(err.message) } });
          return;
        }
        store.dispatch({ type: 'pair_required' });
      }
    },
    selectTab(tab) {
      store.dispatch({ type: 'tab_selected', tab });
      const state = store.getState();
      if (tab === 'editor' && state.editor.status === 'idle') void loadLayout();
      if (tab === 'devices' && state.devices.status === 'idle') void loadDevices();
    },
    showToast,
    dismissToast() {
      const toast = store.getState().toast;
      if (toast) store.dispatch({ type: 'toast_dismissed', id: toast.id });
    },
    setDeviceName(name) {
      store.dispatch({ type: 'pair_device_name', name });
      storeName(win, name);
    },
    async pairWithCode(code) {
      await claim(null, code);
    },
    async retryInfo() {
      if (await loadInfo()) store.dispatch({ type: 'pair_required' });
    },
    tap,
    pressStart(action) {
      const state = store.getState();
      const capability = capabilityFor(state.snapshot, action);
      if (!capability.available) return;
      void tap(action, {});
      if (capability.holdable) hold.begin(action, currentEpoch(state));
    },
    pressEnd() {
      hold.release();
    },
    refreshState,
    loadLayout,
    updateDraft(layout) {
      store.dispatch({ type: 'editor_draft_changed', layout });
    },
    previewLayout() {
      const draft = store.getState().editor.draft;
      if (!draft) return Promise.resolve();
      return withEditor(async () => {
        await api.previewLayout(draft);
        store.dispatch({ type: 'editor_preview', previewing: true });
      }, false);
    },
    endPreview() {
      return withEditor(async () => {
        await api.endPreview();
        store.dispatch({ type: 'editor_preview', previewing: false });
      }, false);
    },
    applyLayout() {
      const state = store.getState();
      const { draft, revision, saved } = state.editor;
      if (!draft || revision === null) return Promise.resolve();
      if (!canWriteLayout(state)) {
        store.dispatch({ type: 'editor_notice', notice: { kind: 'warning', text: t.editor.httpReadOnly } });
        return Promise.resolve();
      }
      if (layoutsEqual(draft, saved)) {
        store.dispatch({ type: 'editor_notice', notice: { kind: 'info', text: t.editor.noChanges } });
        return Promise.resolve();
      }
      return withEditor(async () => {
        const result = await api.putLayout(revision, draft);
        if (result.pending) {
          store.dispatch({
            type: 'editor_pending',
            pending: { revision: result.revision, previous_revision: revision, expires_in_s: -1, source: 'web' },
            at: now(),
          });
        }
        if (state.editor.previewing) store.dispatch({ type: 'editor_preview', previewing: false });
      }, true, t.editor.applied);
    },
    confirmPending() {
      const pending = store.getState().editor.pending;
      if (!pending) return Promise.resolve();
      return withEditor(async () => {
        await api.confirmLayout(pending.revision);
        store.dispatch({ type: 'editor_pending', pending: null, at: now() });
      }, true, t.editor.applied);
    },
    cancelPending() {
      const pending = store.getState().editor.pending;
      if (!pending) return Promise.resolve();
      return withEditor(async () => {
        await api.cancelLayout(pending.revision);
        store.dispatch({ type: 'editor_pending', pending: null, at: now() });
      }, true, t.editor.undone);
    },
    undoLayout() {
      return withEditor(() => api.undoLayout(), true, t.editor.undone);
    },
    resetLayout(sectionId) {
      return withEditor(() => api.resetLayout(sectionId), true, t.editor.reset);
    },
    loadDevices,
    async revokeDevice(id) {
      store.dispatch({ type: 'devices_busy', busy: true });
      const name = store.getState().devices.devices.find((d) => d.id === id)?.name ?? id;
      try {
        await api.revokeDevice(id);
        showToast('success', t.devices.revoked(name));
        if (store.getState().session?.device_id === id) {
          endSession('logout');
          return;
        }
        await loadDevices();
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          endSession('unauthenticated');
          return;
        }
        showToast('error', t.devices.failed(describeError(err)));
      } finally {
        if (store.getState().screen === 'app') store.dispatch({ type: 'devices_busy', busy: false });
      }
    },
    async revokeAllDevices() {
      const state = store.getState();
      const self = state.session?.device_id ?? null;
      const others = state.devices.devices.filter((d) => d.id !== self);
      store.dispatch({ type: 'devices_busy', busy: true });
      try {
        for (const device of others) await api.revokeDevice(device.id);
        if (self) await api.revokeDevice(self);
        endSession('logout');
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          endSession('unauthenticated');
          return;
        }
        showToast('error', t.devices.failed(describeError(err)));
        store.dispatch({ type: 'devices_busy', busy: false });
        await loadDevices();
      }
    },
    async logout() {
      try {
        await api.logout();
      } catch {
        // A failed logout still drops the local session; the cookie expires server-side.
      }
      endSession('logout');
    },
  };

  return app;
}

function normalisePending(value: unknown): LayoutPending | null {
  if (typeof value !== 'object' || value === null) return null;
  const p = value as Record<string, unknown>;
  if (typeof p.revision !== 'number') return null;
  return {
    revision: p.revision,
    previous_revision: typeof p.previous_revision === 'number' ? p.previous_revision : p.revision - 1,
    expires_in_s: typeof p.expires_in_s === 'number' ? p.expires_in_s : -1,
    source: p.source === 'tv' ? 'tv' : 'web',
  };
}

/**
 * Reads and strips the invitation fragment so the token never stays in the address bar:
 * `#pair=<token>` as the TV's QR link carries it (contracts/http.md), or the older `#invite=<token>`.
 * @param win Window-like location and history.
 * @returns The token, or null when the fragment carries none.
 */
export function readInvitation(win: WindowEnvironment): string | null {
  const match = /^#(?:pair|invite)=([^&]+)/.exec(win.location.hash);
  if (!match || !match[1]) return null;
  const token = decodeURIComponent(match[1]);
  win.history.replaceState(null, '', win.location.pathname + win.location.search);
  return token;
}

function readStoredName(win: WindowEnvironment): string {
  try {
    return win.localStorage?.getItem(DEVICE_NAME_KEY) ?? '';
  } catch {
    // Storage access throws in some private modes; the name simply is not remembered.
    return '';
  }
}

function storeName(win: WindowEnvironment, name: string): void {
  try {
    win.localStorage?.setItem(DEVICE_NAME_KEY, name);
  } catch {
    // Storage access throws in some private modes; the name simply is not remembered.
  }
}
