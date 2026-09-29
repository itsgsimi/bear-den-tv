// Pair screen: six-digit code entry plus device name, or the automatic redemption
// state while a QR fragment invitation is claimed. Contract: the code field only
// accepts digits, the submit is disabled until six digits and a name exist, and
// server failures (401/410/429/network) render the i18n message for their code.
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import { t } from '../i18n.ts';
import type { AppState } from '../state.ts';
import { Art, artStyleOf } from '../icons.tsx';
import { Vines } from '../vines.tsx';

export function PairView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const [code, setCode] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);
  const tvName = state.info?.device_name || t.productName;
  const secure = state.info?.https ?? false;
  const name = state.pair.device_name;
  const canSubmit = code.length === 6 && name.trim().length > 0 && !state.pair.busy;

  const submit = (ev: Event) => {
    ev.preventDefault();
    if (code.length !== 6) {
      setLocalError(t.pair.needSixDigits);
      return;
    }
    if (name.trim().length === 0) {
      setLocalError(t.pair.needName);
      return;
    }
    setLocalError(null);
    void app.pairWithCode(code);
  };

  const notice =
    state.pair.notice === 'revoked' ? t.pair.revoked : state.pair.notice === 'logout' ? t.pair.loggedOut : state.pair.notice === 'unauthenticated' ? t.pair.sessionExpired : null;
  const passEnded = state.pair.notice === 'pass_ended';

  const art = artStyleOf(state.snapshot?.appearance);

  return (
    <div class="pair">
      <div class="pair-card vine-host is-grown">
        <Vines art={art} />
        <div class="pair-brand">
          <div class="den-scene" aria-hidden="true">
            <Art name="den" class="den-mound" scale={16} width={200} height={150} art={art} />
            <span class="den-opening">
              <Art name="bear-cub" class="den-cub" scale={4} width={96} art={art} />
            </span>
            <Art name="sparkle" class="den-glint den-glint-1" scale={2} width={16} art={art} />
            <Art name="sparkle" class="den-glint den-glint-2" scale={2} width={11} art={art} />
          </div>
          <h1>{t.pair.heading(tvName)}</h1>
          <Art name="divider" class="ornament-divider" scale={8} width={180} height={18} art={art} />
          <p class="muted">{t.pair.intro}</p>
        </div>
        {state.pair.infoError ? (
          <div class="notice notice-error" role="alert">
            <p>{state.pair.infoError}</p>
            <button type="button" class="btn btn-secondary" onClick={() => void app.retryInfo()}>
              {t.pair.connect}
            </button>
          </div>
        ) : null}
        {notice ? (
          <div class="notice notice-warning" role="status" data-testid="pair-notice">
            {notice}
          </div>
        ) : null}
        {passEnded ? (
          <div class="pass-ended" role="status" data-testid="pass-ended">
            <Art name="bear-sleep" class="pass-ended-bear" scale={3} width={60} art={art} />
            <h2>{t.pair.passEnded}</h2>
            <p class="muted">{t.pair.passEndedBody}</p>
          </div>
        ) : null}
        {state.pair.redeeming ? (
          <div class="notice notice-info" role="status" data-testid="pair-redeeming">
            {t.pair.redeeming}
          </div>
        ) : null}
        <form class="pair-form" onSubmit={submit} noValidate>
          <label class="field">
            <span class="field-label">{t.pair.codeLabel}</span>
            <input
              class="code-input"
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              autoComplete="one-time-code"
              maxLength={6}
              placeholder={t.pair.codePlaceholder}
              value={code}
              disabled={state.pair.busy}
              aria-label={t.pair.codeLabel}
              onInput={(ev) => setCode(ev.currentTarget.value.replace(/\D/g, '').slice(0, 6))}
            />
          </label>
          <label class="field">
            <span class="field-label">{t.pair.deviceNameLabel}</span>
            <input
              class="text-input"
              type="text"
              maxLength={64}
              autoComplete="off"
              placeholder={t.pair.deviceNamePlaceholder}
              value={name}
              disabled={state.pair.busy}
              aria-label={t.pair.deviceNameLabel}
              onInput={(ev) => app.setDeviceName(ev.currentTarget.value)}
            />
          </label>
          {localError || state.pair.error ? (
            <p class="form-error" role="alert" data-testid="pair-error">
              {localError ?? state.pair.error?.message}
            </p>
          ) : null}
          <button type="submit" class="btn btn-primary btn-block" disabled={!canSubmit}>
            {state.pair.busy ? t.pair.connecting : t.pair.connect}
          </button>
        </form>
        {!secure ? <p class="pair-transport muted">{t.pair.httpNotice}</p> : null}
      </div>
    </div>
  );
}
