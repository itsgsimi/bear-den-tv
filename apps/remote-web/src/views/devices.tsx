// Devices screen (owner only): paired phones with remove one/all behind inline
// confirmation. Contract: on the plain-HTTP remote the list is shown read-only
// with where to manage phones instead (the TV's Paired phones; UX-13: it used
// to be a dead end); confirmation is in-page (no window.confirm) so it stays
// touch friendly and testable.
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { Device } from '../contract.ts';
import { t } from '../i18n.ts';
import { type AppState, isSecureTransport } from '../state.ts';

type Confirm = { kind: 'one'; id: string; name: string } | { kind: 'all' } | null;

export function DevicesView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const [confirm, setConfirm] = useState<Confirm>(null);
  const secure = isSecureTransport(state);
  const self = state.session?.device_id ?? state.snapshot?.me?.device_id ?? null;
  const devices = state.devices;

  return (
    <section class="page devices">
      <h2>{t.devices.heading}</h2>
      <p class="muted">{t.devices.intro}</p>
      {!secure ? (
        <div class="notice notice-info" data-testid="devices-http-notice">
          {t.devices.readOnly}
        </div>
      ) : null}
      {devices.status === 'loading' || devices.status === 'idle' ? <p class="muted">{t.devices.loading}</p> : null}
      {devices.status === 'error' ? (
        <div class="notice notice-error" role="alert">
          {devices.error}
        </div>
      ) : null}
      {devices.status === 'ready' && devices.devices.length === 0 ? <p class="muted">{t.devices.empty}</p> : null}
      <DeviceRows devices={devices.devices} self={self} manageable={secure} busy={devices.busy} confirming={confirm && confirm.kind === 'one' ? confirm.id : null} onAsk={(d) => setConfirm(d ? { kind: 'one', id: d.id, name: d.name } : null)} onRemove={(id) => { setConfirm(null); void app.revokeDevice(id); }} />
      {secure && devices.status === 'ready' && devices.devices.length > 0 ? (
        confirm && confirm.kind === 'all' ? (
          <div class="confirm" role="alertdialog" aria-label={t.devices.confirmRevokeAll}>
            <p>{t.devices.confirmRevokeAll}</p>
            <div class="actions-row">
              <button
                type="button"
                class="btn btn-danger"
                data-testid="confirm-revoke-all"
                disabled={devices.busy}
                onClick={() => {
                  setConfirm(null);
                  void app.revokeAllDevices();
                }}
              >
                {t.devices.confirm}
              </button>
              <button type="button" class="btn btn-secondary" onClick={() => setConfirm(null)}>
                {t.devices.cancel}
              </button>
            </div>
          </div>
        ) : (
          <div class="actions-row">
            <button type="button" class="btn btn-danger" data-testid="revoke-all" disabled={devices.busy} onClick={() => setConfirm({ kind: 'all' })}>
              {t.devices.revokeAll}
            </button>
          </div>
        )
      ) : null}
    </section>
  );
}

interface DeviceRowsProps {
  devices: Device[];
  self: string | null;
  /** Remove buttons only where the phone may manage phones (HTTPS). */
  manageable: boolean;
  busy: boolean;
  confirming: string | null;
  onAsk: (device: Device | null) => void;
  onRemove: (id: string) => void;
}

/** The list itself; pure over its props for tests. */
export function DeviceRows({ devices, self, manageable, busy, confirming, onAsk, onRemove }: DeviceRowsProps): JSX.Element {
  return (
    <ul class="device-list">
      {devices.map((device) => (
        <li key={device.id} class="device" data-testid="device-row">
          <div class="device-main">
            <span class="device-name">
              {device.name}
              {device.id === self ? <span class="muted"> · {t.devices.you}</span> : null}
            </span>
            <span class={`device-conn ${device.connected ? 'is-connected' : ''}`}>{device.connected ? t.devices.connected : t.devices.disconnected}</span>
            <span class="chips">
              {device.permissions.map((p) => (
                <span key={p} class="chip">
                  {t.devices.permission(p)}
                </span>
              ))}
            </span>
          </div>
          {!manageable ? null : confirming === device.id ? (
            <div class="confirm" role="alertdialog" aria-label={t.devices.confirmRevoke(device.name)}>
              <p>{t.devices.confirmRevoke(device.name)}</p>
              <div class="actions-row">
                <button type="button" class="btn btn-danger" data-testid="confirm-revoke" disabled={busy} onClick={() => onRemove(device.id)}>
                  {t.devices.confirm}
                </button>
                <button type="button" class="btn btn-secondary" onClick={() => onAsk(null)}>
                  {t.devices.cancel}
                </button>
              </div>
            </div>
          ) : (
            <button type="button" class="btn btn-secondary btn-small" data-testid="revoke-device" disabled={busy} onClick={() => onAsk(device)}>
              {t.devices.revoke}
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}
