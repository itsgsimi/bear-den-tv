// The navigation script: TV-style D-pad control of an ordinary website. The
// coordinator (internal/applications/web) injects dist/nav.js into an
// isolated world of every page of a web app over the Chrome DevTools Protocol
// (docs/decisions/0010-web-apps-over-cdp-pipe.md). The page's own scripts
// cannot see this world; the DOM is shared.
//
// The coordinator calls __bdtv.apply(action, arg) with a name from a closed
// set and gets a Reply back. The script moves its own focus ring itself and
// reports where focus landed; anything that needs trusted input (a click, the
// site's keyboard shortcuts, typing) comes back as an Effect the coordinator
// performs with CDP Input events. It pushes page status (video, text field,
// visibility) through the __bdtvReport binding, which exists only in this
// world. Hints (hints/<adapter>.json, contracts/web-hints.schema.json) tune it
// per site; without them it still works.

import { pick, type Box, type Direction } from './spatial';
import type { BackStep, Effect, FocusInfo, Hints, Key, Reply, Status, VideoState } from './types';

const TARGETS = [
  'a[href]', 'button', 'input:not([type=hidden])', 'select', 'textarea', 'summary',
  '[role=button]', '[role=link]', '[role=menuitem]', '[role=tab]', '[role=option]',
  '[role=checkbox]', '[role=switch]', '[tabindex]:not([tabindex="-1"])',
  '[contenteditable=""]', '[contenteditable=true]',
].join(',');
const POINTER_SCAN = 'div,li,span,img,article,figure,section,p,h1,h2,h3,h4,svg';
const POINTER_SCAN_MAX = 4000;
const MODALS = '[role=dialog],[role=alertdialog],[aria-modal=true],dialog[open]';
const CLOSERS = '[aria-label*="close" i],[aria-label*="dismiss" i],button[class*="close" i],[data-dismiss],[data-bs-dismiss]';
const TEXT_TYPES = new Set(['', 'text', 'search', 'email', 'url', 'tel', 'password', 'number']);
const DEFAULT_BACK: BackStep[] = ['field', 'fullscreen', 'overlay', 'history'];
const RING = '#F5C542';

interface Candidate {
  el: Element;
  box: Box;
}

type Report = (json: string) => void;

export interface Api {
  apply(action: string, arg?: number): Reply;
  status(): Status;
  cursor(x: number, y: number): Reply;
  configure(hints: Hints | null): void;
}

let hints: Hints | null = null;
let current: Element | null = null;
let currentIndex = -1;
let cache: Element[] | null = null;
let ring: HTMLDivElement | null = null;
let pointer: HTMLDivElement | null = null;
let lastReport = '';

function report(): void {
  const fn = (globalThis as unknown as { __bdtvReport?: Report }).__bdtvReport;
  if (typeof fn !== 'function') return;
  const json = JSON.stringify(status());
  if (json === lastReport) return;
  lastReport = json;
  try {
    fn(json);
  } catch {
    // The coordinator went away; nothing to tell.
  }
}

function safeAll(root: ParentNode, selectors: string[] | undefined): Element[] {
  const out: Element[] = [];
  for (const s of selectors ?? []) {
    try {
      out.push(...Array.from(root.querySelectorAll(s)));
    } catch {
      // A hint selector that does not parse is ignored.
    }
  }
  return out;
}

function matchesAny(el: Element, selectors: string[] | undefined): boolean {
  for (const s of selectors ?? []) {
    try {
      if (el.closest(s)) return true;
    } catch {
      // ignored, as above
    }
  }
  return false;
}

function box(el: Element): Box {
  const r = el.getBoundingClientRect();
  return { left: r.left, top: r.top, right: r.right, bottom: r.bottom };
}

function visible(el: Element): boolean {
  const r = el.getBoundingClientRect();
  if (r.width < 4 || r.height < 4) return false;
  const cs = getComputedStyle(el);
  if (cs.visibility === 'hidden' || cs.display === 'none' || Number(cs.opacity) < 0.05) return false;
  if (el.closest('[aria-hidden=true],[inert],[hidden]')) return false;
  if ((el as HTMLButtonElement).disabled) return false;
  return true;
}

function isTextField(el: Element | null): boolean {
  if (!el) return false;
  if (el instanceof HTMLTextAreaElement) return !el.readOnly && !el.disabled;
  if (el instanceof HTMLInputElement) return TEXT_TYPES.has(el.type) && !el.readOnly && !el.disabled;
  return (el as HTMLElement).isContentEditable === true;
}

function roleOf(el: Element): string {
  const role = el.getAttribute('role');
  if (role) return role.slice(0, 24);
  if (isTextField(el)) return 'textbox';
  const tag = el.tagName.toLowerCase();
  if (tag === 'a') return 'link';
  if (tag === 'button' || tag === 'summary') return 'button';
  if (tag === 'select') return 'listbox';
  return 'generic';
}

/** Every element that could take focus, before the per-move visibility check. */
function collect(): Element[] {
  if (cache) return cache;
  const set = new Set<Element>(Array.from(document.querySelectorAll(TARGETS)));
  for (const el of safeAll(document, hints?.prefer)) set.add(el);
  const scan = document.querySelectorAll(POINTER_SCAN);
  for (let i = 0; i < scan.length && i < POINTER_SCAN_MAX; i++) {
    const el = scan[i]!;
    if (set.has(el)) continue;
    if (getComputedStyle(el).cursor !== 'pointer') continue;
    const parent = el.parentElement;
    if (parent && getComputedStyle(parent).cursor === 'pointer') continue;
    set.add(el);
  }
  const all = Array.from(set).filter((el) => !isOurs(el) && !matchesAny(el, hints?.skip));
  // A candidate that wraps other candidates is a container (a row, a card
  // wrapper with buttons in it) unless it is the same size as its only child
  // candidate, in which case the child is the duplicate.
  const drop = new Set<Element>();
  for (const el of all) {
    const inner = all.filter((o) => o !== el && el.contains(o));
    if (inner.length === 0) continue;
    const b = el.getBoundingClientRect();
    const same = inner.length === 1 && sameBox(b, inner[0]!.getBoundingClientRect());
    if (same) drop.add(inner[0]!);
    else drop.add(el);
  }
  cache = all.filter((el) => !drop.has(el));
  return cache;
}

function sameBox(a: DOMRect, b: DOMRect): boolean {
  return Math.abs(a.left - b.left) < 6 && Math.abs(a.top - b.top) < 6 && Math.abs(a.right - b.right) < 6 && Math.abs(a.bottom - b.bottom) < 6;
}

function isOurs(el: Element): boolean {
  return el.hasAttribute('data-bdtv-overlay');
}

/** The topmost open overlay, if any: focus stays inside it. */
function topModal(): Element | null {
  const found = [...Array.from(document.querySelectorAll(MODALS)), ...safeAll(document, hints?.modal)].filter(visible);
  return found.length ? found[found.length - 1]! : null;
}

function candidates(): Candidate[] {
  const modal = topModal();
  const out: Candidate[] = [];
  for (const el of collect()) {
    if (!el.isConnected) continue;
    if (modal && !modal.contains(el)) continue;
    if (!visible(el)) continue;
    out.push({ el, box: box(el) });
  }
  return out;
}

function inViewport(b: Box): boolean {
  return b.bottom > 0 && b.right > 0 && b.top < innerHeight && b.left < innerWidth;
}

function initial(list: Candidate[]): Candidate | null {
  for (const el of safeAll(document, hints?.initial)) {
    const c = list.find((x) => x.el === el);
    if (c) return c;
  }
  const inView = list.filter((c) => inViewport(c.box));
  const pool = inView.length ? inView : list;
  let best: Candidate | null = null;
  for (const c of pool) {
    if (!best || c.box.top < best.box.top - 4 || (Math.abs(c.box.top - best.box.top) <= 4 && c.box.left < best.box.left)) best = c;
  }
  return best;
}

function overlayHost(): ShadowRoot {
  let host = document.querySelector('[data-bdtv-overlay]') as HTMLElement | null;
  if (host && (host as unknown as { __root?: ShadowRoot }).__root) return (host as unknown as { __root: ShadowRoot }).__root;
  host = document.createElement('div');
  host.setAttribute('data-bdtv-overlay', '');
  const root = host.attachShadow({ mode: 'closed' });
  (host as unknown as { __root: ShadowRoot }).__root = root;
  (document.documentElement || document.body).appendChild(host);
  return root;
}

function ensureRing(): HTMLDivElement {
  if (ring && ring.isConnected) return ring;
  ring = document.createElement('div');
  const s = ring.style;
  s.position = 'fixed';
  s.pointerEvents = 'none';
  s.zIndex = '2147483647';
  s.border = `5px solid ${RING}`;
  s.borderRadius = '10px';
  s.boxShadow = '0 0 0 3px rgba(0,0,0,0.65), 0 0 22px 6px rgba(245,197,66,0.75)';
  s.display = 'none';
  overlayHost().appendChild(ring);
  return ring;
}

function placeRing(): void {
  if (!current || !current.isConnected) {
    if (ring) ring.style.display = 'none';
    return;
  }
  const r = current.getBoundingClientRect();
  const el = ensureRing();
  const pad = 6;
  el.style.left = `${r.left - pad}px`;
  el.style.top = `${r.top - pad}px`;
  el.style.width = `${r.width + pad * 2}px`;
  el.style.height = `${r.height + pad * 2}px`;
  el.style.display = 'block';
}

function focusInfo(): FocusInfo | undefined {
  if (!current) return undefined;
  return { role: roleOf(current), text_field: isTextField(current), index: currentIndex };
}

function setFocus(c: Candidate, list: Candidate[]): void {
  if (current) current.removeAttribute('data-bdtv-focused');
  current = c.el;
  currentIndex = list.indexOf(c);
  current.setAttribute('data-bdtv-focused', '');
  const h = current as HTMLElement;
  if (typeof h.focus === 'function') h.focus({ preventScroll: true });
  const b = box(current);
  const margin = 24;
  if (b.top < margin || b.bottom > innerHeight - margin || b.left < margin || b.right > innerWidth - margin) {
    current.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' as ScrollBehavior });
  }
  placeRing();
}

function move(dir: Direction): Reply {
  const list = candidates();
  if (list.length === 0) return { ok: false, outcome: 'none', reason: 'Nothing on this page can take focus.' };
  const from = current && current.isConnected ? list.find((c) => c.el === current) : undefined;
  if (!from) {
    const first = initial(list)!;
    setFocus(first, list);
    return { ok: true, outcome: 'moved', focus: focusInfo() };
  }
  const i = pick(from.box, list.map((c) => c.box), dir);
  if (i >= 0) {
    setFocus(list[i]!, list);
    return { ok: true, outcome: 'moved', focus: focusInfo() };
  }
  if (dir === 'down' || dir === 'up') {
    const before = scrollY;
    scrollBy({ top: (dir === 'down' ? 1 : -1) * innerHeight * 0.6, behavior: 'instant' as ScrollBehavior });
    if (scrollY !== before) {
      cache = null; // more rows may load
      placeRing();
      return { ok: true, outcome: 'scrolled', focus: focusInfo() };
    }
  }
  return { ok: true, outcome: 'edge', focus: focusInfo() };
}

function centre(el: Element): { x: number; y: number } {
  const r = el.getBoundingClientRect();
  const left = Math.max(0, r.left);
  const top = Math.max(0, r.top);
  const right = Math.min(innerWidth, r.right);
  const bottom = Math.min(innerHeight, r.bottom);
  return { x: Math.round((left + right) / 2), y: Math.round((top + bottom) / 2) };
}

function select(): Reply {
  if (!current || !current.isConnected) {
    const r = move('down');
    return r.ok ? { ...r, outcome: 'moved' } : r;
  }
  if (isTextField(current)) {
    (current as HTMLElement).focus({ preventScroll: true });
    report();
    return { ok: true, outcome: 'text_field', focus: focusInfo() };
  }
  const b = box(current);
  if (!inViewport(b)) current.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' as ScrollBehavior });
  const p = centre(current);
  return { ok: true, outcome: 'click', effect: { kind: 'click', x: p.x, y: p.y }, focus: focusInfo() };
}

function closeButton(modal: Element): Element | null {
  for (const el of [...safeAll(modal, hints?.close), ...Array.from(modal.querySelectorAll(CLOSERS))]) {
    if (visible(el)) return el;
  }
  return null;
}

function canGoBack(): boolean {
  const nav = (globalThis as unknown as { navigation?: { canGoBack?: boolean } }).navigation;
  if (nav && typeof nav.canGoBack === 'boolean') return nav.canGoBack;
  return history.length > 1;
}

function back(): Reply {
  for (const step of hints?.back ?? DEFAULT_BACK) {
    switch (step) {
      case 'field': {
        const a = document.activeElement;
        if (isTextField(a)) {
          (a as HTMLElement).blur();
          report();
          return { ok: true, outcome: 'left_field', focus: focusInfo() };
        }
        break;
      }
      case 'fullscreen':
        if (document.fullscreenElement) {
          void document.exitFullscreen().catch(() => undefined);
          return { ok: true, outcome: 'exited_fullscreen' };
        }
        break;
      case 'overlay': {
        const m = topModal();
        if (m) {
          const btn = closeButton(m);
          if (current && m.contains(current)) {
            current.removeAttribute('data-bdtv-focused');
            current = null;
          }
          if (btn) {
            const p = centre(btn);
            return { ok: true, outcome: 'closing_overlay', effect: { kind: 'click', x: p.x, y: p.y } };
          }
          return { ok: true, outcome: 'closing_overlay', effect: { kind: 'keys', keys: ['Escape'] } };
        }
        break;
      }
      case 'history':
        if (canGoBack()) {
          if (current) current.removeAttribute('data-bdtv-focused');
          current = null;
          history.back();
          return { ok: true, outcome: 'history_back' };
        }
        break;
    }
  }
  return { ok: true, outcome: 'at_root' };
}

/** The page's main video: the largest one with media loaded. */
function mainVideo(): HTMLVideoElement | null {
  let best: HTMLVideoElement | null = null;
  let area = 0;
  for (const v of Array.from(document.querySelectorAll('video'))) {
    const r = v.getBoundingClientRect();
    const a = r.width * r.height;
    if (v.readyState === 0 && !v.currentSrc) continue;
    if (a >= area) {
      best = v;
      area = a;
    }
  }
  return best;
}

function videoState(): VideoState {
  const v = mainVideo();
  if (!v) return 'none';
  return v.paused || v.ended ? 'paused' : 'playing';
}

/** Makes sure the site's shortcut keys reach its player, not a focused button. */
function prepareKeys(): void {
  const a = document.activeElement as HTMLElement | null;
  if (a && a !== document.body) a.blur();
  if (hints?.player) {
    try {
      const p = document.querySelector(hints.player) as HTMLElement | null;
      if (p) p.focus({ preventScroll: true });
    } catch {
      // bad selector: keys go to the document
    }
  }
}

function media(action: string, seconds: number): Reply {
  const v = mainVideo();
  if (!v) return { ok: false, outcome: 'none', reason: 'No video on this page.' };
  const keys = hints?.media ?? {};
  const toggle: Key = keys.toggle ?? ' ';
  const paused = v.paused || v.ended;
  switch (action) {
    case 'media.pause':
      if (paused) return { ok: true, outcome: 'already_paused' };
      prepareKeys();
      return { ok: true, outcome: 'keys', effect: { kind: 'keys', keys: [toggle] } };
    case 'media.play':
      if (!paused) return { ok: true, outcome: 'already_playing' };
      prepareKeys();
      return { ok: true, outcome: 'keys', effect: { kind: 'keys', keys: [toggle] } };
    case 'media.seek_relative': {
      if (!seconds) return { ok: false, outcome: 'none', reason: 'Nothing to seek.' };
      const step = keys.seek_step_s ?? 10;
      const n = Math.min(30, Math.max(1, Math.round(Math.abs(seconds) / step)));
      const key: Key = seconds < 0 ? (keys.seek_back ?? 'ArrowLeft') : (keys.seek_forward ?? 'ArrowRight');
      prepareKeys();
      return { ok: true, outcome: 'keys', effect: { kind: 'keys', keys: Array(n).fill(key) as Key[] } };
    }
  }
  return { ok: false, outcome: 'none', reason: 'Unknown media action.' };
}

function textPrepare(): Reply {
  const a = document.activeElement;
  if (!isTextField(a)) return { ok: false, outcome: 'none', reason: 'No text field is focused.' };
  if (a instanceof HTMLInputElement || a instanceof HTMLTextAreaElement) {
    a.select();
  } else {
    const range = document.createRange();
    range.selectNodeContents(a as Node);
    const sel = getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
  }
  return { ok: true, outcome: 'text', effect: { kind: 'text' } as Effect };
}

function cursor(x: number, y: number): Reply {
  if (!pointer || !pointer.isConnected) {
    pointer = document.createElement('div');
    const s = pointer.style;
    s.position = 'fixed';
    s.pointerEvents = 'none';
    s.zIndex = '2147483647';
    s.width = '28px';
    s.height = '28px';
    s.marginLeft = '-14px';
    s.marginTop = '-14px';
    s.borderRadius = '50%';
    s.background = 'rgba(245,197,66,0.85)';
    s.border = '3px solid rgba(0,0,0,0.7)';
    overlayHost().appendChild(pointer);
  }
  pointer.style.left = `${Math.round(x)}px`;
  pointer.style.top = `${Math.round(y)}px`;
  pointer.style.display = 'block';
  return { ok: true, outcome: 'cursor' };
}

export function status(): Status {
  const s: Status = {
    v: 1,
    visible: document.visibilityState === 'visible',
    video: videoState(),
    text_field: isTextField(document.activeElement),
    fullscreen: !!document.fullscreenElement,
    viewport: { w: innerWidth, h: innerHeight },
  };
  const f = focusInfo();
  if (f) s.focus = f;
  return s;
}

export function apply(action: string, arg?: number): Reply {
  let r: Reply;
  switch (action) {
    case 'nav.up':
      r = move('up');
      break;
    case 'nav.down':
      r = move('down');
      break;
    case 'nav.left':
      r = move('left');
      break;
    case 'nav.right':
      r = move('right');
      break;
    case 'select':
      r = select();
      break;
    case 'back':
      r = back();
      break;
    case 'media.play':
    case 'media.pause':
    case 'media.seek_relative':
      r = media(action, arg ?? 0);
      break;
    case 'text.prepare':
      r = textPrepare();
      break;
    default:
      r = { ok: false, outcome: 'none', reason: 'Unknown action.' };
  }
  report();
  return r;
}

let installed = false;

/** Installs the page hooks once per document; later calls only swap hints. */
export function install(h: Hints | null): Api {
  hints = h;
  cache = null;
  const api: Api = { apply, status, cursor, configure: (n) => { hints = n; cache = null; } };
  (globalThis as unknown as { __bdtv: Api }).__bdtv = api;
  if (installed) return api;
  installed = true;
  const invalidate = () => {
    cache = null;
  };
  const start = () => {
    new MutationObserver(invalidate).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style', 'hidden', 'aria-hidden', 'open', 'role'] });
    report();
  };
  if (document.documentElement) start();
  else document.addEventListener('DOMContentLoaded', start, { once: true });
  addEventListener('scroll', placeRing, { capture: true, passive: true });
  addEventListener('resize', () => { placeRing(); report(); }, { passive: true });
  document.addEventListener('visibilitychange', report);
  document.addEventListener('fullscreenchange', report);
  document.addEventListener('focusin', report, true);
  document.addEventListener('focusout', () => setTimeout(report, 0), true);
  for (const ev of ['play', 'playing', 'pause', 'ended', 'emptied', 'loadeddata']) document.addEventListener(ev, report, true);
  addEventListener('load', report);
  return api;
}
