// Touchpad gestures for web apps (views/touchpad.tsx; contracts/actions.md
// "Pointer"): one finger drags the TV's pointer (pointer.move), a quick tap
// clicks (pointer.click left), a two-finger tap right-clicks, and two fingers
// dragging scroll the page (pointer.scroll). Pure logic with an injected
// sink, clock and scheduler so it is unit tested without a browser
// (tests/unit/touchpad.spec.ts). Movement is coalesced: at most one move and
// one scroll request in flight, at most one every MIN_GAP_MS, each within the
// contract's limits, so the coordinator's rate limit (60 moves, 30 scrolls a
// second) is never the thing that drops a gesture.

/** Where gestures go: the controller sends them as named actions. */
export interface PointerSink {
  move(dx: number, dy: number): Promise<void>;
  scroll(dy: number): Promise<void>;
  click(button: 'left' | 'right'): void;
}

export interface TouchpadOptions {
  now: () => number;
  /** Runs fn after about ms milliseconds (setTimeout in the browser). */
  schedule: (fn: () => void, ms: number) => void;
  /** Screen pixels of pointer travel per pixel of finger travel. */
  sensitivity?: number;
  /** Page pixels scrolled per pixel of two-finger travel. */
  scrollGain?: number;
}

/** The contract's per-request limits (action.schema.json). */
export const MAX_MOVE = 400;
export const MAX_SCROLL = 2000;
/** Minimum spacing between two requests of one kind (≤ 50 a second). */
export const MIN_GAP_MS = 20;
/** A tap is shorter than this and moves less than TAP_SLOP pixels. */
export const TAP_MS = 250;
export const TAP_SLOP = 8;

interface Finger {
  x: number;
  y: number;
  startX: number;
  startY: number;
}

const clamp = (v: number, lim: number): number => Math.max(-lim, Math.min(lim, v));

/** One channel (move or scroll): accumulates, sends one request at a time. */
class Channel {
  private dx = 0;
  private dy = 0;
  private busy = false;
  private scheduled = false;
  private last = -Infinity;

  constructor(
    private readonly send: (dx: number, dy: number) => Promise<void>,
    private readonly limit: number,
    private readonly opts: TouchpadOptions,
  ) {}

  add(dx: number, dy: number): void {
    this.dx += dx;
    this.dy += dy;
    this.kick();
  }

  private kick(): void {
    if (this.busy || this.scheduled) return;
    if (Math.round(this.dx) === 0 && Math.round(this.dy) === 0) return;
    const wait = Math.max(0, this.last + MIN_GAP_MS - this.opts.now());
    this.scheduled = true;
    this.opts.schedule(() => {
      this.scheduled = false;
      this.flush();
    }, wait);
  }

  private flush(): void {
    const dx = clamp(Math.round(this.dx), this.limit);
    const dy = clamp(Math.round(this.dy), this.limit);
    if (dx === 0 && dy === 0) return;
    this.dx -= dx;
    this.dy -= dy;
    this.busy = true;
    this.last = this.opts.now();
    this.send(dx, dy)
      .catch(() => undefined)
      .finally(() => {
        this.busy = false;
        this.kick();
      });
  }
}

export class Touchpad {
  private readonly fingers = new Map<number, Finger>();
  private downAt = 0;
  private maxFingers = 0;
  private travelled = 0;
  private readonly moves: Channel;
  private readonly scrolls: Channel;
  private readonly sensitivity: number;
  private readonly scrollGain: number;

  constructor(
    private readonly sink: PointerSink,
    private readonly opts: TouchpadOptions,
  ) {
    this.sensitivity = opts.sensitivity ?? 1.6;
    this.scrollGain = opts.scrollGain ?? 2;
    this.moves = new Channel((dx, dy) => sink.move(dx, dy), MAX_MOVE, opts);
    this.scrolls = new Channel((_dx, dy) => sink.scroll(dy), MAX_SCROLL, opts);
  }

  down(id: number, x: number, y: number): void {
    if (this.fingers.size === 0) {
      this.downAt = this.opts.now();
      this.maxFingers = 0;
      this.travelled = 0;
    }
    this.fingers.set(id, { x, y, startX: x, startY: y });
    this.maxFingers = Math.max(this.maxFingers, this.fingers.size);
  }

  move(id: number, x: number, y: number): void {
    const f = this.fingers.get(id);
    if (!f) return;
    const dx = x - f.x;
    const dy = y - f.y;
    f.x = x;
    f.y = y;
    this.travelled = Math.max(this.travelled, Math.hypot(x - f.startX, y - f.startY));
    if (this.fingers.size === 1 && this.maxFingers === 1) {
      this.moves.add(dx * this.sensitivity, dy * this.sensitivity);
    } else if (this.fingers.size >= 2) {
      // Each finger reports its own share; natural scrolling: the page
      // follows the fingers (fingers up, content up, so scroll down).
      this.scrolls.add(0, (-dy * this.scrollGain) / this.fingers.size);
    }
  }

  up(id: number): void {
    if (!this.fingers.delete(id) || this.fingers.size > 0) return;
    const quick = this.opts.now() - this.downAt < TAP_MS && this.travelled < TAP_SLOP;
    if (quick) this.sink.click(this.maxFingers >= 2 ? 'right' : 'left');
  }

  cancel(id: number): void {
    this.fingers.delete(id);
    if (this.fingers.size === 0) this.travelled = TAP_SLOP; // never a tap
  }
}
