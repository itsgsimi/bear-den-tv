// About screen: transport honesty text, this phone's name and what it may do,
// the phone id and remote version under Details (UX-15), and Log out, which
// asks first with Cancel (UX-19). Contract: transport copy comes from i18n
// keyed on the freshest transport fact (snapshot, then session, then info).
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import { PROTOCOL } from '../contract.ts';
import { t } from '../i18n.ts';
import { type AppState, isSecureTransport, permissionsOf } from '../state.ts';

export function AboutView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const transport = state.snapshot?.remote.transport ?? state.info?.transport ?? 'local-only';
  const secure = isSecureTransport(state);
  const me = state.snapshot?.me;
  const tvName = state.snapshot?.device_name || state.info?.device_name || t.productName;
  const [asking, setAsking] = useState(false);
  return (
    <section class="page about">
      <h2>{t.about.heading}</h2>
      <dl class="facts">
        <dt>{t.about.tv}</dt>
        <dd>{tvName}</dd>
        <dt>{t.about.transport}</dt>
        <dd data-testid="about-transport">{t.about.transportText(transport, secure)}</dd>
        <dt>{t.about.deviceName}</dt>
        <dd>{me?.device_name ?? state.session?.device_name ?? ''}</dd>
        <dt>{t.about.permissions}</dt>
        <dd>
          <span class="chips">
            {permissionsOf(state).map((p) => (
              <span key={p} class="chip">
                {t.devices.permission(p)}
              </span>
            ))}
          </span>
        </dd>
      </dl>
      <details class="about-details" data-testid="about-details">
        <summary>{t.about.details}</summary>
        <dl class="facts">
          <dt>{t.about.deviceId}</dt>
          <dd class="mono">{me?.device_id ?? state.session?.device_id ?? ''}</dd>
          <dt>{t.about.protocol}</dt>
          <dd>{PROTOCOL}</dd>
        </dl>
      </details>
      {!secure ? <p class="muted">{t.about.noPwa}</p> : null}
      <LogoutControl asking={asking} onAsk={setAsking} onLogout={() => void app.logout()} />
      <p class="muted small">{t.about.logoutHint}</p>
    </section>
  );
}

/** Log out, asking first (Cancel first); pure over its props for tests. */
export function LogoutControl({ asking, onAsk, onLogout }: { asking: boolean; onAsk: (on: boolean) => void; onLogout: () => void }): JSX.Element {
  if (!asking) {
    return (
      <div class="actions-row">
        <button type="button" class="btn btn-danger" data-testid="logout" onClick={() => onAsk(true)}>
          {t.about.logout}
        </button>
      </div>
    );
  }
  return (
    <div class="confirm" role="alertdialog" aria-label={t.about.logoutAsk} data-testid="logout-confirm">
      <p>{t.about.logoutAsk}</p>
      <div class="actions-row">
        <button type="button" class="btn btn-secondary" data-testid="logout-cancel" onClick={() => onAsk(false)}>
          {t.about.logoutCancel}
        </button>
        <button type="button" class="btn btn-danger" data-testid="logout-yes" onClick={onLogout}>
          {t.about.logoutYes}
        </button>
      </div>
    </div>
  );
}
