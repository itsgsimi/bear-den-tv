// Client side of server-bounded directional holds (contracts/actions.md, "Holds").
// Contract: `HoldController.begin` sends one `hold.start` with a fresh UUID and the
// caller's context epoch, then `hold.renew` every `renewMs` until `release` or
// `cancel`, which send `hold.stop` (no message when the socket is already gone).
// The controller never generates repeats itself; the server repeats. A server
// `hold` event that ends the lease stops renewals. A cancelled or disconnected
// hold is forgotten: reconnecting never resumes it. Exactly one hold exists at a
// time; beginning another releases the previous one first.

import type { HoldEvent, HoldMessage, NavAction } from './contract.ts';

export interface HoldTransport {
  /** Sends a message; returns false (and drops it) when the socket is not open. */
  send(message: HoldMessage): boolean;
}

export interface HoldTimers {
  setInterval(handler: () => void, ms: number): unknown;
  clearInterval(id: unknown): void;
}

export type HoldEndReason = 'released' | 'hidden' | 'blur' | 'disconnected' | 'replaced' | 'server';

export interface HoldListener {
  onStart(holdId: string, action: NavAction): void;
  onEnd(holdId: string, reason: HoldEndReason, event?: HoldEvent): void;
  /** The socket was not open; the tap still happened but nothing will repeat. */
  onUnavailable(action: NavAction): void;
}

export interface HoldOptions {
  renewMs: number;
  newId: () => string;
  timers?: HoldTimers;
}

interface CurrentHold {
  holdId: string;
  action: NavAction;
  timer: unknown;
}

const defaultTimers: HoldTimers = {
  setInterval: (handler, ms) => setInterval(handler, ms),
  clearInterval: (id) => clearInterval(id as ReturnType<typeof setInterval>),
};

export class HoldController {
  private current: CurrentHold | null = null;
  private readonly timers: HoldTimers;

  /**
   * @param transport Socket wrapper that sends hold messages.
   * @param options Renew cadence and id source.
   * @param listener Lifecycle callbacks for the UI.
   */
  constructor(
    private readonly transport: HoldTransport,
    private readonly options: HoldOptions,
    private readonly listener: HoldListener,
  ) {
    this.timers = options.timers ?? defaultTimers;
  }

  /** @returns True while a lease is being renewed. */
  get active(): boolean {
    return this.current !== null;
  }

  /** @returns The current hold id or null. */
  get holdId(): string | null {
    return this.current?.holdId ?? null;
  }

  /**
   * @param action Holdable navigation action.
   * @param contextEpoch Epoch of the last snapshot the UI rendered.
   * @returns The new hold id, or null when the socket was not open.
   */
  begin(action: NavAction, contextEpoch: number): string | null {
    if (this.current) this.end('replaced', true);
    const holdId = this.options.newId();
    if (!this.transport.send({ type: 'hold.start', hold_id: holdId, action, context_epoch: contextEpoch })) {
      this.listener.onUnavailable(action);
      return null;
    }
    const timer = this.timers.setInterval(() => this.renew(), this.options.renewMs);
    this.current = { holdId, action, timer };
    this.listener.onStart(holdId, action);
    return holdId;
  }

  /** Pointer released: stops the lease. */
  release(): void {
    if (this.current) this.end('released', true);
  }

  /**
   * @param reason Why the hold ends without a release; `disconnected` sends nothing.
   */
  cancel(reason: Exclude<HoldEndReason, 'released' | 'replaced' | 'server'>): void {
    if (this.current) this.end(reason, reason !== 'disconnected');
  }

  /**
   * @param event Server lease report; ignored unless it names the current hold.
   */
  serverEvent(event: HoldEvent): void {
    if (!this.current || this.current.holdId !== event.hold_id) return;
    if (event.state === 'active') return;
    this.end('server', false, event);
  }

  private renew(): void {
    if (!this.current) return;
    if (!this.transport.send({ type: 'hold.renew', hold_id: this.current.holdId })) {
      this.end('disconnected', false);
    }
  }

  private end(reason: HoldEndReason, sendStop: boolean, event?: HoldEvent): void {
    const current = this.current;
    if (!current) return;
    this.current = null;
    this.timers.clearInterval(current.timer);
    if (sendStop) this.transport.send({ type: 'hold.stop', hold_id: current.holdId });
    this.listener.onEnd(current.holdId, reason, event);
  }
}

export interface VisibilityDocument {
  readonly hidden: boolean;
  addEventListener(type: 'visibilitychange', listener: () => void): void;
  removeEventListener(type: 'visibilitychange', listener: () => void): void;
}

export interface BlurWindow {
  addEventListener(type: 'blur', listener: () => void): void;
  removeEventListener(type: 'blur', listener: () => void): void;
}

/**
 * Binds the page-level cancel triggers: a hidden tab and a blurred window.
 * @param controller Hold controller to cancel.
 * @param doc Document providing `hidden` and `visibilitychange`.
 * @param win Window providing `blur`.
 * @returns A function that removes both listeners.
 */
export function bindHoldLifecycle(controller: HoldController, doc: VisibilityDocument, win: BlurWindow): () => void {
  const onVisibility = () => {
    if (doc.hidden) controller.cancel('hidden');
  };
  const onBlur = () => controller.cancel('blur');
  doc.addEventListener('visibilitychange', onVisibility);
  win.addEventListener('blur', onBlur);
  return () => {
    doc.removeEventListener('visibilitychange', onVisibility);
    win.removeEventListener('blur', onBlur);
  };
}
