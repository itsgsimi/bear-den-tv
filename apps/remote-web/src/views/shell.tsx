// Application frame: header with TV name and connection dot, the target bar,
// tab routing, toast, the tab bar, and the guest pass chip. Contract: `Root` subscribes to the store
// and re-renders the whole tree from `AppState`; child views are pure functions
// of state plus the controller. Copy comes from i18n; no view writes literals.
import { useEffect, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import { passEndLabel, t } from '../i18n.ts';
import { type AppState, type Tab, guestEndsAt, subscribeNow, tvLost, visibleTabs } from '../state.ts';
import { Art, artStyleOf, Icon, type IconName } from '../icons.tsx';
import type { ArtStyle } from '../contract.ts';
import { PairView } from './pair.tsx';
import { RemoteView } from './remote.tsx';
import { EditorView } from './editor.tsx';
import { DevicesView } from './devices.tsx';
import { AboutView } from './about.tsx';
import { BadgesView } from './badges.tsx';

const TAB_ICONS: Record<Tab, IconName> = { remote: 'remote', editor: 'layout', devices: 'devices', badges: 'badge', about: 'about' };
/** How long the decorative "paired" moment stays over the freshly shown remote. */
const CELEBRATE_MS = 1700;
const FIREFLIES = 7;
const BURST = ['sparkle', 'heart', 'sparkle', 'heart', 'sparkle', 'heart', 'sparkle', 'heart'] as const;

/**
 * @param props The controller whose store drives the tree.
 * @returns The root element.
 */
export function Root({ app }: { app: App }): JSX.Element {
  const [state, setState] = useState<AppState>(app.store.getState());
  useEffect(() => subscribeNow(app.store, setState), [app]);
  const celebrating = useCelebration(state);

  let screen: JSX.Element;
  if (state.screen === 'loading') {
    screen = (
      <div class="splash" role="status">
        <Art name="bear-mark" class="splash-bear" scale={4} width={72} art={artStyleOf(state.snapshot?.appearance)} />
        <p>{t.pair.loadingInfo}</p>
      </div>
    );
  } else if (state.screen === 'pair') {
    screen = <PairView app={app} state={state} />;
  } else {
    screen = <Frame app={app} state={state} />;
  }
  const tvName = state.snapshot?.device_name || state.info?.device_name || t.productName;
  return (
    <>
      <Ambient />
      {screen}
      {celebrating ? <Celebration tvName={tvName} art={artStyleOf(state.snapshot?.appearance)} /> : null}
    </>
  );
}

/**
 * @param state Current store state.
 * @returns True for a short moment right after the pair screen gives way to the
 *   remote (a fresh pairing), never on a reload that resumes a stored session.
 */
function useCelebration(state: AppState): boolean {
  const previous = useRef(state.screen);
  const [on, setOn] = useState(false);
  useEffect(() => {
    const was = previous.current;
    previous.current = state.screen;
    if (was === 'pair' && state.screen === 'app') setOn(true);
  }, [state.screen]);
  useEffect(() => {
    if (!on) return undefined;
    const id = setTimeout(() => setOn(false), CELEBRATE_MS);
    return () => clearTimeout(id);
  }, [on]);
  return on;
}

/** Fixed den backdrop with a few drifting fireflies; purely decorative. */
function Ambient(): JSX.Element {
  return (
    <div class="ambient" aria-hidden="true">
      <div class="ambient-backdrop" />
      {Array.from({ length: FIREFLIES }, (_, i) => (
        <span key={i} class={`firefly firefly-${i + 1}`} />
      ))}
    </div>
  );
}

/** Short decorative burst shown once after pairing; the status region announces the connection. */
function Celebration({ tvName, art }: { tvName: string; art: ArtStyle }): JSX.Element {
  return (
    <div class="celebrate" aria-hidden="true" data-testid="celebrate">
      <div class="celebrate-center">
        <div class="celebrate-burst">
          {BURST.map((name, i) => (
            <span key={i} class={`burst-ray burst-ray-${i + 1}`}>
              <Art name={name} class="burst-piece" scale={3} width={name === 'heart' ? 22 : 26} art={art} />
            </span>
          ))}
        </div>
        <Art name="bear-cub" class="celebrate-cub" scale={6} width={120} art={art} />
        <p class="celebrate-text">{t.pair.paired(tvName)}</p>
      </div>
    </div>
  );
}

/** "Guest · ends 04:00" for a phone on a guest pass; nothing otherwise. */
export function GuestChip({ state }: { state: AppState }): JSX.Element | null {
  const ends = guestEndsAt(state);
  if (ends === null) return null;
  // snapshotAt is the phone's clock when the snapshot arrived: close enough
  // for a time of day, and keeps this view a pure function of the state.
  const label = passEndLabel(ends, state.snapshotAt || ends);
  return (
    <div class="guest-strip">
      <span class="guest-chip" data-testid="guest-chip" title={t.guest.chipLabel(label)}>
        {t.guest.chip(label)}
      </span>
    </div>
  );
}

function Frame({ app, state }: { app: App; state: AppState }): JSX.Element {
  const tabs = visibleTabs(state);
  const tvName = state.snapshot?.device_name || state.info?.device_name || t.productName;
  return (
    <div class={`app tab-${state.tab}`}>
      <header class="header">
        <div class="header-brand">
          <span class={`header-bear bear-${state.connection === 'online' ? 'awake' : 'asleep'}`} aria-hidden="true">
            {/* Both loaded while online, so the sleeping bear is already
                there when the TV goes away (it used to be a broken image). */}
            <Art name="bear-mark" class="header-bear-img bear-when-awake" scale={2} width={40} art={artStyleOf(state.snapshot?.appearance)} />
            <Art name="bear-sleep" class="header-bear-img bear-when-asleep" scale={2} width={40} art={artStyleOf(state.snapshot?.appearance)} />
            {state.connection === 'online' ? null : <span class="bear-zz" />}
          </span>
          <div class="header-titles">
            <span class="header-tv">{tvName}</span>
            <span class="header-sub">
              {t.productName}
              {state.snapshot?.dev_mode ? <span class="demo-badge" data-testid="demo-badge"> {t.remote.demo}</span> : null}
            </span>
          </div>
        </div>
        <div class={`status status-${state.connection}`} role="status" aria-live="polite">
          <span class="status-dot" aria-hidden="true" />
          <span class="status-text">{t.status[state.connection]}</span>
        </div>
      </header>
      <GuestChip state={state} />
      <LostBanner state={state} />
      {state.tab === 'remote' ? <TargetBar state={state} /> : null}
      <main class={`content ${tvLost(state) ? 'content-lost' : ''}`} id="main" inert={tvLost(state) ? true : undefined}>
        {state.tab === 'remote' ? <RemoteView app={app} state={state} /> : null}
        {state.tab === 'editor' ? <EditorView app={app} state={state} /> : null}
        {state.tab === 'devices' ? <DevicesView app={app} state={state} /> : null}
        {state.tab === 'badges' ? <BadgesView state={state} /> : null}
        {state.tab === 'about' ? <AboutView app={app} state={state} /> : null}
      </main>
      <nav class="tabbar" aria-label="Sections">
        {tabs.map((tab) => (
          <button
            key={tab}
            type="button"
            class={`tab ${state.tab === tab ? 'tab-active' : ''}`}
            aria-current={state.tab === tab ? 'page' : undefined}
            data-tab={tab}
            onClick={() => app.selectTab(tab)}
          >
            <span class="tab-icon">
              <Icon name={TAB_ICONS[tab]} size={22} />
            </span>
            <span>{t.tabs[tab]}</span>
          </button>
        ))}
      </nav>
      {state.toast ? (
        <div class={`toast toast-${state.toast.kind}`} role="status" aria-live="assertive" data-testid="toast">
          <span class="toast-text">{state.toast.text}</span>
          <button type="button" class="toast-close" aria-label={t.toast.dismiss} onClick={() => app.dismissToast()}>
            <Icon name="close" size={18} />
          </button>
        </div>
      ) : null}
    </div>
  );
}

function TargetBar({ state }: { state: AppState }): JSX.Element {
  const target = state.snapshot?.target;
  const kind = target?.kind ?? 'none';
  const label = target ? t.remote.targetLabel(target.kind, target.label) : t.status.connecting;
  const unverified = target ? !target.observed : false;
  return (
    <div class={`target-bar target-${kind} ${unverified ? 'target-unverified' : ''}`} role="status" aria-live="polite" data-testid="target-bar">
      <span class="target-caption">{t.remote.controlling}</span>
      <span class="target-label">
        {label}
        {unverified ? <span class="target-unverified-tag"> {t.remote.unverified}</span> : null}
      </span>
    </div>
  );
}

/**
 * The TV went away (UX-12): a banner says so and since when, and the
 * controls below are dimmed and inert until the connection is back.
 */
export function LostBanner({ state }: { state: AppState }): JSX.Element | null {
  if (!tvLost(state)) return null;
  const since = state.lostAt !== null ? new Date(state.lostAt).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : null;
  return (
    <div class="notice notice-warning lost-banner" role="status" data-testid="lost-banner">
      {since ? t.status.lostSince(since) : t.status.lost}
    </div>
  );
}
