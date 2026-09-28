// About screen: transport honesty text, this phone's identity and permissions,
// protocol, and logout. Contract: transport copy comes from i18n keyed on the
// freshest transport fact (snapshot, then session, then info).
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
        <dt>{t.about.deviceId}</dt>
        <dd class="mono">{me?.device_id ?? state.session?.device_id ?? ''}</dd>
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
        <dt>{t.about.protocol}</dt>
        <dd>{PROTOCOL}</dd>
      </dl>
      {!secure ? <p class="muted">{t.about.noPwa}</p> : null}
      <div class="actions-row">
        <button type="button" class="btn btn-danger" data-testid="logout" onClick={() => void app.logout()}>
          {t.about.logout}
        </button>
      </div>
      <p class="muted small">{t.about.logoutHint}</p>
    </section>
  );
}
