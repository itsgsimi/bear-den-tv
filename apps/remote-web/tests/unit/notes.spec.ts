// Unit tests for app notes on the tiles (src/views/notes.tsx, the app tile in
// src/views/remote.tsx): a tile whose app has notes carries an ⓘ button
// (aria-expanded, a tap, no hover) that opens them in the Good to know panel
// under the grid; an app without notes gets no button and no panel.
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { Application, StateSnapshot } from '../../src/contract.ts';
import type { App } from '../../src/app.ts';
import type { AppState } from '../../src/state.ts';
import { NotesPanel, NotesToggle, notesOf } from '../../src/views/notes.tsx';
import { AppButton } from '../../src/views/remote.tsx';

type Props = Record<string, unknown> & { children?: unknown };

function walk(node: unknown, out: VNode<Props>[] = []): VNode<Props>[] {
  if (Array.isArray(node)) {
    for (const c of node) walk(c, out);
  } else if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    out.push(v);
    if (typeof v.type === 'function') walk((v.type as (p: Props) => unknown)(v.props), out);
    else walk(v.props.children, out);
  }
  return out;
}

function text(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(text).join(' ');
  if (node && typeof node === 'object' && 'props' in node) {
    const v = node as VNode<Props>;
    if (typeof v.type === 'function') return text((v.type as (p: Props) => unknown)(v.props));
    return text(v.props.children);
  }
  return '';
}

const byTestId = (tree: VNode<Props>[], id: string) => tree.find((v) => v.props['data-testid'] === id);

function application(notes?: string[]): Application {
  return {
    id: 'demo-app', label: 'Demo App', adapter: 'demo', installed: true, version: null, installation: 'user', running: false, foreground: false,
    launch_state: 'idle', last_error: null, ...(notes ? { notes } : {}),
  };
}

const state = { snapshot: { capabilities: { 'app.launch': { available: true } }, applications: [] } as unknown as StateSnapshot, installReady: {}, session: null } as unknown as AppState;
const fakeApp = { tap: async () => undefined } as unknown as App;

describe('app notes on tiles', () => {
  it('puts an ⓘ button on a tile whose app has notes, and it toggles that app', () => {
    const toggled: string[] = [];
    const tree = walk(AppButton({ app: fakeApp, state, application: application(['Needs a server.']), notesOpen: false, onNotes: (id) => toggled.push(id) }));
    const button = byTestId(tree, 'notes-toggle-demo-app');
    expect(button?.type).toBe('button');
    expect(button?.props['aria-expanded']).toBe(false);
    expect(button?.props['aria-label']).toBe('Good to know about Demo App');
    (button?.props.onClick as () => void)();
    expect(toggled).toEqual(['demo-app']);
    expect(walk(NotesToggle({ application: application(['x']), open: true, onToggle: () => undefined }))[0]?.props['aria-expanded']).toBe(true);
  });

  it('opens the notes under the grid, as sent', () => {
    let closed = false;
    const panel = NotesPanel({ application: application(['Needs a server.', 'Home pauses it.']), onClose: () => (closed = true) });
    const tree = walk(panel);
    expect(text(byTestId(tree, 'app-notes'))).toBe('Needs a server. Home pauses it.');
    expect(text(panel)).toContain('Good to know: Demo App');
    (byTestId(tree, 'notes-close')?.props.onClick as () => void)();
    expect(closed).toBe(true);
  });

  it('draws nothing for an app without notes', () => {
    expect(notesOf(application())).toEqual([]);
    for (const a of [application(), application([])]) {
      expect(byTestId(walk(AppButton({ app: fakeApp, state, application: a })), 'notes-toggle-demo-app')).toBeUndefined();
      expect(NotesPanel({ application: a, onClose: () => undefined })).toBeNull();
    }
    expect(NotesPanel({ application: null, onClose: () => undefined })).toBeNull();
  });
});
