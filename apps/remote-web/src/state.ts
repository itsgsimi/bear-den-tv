// Application store: a pure reducer over immutable state plus a tiny subscription store.
// Contract: `reduce(state, event)` is a pure function of its arguments and never
// touches the DOM, timers, or network; every side effect lives in app.ts. The
// store holds the last server snapshot verbatim (the server is the only authority
// on target, epoch, and capabilities), the connection status, pending action
// results keyed by request_id, the local hold status, the last failure, and the
// per-screen editing state. Selectors at the bottom derive view facts from it,
// including what a guest pass may use (`isGuest`, `mayUse`, `guestEndsAt`).

import type {
  ActionName,
  ActionRequest,
  ActionResult,
  Application,
  Capability,
  Device,
  HoldEvent,
  Info,
  Layout,
  LayoutDocument,
  LayoutPending,
  NavAction,
  Outcome,
  Permission,
  ResultCode,
  Session,
  StateSnapshot,
} from './contract.ts';
import { GUEST_ACTIONS } from './contract.ts';

export type Connection = 'idle' | 'connecting' | 'online' | 'reconnecting' | 'offline';
export type Screen = 'loading' | 'pair' | 'app';
export type Tab = 'remote' | 'editor' | 'devices' | 'about';
/** `pass_ended`: a guest pass was revoked (it ran out, or the owner removed it). */
export type SessionEndReason = 'revoked' | 'logout' | 'unauthenticated' | 'pass_ended';
export type NoticeKind = 'info' | 'success' | 'warning' | 'error';

export interface PendingAction {
  request_id: string;
  action: ActionName;
  sent_at: number;
  updated_at: number;
  /** `null` while no result has arrived. */
  outcome: Outcome | null;
  code: ResultCode | 'transport' | null;
  message: string;
}

export type HoldPhase = 'idle' | 'active' | 'busy' | 'expired' | 'cancelled';

export interface HoldStatus {
  phase: HoldPhase;
  hold_id: string | null;
  action: NavAction | null;
  reason: string | null;
}

export interface Toast {
  id: number;
  kind: NoticeKind;
  text: string;
}

export interface Notice {
  kind: NoticeKind;
  text: string;
}

export interface PairError {
  code: string;
  message: string;
}

export interface PairState {
  busy: boolean;
  /** True while a QR invitation is being redeemed without user input. */
  redeeming: boolean;
  error: PairError | null;
  notice: string | null;
  device_name: string;
  infoError: string | null;
}

export interface EditorState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  revision: number | null;
  draft: Layout | null;
  saved: Layout | null;
  defaults: Layout | null;
  pending: LayoutPending | null;
  /** Timestamp when `pending` was received, for the local countdown. */
  pendingAt: number;
  previewing: boolean;
  busy: boolean;
  notice: Notice | null;
  error: string | null;
}

export interface DevicesState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  devices: Device[];
  busy: boolean;
  error: string | null;
}

export interface AppState {
  screen: Screen;
  tab: Tab;
  info: Info | null;
  session: Session | null;
  connection: Connection;
  snapshot: StateSnapshot | null;
  /** When the last snapshot arrived, on the controller clock (`now()` in app.ts); 0 before any. */
  snapshotAt: number;
  /** The page is hidden (another tab, screen off): nothing needs to tick. */
  hidden: boolean;
  pending: Record<string, PendingAction>;
  hold: HoldStatus;
  lastError: PairError | null;
  toast: Toast | null;
  pair: PairState;
  editor: EditorState;
  devices: DevicesState;
}

export type Event =
  | { type: 'info_loaded'; info: Info }
  | { type: 'info_failed'; message: string }
  | { type: 'session_started'; session: Session }
  | { type: 'session_ended'; reason: SessionEndReason }
  | { type: 'pair_required' }
  | { type: 'connection_changed'; connection: Connection }
  | { type: 'state_received'; snapshot: StateSnapshot; at: number }
  | { type: 'visibility_changed'; hidden: boolean }
  | { type: 'action_sent'; request: ActionRequest; at: number }
  | { type: 'action_result'; result: ActionResult; at: number }
  | { type: 'action_send_failed'; request_id: string; message: string; at: number }
  | { type: 'pending_pruned'; before: number }
  | { type: 'hold_started'; hold_id: string; action: NavAction }
  | { type: 'hold_stopped' }
  | { type: 'hold_cancelled'; reason: string }
  | { type: 'hold_event'; event: HoldEvent }
  | { type: 'toast_shown'; toast: Toast }
  | { type: 'toast_dismissed'; id: number }
  | { type: 'tab_selected'; tab: Tab }
  | { type: 'pair_device_name'; name: string }
  | { type: 'pair_started'; redeeming: boolean }
  | { type: 'pair_failed'; error: PairError }
  | { type: 'editor_loading' }
  | { type: 'editor_loaded'; doc: LayoutDocument; at: number }
  | { type: 'editor_load_failed'; message: string }
  | { type: 'editor_draft_changed'; layout: Layout }
  | { type: 'editor_busy'; busy: boolean }
  | { type: 'editor_preview'; previewing: boolean }
  | { type: 'editor_notice'; notice: Notice | null }
  | { type: 'editor_pending'; pending: LayoutPending | null; at: number }
  | { type: 'devices_loading' }
  | { type: 'devices_loaded'; devices: Device[] }
  | { type: 'devices_load_failed'; message: string }
  | { type: 'devices_busy'; busy: boolean };

const IDLE_HOLD: HoldStatus = { phase: 'idle', hold_id: null, action: null, reason: null };

const IDLE_EDITOR: EditorState = {
  status: 'idle',
  revision: null,
  draft: null,
  saved: null,
  defaults: null,
  pending: null,
  pendingAt: 0,
  previewing: false,
  busy: false,
  notice: null,
  error: null,
};

const IDLE_DEVICES: DevicesState = { status: 'idle', devices: [], busy: false, error: null };

/**
 * @param deviceName Remembered device name for the pair form.
 * @returns The state before the TV has answered `/api/v1/info`.
 */
export function initialState(deviceName = ''): AppState {
  return {
    screen: 'loading',
    tab: 'remote',
    info: null,
    session: null,
    connection: 'idle',
    snapshot: null,
    snapshotAt: 0,
    hidden: false,
    pending: {},
    hold: IDLE_HOLD,
    lastError: null,
    toast: null,
    pair: { busy: false, redeeming: false, error: null, notice: null, device_name: deviceName, infoError: null },
    editor: IDLE_EDITOR,
    devices: IDLE_DEVICES,
  };
}

/**
 * @param state Current state.
 * @param event Event to apply.
 * @returns The next state; the same object when the event changes nothing.
 */
export function reduce(state: AppState, event: Event): AppState {
  switch (event.type) {
    case 'info_loaded':
      return { ...state, info: event.info, pair: { ...state.pair, infoError: null } };
    case 'info_failed':
      return { ...state, screen: 'pair', pair: { ...state.pair, infoError: event.message, busy: false, redeeming: false } };
    case 'session_started':
      return {
        ...state,
        screen: 'app',
        tab: 'remote',
        session: event.session,
        pair: { ...state.pair, busy: false, redeeming: false, error: null, notice: null, device_name: event.session.device_name },
      };
    case 'session_ended': {
      // A guest losing its session means the pass is over; say so plainly.
      const reason = isGuest(state) && (event.reason === 'revoked' || event.reason === 'unauthenticated') ? 'pass_ended' : event.reason;
      return {
        ...initialState(state.pair.device_name),
        screen: 'pair',
        info: state.info,
        pair: { ...initialState(state.pair.device_name).pair, notice: reason },
      };
    }
    case 'pair_required':
      return { ...state, screen: 'pair', pair: { ...state.pair, busy: false, redeeming: false } };
    case 'connection_changed': {
      const next: AppState = { ...state, connection: event.connection };
      if (state.hold.phase === 'active' && event.connection !== 'online') {
        next.hold = { ...state.hold, phase: 'cancelled', reason: 'disconnected' };
      }
      return next;
    }
    case 'state_received': {
      const snapshot = event.snapshot;
      const permissions = snapshot.me?.permissions ?? state.session?.permissions ?? [];
      const session = state.session && snapshot.me ? { ...state.session, permissions, device_name: snapshot.me.device_name } : state.session;
      let tab = state.tab;
      if (tab === 'editor' && !permissions.includes('layout_editor')) tab = 'remote';
      if (tab === 'devices' && !permissions.includes('owner')) tab = 'remote';
      const editor =
        state.editor.status === 'ready' && snapshot.layout_pending !== undefined
          ? { ...state.editor, pending: snapshot.layout_pending, pendingAt: event.at }
          : state.editor;
      return { ...state, snapshot, snapshotAt: event.at, session, tab, editor };
    }
    case 'action_sent':
      return {
        ...state,
        pending: {
          ...state.pending,
          [event.request.request_id]: {
            request_id: event.request.request_id,
            action: event.request.action,
            sent_at: event.at,
            updated_at: event.at,
            outcome: null,
            code: null,
            message: '',
          },
        },
      };
    case 'action_result': {
      const result = event.result;
      const previous = state.pending[result.request_id];
      const entry: PendingAction = {
        request_id: result.request_id,
        action: previous?.action ?? 'select',
        sent_at: previous?.sent_at ?? event.at,
        updated_at: event.at,
        outcome: result.outcome,
        code: result.code,
        message: result.message,
      };
      if (!previous) return state;
      const next: AppState = { ...state, pending: { ...state.pending, [result.request_id]: entry } };
      if (result.outcome === 'failed') {
        next.lastError = { code: result.code, message: result.message };
      }
      return next;
    }
    case 'action_send_failed': {
      const previous = state.pending[event.request_id];
      if (!previous) return state;
      return {
        ...state,
        pending: {
          ...state.pending,
          [event.request_id]: { ...previous, updated_at: event.at, outcome: 'failed', code: 'transport', message: event.message },
        },
        lastError: { code: 'transport', message: event.message },
      };
    }
    case 'pending_pruned': {
      const pending: Record<string, PendingAction> = {};
      let changed = false;
      for (const entry of Object.values(state.pending)) {
        if (entry.updated_at < event.before) changed = true;
        else pending[entry.request_id] = entry;
      }
      return changed ? { ...state, pending } : state;
    }
    case 'hold_started':
      return { ...state, hold: { phase: 'active', hold_id: event.hold_id, action: event.action, reason: null } };
    case 'hold_stopped':
      return state.hold.phase === 'idle' ? state : { ...state, hold: IDLE_HOLD };
    case 'hold_cancelled':
      return state.hold.phase === 'active' ? { ...state, hold: { ...state.hold, phase: 'cancelled', reason: event.reason } } : state;
    case 'hold_event': {
      const ev = event.event;
      if (state.hold.hold_id !== ev.hold_id) return state;
      if (ev.state === 'active') return state.hold.phase === 'active' ? state : { ...state, hold: { ...state.hold, phase: 'active', reason: null } };
      return { ...state, hold: { ...state.hold, phase: ev.state, reason: ev.reason ?? null } };
    }
    case 'toast_shown':
      return { ...state, toast: event.toast };
    case 'toast_dismissed':
      return state.toast && state.toast.id === event.id ? { ...state, toast: null } : state;
    case 'visibility_changed':
      return state.hidden === event.hidden ? state : { ...state, hidden: event.hidden };
    case 'tab_selected':
      return state.tab === event.tab ? state : { ...state, tab: event.tab };
    case 'pair_device_name':
      return { ...state, pair: { ...state.pair, device_name: event.name } };
    case 'pair_started':
      return { ...state, screen: 'pair', pair: { ...state.pair, busy: true, redeeming: event.redeeming, error: null } };
    case 'pair_failed':
      return { ...state, screen: 'pair', pair: { ...state.pair, busy: false, redeeming: false, error: event.error } };
    case 'editor_loading':
      return { ...state, editor: { ...state.editor, status: 'loading', error: null } };
    case 'editor_loaded':
      return {
        ...state,
        editor: {
          ...state.editor,
          status: 'ready',
          revision: event.doc.revision,
          draft: cloneLayout(event.doc.layout),
          saved: event.doc.layout,
          defaults: event.doc.defaults,
          pending: event.doc.pending,
          pendingAt: event.at,
          busy: false,
          error: null,
        },
      };
    case 'editor_load_failed':
      return { ...state, editor: { ...state.editor, status: 'error', busy: false, error: event.message } };
    case 'editor_draft_changed':
      return { ...state, editor: { ...state.editor, draft: event.layout } };
    case 'editor_busy':
      return { ...state, editor: { ...state.editor, busy: event.busy } };
    case 'editor_preview':
      return { ...state, editor: { ...state.editor, previewing: event.previewing } };
    case 'editor_notice':
      return { ...state, editor: { ...state.editor, notice: event.notice } };
    case 'editor_pending':
      return { ...state, editor: { ...state.editor, pending: event.pending, pendingAt: event.at } };
    case 'devices_loading':
      return { ...state, devices: { ...state.devices, status: 'loading', error: null } };
    case 'devices_loaded':
      return { ...state, devices: { status: 'ready', devices: event.devices, busy: false, error: null } };
    case 'devices_load_failed':
      return { ...state, devices: { ...state.devices, status: 'error', busy: false, error: event.message } };
    case 'devices_busy':
      return { ...state, devices: { ...state.devices, busy: event.busy } };
    default:
      return assertNever(event);
  }
}

function assertNever(value: never): never {
  throw new Error(`Unhandled event ${JSON.stringify(value)}`);
}

/**
 * @param layout Layout to copy.
 * @returns A structurally independent copy safe to edit as a draft.
 */
export function cloneLayout(layout: Layout): Layout {
  return {
    ui: { ...layout.ui },
    sections: layout.sections.map((s) => ({ ...s, ...(s.application_ids ? { application_ids: [...s.application_ids] } : {}) })),
  };
}

/**
 * @param a First layout.
 * @param b Second layout.
 * @returns True when both serialise identically.
 */
export function layoutsEqual(a: Layout | null, b: Layout | null): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

export interface Store {
  getState(): AppState;
  dispatch(event: Event): void;
  subscribe(listener: (state: AppState) => void): () => void;
}

/**
 * @param initial Starting state.
 * @returns A store that notifies subscribers after every state-changing dispatch.
 */
export function createStore(initial: AppState = initialState()): Store {
  let state = initial;
  const listeners = new Set<(state: AppState) => void>();
  return {
    getState: () => state,
    dispatch(event) {
      const next = reduce(state, event);
      if (next === state) return;
      state = next;
      for (const listener of Array.from(listeners)) listener(state);
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

// Selectors.

/**
 * @param state Application state.
 * @returns The viewer's permissions from the latest snapshot, else the session.
 */
export function permissionsOf(state: AppState): Permission[] {
  return state.snapshot?.me?.permissions ?? state.session?.permissions ?? [];
}

/**
 * @param state Application state.
 * @returns True when this phone holds a guest pass.
 */
export function isGuest(state: AppState): boolean {
  return permissionsOf(state).includes('guest');
}

/**
 * @param state Application state.
 * @param action Action name.
 * @returns False for actions a guest pass may not send (the server refuses
 *   them anyway); true for everyone else, whose controls stay capability-gated.
 */
export function mayUse(state: AppState, action: ActionName): boolean {
  return !isGuest(state) || GUEST_ACTIONS.has(action);
}

/**
 * @param state Application state.
 * @returns When this phone's guest pass ends (Unix epoch ms), or null.
 */
export function guestEndsAt(state: AppState): number | null {
  return isGuest(state) ? (state.snapshot?.me?.expires_at_ms ?? null) : null;
}

/**
 * @param state Application state.
 * @returns Tabs the viewer may open, in display order.
 */
export function visibleTabs(state: AppState): Tab[] {
  const permissions = permissionsOf(state);
  const tabs: Tab[] = ['remote'];
  if (permissions.includes('layout_editor')) tabs.push('editor');
  if (permissions.includes('owner')) tabs.push('devices');
  tabs.push('about');
  return tabs;
}

/**
 * @param state Application state.
 * @returns True when the transport is HTTPS according to the freshest source.
 */
export function isSecureTransport(state: AppState): boolean {
  if (state.snapshot) return state.snapshot.remote.https || state.snapshot.remote.transport === 'https';
  if (state.session) return state.session.transport_secure;
  return state.info?.https ?? false;
}

/**
 * @param state Application state.
 * @returns True when layout writes are accepted for this viewer and transport.
 */
export function canWriteLayout(state: AppState): boolean {
  if (!permissionsOf(state).includes('layout_editor')) return false;
  if (isSecureTransport(state)) return true;
  return state.snapshot?.remote.http_layout_editing ?? false;
}

/**
 * @param snapshot Latest server snapshot or null.
 * @param action Action name.
 * @returns The capability record, or an unavailable placeholder when the server did not list it.
 */
export function capabilityFor(snapshot: StateSnapshot | null, action: ActionName): Capability {
  const found = snapshot?.capabilities[action];
  if (found) return found;
  return { available: false };
}

/**
 * @param snapshot Last rendered snapshot.
 * @returns The apps that get a tile: every application except the optional
 *   ones the coordinator marks `hidden` (not installed).
 */
export function visibleApps(snapshot: StateSnapshot | null): Application[] {
  return (snapshot?.applications ?? []).filter((a) => a.hidden !== true);
}

/**
 * @param snapshot Last rendered snapshot.
 * @returns The app the Close button acts on: the one in front, else one left
 *   running behind Bear Den Home; null when nothing is running.
 */
export function closableApp(snapshot: StateSnapshot | null): Application | null {
  const apps = snapshot?.applications ?? [];
  return apps.find((a) => a.foreground) ?? apps.find((a) => a.running) ?? null;
}

/**
 * @param state Application state.
 * @param action Action name.
 * @returns The most recently updated pending entry for the action, or null.
 */
export function latestResultFor(state: AppState, action: ActionName): PendingAction | null {
  let best: PendingAction | null = null;
  for (const entry of Object.values(state.pending)) {
    if (entry.action !== action) continue;
    if (!best || entry.updated_at > best.updated_at || (entry.updated_at === best.updated_at && entry.sent_at > best.sent_at)) best = entry;
  }
  return best;
}

/**
 * @param state Application state.
 * @returns The context epoch of the last snapshot, or 0 before any snapshot.
 */
export function currentEpoch(state: AppState): number {
  return state.snapshot?.context_epoch ?? 0;
}
