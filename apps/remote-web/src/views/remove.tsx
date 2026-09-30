// "Remove apps" on the owner's phone (TV test 2026-09-29 item 9;
// contracts/actions.md "App removal"): the installed apps one row per
// Flatpak (the web apps share their browser's row, named from
// state.apps.browsers, as in Add apps: install.tsx browserKey), each with
// the size Remove frees (`install.installed_bytes`), which other apps it
// turns off (the apps that run in the same browser and are on), and Remove.
// Remove asks first, inline: Cancel (first), Remove, Remove and delete its
// data, each sending `app.uninstall`. A system-wide install says it can
// only be removed with the PC's own software tool and has no button; while
// flatpak works the row says Removing…; a failure shows the coordinator's
// words (`install.message`). Contract: owner phones only (mayInstall),
// never a guest pass or a family phone, and only while
// `capabilities["app.uninstall"]` is listed. No timer here.
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { AppIcons, Application, Install, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { AppArt, artStyleOf } from '../icons.tsx';
import { browserKey, mayInstall } from './install.tsx';
import { type AppState, capabilityFor } from '../state.ts';

export interface RemoveEntry {
  /** The config app id app.uninstall names. */
  id: string;
  adapter: string;
  /** What is removed: the app, or for web apps their browser. */
  label: string;
  install: Install;
  /** Installed for everyone on the PC: not removable here. */
  system: boolean;
  /** The apps that run in the same browser and are on: removing it turns them off. */
  alsoOff: string[];
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns One entry per installed Flatpak that carries install state, first app wins.
 */
export function removeEntries(snapshot: StateSnapshot | null): RemoveEntry[] {
  const apps = snapshot?.apps;
  const groups = new Map<string, { first: Application & { install: Install }; members: Application[]; browser: string | null }>();
  for (const a of snapshot?.applications ?? ([] as Application[])) {
    if (!a.installed || !a.install) continue;
    const browser = browserKey(a.adapter, apps);
    const key = browser === null ? `app:${a.id}` : `browser:${browser}`;
    const group = groups.get(key);
    if (group) group.members.push(a);
    else groups.set(key, { first: { ...a, install: a.install }, members: [a], browser });
  }
  return [...groups.values()].map(({ first, members, browser }) => ({
    id: first.id,
    adapter: first.adapter,
    label: browser === null ? first.label : (apps?.browsers?.find((b) => b.id === browser)?.label ?? t.install.browserFallback),
    install: first.install,
    system: first.installation === 'system',
    alsoOff: browser === null ? [] : members.filter((m) => m.enabled !== false && m.hidden !== true).map((m) => m.label),
  }));
}

function sizeText(bytes: number): string {
  return bytes >= 1e9 ? `${(bytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(bytes / 1e6))} MB`;
}

/**
 * @param e A remove entry.
 * @returns The row's plain lines: what it frees and what it turns off, or why it can't go.
 */
export function removeLines(e: RemoveEntry): string[] {
  if (e.system) return [t.remove.system];
  if (e.install.state === 'removing') return [t.remove.removing];
  const lines: string[] = [];
  if (e.install.installed_bytes) lines.push(t.remove.frees(sizeText(e.install.installed_bytes)));
  if (e.alsoOff.length > 0) lines.push(t.remove.alsoOff(e.alsoOff, e.label));
  if (e.install.message) lines.push(e.install.message);
  return lines;
}

export interface RemoveAppsSectionProps {
  entries: RemoveEntry[];
  available: boolean;
  reason: string | null;
  art: 'pixel' | 'classic';
  icons?: AppIcons;
  /** The row asking "Remove …?", or null. */
  confirming: string | null;
  onAsk: (appId: string | null) => void;
  onRemove: (appId: string, deleteData: boolean) => void;
}

/** The section itself: nothing when no app is installed. */
export function RemoveAppsSection({ entries, available, reason, art, icons, confirming, onAsk, onRemove }: RemoveAppsSectionProps): JSX.Element | null {
  if (entries.length === 0) return null;
  return (
    <div class="group remove-apps" aria-labelledby="remove-apps-heading" data-testid="remove-apps">
      <h3 id="remove-apps-heading">{t.remove.heading}</h3>
      <p class="muted small">{available ? t.remove.note : reason}</p>
      <ul class="install-list">
        {entries.map((e) => (
          <li key={e.id} class={`install-row is-${e.install.state}`} data-testid={`remove-${e.id}`}>
            <span class="app-icon">
              <AppArt adapter={e.adapter} size={32} art={art} icons={icons} installed={true} />
            </span>
            <span class="install-text">
              <span class="install-label">{e.label}</span>
              {removeLines(e).map((line) => (
                <span key={line} class="muted small">
                  {line}
                </span>
              ))}
              {confirming === e.id ? (
                <span class="confirm remove-confirm" role="group" aria-label={t.remove.ask(e.label)} data-testid={`remove-confirm-${e.id}`}>
                  <span class="small">{t.remove.ask(e.label)}</span>
                  <span class="actions-row">
                    <button type="button" class="btn btn-secondary" data-testid={`remove-cancel-${e.id}`} onClick={() => onAsk(null)}>
                      {t.remove.cancel}
                    </button>
                    <button type="button" class="btn btn-danger" data-action="app.uninstall" data-testid={`remove-go-${e.id}`} onClick={() => onRemove(e.id, false)}>
                      {t.remove.remove}
                    </button>
                    <button type="button" class="btn btn-danger" data-action="app.uninstall" data-testid={`remove-data-${e.id}`} onClick={() => onRemove(e.id, true)}>
                      {t.remove.removeData}
                    </button>
                  </span>
                </span>
              ) : null}
            </span>
            {!e.system && e.install.state !== 'removing' && confirming !== e.id ? (
              <button type="button" class="btn btn-control btn-danger" disabled={!available} title={available ? undefined : (reason ?? undefined)} data-testid={`remove-button-${e.id}`} onClick={() => onAsk(e.id)}>
                <span>{t.remove.remove}</span>
              </button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The section wired to the store and the controller; owner phones only. */
export function RemoveAppsPanel({ app, state }: { app: App; state: AppState }): JSX.Element | null {
  const [confirming, setConfirming] = useState<string | null>(null);
  const snapshot = state.snapshot;
  if (!mayInstall(state) || snapshot?.capabilities['app.uninstall'] === undefined) return null;
  const cap = capabilityFor(snapshot, 'app.uninstall');
  return (
    <RemoveAppsSection
      entries={removeEntries(snapshot)}
      available={cap.available}
      reason={cap.reason ?? null}
      art={artStyleOf(snapshot?.appearance)}
      icons={snapshot?.appearance?.app_icons}
      confirming={confirming}
      onAsk={setConfirming}
      onRemove={(id, deleteData) => {
        setConfirming(null);
        void app.tap('app.uninstall', { app_id: id, delete_data: deleteData });
      }}
    />
  );
}
