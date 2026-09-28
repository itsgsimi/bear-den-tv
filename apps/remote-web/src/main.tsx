// Browser entry point: mounts the Preact tree and starts the controller.
// Contract: this module is the only place that reads browser globals for wiring;
// everything else receives them through the environments built here.
import { render } from 'preact';
import { browserEnvironment } from './api.ts';
import { createApp } from './app.ts';
import { Root } from './views/shell.tsx';
import { installPressFeedback } from './press-fx.ts';
import { artStyleOf } from './icons.tsx';

const app = createApp(browserEnvironment(), document, window);
const mount = document.getElementById('app');
if (!mount) throw new Error('missing #app mount point');
installPressFeedback(document);
// Mirror the TV's theme (state.appearance, resolved by internal/themes): the
// accent and palette become CSS variables (set through the CSSOM, which the
// page's CSP allows), the backdrop and veil style .ambient-backdrop, and
// data-particles / data-style pick the particle set and Plain, data-art the
// art style (pixel or classic), and data-pixel draws a pixel-art backdrop
// without smoothing (app.css).
const hexToRgb = (hex: string | undefined): string | null => {
  const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex ?? '');
  return m ? `${parseInt(m[1] ?? '0', 16)}, ${parseInt(m[2] ?? '0', 16)}, ${parseInt(m[3] ?? '0', 16)}` : null;
};
let appliedKey = '';
const applyAppearance = (): void => {
  const appearance = app.store.getState().snapshot?.appearance;
  const key = JSON.stringify(appearance ?? null);
  if (key === appliedKey) return;
  appliedKey = key;
  const root = document.documentElement;
  const style = root.style;
  const set = (name: string, value: string | null | undefined) => (value ? style.setProperty(name, value) : style.removeProperty(name));
  const accent = appearance?.accent;
  const deep = appearance?.palette?.dark;
  set('--sage', accent);
  set('--sage-rgb', hexToRgb(accent));
  set('--sage-deep', deep);
  set('--sage-deep-rgb', hexToRgb(deep));
  set('--sage-light', appearance?.palette?.light);
  set('--veil-rgb', hexToRgb(appearance?.phone?.veil));
  // No appearance yet (pairing): the den backdrop from app.css. A theme without
  // a backdrop (a gradient-only theme such as Winter) gets none.
  set('--backdrop', !appearance ? null : appearance.phone?.backdrop ? `url("${appearance.phone.backdrop.replace(/"/g, '')}")` : 'none');
  root.dataset.particles = appearance?.phone?.particles || 'fireflies';
  root.dataset.style = appearance?.theme ?? 'den-dark';
  // Pixel-art wallpaper (and the built-in den backdrop before pairing): draw it unsmoothed.
  root.dataset.pixel = !appearance || appearance.pixel ? 'on' : 'off';
  root.dataset.art = artStyleOf(appearance);
};
applyAppearance();
app.store.subscribe(applyAppearance);
render(<Root app={app} />, mount);
void app.start();
