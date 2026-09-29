// Remote screen: D-pad with Select, Back, Bear Den Home and Close app, app shortcuts from
// `snapshot.applications` (minus optional apps marked `hidden`), playback with the Now playing card above its buttons
// (nowplaying.tsx), volume (PC or, with HDMI-CEC, the TV: tv.tsx), TV power (tv.tsx, drawn only while
// available), Sleep (sleep.tsx: timer and screen off), and text entry. Contract: every
// control is gated by `snapshot.capabilities[action]`; a listed-but-unavailable
// capability renders disabled with the server's reason, an unlisted optional one
// is not rendered. Directions call `pressStart`/`pressEnd` on pointer down/up so
// the controller owns the hold lease; keyboard activation sends a single tap.
// The last result is shown with its outcome (accepted/delivered/observed/failed)
// kept distinct; nothing here pretends a press was observed when it was delivered.
// A guest pass sees only what it may use (`mayUse`): no Close app, no restart,
// no sleep timer or screen off.
import { useEffect, useState } from 'preact/hooks';
import type { ComponentChildren, JSX } from 'preact';
import type { App } from '../app.ts';
import type { ActionArgs, ActionName, Application, NavAction } from '../contract.ts';
import { t } from '../i18n.ts';
import { AppArt, Art, artStyleOf, Icon, type IconName } from '../icons.tsx';
import { Vines } from '../vines.tsx';
import { NowPlayingPanel, nowPlayingOf } from './nowplaying.tsx';
import { SleepPanel } from './sleep.tsx';
import { TvPanel, volumeHeading } from './tv.tsx';
import { type AppState, type PendingAction, capabilityFor, closableApp, isSecureTransport, mayUse, permissionsOf, visibleApps } from '../state.ts';

const TEXT_MAX = 256;
const VOLUME_STEP = 5;
const SEEK_SECONDS = 30;
const CLOSE_CONFIRM_MS = 3000;
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\u0000-\u001f\u007f]/;

interface Gate {
  disabled: boolean;
  reason: string | null;
}

function gate(state: AppState, action: ActionName): Gate {
  const capability = capabilityFor(state.snapshot, action);
  if (capability.available) return { disabled: false, reason: null };
  return { disabled: true, reason: capability.reason || t.remote.capabilityUnknown };
}

function listed(state: AppState, action: ActionName): boolean {
  return state.snapshot?.capabilities[action] !== undefined && mayUse(state, action);
}

export function RemoteView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const snapshot = state.snapshot;
  const reasons = new Set<string>();
  const note = (g: Gate): Gate => {
    if (g.reason) reasons.add(g.reason);
    return g;
  };

  return (
    <section class="page remote">
      {snapshot?.dev_mode ? (
        <div class="notice notice-warning demo" role="note" data-testid="demo-notice">
          <strong>{t.remote.demo}</strong> {t.remote.demoBody}
        </div>
      ) : null}
      <Blockers app={app} state={state} />

      <div class="dpad" data-testid="dpad">
        <NavButton app={app} state={state} action="nav.up" icon="up" label={t.remote.up} />
        <NavButton app={app} state={state} action="nav.left" icon="left" label={t.remote.left} />
        <TapButton app={app} state={state} action="select" args={{}} class="dpad-select" gate={note(gate(state, 'select'))}>
          {t.remote.select}
        </TapButton>
        <NavButton app={app} state={state} action="nav.right" icon="right" label={t.remote.right} />
        <NavButton app={app} state={state} action="nav.down" icon="down" label={t.remote.down} />
      </div>
      <p class="muted small dpad-hint">{t.remote.dpadHint}</p>

      <div class="button-row">
        <TapButton app={app} state={state} action="back" args={{}} icon="back" gate={note(gate(state, 'back'))}>
          {t.remote.back}
        </TapButton>
        <TapButton app={app} state={state} action="home" args={{}} icon="home" class="btn-home" gate={note(gate(state, 'home'))}>
          {t.remote.home}
        </TapButton>
      </div>
      {mayUse(state, 'app.close') ? (
        <div class="button-row">
          <CloseButton app={app} state={state} />
        </div>
      ) : null}

      <LastResult state={state} />

      {visibleApps(snapshot).length > 0 ? (
        <div class="group" aria-labelledby="apps-heading">
          <h3 id="apps-heading">{t.remote.apps}</h3>
          <div class="app-grid">
            {visibleApps(snapshot).map((application) => (
              <AppButton key={application.id} app={app} state={state} application={application} />
            ))}
          </div>
        </div>
      ) : null}

      {listed(state, 'media.play') || listed(state, 'media.pause') || listed(state, 'media.seek_relative') || nowPlayingOf(snapshot) ? (
        <div class="group" aria-labelledby="playback-heading">
          <h3 id="playback-heading">{t.remote.playback}</h3>
          <NowPlayingPanel state={state} />
          <div class="button-row">
            {listed(state, 'media.seek_relative') ? (
              <TapButton app={app} state={state} action="media.seek_relative" args={{ seconds: -SEEK_SECONDS }} icon="rewind" label={t.remote.seekBackLabel} gate={note(gate(state, 'media.seek_relative'))}>
                {t.remote.seekBack}
              </TapButton>
            ) : null}
            {listed(state, 'media.play') ? (
              <TapButton app={app} state={state} action="media.play" args={{}} icon="play" gate={note(gate(state, 'media.play'))}>
                {t.remote.play}
              </TapButton>
            ) : null}
            {listed(state, 'media.pause') ? (
              <TapButton app={app} state={state} action="media.pause" args={{}} icon="pause" gate={note(gate(state, 'media.pause'))}>
                {t.remote.pause}
              </TapButton>
            ) : null}
            {listed(state, 'media.seek_relative') ? (
              <TapButton app={app} state={state} action="media.seek_relative" args={{ seconds: SEEK_SECONDS }} icon="forward" label={t.remote.seekForwardLabel} gate={note(gate(state, 'media.seek_relative'))}>
                {t.remote.seekForward}
              </TapButton>
            ) : null}
          </div>
        </div>
      ) : null}

      {listed(state, 'audio.volume_delta') || listed(state, 'audio.mute') ? (
        <div class="group" aria-labelledby="volume-heading">
          <h3 id="volume-heading">{volumeHeading(snapshot)}</h3>
          <div class="button-row">
            {listed(state, 'audio.volume_delta') ? (
              <>
                <TapButton app={app} state={state} action="audio.volume_delta" args={{ delta: -VOLUME_STEP }} icon="volume-down" label={t.remote.volumeDown} gate={note(gate(state, 'audio.volume_delta'))} />
                <TapButton app={app} state={state} action="audio.volume_delta" args={{ delta: VOLUME_STEP }} icon="volume-up" label={t.remote.volumeUp} gate={note(gate(state, 'audio.volume_delta'))} />
              </>
            ) : null}
            {listed(state, 'audio.mute') ? (
              <>
                <TapButton app={app} state={state} action="audio.mute" args={{ muted: true }} icon="mute" label={t.remote.mute} gate={note(gate(state, 'audio.mute'))} />
                <TapButton app={app} state={state} action="audio.mute" args={{ muted: false }} icon="unmute" label={t.remote.unmute} gate={note(gate(state, 'audio.mute'))} />
              </>
            ) : null}
          </div>
        </div>
      ) : null}

      {mayUse(state, 'tv.power') ? <TvPanel app={app} state={state} /> : null}

      {mayUse(state, 'power.sleep_timer') || mayUse(state, 'display.off') ? <SleepPanel app={app} state={state} /> : null}

      {listed(state, 'text.submit') ? <TextEntry app={app} state={state} gate={note(gate(state, 'text.submit'))} /> : null}

      {reasons.size > 0 ? (
        <ul class="reasons muted small" data-testid="unavailable-reasons">
          {Array.from(reasons).map((reason) => (
            <li key={reason}>{reason}</li>
          ))}
        </ul>
      ) : null}

      {!isSecureTransport(state) && snapshot?.remote.transport === 'trusted-lan-http' ? <p class="muted small">{t.pair.httpNotice}</p> : null}
    </section>
  );
}

function Blockers({ app, state }: { app: App; state: AppState }): JSX.Element | null {
  const snapshot = state.snapshot;
  if (!snapshot) return null;
  const shellState = snapshot.session.shell_state;
  if (snapshot.session.locked || snapshot.target.kind === 'locked') {
    return (
      <div class="notice notice-warning" role="alert" data-testid="blocker-locked">
        <strong>{t.remote.lockedTitle}</strong>
        <p>{t.remote.lockedBody}</p>
      </div>
    );
  }
  if (shellState === 'crashed' || shellState === 'circuit_open' || shellState === 'restarting') {
    const owner = permissionsOf(state).includes('owner');
    const restart = gate(state, 'shell.restart');
    return (
      <div class="notice notice-error" role="alert" data-testid="blocker-shell">
        <strong>{t.remote.shellCrashedTitle}</strong>
        <p>{t.remote.shellCrashedBody(shellState)}</p>
        {owner ? (
          <button type="button" class="btn btn-secondary" disabled={restart.disabled} title={restart.reason ?? undefined} onClick={() => void app.tap('shell.restart', {})}>
            <Icon name="restart" size={18} /> {t.remote.restartShell}
          </button>
        ) : (
          <p class="muted small">{t.remote.restartShellOwnerOnly}</p>
        )}
      </div>
    );
  }
  if (snapshot.target.kind === 'unknown') {
    return (
      <div class="notice notice-warning" role="alert" data-testid="blocker-unknown">
        <strong>{t.remote.unknownTitle}</strong>
        <p>{t.remote.unknownBody}</p>
      </div>
    );
  }
  return null;
}

function NavButton({ app, state, action, icon, label }: { app: App; state: AppState; action: NavAction; icon: IconName; label: string }): JSX.Element {
  const g = gate(state, action);
  const holding = state.hold.phase === 'active' && state.hold.action === action;
  const end = () => app.pressEnd();
  return (
    <button
      type="button"
      class={`btn dpad-${action.slice(4)} ${holding ? 'is-holding' : ''}`}
      aria-label={label}
      title={g.reason ?? undefined}
      disabled={g.disabled}
      data-action={action}
      onPointerDown={(ev) => {
        if (ev.button !== 0) return;
        ev.currentTarget.setPointerCapture?.(ev.pointerId);
        app.pressStart(action);
      }}
      onPointerUp={end}
      onPointerCancel={end}
      onLostPointerCapture={end}
      onContextMenu={(ev) => ev.preventDefault()}
      onClick={(ev) => {
        // Pointer presses were sent on pointerdown; only keyboard activation (detail 0) taps here.
        if (ev.detail === 0) void app.tap(action, {});
      }}
    >
      <Icon name={icon} size={36} />
    </button>
  );
}

interface TapButtonProps<A extends ActionName> {
  app: App;
  state: AppState;
  action: A;
  args: ActionArgs[A];
  gate: Gate;
  icon?: IconName;
  label?: string;
  class?: string;
  children?: ComponentChildren;
}

function TapButton<A extends ActionName>({ app, action, args, gate: g, icon, label, class: extra, children }: TapButtonProps<A>): JSX.Element {
  return (
    <button
      type="button"
      class={`btn btn-control ${extra ?? ''}`}
      aria-label={label}
      title={g.reason ?? undefined}
      disabled={g.disabled}
      data-action={action}
      onClick={() => void app.tap(action, args)}
    >
      {icon ? <Icon name={icon} size={22} /> : null}
      {children ? <span>{children}</span> : null}
    </button>
  );
}

// Closes the app in front, or the one left running behind Home. It takes a
// second tap within CLOSE_CONFIRM_MS so a stray press cannot end a stream.
function CloseButton({ app, state }: { app: App; state: AppState }): JSX.Element {
  const target = closableApp(state.snapshot);
  const g = gate(state, 'app.close');
  const [armed, setArmed] = useState<string | null>(null);
  useEffect(() => {
    if (!armed) return undefined;
    const timer = setTimeout(() => setArmed(null), CLOSE_CONFIRM_MS);
    return () => clearTimeout(timer);
  }, [armed]);
  const isArmed = target !== null && armed === target.id;
  const label = !target ? t.remote.closeNothing : isArmed ? t.remote.closeConfirm(target.label) : t.remote.closeApp(target.label);
  return (
    <button
      type="button"
      class={`btn btn-control btn-close ${isArmed ? 'is-armed' : ''}`}
      title={g.reason ?? undefined}
      disabled={g.disabled || !target}
      data-action="app.close"
      data-testid="close-app"
      onClick={() => {
        if (!target) return;
        if (!isArmed) return setArmed(target.id);
        setArmed(null);
        void app.tap('app.close', { app_id: target.id });
      }}
    >
      <Icon name="close" size={22} />
      <span aria-live="polite">{label}</span>
    </button>
  );
}

function AppButton({ app, state, application }: { app: App; state: AppState; application: Application }): JSX.Element {
  const launch = gate(state, 'app.launch');
  const reason = !application.installed ? t.remote.notInstalled(application.label) : launch.reason;
  const status = application.launch_state === 'launching' ? t.remote.launching : application.foreground ? t.remote.inFront : application.running ? t.remote.running : null;
  const live = application.launch_state === 'launching' ? 'launching' : application.foreground ? 'front' : application.running ? 'running' : null;
  const disabled = !application.installed || launch.disabled;
  return (
    <div class={`vine-host app-tile-host ${disabled ? 'is-disabled' : ''}`}>
      <button
        type="button"
        class={`btn app-tile ${application.foreground ? 'is-foreground' : ''} ${live ? `is-${live}` : ''}`}
        aria-label={t.remote.launch(application.label)}
        title={reason ?? undefined}
        disabled={disabled}
        data-app-id={application.id}
        onClick={() => void app.tap('app.launch', { app_id: application.id })}
      >
        <span class="app-icon">
          <AppArt adapter={application.adapter} size={32} art={artStyleOf(state.snapshot?.appearance)} />
          {live ? <span class="run-dot" aria-hidden="true" /> : null}
        </span>
        <span class="app-label">{application.label}</span>
        {status ? <span class="app-status small">{status}</span> : null}
        {!application.installed ? <span class="app-status small">{t.remote.notInstalled(application.label)}</span> : null}
      </button>
      <Vines appearance={state.snapshot?.appearance} />
    </div>
  );
}

function latest(state: AppState): PendingAction | null {
  let best: PendingAction | null = null;
  for (const entry of Object.values(state.pending)) {
    if (!best || entry.updated_at > best.updated_at || (entry.updated_at === best.updated_at && entry.sent_at > best.sent_at)) best = entry;
  }
  return best;
}

function LastResult({ state }: { state: AppState }): JSX.Element {
  const entry = latest(state);
  const outcome = entry?.outcome ?? null;
  return (
    <div class={`last-result outcome-${entry ? (outcome ?? 'pending') : 'none'}`} role="status" aria-live="polite" data-testid="last-result" data-outcome={outcome ?? ''}>
      {entry ? (
        <span class="last-body" key={`${entry.request_id}:${outcome ?? 'pending'}`}>
          <span class="muted small">{t.remote.lastResult}: </span>
          <span class="last-action">{t.remote.actionName(entry.action)}</span>
          <span class="last-outcome">
            {outcome === 'observed' ? <Art name="sparkle" class="result-glint" scale={2} width={14} art={artStyleOf(state.snapshot?.appearance)} /> : null}
            {outcome ? t.remote.ack(outcome) : t.remote.sending}
          </span>
          {entry.message ? <span class="last-message small">{entry.message}</span> : null}
        </span>
      ) : null}
    </div>
  );
}

function TextEntry({ app, gate: g }: { app: App; state: AppState; gate: Gate }): JSX.Element {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);

  if (!open) {
    return (
      <div class="group">
        <button type="button" class="btn btn-control" disabled={g.disabled} title={g.reason ?? undefined} data-action="text.submit" onClick={() => setOpen(true)}>
          <Icon name="keyboard" size={22} />
          <span>{t.remote.text}</span>
        </button>
      </div>
    );
  }

  const submit = (ev: Event) => {
    ev.preventDefault();
    if (text.length === 0) return setError(t.remote.textEmpty);
    if (text.length > TEXT_MAX) return setError(t.remote.textTooLong);
    if (CONTROL_CHARS.test(text)) return setError(t.remote.textControlChars);
    setError(null);
    void app.tap('text.submit', { text });
    setText('');
    setOpen(false);
  };

  return (
    <form class="group text-entry" onSubmit={submit} noValidate>
      <label class="field">
        <span class="field-label">{t.remote.textTitle}</span>
        <input
          class="text-input"
          type="text"
          maxLength={TEXT_MAX}
          autoComplete="off"
          placeholder={t.remote.textPlaceholder}
          value={text}
          onInput={(ev) => setText(ev.currentTarget.value)}
        />
      </label>
      <p class="muted small">{t.remote.textHint}</p>
      {error ? (
        <p class="form-error" role="alert">
          {error}
        </p>
      ) : null}
      <div class="actions-row">
        <button type="submit" class="btn btn-primary" disabled={g.disabled}>
          {t.remote.textSend}
        </button>
        <button type="button" class="btn btn-secondary" onClick={() => setOpen(false)}>
          {t.remote.textCancel}
        </button>
      </div>
    </form>
  );
}
