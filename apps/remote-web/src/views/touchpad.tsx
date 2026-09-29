// The touchpad for web apps (streaming sites, the Browser tile): the escape
// hatch for anything the D-pad cannot reach. Drawn only while
// `capabilities["pointer.move"]` is available (a web app in front, never a
// guest pass); gestures come from ../touchpad.ts and go out through
// `app.pointer` as pointer.move / pointer.click / pointer.scroll
// (contracts/actions.md "Pointer"). Click and Right-click buttons for
// people who prefer them to taps.
import { useEffect, useMemo, useRef } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import { t } from '../i18n.ts';
import { Touchpad } from '../touchpad.ts';
import { type AppState, capabilityFor, mayUse } from '../state.ts';

/** Whether the touchpad is drawn: a web app in front that accepts pointer input. */
export function touchpadShown(state: AppState): boolean {
  return mayUse(state, 'pointer.move') && capabilityFor(state.snapshot, 'pointer.move').available === true;
}

export function TouchpadPanel({ app, state }: { app: App; state: AppState }): JSX.Element | null {
  const shown = touchpadShown(state);
  const pad = useMemo(
    () =>
      new Touchpad(
        {
          move: (dx, dy) => app.pointer('pointer.move', { dx, dy }),
          scroll: (dy) => app.pointer('pointer.scroll', { dy }),
          click: (button) => void app.pointer('pointer.click', { button }),
        },
        { now: () => performance.now(), schedule: (fn, ms) => void setTimeout(fn, ms) },
      ),
    [app],
  );
  const area = useRef<HTMLDivElement>(null);
  useEffect(() => {
    // Keep the page from scrolling or zooming under the fingers.
    const el = area.current;
    if (!el) return undefined;
    const stop = (e: TouchEvent) => e.preventDefault();
    el.addEventListener('touchmove', stop, { passive: false });
    return () => el.removeEventListener('touchmove', stop);
  }, [shown]);
  if (!shown) return null;
  const clickReason = capabilityFor(state.snapshot, 'pointer.click');
  return (
    <div class="group touchpad-group" aria-labelledby="touchpad-heading" data-testid="touchpad">
      <h3 id="touchpad-heading">{t.touchpad.title}</h3>
      <div
        ref={area}
        class="touchpad"
        role="application"
        aria-label={t.touchpad.area}
        onPointerDown={(e) => {
          (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
          pad.down(e.pointerId, e.clientX, e.clientY);
        }}
        onPointerMove={(e) => pad.move(e.pointerId, e.clientX, e.clientY)}
        onPointerUp={(e) => pad.up(e.pointerId)}
        onPointerCancel={(e) => pad.cancel(e.pointerId)}
      >
        <span class="touchpad-hint">{t.touchpad.hint}</span>
      </div>
      <div class="button-row">
        <button type="button" class="btn btn-control" data-action="pointer.click" disabled={!clickReason.available} onClick={() => void app.pointer('pointer.click', { button: 'left' })}>
          <span>{t.touchpad.click}</span>
        </button>
        <button type="button" class="btn btn-control" data-action="pointer.click-right" disabled={!clickReason.available} onClick={() => void app.pointer('pointer.click', { button: 'right' })}>
          <span>{t.touchpad.rightClick}</span>
        </button>
      </div>
    </div>
  );
}
