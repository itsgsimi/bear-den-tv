// Layout editor (layout_editor permission): section order, visibility and titles,
// plus the cosmetic `ui` fields from layout.schema.json. Contract: edits only change
// the local draft through `app.updateDraft`; the TV sees them via Preview (not
// saved) or Apply (`PUT /api/v1/layout` with the loaded revision). A pending
// confirmation shows a countdown from `editor.pending.expires_in_s` measured from
// `editor.pendingAt`, with Keep/Revert. Revision conflicts, validation errors and
// the trusted-LAN HTTP write refusal are reported by the controller as notices.
import { useEffect, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { App } from '../app.ts';
import type { ArtStyle, BackgroundPreset, Layout, LayoutPending, LayoutSection, LayoutUi, TileDensity } from '../contract.ts';
import { t } from '../i18n.ts';
import { type AppState, canWriteLayout, layoutsEqual } from '../state.ts';

// Built-in themes, used until the TV reports its installed list (state.appearance.themes).
const BUILT_IN_THEMES: { id: BackgroundPreset; name: string; accent?: string }[] = [
  { id: 'den', name: 'Den' },
  { id: 'forest', name: 'Forest' },
  { id: 'midnight', name: 'Midnight' },
  { id: 'campfire', name: 'Campfire' },
  { id: 'winter', name: 'Winter' },
];
const DENSITIES: TileDensity[] = ['comfortable', 'large'];
const ART_STYLES: ArtStyle[] = ['pixel', 'classic'];
const HEX = /^#[0-9A-Fa-f]{6}$/;

type Confirm = { kind: 'all' } | { kind: 'section'; id: string; title: string } | null;

export function EditorView({ app, state }: { app: App; state: AppState }): JSX.Element {
  const appearance = state.snapshot?.appearance;
  const themes = appearance?.themes && appearance.themes.length > 0 ? appearance.themes : BUILT_IN_THEMES;
  const editor = state.editor;
  const [confirm, setConfirm] = useState<Confirm>(null);

  useEffect(() => {
    if (editor.status === 'idle') void app.loadLayout();
  }, [app, editor.status]);

  if (editor.status === 'idle' || editor.status === 'loading' || (editor.status === 'ready' && !editor.draft)) {
    return (
      <section class="page editor">
        <h2>{t.editor.heading}</h2>
        <p class="muted" role="status">
          {t.editor.loading}
        </p>
      </section>
    );
  }
  if (editor.status === 'error' || !editor.draft) {
    return (
      <section class="page editor">
        <h2>{t.editor.heading}</h2>
        <div class="notice notice-error" role="alert">
          <p>{t.editor.loadFailed(editor.error ?? '')}</p>
          <button type="button" class="btn btn-secondary" onClick={() => void app.loadLayout()}>
            {t.editor.retry}
          </button>
        </div>
      </section>
    );
  }

  const draft = editor.draft;
  // Older layouts may hold an alias (den-gradient, charcoal); show its theme.
  const ALIASES: Record<string, string> = { 'den-gradient': 'den', charcoal: 'campfire' };
  const themeValue = themes.some((th) => th.id === draft.ui.background) ? draft.ui.background : (ALIASES[draft.ui.background] ?? draft.ui.background);
  const writable = canWriteLayout(state);
  const dirty = !layoutsEqual(draft, editor.saved);
  const titlesOk = draft.sections.every((s) => s.title.trim().length > 0);
  const busy = editor.busy;

  const setUi = <K extends keyof LayoutUi>(key: K, value: LayoutUi[K]) => app.updateDraft({ ...draft, ui: { ...draft.ui, [key]: value } });
  const setSection = (index: number, patch: Partial<LayoutSection>) =>
    app.updateDraft({ ...draft, sections: draft.sections.map((s, i) => (i === index ? { ...s, ...patch } : s)) });
  const move = (index: number, delta: -1 | 1) => app.updateDraft(moveSection(draft, index, delta));

  return (
    <section class="page editor">
      <h2>{t.editor.heading}</h2>
      <p class="muted">{t.editor.intro}</p>
      {editor.revision !== null ? <p class="muted small">{t.editor.revision(editor.revision)}</p> : null}
      {!writable ? (
        <div class="notice notice-warning" data-testid="editor-http-notice">
          {t.editor.httpReadOnly}
        </div>
      ) : null}
      {editor.pending ? <PendingBanner app={app} pending={editor.pending} pendingAt={editor.pendingAt} busy={busy} /> : null}
      {editor.previewing ? (
        <div class="notice notice-info" role="status" data-testid="editor-previewing">
          <strong>{t.editor.previewing}</strong>
          <p>{t.editor.previewingBody}</p>
        </div>
      ) : null}
      {editor.notice ? (
        <div class={`notice notice-${editor.notice.kind}`} role={editor.notice.kind === 'error' ? 'alert' : 'status'} data-testid="editor-notice">
          {editor.notice.text}
        </div>
      ) : null}

      <h3>{t.editor.sections}</h3>
      <ol class="section-list">
        {draft.sections.map((section, index) => (
          <li key={section.id} class={`section-row ${section.enabled ? '' : 'is-disabled'}`} data-testid="section-row" data-section-id={section.id}>
            <div class="section-head">
              <span class="muted small">{t.editor.sectionKind(section.kind)}</span>
              <div class="section-order">
                <button type="button" class="btn btn-secondary btn-small" aria-label={t.editor.moveUp} disabled={busy || index === 0} onClick={() => move(index, -1)}>
                  ↑
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-small"
                  aria-label={t.editor.moveDown}
                  disabled={busy || index === draft.sections.length - 1}
                  onClick={() => move(index, 1)}
                >
                  ↓
                </button>
              </div>
            </div>
            <label class="field">
              <span class="field-label">{t.editor.title}</span>
              <input
                class="text-input"
                type="text"
                maxLength={64}
                value={section.title}
                disabled={busy}
                aria-invalid={section.title.trim().length === 0}
                onInput={(ev) => setSection(index, { title: ev.currentTarget.value })}
              />
            </label>
            <Toggle label={t.editor.enabled} checked={section.enabled} disabled={busy} onChange={(v) => setSection(index, { enabled: v })} />
            <Toggle label={t.editor.hideWhenEmpty} checked={section.hide_when_empty} disabled={busy} onChange={(v) => setSection(index, { hide_when_empty: v })} />
            {confirm?.kind === 'section' && confirm.id === section.id ? (
              <ConfirmBox
                text={t.editor.confirmResetSection(confirm.title)}
                onYes={() => {
                  setConfirm(null);
                  void app.resetLayout(section.id);
                }}
                onNo={() => setConfirm(null)}
              />
            ) : (
              <button type="button" class="btn btn-secondary btn-small" disabled={busy || !writable} onClick={() => setConfirm({ kind: 'section', id: section.id, title: section.title })}>
                {t.editor.resetSection}
              </button>
            )}
          </li>
        ))}
      </ol>

      <h3>{t.editor.appearance}</h3>
      <div class="appearance">
        <label class="field">
          <span class="field-label">
            {t.editor.textScale} <span class="muted">{draft.ui.text_scale.toFixed(1)}×</span>
          </span>
          <input type="range" min={0.8} max={2} step={0.1} value={draft.ui.text_scale} disabled={busy} onInput={(ev) => setUi('text_scale', round1(Number(ev.currentTarget.value)))} />
        </label>
        <label class="field">
          <span class="field-label">
            {t.editor.safeMargin} <span class="muted">{draft.ui.safe_margin_percent}%</span>
          </span>
          <input type="range" min={0} max={10} step={1} value={draft.ui.safe_margin_percent} disabled={busy} onInput={(ev) => setUi('safe_margin_percent', Number(ev.currentTarget.value))} />
        </label>
        <label class="field">
          <span class="field-label">{t.editor.density}</span>
          <select class="text-input" value={draft.ui.tile_density} disabled={busy} onChange={(ev) => setUi('tile_density', ev.currentTarget.value as TileDensity)}>
            {DENSITIES.map((d) => (
              <option key={d} value={d}>
                {d === 'large' ? t.editor.densityLarge : t.editor.densityComfortable}
              </option>
            ))}
          </select>
        </label>
        <label class="field">
          <span class="field-label">{t.editor.background}</span>
          <select
            class="text-input"
            value={themeValue}
            disabled={busy}
            onChange={(ev) => {
              const picked = themes.find((th) => th.id === ev.currentTarget.value);
              setUi('background', ev.currentTarget.value as BackgroundPreset);
              if (picked?.accent) setUi('accent', picked.accent);
            }}
          >
            {themes.map((th) => (
              <option key={th.id} value={th.id}>
                {th.name}
              </option>
            ))}
          </select>
        </label>
        <label class="field">
          <span class="field-label">{t.editor.artStyle}</span>
          <select class="text-input" value={draft.ui.art_style ?? 'pixel'} disabled={busy} onChange={(ev) => setUi('art_style', ev.currentTarget.value as ArtStyle)}>
            {ART_STYLES.map((a) => (
              <option key={a} value={a}>
                {a === 'classic' ? t.editor.artClassic : t.editor.artPixel}
              </option>
            ))}
          </select>
        </label>
        <AccentField value={draft.ui.accent} disabled={busy} onChange={(v) => setUi('accent', v)} />
        <Toggle label={t.editor.reducedMotion} checked={draft.ui.reduced_motion} disabled={busy} onChange={(v) => setUi('reduced_motion', v)} />
        <Toggle label={t.editor.highContrast} checked={draft.ui.high_contrast_focus} disabled={busy} onChange={(v) => setUi('high_contrast_focus', v)} />
        <Toggle label={t.editor.hero} checked={draft.ui.hero_enabled} disabled={busy} onChange={(v) => setUi('hero_enabled', v)} />
        <Toggle label={t.editor.clock} checked={draft.ui.clock_enabled} disabled={busy} onChange={(v) => setUi('clock_enabled', v)} />
      </div>

      {!titlesOk ? (
        <p class="form-error" role="alert">
          {t.editor.titleRequired}
        </p>
      ) : null}
      {dirty ? <p class="muted small" data-testid="editor-dirty">{t.editor.unsaved}</p> : null}
      <div class="actions-row editor-actions">
        {editor.previewing ? (
          <button type="button" class="btn btn-secondary" disabled={busy} onClick={() => void app.endPreview()}>
            {t.editor.endPreview}
          </button>
        ) : (
          <button type="button" class="btn btn-secondary" data-testid="editor-preview" disabled={busy || !titlesOk} onClick={() => void app.previewLayout()}>
            {t.editor.preview}
          </button>
        )}
        <button type="button" class="btn btn-primary" data-testid="editor-apply" disabled={busy || !writable || !dirty || !titlesOk} onClick={() => void app.applyLayout()}>
          {busy ? t.editor.applying : t.editor.apply}
        </button>
      </div>
      <div class="actions-row">
        <button type="button" class="btn btn-secondary" disabled={busy || !writable} onClick={() => void app.undoLayout()}>
          {t.editor.undo}
        </button>
        {confirm?.kind === 'all' ? (
          <ConfirmBox
            text={t.editor.confirmResetAll}
            onYes={() => {
              setConfirm(null);
              void app.resetLayout();
            }}
            onNo={() => setConfirm(null)}
          />
        ) : (
          <button type="button" class="btn btn-danger" disabled={busy || !writable} onClick={() => setConfirm({ kind: 'all' })}>
            {t.editor.resetAll}
          </button>
        )}
      </div>
    </section>
  );
}

/**
 * @param layout Draft layout.
 * @param index Section to move.
 * @param delta Direction.
 * @returns A new layout with the section swapped with its neighbour; unchanged at the edges.
 */
export function moveSection(layout: Layout, index: number, delta: -1 | 1): Layout {
  const target = index + delta;
  if (target < 0 || target >= layout.sections.length) return layout;
  const sections = [...layout.sections];
  const a = sections[index];
  const b = sections[target];
  if (!a || !b) return layout;
  sections[index] = b;
  sections[target] = a;
  return { ...layout, sections };
}

/**
 * @param pending Pending confirmation from the TV.
 * @param pendingAt Local timestamp when it was received.
 * @param now Current local time.
 * @returns Seconds left (never negative), or null when the server gave no expiry.
 */
export function pendingSecondsLeft(pending: LayoutPending, pendingAt: number, now: number): number | null {
  if (pending.expires_in_s < 0) return null;
  return Math.max(0, pending.expires_in_s - Math.floor((now - pendingAt) / 1000));
}

function PendingBanner({ app, pending, pendingAt, busy }: { app: App; pending: LayoutPending; pendingAt: number; busy: boolean }): JSX.Element {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const left = pendingSecondsLeft(pending, pendingAt, now);
  return (
    <div class="notice notice-warning" role="alertdialog" data-testid="editor-pending">
      <strong>{left === null ? t.editor.pendingTitleUnknown : t.editor.pendingTitle(left)}</strong>
      <p>{t.editor.pendingBody}</p>
      <div class="actions-row">
        <button type="button" class="btn btn-primary" disabled={busy} onClick={() => void app.confirmPending()}>
          {t.editor.keep}
        </button>
        <button type="button" class="btn btn-secondary" disabled={busy} onClick={() => void app.cancelPending()}>
          {t.editor.revert}
        </button>
      </div>
    </div>
  );
}

function Toggle({ label, checked, disabled, onChange }: { label: string; checked: boolean; disabled: boolean; onChange: (v: boolean) => void }): JSX.Element {
  return (
    <label class="toggle">
      <input type="checkbox" checked={checked} disabled={disabled} onChange={(ev) => onChange(ev.currentTarget.checked)} />
      <span>{label}</span>
    </label>
  );
}

function AccentField({ value, disabled, onChange }: { value: string; disabled: boolean; onChange: (v: string) => void }): JSX.Element {
  const [text, setText] = useState(value);
  useEffect(() => setText(value), [value]);
  return (
    <div class="field accent-field">
      <span class="field-label">{t.editor.accent}</span>
      <div class="accent-inputs">
        <input type="color" aria-label={t.editor.accent} value={value.toLowerCase()} disabled={disabled} onInput={(ev) => onChange(ev.currentTarget.value.toUpperCase())} />
        <input
          class="text-input mono"
          type="text"
          maxLength={7}
          aria-label={t.editor.accentCustom}
          aria-invalid={!HEX.test(text)}
          value={text}
          disabled={disabled}
          onInput={(ev) => {
            const next = ev.currentTarget.value.trim();
            setText(next);
            if (HEX.test(next)) onChange(next.toUpperCase());
          }}
        />
      </div>
    </div>
  );
}

function ConfirmBox({ text, onYes, onNo }: { text: string; onYes: () => void; onNo: () => void }): JSX.Element {
  return (
    <div class="confirm" role="alertdialog" aria-label={text}>
      <p>{text}</p>
      <div class="actions-row">
        <button type="button" class="btn btn-danger" onClick={onYes}>
          {t.editor.confirmYes}
        </button>
        <button type="button" class="btn btn-secondary" onClick={onNo}>
          {t.editor.confirmNo}
        </button>
      </div>
    </div>
  );
}

function round1(n: number): number {
  return Math.round(n * 10) / 10;
}
