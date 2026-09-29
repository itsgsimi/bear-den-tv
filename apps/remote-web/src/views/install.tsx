// "Add apps" on the owner's phone: the apps Bear Den knows that are not
// installed on the TV, one row per Flatpak (the four web apps are one
// Chromium row), with Install, the install's progress from
// `state.applications[].install`, and Cancel (`app.install`,
// `app.install_cancel`; contracts/actions.md "App installs",
// docs/decisions/0011-per-user-flathub-installs.md). Contract: drawn only
// for a phone holding `owner`, never on a guest pass or a family phone (the
// server does not send them install state either, and refuses the actions),
// and only while `capabilities["app.install"]` is listed. Progress moves on
// snapshots only: no timer here.
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { AppIcons, Application, Install, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { AppArt, artStyleOf } from '../icons.tsx';
import { type AppState, capabilityFor, isGuest, permissionsOf } from '../state.ts';

/** Adapters that install a shared Flatpak rather than an app of their own (the web apps: Chromium). */
const SHARED_INSTALL: Readonly<Record<string, string>> = { netflix: 'chromium', 'disney-plus': 'chromium', hulu: 'chromium', browser: 'chromium' };

export interface InstallEntry {
  /** The config app id the actions name. */
  id: string;
  adapter: string;
  label: string;
  why: string;
  install: Install;
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns One entry per missing Flatpak that carries install state, first app wins.
 */
export function installEntries(snapshot: StateSnapshot | null): InstallEntry[] {
  const out: InstallEntry[] = [];
  const seen = new Set<string>();
  for (const a of snapshot?.applications ?? ([] as Application[])) {
    if (a.installed || !a.install) continue;
    const key = SHARED_INSTALL[a.adapter] ?? a.id;
    if (seen.has(key)) continue;
    seen.add(key);
    const shared = SHARED_INSTALL[a.adapter] !== undefined;
    out.push({ id: a.id, adapter: a.adapter, label: shared ? t.install.chromium : a.label, why: shared ? t.install.chromiumWhy : '', install: a.install });
  }
  return out;
}

/**
 * @param state Application state.
 * @returns Whether this phone may see install controls: the owner, never a guest pass.
 */
export function mayInstall(state: AppState): boolean {
  return !isGuest(state) && permissionsOf(state).includes('owner');
}

const running = (i: Install) => i.state === 'preparing' || i.state === 'downloading' || i.state === 'installing';

function sizeText(bytes: number): string {
  return bytes >= 1e9 ? `${(bytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(bytes / 1e6))} MB`;
}

function statusText(i: Install): string {
  switch (i.state) {
    case 'preparing':
      return t.install.preparing;
    case 'downloading':
      return t.install.installing(i.progress);
    case 'installing':
      return t.install.finishing;
    case 'done':
      return t.install.ready;
    case 'failed':
      return i.message ?? t.install.failed;
    case 'none':
      return i.message ?? '';
    default:
      return i.size_bytes ? t.install.size(sizeText(i.size_bytes)) : t.install.from;
  }
}

export interface AddAppsSectionProps {
  entries: InstallEntry[];
  /** app.install is available (else `reason`). */
  available: boolean;
  reason: string | null;
  art: 'pixel' | 'classic';
  /** The TV's app icon choice (appearance.app_icons). */
  icons?: AppIcons;
  onInstall: (appId: string) => void;
  onCancel: (appId: string) => void;
}

/** The section itself: nothing when every app is installed. */
export function AddAppsSection({ entries, available, reason, art, icons, onInstall, onCancel }: AddAppsSectionProps): JSX.Element | null {
  if (entries.length === 0) return null;
  return (
    <div class="group add-apps" aria-labelledby="add-apps-heading" data-testid="add-apps">
      <h3 id="add-apps-heading">{t.install.heading}</h3>
      <p class="muted small">{available ? t.install.note : reason}</p>
      <ul class="install-list">
        {entries.map((e) => (
          <li key={e.id} class={`install-row is-${e.install.state}`} data-testid={`install-${e.id}`}>
            <span class="app-icon">
              <AppArt adapter={e.adapter} size={32} art={art} icons={icons} installed={false} />
            </span>
            <span class="install-text">
              <span class="install-label">{e.label}</span>
              {e.why ? <span class="muted small">{e.why}</span> : null}
              <span class="muted small install-status" role="status">
                {statusText(e.install)}
              </span>
              {running(e.install) ? <progress class="install-progress" max={100} value={e.install.progress} data-testid={`install-progress-${e.id}`} /> : null}
            </span>
            {running(e.install) ? (
              <button type="button" class="btn btn-control btn-secondary" data-action="app.install_cancel" data-testid={`install-cancel-${e.id}`} onClick={() => onCancel(e.id)}>
                <span>{t.install.cancel}</span>
              </button>
            ) : e.install.state === 'available' || e.install.state === 'failed' ? (
              <button type="button" class="btn btn-control" disabled={!available} title={available ? undefined : (reason ?? undefined)} data-action="app.install" data-testid={`install-button-${e.id}`} onClick={() => onInstall(e.id)}>
                <span>{e.install.state === 'failed' ? t.install.tryAgain : t.install.install}</span>
              </button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The section wired to the store and the controller; owner phones only. */
export function AddAppsPanel({ app, state }: { app: App; state: AppState }): JSX.Element | null {
  const snapshot = state.snapshot;
  if (!mayInstall(state) || snapshot?.capabilities['app.install'] === undefined) return null;
  const cap = capabilityFor(snapshot, 'app.install');
  return (
    <AddAppsSection
      entries={installEntries(snapshot)}
      available={cap.available}
      reason={cap.reason ?? null}
      art={artStyleOf(snapshot?.appearance)}
      icons={snapshot?.appearance?.app_icons}
      onInstall={(id) => void app.tap('app.install', { app_id: id })}
      onCancel={(id) => void app.tap('app.install_cancel', { app_id: id })}
    />
  );
}
