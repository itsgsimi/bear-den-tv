// Tactile press feedback for buttons. Contract: purely presentational. On every
// primary pointerdown on an enabled `.btn` or `.tab`, the element's `data-press`
// attribute flips between "a" and "b"; app.css keys a ripple animation on each
// value, so every press restarts it without forced reflow. Preact never renders
// `data-press`, so the attribute does not fight the virtual DOM. A no-op
// touchstart listener lets iOS Safari apply `:active` for the scale-down. A tap
// inside a `.vine-host` also sets `data-vine="on"` on the host for VINE_TAP_MS so
// its corner vines grow briefly on touch screens, then retract.

const VINE_TAP_MS = 1400;

/**
 * @param doc Document to observe.
 * @returns A function that removes the listeners.
 */
export function installPressFeedback(doc: Document): () => void {
  const vineTimers = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>();
  const onDown = (ev: PointerEvent) => {
    if (ev.button !== 0) return;
    const el = ev.target instanceof Element ? ev.target.closest<HTMLElement>('.btn, .tab') : null;
    if (!el || (el instanceof HTMLButtonElement && el.disabled)) return;
    el.dataset.press = nextPress(el.dataset.press);
    const host = el.closest<HTMLElement>('.vine-host');
    if (host) {
      host.dataset.vine = 'on';
      clearTimeout(vineTimers.get(host));
      vineTimers.set(host, setTimeout(() => delete host.dataset.vine, VINE_TAP_MS));
    }
  };
  const onTouch = () => undefined;
  doc.addEventListener('pointerdown', onDown, { capture: true, passive: true });
  doc.addEventListener('touchstart', onTouch, { passive: true });
  return () => {
    doc.removeEventListener('pointerdown', onDown, { capture: true });
    doc.removeEventListener('touchstart', onTouch);
  };
}

/**
 * @param current Current `data-press` value.
 * @returns The alternate value, so the keyed CSS animation restarts.
 */
export function nextPress(current: string | undefined): 'a' | 'b' {
  return current === 'a' ? 'b' : 'a';
}
