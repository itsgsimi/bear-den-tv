// "Add apps" on the owner's phone: the apps Bear Den knows that are not
// installed on the TV, one row per Flatpak (the web apps share their
// browser's row: the streaming sites `state.apps.streaming_browser`'s, the
// Browser tile `state.apps.browser`'s, named from `state.apps.browsers`, one
// row when both are the same browser), each with the app's notes
// (`applications[].notes`; notes.tsx) before Install is pressed, Install, the install's progress from
// `state.applications[].install`, and Cancel (`app.install`,
// `app.install_cancel`; contracts/actions.md "App installs",
// docs/decisions/0011-per-user-flathub-installs.md). Contract: drawn only
// for a phone holding `owner`, never on a guest pass or a family phone (the
// server does not send them install state either, and refuses the actions),
// and only while `capabilities["app.install"]` is listed. Progress moves on
// snapshots only: no timer here. The "+ Add apps" tile at the end of the
// Apps grid (AddAppsTile, same rules plus at least one entry) names what is
// left to add and, tapped, scrolls this section into view and focuses its
// heading (revealSection).
import type { JSX, Ref } from 'preact';
import type { App } from '../app.ts';
import type { AppIcons, Application, AppsSettings, Install, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { AppArt, artStyleOf, PlusArt } from '../icons.tsx';
import { Vines } from '../vines.tsx';
import { NotesList, notesOf } from './notes.tsx';
import { type AppState, capabilityFor, isGuest, permissionsOf } from '../state.ts';

/**
 * Adapters that run in a browser instead of installing an app of their own
 * (the adapter table's web apps): the streaming sites run in
 * apps.streaming_browser, the Browser tile in apps.browser.
 */
const WEB_APPS: Readonly<Record<string, 'streaming' | 'browser'>> = { netflix: 'streaming', 'disney-plus': 'streaming', hulu: 'streaming', browser: 'browser' };

/**
 * @param adapter An application's adapter.
 * @param apps state.apps, or undefined (older coordinators).
 * @returns The browser id a web app installs (the same key for the same
 *   browser), or null for an app that installs itself.
 */
export function browserKey(adapter: string, apps: AppsSettings | undefined): string | null {
  const role = WEB_APPS[adapter];
  if (!role) return null;
  return (role === 'streaming' ? apps?.streaming_browser : apps?.browser) ?? '';
}

/**
 * @param id A browser id from browserKey.
 * @param apps state.apps.
 * @returns Its name from state.apps.browsers, else a plain "Web browser".
 */
function browserLabel(id: string, apps: AppsSettings | undefined): string {
  return apps?.browsers?.find((b) => b.id === id)?.label ?? t.install.browserFallback;
}

export interface InstallEntry {
  /** The config app id the actions name. */
  id: string;
  adapter: string;
  label: string;
  /** The first missing app's own label (Netflix for the streaming sites' browser row). */
  appLabel: string;
  why: string;
  install: Install;
  /** The app's notes; a browser row carries its first streaming site's (else its first app's). */
  notes: readonly string[];
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns One entry per missing Flatpak that carries install state, first app wins.
 */
export function installEntries(snapshot: StateSnapshot | null): InstallEntry[] {
  const apps = snapshot?.apps;
  const groups = new Map<string, { first: Application & { install: Install }; members: Application[] }>();
  for (const a of snapshot?.applications ?? ([] as Application[])) {
    if (a.installed || !a.install) continue;
    const browser = browserKey(a.adapter, apps);
    const key = browser === null ? `app:${a.id}` : `browser:${browser}`;
    const group = groups.get(key);
    if (group) group.members.push(a);
    else groups.set(key, { first: { ...a, install: a.install }, members: [a] });
  }
  return [...groups.entries()].map(([key, { first, members }]) => {
    const browser = key.startsWith('browser:') ? key.slice('browser:'.length) : null;
    const noted = members.find((m) => WEB_APPS[m.adapter] === 'streaming') ?? first;
    return {
      id: first.id,
      adapter: first.adapter,
      label: browser === null ? first.label : browserLabel(browser, apps),
      appLabel: first.label,
      why: browser === null ? '' : t.install.neededFor(members.map((m) => m.label)),
      install: first.install,
      notes: notesOf(noted),
    };
  });
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
  /** The heading, for the tile to scroll to and focus. */
  headingRef?: Ref<HTMLHeadingElement>;
}

/** The section itself: nothing when every app is installed. */
export function AddAppsSection({ entries, available, reason, art, icons, onInstall, onCancel, headingRef }: AddAppsSectionProps): JSX.Element | null {
  if (entries.length === 0) return null;
  return (
    <div class="group add-apps" aria-labelledby="add-apps-heading" data-testid="add-apps">
      <h3 id="add-apps-heading" tabIndex={-1} ref={headingRef}>
        {t.install.heading}
      </h3>
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
              <NotesList notes={e.notes} class="muted small install-notes" />
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
export function AddAppsPanel({ app, state, headingRef }: { app: App; state: AppState; headingRef?: Ref<HTMLHeadingElement> }): JSX.Element | null {
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
      headingRef={headingRef}
    />
  );
}

/**
 * @param state Application state.
 * @returns Whether the "+ Add apps" tile is drawn: the owner's phone (never a
 *   guest pass or a family phone), app.install listed, something left to add.
 */
export function addAppsTileShown(state: AppState): boolean {
  return mayInstall(state) && state.snapshot?.capabilities['app.install'] !== undefined && installEntries(state.snapshot).length > 0;
}

/**
 * @param entries installEntries(): what is not installed.
 * @returns The tile's subline from the first two apps: "Spotify, Netflix and
 *   more", "Spotify and Netflix", "Spotify"; empty with nothing to add.
 */
export function addAppsSubline(entries: readonly InstallEntry[]): string {
  const labels = entries.slice(0, 2).map((e) => e.appLabel);
  return labels.length === 0 ? '' : t.install.tileSub(labels, entries.length > 2);
}

/** What the tile needs of the section's heading (an HTMLElement on the page). */
export interface Revealable {
  scrollIntoView(options?: ScrollIntoViewOptions): void;
  focus(options?: FocusOptions): void;
}

/**
 * Scrolls the Add apps section into view and moves focus to its heading.
 * @param heading The heading, or null when the section is not drawn.
 */
export function revealSection(heading: Revealable | null): void {
  if (!heading) return;
  heading.scrollIntoView({ block: 'start' });
  heading.focus({ preventScroll: true });
}

/** The "+ Add apps" tile at the end of the Apps grid; owner phones only. */
export function AddAppsTile({ state, heading }: { state: AppState; heading: { current: Revealable | null } }): JSX.Element | null {
  if (!addAppsTileShown(state)) return null;
  const sub = addAppsSubline(installEntries(state.snapshot));
  return (
    <div class="vine-host app-tile-host">
      <button type="button" class="btn app-tile add-apps-tile" aria-label={t.install.tileLabel(sub)} data-testid="add-apps-tile" onClick={() => revealSection(heading.current)}>
        <span class="app-icon">
          <PlusArt art={artStyleOf(state.snapshot?.appearance)} />
        </span>
        <span class="app-label">{t.install.tile}</span>
        <span class="app-status small" data-testid="add-apps-tile-sub">
          {sub}
        </span>
      </button>
      <Vines appearance={state.snapshot?.appearance} />
    </div>
  );
}
