// Now playing card: what the foreground app's own player reports
// (`state.now_playing`, contracts/http.md "Now playing"): the app, the title
// (ellipsized), an optional subtitle, play/pause state and a progress bar.
// Contract: the coordinator sends a reading, not a stream; the position is
// extrapolated here from position_ms + rate × (generated_at_ms − position_at +
// time since the snapshot arrived), only while playing. The card re-renders
// once a second, and only while it is shown, playing and the page is visible.
// Absent (or null) now_playing renders nothing: locked, switched off by the
// owner, no controller permission, or nothing known.
import { useEffect, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { NowPlaying, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { Icon } from '../icons.tsx';
import type { AppState } from '../state.ts';

/** How often a playing card re-renders its position. */
export const TICK_MS = 1000;

/**
 * @param snapshot Latest snapshot or null.
 * @returns The reading to show, or null when there is none.
 */
export function nowPlayingOf(snapshot: StateSnapshot | null): NowPlaying | null {
  return snapshot?.now_playing ?? null;
}

/**
 * @param np The reading.
 * @param generatedAtMs The snapshot's generated_at_ms (coordinator clock, like position_at).
 * @param sinceArrivalMs Milliseconds since that snapshot arrived, on the phone's clock.
 * @returns The position to draw in milliseconds, or null when the player reports none.
 */
export function extrapolatePosition(np: NowPlaying, generatedAtMs: number, sinceArrivalMs: number): number | null {
  if (np.position_ms === undefined) return null;
  let position = np.position_ms;
  if (np.status === 'playing') position += np.rate * Math.max(0, generatedAtMs - np.position_at + Math.max(0, sinceArrivalMs));
  if (np.length_ms !== undefined) position = Math.min(position, np.length_ms);
  return Math.max(0, Math.floor(position));
}

/**
 * @param np The reading, or null.
 * @param hidden The page is hidden.
 * @returns Whether the card needs a clock: only a visible, playing card with a position.
 */
export function shouldTick(np: NowPlaying | null, hidden: boolean): boolean {
  return np !== null && !hidden && np.status === 'playing' && np.position_ms !== undefined && np.rate > 0;
}

/**
 * @param ms Milliseconds (non-negative).
 * @returns `m:ss`, or `h:mm:ss` from an hour up.
 */
export function formatClock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`;
}

export interface NowPlayingCardProps {
  np: NowPlaying | null;
  /** The app's display label (applications[].label), or the target label. */
  appLabel: string;
  /** Extrapolated position, or null when unknown. */
  positionMs: number | null;
}

/** The card itself: a pure function of the reading and the position to draw. */
export function NowPlayingCard({ np, appLabel, positionMs }: NowPlayingCardProps): JSX.Element | null {
  if (!np) return null;
  const length = np.length_ms;
  const progress = positionMs !== null && length !== undefined && length > 0;
  return (
    <div class={`now-playing np-${np.status}`} data-testid="now-playing" data-status={np.status} aria-label={t.remote.nowPlayingIn(appLabel)} role="group">
      <div class="np-head">
        <span class="np-caption small">{t.remote.nowPlaying}</span>
        <span class="np-app chip">{appLabel}</span>
        <span class="np-status small" data-testid="now-playing-status">
          <Icon name={np.status === 'playing' ? 'play' : 'pause'} size={14} />
          {t.remote.nowPlayingStatus(np.status)}
        </span>
      </div>
      <p class="np-title" title={np.title} data-testid="now-playing-title">
        {np.title}
      </p>
      {np.subtitle ? (
        <p class="np-subtitle small" title={np.subtitle}>
          {np.subtitle}
        </p>
      ) : null}
      {progress ? (
        <>
          <progress
            class="np-bar"
            max={length}
            value={positionMs}
            aria-label={t.remote.nowPlayingProgress(formatClock(positionMs), formatClock(length))}
            data-testid="now-playing-bar"
          />
          <div class="np-times small" aria-hidden="true">
            <span data-testid="now-playing-position">{formatClock(positionMs)}</span>
            <span>{formatClock(length)}</span>
          </div>
        </>
      ) : positionMs !== null ? (
        <div class="np-times small">
          <span data-testid="now-playing-position">{formatClock(positionMs)}</span>
        </div>
      ) : null}
    </div>
  );
}

/**
 * @param snapshot Latest snapshot.
 * @param np The reading.
 * @returns The label of the app the reading belongs to.
 */
export function appLabelFor(snapshot: StateSnapshot, np: NowPlaying): string {
  return snapshot.applications.find((a) => a.id === np.app_id)?.label ?? snapshot.target.label;
}

/** The card wired to the store: a one-second tick while it needs one. */
export function NowPlayingPanel({ state, now = Date.now }: { state: AppState; now?: () => number }): JSX.Element | null {
  const snapshot = state.snapshot;
  const np = nowPlayingOf(snapshot);
  const ticking = shouldTick(np, state.hidden);
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!ticking) return undefined;
    const timer = setInterval(() => setTick((n) => n + 1), TICK_MS);
    return () => clearInterval(timer);
  }, [ticking]);
  if (!snapshot || !np) return null;
  const position = extrapolatePosition(np, snapshot.generated_at_ms, now() - state.snapshotAt);
  return <NowPlayingCard np={np} appLabel={appLabelFor(snapshot, np)} positionMs={position} />;
}
