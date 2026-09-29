// TV section of the remote: turn the TV on (and to Bear Den's input) or to
// standby over HDMI-CEC (`tv.power`, `state.cec`; contracts/http.md "TV
// control over HDMI-CEC"), and the volume group's heading, which says which
// volume the buttons change (`state.cec.volume_target`). Contract: unlike
// the other controls, the TV buttons are not drawn at all unless
// `capabilities["tv.power"]` is available: most TV boxes have no HDMI-CEC
// adapter, and a disabled row with that reason on every phone would be noise.
// The owner turns HDMI-CEC on in TV Settings.
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { Cec, StateSnapshot } from '../contract.ts';
import { t } from '../i18n.ts';
import { Icon } from '../icons.tsx';
import { type AppState, capabilityFor } from '../state.ts';

/**
 * @param snapshot Latest snapshot or null.
 * @returns state.cec, or null when absent (older coordinator, not yet loaded).
 */
export function cecOf(snapshot: StateSnapshot | null): Cec | null {
  return snapshot?.cec ?? null;
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns Whether the volume buttons drive the TV: HDMI-CEC on and the TV chosen.
 */
export function volumeDrivesTV(snapshot: StateSnapshot | null): boolean {
  const cec = cecOf(snapshot);
  return cec !== null && cec.enabled && cec.volume_target === 'tv';
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns The volume group's heading: "TV volume" or "PC volume".
 */
export function volumeHeading(snapshot: StateSnapshot | null): string {
  return volumeDrivesTV(snapshot) ? t.remote.tvVolume : t.remote.pcVolume;
}

export interface TvSectionProps {
  /** tv.power is available (HDMI-CEC adapter present and turned on). */
  available: boolean;
  cec: Cec | null;
  onPower: (power: 'on' | 'standby') => void;
}

/** The section itself: nothing unless tv.power is available. */
export function TvSection({ available, cec, onPower }: TvSectionProps): JSX.Element | null {
  if (!available) return null;
  const status = cec?.tv_power === 'on' ? t.tv.isOn : cec?.tv_power === 'standby' ? t.tv.isStandby : t.tv.isUnknown;
  return (
    <div class="group tv" aria-labelledby="tv-heading" data-testid="tv">
      <h3 id="tv-heading">{t.tv.heading}</h3>
      <p class="muted small tv-status" role="status" data-testid="tv-status">
        {status}
      </p>
      <div class="button-row">
        <button type="button" class="btn btn-control" data-action="tv.power" data-power="on" data-testid="tv-on" onClick={() => onPower('on')}>
          <Icon name="power" size={22} />
          <span>{t.tv.on}</span>
        </button>
        <button type="button" class="btn btn-control btn-secondary" data-action="tv.power" data-power="standby" data-testid="tv-standby" onClick={() => onPower('standby')}>
          <Icon name="screen-off" size={22} />
          <span>{t.tv.standby}</span>
        </button>
      </div>
    </div>
  );
}

/** The section wired to the store and the controller. */
export function TvPanel({ app, state }: { app: App; state: AppState }): JSX.Element | null {
  return (
    <TvSection
      available={capabilityFor(state.snapshot, 'tv.power').available}
      cec={cecOf(state.snapshot)}
      onPower={(power) => void app.tap('tv.power', { power })}
    />
  );
}
