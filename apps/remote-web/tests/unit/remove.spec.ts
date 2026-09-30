// Remove apps on the owner's phone (src/views/remove.tsx): one row per
// installed Flatpak (the web apps under their browser's name), the size it
// frees, which apps it turns off, a plain refusal for a system-wide
// install, Removing… while it runs; owner phones only, while app.uninstall
// is listed; Remove asks first and sends app.uninstall (delete_data on
// "Remove and delete its data").
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { Session, StateSnapshot } from '../../src/contract.ts';
import { type AppState, initialState, reduce } from '../../src/state.ts';
import { RemoveAppsSection, removeEntries, removeLines } from '../../src/views/remove.tsx';
import { mayInstall } from '../../src/views/install.tsx';

type Props = Record<string, unknown> & { children?: unknown };
function walk(node: unknown, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) for (const c of node) walk(c, out);
  else if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    out.push(v);
    if (typeof v.type === 'function') walk((v.type as (p: Props) => unknown)(v.props), out);
    else walk(v.props.children, out);
  }
  return out;
}
const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);

const fixture = (name: string): StateSnapshot => JSON.parse(readFileSync(new URL(`../../../../contracts/fixtures/${name}`, import.meta.url), 'utf8')) as StateSnapshot;
const online = (snap: StateSnapshot, permissions: Session['permissions']): AppState => {
  const s = reduce(initialState('Phone'), { type: 'session_started', session: { device_id: 'd', device_name: 'Phone', permissions, csrf_token: 'c', transport_secure: false } });
  return reduce(s, { type: 'state_received', snapshot: snap, at: 1 });
};

describe('remove apps', () => {
  const owner = fixture('state.phone-owner-installs.valid.json');

  it('lists installed apps with what Remove frees, refuses system-wide installs, shows removing', () => {
    const entries = removeEntries(owner);
    const byId = Object.fromEntries(entries.map((e) => [e.id, e]));
    expect(byId['youtube']?.label).toBe('YouTube');
    expect(removeLines(byId['youtube']!)).toEqual(['Frees about 142 MB on the TV, and more if no other app uses its shared parts']);
    expect(byId['plex-htpc']?.system).toBe(true);
    expect(removeLines(byId['plex-htpc']!)[0]).toMatch(/only the PC's own software tool/);
    expect(removeLines(byId['spotify']!)).toEqual(['Removing…']);
    expect(entries.some((e) => e.id === 'retroarch')).toBe(false); // not installed
  });

  it('names the browser and the sites it turns off', () => {
    const yt = owner.applications[1]!;
    const snap: StateSnapshot = {
      ...owner,
      apps: { auto_update: true, browser: 'brave', streaming_browser: 'chrome', browsers: [{ id: 'chrome', label: 'Google Chrome', flatpak_id: 'com.google.Chrome', streaming_unverified: false }] },
      applications: [
        ...owner.applications.filter((a) => a.id !== 'netflix'),
        { ...yt, id: 'netflix', label: 'Netflix', adapter: 'netflix', enabled: true, hidden: false },
        { ...yt, id: 'hulu', label: 'Hulu', adapter: 'hulu', enabled: true, hidden: false },
        { ...yt, id: 'disney-plus', label: 'Disney+', adapter: 'disney-plus', enabled: false, hidden: true },
      ],
    };
    const chrome = removeEntries(snap).find((e) => e.id === 'netflix')!;
    expect(chrome.label).toBe('Google Chrome');
    expect(removeEntries(snap).filter((e) => e.label === 'Google Chrome')).toHaveLength(1);
    expect(removeLines(chrome)).toEqual(['Frees about 142 MB on the TV, and more if no other app uses its shared parts', 'Netflix and Hulu use Google Chrome, so removing it turns them off.']);
  });

  it('asks first, then sends app.uninstall with or without the data', () => {
    const sent: [string, boolean][] = [];
    const asked: (string | null)[] = [];
    const props = {
      entries: removeEntries(owner),
      available: true,
      reason: null,
      art: 'pixel' as const,
      onAsk: (id: string | null) => asked.push(id),
      onRemove: (id: string, d: boolean) => sent.push([id, d]),
    };
    let tree = walk(RemoveAppsSection({ ...props, confirming: null }));
    expect(byTestId(tree, 'remove-button-plex-htpc')).toBeUndefined(); // system-wide: no button
    expect(byTestId(tree, 'remove-button-spotify')).toBeUndefined(); // removing
    expect(byTestId(tree, 'remove-confirm-youtube')).toBeUndefined();
    (byTestId(tree, 'remove-button-youtube')!.props.onClick as () => void)();
    expect(asked).toEqual(['youtube']);
    expect(sent).toEqual([]);
    tree = walk(RemoveAppsSection({ ...props, confirming: 'youtube' }));
    (byTestId(tree, 'remove-cancel-youtube')!.props.onClick as () => void)();
    expect(asked).toEqual(['youtube', null]);
    (byTestId(tree, 'remove-data-youtube')!.props.onClick as () => void)();
    (byTestId(tree, 'remove-go-youtube')!.props.onClick as () => void)();
    expect(sent).toEqual([
      ['youtube', true],
      ['youtube', false],
    ]);
  });

  it('owner phones only', () => {
    expect(mayInstall(online(owner, ['owner', 'controller']))).toBe(true);
    expect(mayInstall(online({ ...owner, me: { ...owner.me!, permissions: ['controller'] } }, ['controller']))).toBe(false);
  });
});
