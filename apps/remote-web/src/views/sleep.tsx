// Sleep section of the remote: sleep timer chips (15–120 min), the time left,
// Cancel, and Screen off (`state.power`; contracts/actions.md
// power.sleep_timer and display.off, http.md "Sleep timer and screen off").
// Contract: sleep_at_ms is on the coordinator's clock, like generated_at_ms;
// the time left is sleep_at_ms − generated_at_ms − time since the snapshot
// arrived. Controls are gated by `snapshot.capabilities[action]` like every
// other control: unavailable renders disabled with the server's reason,
// unlisted is not rendered. The countdown re-renders once a second only
// while a timer is set and the page is visible.
import { useEffect, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { Power, SleepMinutes, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { Art, artStyleOf, Icon } from '../icons.tsx';
import { type AppState, capabilityFor } from '../state.ts';
import { formatClock } from './nowplaying.tsx';

/** The choices, in the order the TV offers them (contract.SleepChoices). */
export const SLEEP_CHOICES: readonly Exclude<SleepMinutes, 0>[] = [15, 30, 45, 60, 90, 120];

/** How often a running countdown re-renders. */
export const SLEEP_TICK_MS = 1000;

/**
 * @param snapshot Latest snapshot or null.
 * @returns state.power, or null when absent (older coordinator, not yet loaded).
 */
export function powerOf(snapshot: StateSnapshot | null): Power | null {
  return snapshot?.power ?? null;
}

/**
 * @param power The power state.
 * @param generatedAtMs The snapshot's generated_at_ms (coordinator clock, like sleep_at_ms).
 * @param sinceArrivalMs Milliseconds since that snapshot arrived, on the phone's clock.
 * @returns Milliseconds until the timer fires (never negative), or null with no timer.
 */
export function remainingMs(power: Power | null, generatedAtMs: number, sinceArrivalMs: number): number | null {
  if (!power || power.sleep_at_ms === null) return null;
  return Math.max(0, power.sleep_at_ms - generatedAtMs - Math.max(0, sinceArrivalMs));
}

/**
 * @param power The power state, or null.
 * @param hidden The page is hidden.
 * @returns Whether the countdown needs a clock.
 */
export function shouldTickSleep(power: Power | null, hidden: boolean): boolean {
  return power !== null && power.sleep_at_ms !== null && !hidden;
}

export interface SleepSectionProps {
  power: Power | null;
  /** Time left, or null with no timer. */
  remaining: number | null;
  sleepGate: { listed: boolean; disabled: boolean; reason: string | null };
  offGate: { listed: boolean; disabled: boolean; reason: string | null };
  classic: boolean;
  onSleep: (minutes: SleepMinutes) => void;
  onScreenOff: () => void;
}

/** The section itself: a pure function of the power state and the gates. */
export function SleepSection({ power, remaining, sleepGate, offGate, classic, onSleep, onScreenOff }: SleepSectionProps): JSX.Element | null {
  if (!sleepGate.listed && !offGate.listed) return null;
  const running = power !== null && power.sleep_at_ms !== null;
  const status =
    power?.display === 'off'
      ? t.sleep.screenIsOff
      : running && remaining !== null
        ? power?.warning
          ? t.sleep.warning(formatClock(remaining))
          : t.sleep.sleepingIn(formatClock(remaining))
        : t.sleep.noTimer;
  return (
    <div class={`group sleep ${running ? 'is-running' : ''} ${power?.warning ? 'is-warning' : ''}`} aria-labelledby="sleep-heading" data-testid="sleep">
      <h3 id="sleep-heading">{t.sleep.heading}</h3>
      <div class="sleep-status" role="status" aria-live="polite" data-testid="sleep-status">
        {running || power?.display === 'off' ? <Art name="bear-sleep" class="sleep-bear" scale={2} width={42} art={classic ? 'classic' : 'pixel'} /> : <Icon name="moon" size={22} />}
        <span>{status}</span>
      </div>
      {sleepGate.listed ? (
        <div class="sleep-chips" role="group" aria-label={t.sleep.choicesLabel}>
          {SLEEP_CHOICES.map((m) => (
            <button
              type="button"
              key={m}
              class={`btn btn-small sleep-chip ${power?.sleep_minutes === m ? 'is-chosen' : ''}`}
              aria-pressed={power?.sleep_minutes === m}
              aria-label={t.sleep.chipLabel(m)}
              title={sleepGate.reason ?? undefined}
              disabled={sleepGate.disabled}
              data-action="power.sleep_timer"
              data-minutes={m}
              onClick={() => onSleep(m)}
            >
              {t.sleep.chip(m)}
            </button>
          ))}
        </div>
      ) : null}
      <div class="button-row">
        {sleepGate.listed && running ? (
          <button type="button" class="btn btn-control btn-secondary" disabled={sleepGate.disabled} title={sleepGate.reason ?? undefined} data-action="power.sleep_timer" data-minutes={0} data-testid="sleep-cancel" onClick={() => onSleep(0)}>
            <Icon name="close" size={20} />
            <span>{t.sleep.cancel}</span>
          </button>
        ) : null}
        {offGate.listed ? (
          <button type="button" class="btn btn-control" disabled={offGate.disabled || power?.display === 'off'} title={offGate.reason ?? undefined} data-action="display.off" data-testid="screen-off" onClick={onScreenOff}>
            <Icon name="screen-off" size={22} />
            <span>{t.sleep.screenOff}</span>
          </button>
        ) : null}
      </div>
    </div>
  );
}

function gateOf(state: AppState, action: 'power.sleep_timer' | 'display.off') {
  const listed = state.snapshot?.capabilities[action] !== undefined;
  const capability = capabilityFor(state.snapshot, action);
  return { listed, disabled: !capability.available, reason: capability.available ? null : capability.reason || t.remote.capabilityUnknown };
}

/** The section wired to the store and the controller, with its one-second tick. */
export function SleepPanel({ app, state, now = Date.now }: { app: App; state: AppState; now?: () => number }): JSX.Element | null {
  const snapshot = state.snapshot;
  const power = powerOf(snapshot);
  const ticking = shouldTickSleep(power, state.hidden);
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!ticking) return undefined;
    const timer = setInterval(() => setTick((n) => n + 1), SLEEP_TICK_MS);
    return () => clearInterval(timer);
  }, [ticking]);
  if (!snapshot) return null;
  return (
    <SleepSection
      power={power}
      remaining={remainingMs(power, snapshot.generated_at_ms, now() - state.snapshotAt)}
      sleepGate={gateOf(state, 'power.sleep_timer')}
      offGate={gateOf(state, 'display.off')}
      classic={artStyleOf(snapshot.appearance) === 'classic'}
      onSleep={(minutes) => void app.tap('power.sleep_timer', { minutes })}
      onScreenOff={() => void app.tap('display.off', {})}
    />
  );
}
