// Unit tests for the settings' one-line help (SETTING_HELP in src/i18n.ts,
// SettingHelp in src/views/help.tsx): the table holds every TV Settings row
// the phone and the TV share, every help id the phone's views use exists in
// it, the phone draws the ones it should, and the TV, Sleep and volume
// sections draw their help only while their controls are there.
import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { VNode } from 'preact';
import type { Cec, Power, StateSnapshot } from '../../src/contract.ts';
import { SETTING_HELP } from '../../src/i18n.ts';
import { SettingHelp, helpId } from '../../src/views/help.tsx';
import { SleepSection, type SleepSectionProps } from '../../src/views/sleep.tsx';
import { TvSection, VolumeHelp } from '../../src/views/tv.tsx';

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

/** The help ids a rendered tree draws, in order. */
const helps = (node: unknown): string[] => walk(node).flatMap((v) => (typeof v.props['data-help'] === 'string' ? [v.props['data-help']] : []));

const views = new URL('../../src/views/', import.meta.url);

/** Every help id named in the views' source: <SettingHelp id="…">, help="…", helpId('…'). */
function usedInViews(): Set<string> {
  const used = new Set<string>();
  for (const file of readdirSync(views).filter((f) => f.endsWith('.tsx'))) {
    const src = readFileSync(new URL(file, views), 'utf8');
    for (const m of src.matchAll(/<SettingHelp id="([^"]+)"|\bhelp="([^"]+)"|helpId\('([^']+)'\)/g)) used.add(m[1] ?? m[2] ?? m[3] ?? '');
  }
  return used;
}

describe('settings help table', () => {
  it('holds every TV Settings row the phone and the TV share, one line each', () => {
    const rows = ['background', 'style', 'art', 'app-icons', 'text', 'density', 'margin', 'motion', 'contrast', 'hero', 'clock', 'cec', 'cec-volume', 'sleep', 'screen-off', 'now-playing', 'auto-update'];
    expect(Object.keys(SETTING_HELP).sort()).toEqual([...rows].sort());
    for (const line of Object.values(SETTING_HELP)) {
      expect(line.length).toBeGreaterThan(0);
      expect(line).not.toContain('\n');
    }
  });

  it('has every help id the phone uses, and the phone shows the settings it has', () => {
    const used = usedInViews();
    for (const id of used) expect(Object.keys(SETTING_HELP), `help id "${id}" is not in SETTING_HELP`).toContain(id);
    // Layout (editor.tsx), TV (tv.tsx), volume (tv.tsx VolumeHelp) and Sleep (sleep.tsx).
    const phone = ['text', 'margin', 'density', 'background', 'art', 'app-icons', 'motion', 'contrast', 'hero', 'clock', 'cec', 'cec-volume', 'sleep', 'screen-off'];
    expect([...used].sort()).toEqual([...phone].sort());
  });

  it('draws the table text with an id inputs can point at', () => {
    const [p] = walk(SettingHelp({ id: 'margin' }));
    expect(p?.props.id).toBe(helpId('margin'));
    expect(p?.props.children).toBe(SETTING_HELP.margin);
  });
});

describe('help in the remote sections', () => {
  const ON: Cec = { available: true, enabled: true, volume_target: 'pc', tv_power: 'on' };
  const sleep = (overrides: Partial<SleepSectionProps> = {}) =>
    SleepSection({
      power: { sleep_at_ms: null, warning: false, display: 'on' } as Power,
      remaining: null,
      sleepGate: { listed: true, disabled: false, reason: null },
      offGate: { listed: true, disabled: false, reason: null },
      classic: false,
      onSleep: () => undefined,
      onScreenOff: () => undefined,
      ...overrides,
    });

  it('explains HDMI-CEC on the TV section', () => {
    expect(helps(TvSection({ available: true, cec: ON, onPower: () => undefined }))).toEqual(['cec']);
  });

  it('explains the volume buttons only while HDMI-CEC is on', () => {
    expect(helps(VolumeHelp({ snapshot: { cec: ON } as unknown as StateSnapshot }))).toEqual(['cec-volume']);
    expect(VolumeHelp({ snapshot: { cec: { ...ON, enabled: false } } as unknown as StateSnapshot })).toBeNull();
    expect(VolumeHelp({ snapshot: {} as StateSnapshot })).toBeNull();
  });

  it('explains the sleep timer and screen off only where they are drawn', () => {
    expect(helps(sleep())).toEqual(['sleep', 'screen-off']);
    expect(helps(sleep({ offGate: { listed: false, disabled: true, reason: null } }))).toEqual(['sleep']);
    expect(helps(sleep({ sleepGate: { listed: false, disabled: true, reason: null } }))).toEqual(['screen-off']);
  });
});
