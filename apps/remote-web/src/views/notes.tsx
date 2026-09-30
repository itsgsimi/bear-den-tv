// App notes on the phone: `state.applications[].notes` (contracts/http.md
// "App notes"), short plain sentences from the adapter table and live data,
// shown as they are. Every phone that sees app tiles can open an app's notes
// with the ⓘ button on its tile (`NotesToggle`, a real button with
// aria-expanded, no hover); they open in one "Good to know" panel under the
// Apps grid (`NotesPanel`). Add apps (install.tsx) lists an app's notes in
// its install row (`NotesList`) before Install is pressed. Absent or empty
// notes draw nothing. Pure components: the open app lives in RemoteView.
import type { JSX } from 'preact';
import type { Application } from '../contract.ts';
import { t } from '../i18n.ts';
import { Icon } from '../icons.tsx';

/**
 * @param application An application from the snapshot.
 * @returns Its notes, or an empty list when absent (older coordinators).
 */
export function notesOf(application: Pick<Application, 'notes'> | null | undefined): readonly string[] {
  return application?.notes ?? [];
}

/** The sentences as a list; nothing when there are none. */
export function NotesList({ notes, class: cls }: { notes: readonly string[]; class?: string }): JSX.Element | null {
  if (notes.length === 0) return null;
  return (
    <ul class={`app-notes ${cls ?? ''}`} data-testid="app-notes">
      {notes.map((n) => (
        <li key={n}>{n}</li>
      ))}
    </ul>
  );
}

/** The DOM id of the notes panel the tiles' buttons control. */
export const NOTES_PANEL_ID = 'app-notes-panel';

/** The ⓘ button on an app tile: nothing for an app without notes. */
export function NotesToggle({ application, open, onToggle }: { application: Application; open: boolean; onToggle: (appId: string) => void }): JSX.Element | null {
  if (notesOf(application).length === 0) return null;
  return (
    <button
      type="button"
      class={`btn notes-toggle ${open ? 'is-open' : ''}`}
      aria-label={t.notes.show(application.label)}
      aria-expanded={open}
      aria-controls={NOTES_PANEL_ID}
      data-testid={`notes-toggle-${application.id}`}
      onClick={() => onToggle(application.id)}
    >
      <Icon name="about" size={18} />
    </button>
  );
}

/** "Good to know" for the open app, under the Apps grid; nothing when closed or empty. */
export function NotesPanel({ application, onClose }: { application: Application | null; onClose: () => void }): JSX.Element | null {
  const notes = notesOf(application);
  if (!application || notes.length === 0) return null;
  return (
    <div class="notes-panel" id={NOTES_PANEL_ID} role="region" aria-label={t.notes.panel(application.label)} data-testid="notes-panel">
      <div class="notes-head">
        <strong>{t.notes.panel(application.label)}</strong>
        <button type="button" class="btn btn-secondary btn-small" data-testid="notes-close" onClick={onClose}>
          {t.notes.close}
        </button>
      </div>
      <NotesList notes={notes} />
    </div>
  );
}
