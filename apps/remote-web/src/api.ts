// Typed client for the coordinator's HTTP + WebSocket API (contracts/http.md).
// Contract: every method resolves with the parsed JSON body on 2xx and rejects with
// `ApiError {status, error, message}` otherwise; network failures reject with
// status 0 and error `network`. State-changing requests carry `X-BDTV-CSRF` from
// the current session and `Content-Type: application/json`. `EventsSocket` keeps
// `/api/v1/events` open with exponential backoff, sends `visibility` on tab
// visibility changes and `ping` on an interval, and on every (re)open fetches
// `/api/v1/state` through `onOpen`; it never queues or replays messages: `send`
// returns false when the socket is not open. All browser globals are injected
// through `ApiEnvironment` so the client is testable without a DOM.

import type {
  ActionRequest,
  ActionResult,
  CapabilitiesResponse,
  ClaimRequest,
  ClientMessage,
  Device,
  ErrorBody,
  Info,
  Layout,
  LayoutDocument,
  LayoutPutResponse,
  ServerMessage,
  Session,
  StateSnapshot,
} from './contract.ts';

export class ApiError extends Error {
  /**
   * @param status HTTP status, or 0 for a network failure.
   * @param error Machine-readable code from the error body.
   * @param message Human-readable text from the error body.
   * @param body Parsed error body, when the server sent one.
   */
  constructor(
    readonly status: number,
    readonly error: string,
    message: string,
    readonly body: (ErrorBody & Record<string, unknown>) | null = null,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

export interface SocketLike {
  readonly readyState: number;
  send(data: string): void;
  close(code?: number, reason?: string): void;
  onopen: ((ev: unknown) => void) | null;
  onclose: ((ev: { code: number; reason: string }) => void) | null;
  onerror: ((ev: unknown) => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
}

export interface ApiEnvironment {
  fetch: FetchLike;
  createSocket(url: string): SocketLike;
  setTimeout(handler: () => void, ms: number): unknown;
  clearTimeout(id: unknown): void;
  setInterval(handler: () => void, ms: number): unknown;
  clearInterval(id: unknown): void;
  random(): number;
  /** Absolute origin for the WebSocket URL, such as `http://192.0.2.10:8090`. */
  origin: string;
}

export const OPEN_STATE = 1;

export class ApiClient {
  private csrf: string | null = null;

  /**
   * @param env Injected fetch and socket factories.
   */
  constructor(private readonly env: ApiEnvironment) {}

  /** @returns The current CSRF token, or null before a session exists. */
  get csrfToken(): string | null {
    return this.csrf;
  }

  /** @param token Token from `/api/v1/pair/claim` or `/api/v1/session`. */
  setCsrf(token: string | null): void {
    this.csrf = token;
  }

  /** @returns Public TV information; never private state. */
  info(): Promise<Info> {
    return this.request<Info>('GET', '/api/v1/info', undefined, false);
  }

  /**
   * @param body Invitation token or code plus the device name.
   * @returns The new session; the CSRF token is remembered.
   */
  async claim(body: ClaimRequest): Promise<Session> {
    const result = await this.request<Omit<Session, 'transport_secure'> & Partial<Session>>('POST', '/api/v1/pair/claim', body, false);
    this.csrf = result.csrf_token;
    return { ...result, transport_secure: result.transport_secure ?? false };
  }

  /** @returns The current session; 401 rejects. The CSRF token is remembered. */
  async session(): Promise<Session> {
    const result = await this.request<Session>('GET', '/api/v1/session');
    this.csrf = result.csrf_token;
    return result;
  }

  /** Ends the session. */
  async logout(): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/logout', {});
    this.csrf = null;
  }

  /** @returns The redacted state snapshot for this viewer. */
  state(): Promise<StateSnapshot> {
    return this.request<StateSnapshot>('GET', '/api/v1/state');
  }

  /** @returns Capabilities for the current target. */
  capabilities(): Promise<CapabilitiesResponse> {
    return this.request<CapabilitiesResponse>('GET', '/api/v1/capabilities');
  }

  /**
   * @param request Validated action request.
   * @returns The first terminal or accepted result.
   */
  action(request: ActionRequest): Promise<ActionResult> {
    return this.request<ActionResult>('POST', '/api/v1/actions', request);
  }

  /** @returns Layout document with revision, defaults, and pending change. */
  layout(): Promise<LayoutDocument> {
    return this.request<LayoutDocument>('GET', '/api/v1/layout');
  }

  /**
   * @param baseRevision Revision the draft was based on.
   * @param layout Draft to persist.
   * @returns The new revision and whether it awaits a timed confirmation.
   */
  putLayout(baseRevision: number, layout: Layout): Promise<LayoutPutResponse> {
    return this.request<LayoutPutResponse>('PUT', '/api/v1/layout', { base_revision: baseRevision, layout });
  }

  /** @param layout Draft to show on the TV for 60 s. */
  async previewLayout(layout: Layout): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/preview', { layout });
  }

  /** Ends a preview. */
  async endPreview(): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/preview/end', {});
  }

  /** @param revision Pending revision to keep. */
  async confirmLayout(revision: number): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/confirm', { revision });
  }

  /** @param revision Pending revision to revert now. */
  async cancelLayout(revision: number): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/cancel', { revision });
  }

  /** Restores the previous revision. */
  async undoLayout(): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/undo', {});
  }

  /** @param sectionId Section to reset, or undefined for everything. */
  async resetLayout(sectionId?: string): Promise<void> {
    await this.request<unknown>('POST', '/api/v1/layout/reset', sectionId ? { section_id: sectionId } : {});
  }

  /** @returns Paired devices (owner only). */
  devices(): Promise<Device[]> {
    return this.request<Device[] | { devices: Device[] }>('GET', '/api/v1/devices').then((body) =>
      Array.isArray(body) ? body : body.devices,
    );
  }

  /** @param id Device to revoke. */
  async revokeDevice(id: string): Promise<void> {
    await this.request<unknown>('DELETE', `/api/v1/devices/${encodeURIComponent(id)}`);
  }

  private async request<T>(method: string, path: string, body?: unknown, withCsrf = method !== 'GET'): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (withCsrf && this.csrf) headers['X-BDTV-CSRF'] = this.csrf;
    const init: RequestInit = { method, headers, credentials: 'same-origin', cache: 'no-store' };
    if (body !== undefined) init.body = JSON.stringify(body);
    let response: Response;
    try {
      response = await this.env.fetch(path, init);
    } catch (err) {
      throw new ApiError(0, 'network', err instanceof Error ? err.message : String(err));
    }
    const text = await response.text();
    let parsed: unknown = null;
    if (text.length > 0) {
      try {
        parsed = JSON.parse(text);
      } catch {
        // Non-JSON bodies (proxies, HTML error pages) are reported by status only.
        parsed = null;
      }
    }
    if (response.ok) return parsed as T;
    const errBody = isErrorBody(parsed) ? parsed : null;
    throw new ApiError(response.status, errBody?.error ?? `http_${response.status}`, errBody?.message ?? `HTTP ${response.status}`, errBody);
  }
}

function isErrorBody(value: unknown): value is ErrorBody & Record<string, unknown> {
  return typeof value === 'object' && value !== null && typeof (value as ErrorBody).error === 'string';
}

export type SocketStatus = 'connecting' | 'open' | 'reconnecting' | 'closed';

export interface EventsSocketHandlers {
  onStatus(status: SocketStatus): void;
  onMessage(message: ServerMessage): void;
  /** Called after each successful open with the freshly fetched state (never replays). */
  onOpen(): void;
  /** Called once when the server sends `revoked` or closes with 4001. */
  onRevoked(): void;
}

export interface EventsSocketOptions {
  pingIntervalMs: number;
  pongTimeoutMs: number;
  backoffMinMs: number;
  backoffMaxMs: number;
}

export const DEFAULT_SOCKET_OPTIONS: EventsSocketOptions = {
  pingIntervalMs: 15_000,
  pongTimeoutMs: 8_000,
  backoffMinMs: 500,
  backoffMaxMs: 10_000,
};

export class EventsSocket {
  private socket: SocketLike | null = null;
  private attempts = 0;
  private stopped = true;
  private reconnectTimer: unknown = null;
  private pingTimer: unknown = null;
  private pongTimer: unknown = null;
  private hidden = false;

  /**
   * @param env Injected socket factory and timers.
   * @param handlers Status, message, open, and revocation callbacks.
   * @param options Ping cadence and backoff bounds.
   */
  constructor(
    private readonly env: ApiEnvironment,
    private readonly handlers: EventsSocketHandlers,
    private readonly options: EventsSocketOptions = DEFAULT_SOCKET_OPTIONS,
  ) {}

  /** @returns True while the socket is open. */
  get isOpen(): boolean {
    return this.socket !== null && this.socket.readyState === OPEN_STATE;
  }

  /** Opens the socket and keeps it open until `stop`. */
  start(): void {
    if (!this.stopped) return;
    this.stopped = false;
    this.attempts = 0;
    this.connect();
  }

  /** Closes the socket and cancels reconnects. */
  stop(): void {
    this.stopped = true;
    this.clearTimers();
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      socket.onclose = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onopen = null;
      socket.close(1000, 'client stop');
    }
    this.handlers.onStatus('closed');
  }

  /**
   * @param message Message to send now.
   * @returns False when the socket is not open; the message is dropped, never queued.
   */
  send(message: ClientMessage): boolean {
    if (!this.isOpen || !this.socket) return false;
    try {
      this.socket.send(JSON.stringify(message));
      return true;
    } catch {
      // A socket that throws on send is closing; onclose schedules the reconnect.
      return false;
    }
  }

  /** @param hidden Current document visibility; forwarded as a `visibility` message. */
  setHidden(hidden: boolean): void {
    this.hidden = hidden;
    this.send({ type: 'visibility', hidden });
  }

  private connect(): void {
    if (this.stopped) return;
    this.handlers.onStatus(this.attempts === 0 ? 'connecting' : 'reconnecting');
    const url = this.env.origin.replace(/^http/, 'ws') + '/api/v1/events';
    let socket: SocketLike;
    try {
      socket = this.env.createSocket(url);
    } catch {
      // The constructor throws on a malformed URL or a blocked origin; retry like a close.
      this.scheduleReconnect();
      return;
    }
    this.socket = socket;
    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.attempts = 0;
      this.handlers.onStatus('open');
      this.startPing();
      if (this.hidden) this.send({ type: 'visibility', hidden: true });
      this.handlers.onOpen();
    };
    socket.onmessage = (ev) => {
      if (this.socket !== socket) return;
      this.armPongTimeout(false);
      const parsed = parseServerMessage(ev.data);
      if (!parsed) return;
      if (parsed.type === 'revoked') {
        this.stop();
        this.handlers.onRevoked();
        return;
      }
      this.handlers.onMessage(parsed);
    };
    socket.onerror = () => {
      // The close event that follows carries the retry; errors alone are not actionable.
    };
    socket.onclose = (ev) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.clearTimers();
      if (ev.code === 4001) {
        this.stopped = true;
        this.handlers.onStatus('closed');
        this.handlers.onRevoked();
        return;
      }
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect(): void {
    if (this.stopped) return;
    this.handlers.onStatus('reconnecting');
    const base = Math.min(this.options.backoffMaxMs, this.options.backoffMinMs * 2 ** this.attempts);
    const jitter = base * 0.2 * (this.env.random() * 2 - 1);
    const delay = Math.max(0, Math.round(base + jitter));
    this.attempts += 1;
    this.reconnectTimer = this.env.setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private startPing(): void {
    this.pingTimer = this.env.setInterval(() => {
      if (this.send({ type: 'ping' })) this.armPongTimeout(true);
    }, this.options.pingIntervalMs);
  }

  private armPongTimeout(arm: boolean): void {
    if (this.pongTimer !== null) {
      this.env.clearTimeout(this.pongTimer);
      this.pongTimer = null;
    }
    if (!arm) return;
    this.pongTimer = this.env.setTimeout(() => {
      this.pongTimer = null;
      const socket = this.socket;
      if (socket) socket.close(4000, 'pong timeout');
    }, this.options.pongTimeoutMs);
  }

  private clearTimers(): void {
    if (this.reconnectTimer !== null) this.env.clearTimeout(this.reconnectTimer);
    if (this.pingTimer !== null) this.env.clearInterval(this.pingTimer);
    if (this.pongTimer !== null) this.env.clearTimeout(this.pongTimer);
    this.reconnectTimer = null;
    this.pingTimer = null;
    this.pongTimer = null;
  }
}

const SERVER_TYPES = new Set(['state', 'action_result', 'hold', 'revoked', 'pong']);

/**
 * @param data Raw socket payload.
 * @returns The typed message, or null for anything that is not a known server message.
 */
export function parseServerMessage(data: unknown): ServerMessage | null {
  if (typeof data !== 'string') return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(data);
  } catch {
    // Malformed frames are dropped; the server never sends non-JSON text.
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) return null;
  const type = (parsed as { type?: unknown }).type;
  if (typeof type !== 'string' || !SERVER_TYPES.has(type)) return null;
  return parsed as ServerMessage;
}

/**
 * @returns An environment backed by the browser globals of the current page.
 */
export function browserEnvironment(): ApiEnvironment {
  return {
    fetch: (input, init) => fetch(input, init),
    createSocket: (url) => new WebSocket(url) as unknown as SocketLike,
    setTimeout: (handler, ms) => window.setTimeout(handler, ms),
    clearTimeout: (id) => window.clearTimeout(id as number),
    setInterval: (handler, ms) => window.setInterval(handler, ms),
    clearInterval: (id) => window.clearInterval(id as number),
    random: () => Math.random(),
    origin: window.location.origin,
  };
}
