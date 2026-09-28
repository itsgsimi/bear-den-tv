// Devices screen (owner only): paired phones with revoke one/all behind inline
// confirmation. Contract: writes are hidden on trusted-LAN HTTP with an
// explanation; confirmation is in-page (no window.confirm) so it stays touch
// friendly and testable.
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import { t } from '../i18n.ts';
import { type AppState, isSecureTransport } from '../state.ts';

type Confirm = { kind: 'one'; id: string; name: string } | { kind: 'all' } | null;

export function DevicesView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const [confirm, setConfirm] = useState<Confirm>(null);
  const secure = isSecureTransport(state);
  const self = state.session?.device_id ?? state.snapshot?.me?.device_id ?? null;
  const devices = state.devices;

  if (!secure) {
    return (
      <section class="page devices">
        <h2>{t.devices.heading}</h2>
        <div class="notice notice-warning" data-testid="devices-http-notice">
          {t.devices.hiddenOnHttp}
        </div>
      </section>
    );
  }

  return (
    <section class="page devices">
      <h2>{t.devices.heading}</h2>
      <p class="muted">{t.devices.intro}</p>
      {devices.status === 'loading' || devices.status === 'idle' ? <p class="muted">{t.devices.loading}</p> : null}
      {devices.status === 'error' ? (
        <div class="notice notice-error" role="alert">
          {devices.error}
        </div>
      ) : null}
      {devices.status === 'ready' && devices.devices.length === 0 ? <p class="muted">{t.devices.empty}</p> : null}
      <ul class="device-list">
        {devices.devices.map((device) => (
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
            {confirm && confirm.kind === 'one' && confirm.id === device.id ? (
              <div class="confirm" role="alertdialog" aria-label={t.devices.confirmRevoke(device.name)}>
                <p>{t.devices.confirmRevoke(device.name)}</p>
                <div class="actions-row">
                  <button
                    type="button"
                    class="btn btn-danger"
                    data-testid="confirm-revoke"
                    disabled={devices.busy}
                    onClick={() => {
                      setConfirm(null);
                      void app.revokeDevice(device.id);
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
              <button
                type="button"
                class="btn btn-secondary btn-small"
                data-testid="revoke-device"
                disabled={devices.busy}
                onClick={() => setConfirm({ kind: 'one', id: device.id, name: device.name })}
              >
                {t.devices.revoke}
              </button>
            )}
          </li>
        ))}
      </ul>
      {devices.status === 'ready' && devices.devices.length > 0 ? (
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
