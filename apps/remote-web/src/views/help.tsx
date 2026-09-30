// One-line help under a setting the phone shares with the TV: the words come
// from SETTING_HELP in i18n.ts, keyed by the TV's Settings row id
// (apps/tv-shell/qml/SettingsScreen.qml), so the phone and the TV explain a
// setting the same way. `helpId` gives the paragraph's element id for an
// input's aria-describedby. Used by editor.tsx, tv.tsx, sleep.tsx and
// remote.tsx (the volume group); tests/unit/help.spec.ts checks every id used
// here exists in the table.
import type { JSX } from 'preact';
import { SETTING_HELP, type SettingHelpId } from '../i18n.ts';

/**
 * @param id A TV Settings row id.
 * @returns The id of that row's help paragraph on the page.
 */
export function helpId(id: SettingHelpId): string {
  return `help-${id}`;
}

/** The help line itself. */
export function SettingHelp({ id }: { id: SettingHelpId }): JSX.Element {
  return (
    <p class="muted small setting-help" id={helpId(id)} data-help={id}>
      {SETTING_HELP[id]}
    </p>
  );
}
